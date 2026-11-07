package node

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
)

// DeploySSHRequest SSH 远程部署请求参数
type DeploySSHRequest struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	AuthType      string `json:"auth_type"` // "password" | "key"
	SSHPassword   string `json:"ssh_password"`
	SSHPrivateKey string `json:"ssh_private_key"`
	PanelPort     int    `json:"panel_port"`
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
	SecretPath    string `json:"secret_path"`
}

// GenerateRandomPassword 生成高强度随机密码
func GenerateRandomPassword(length int) string {
	const charset = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*"
	res := make([]byte, length)
	for i := 0; i < length; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		res[i] = charset[num.Int64()]
	}
	return string(res)
}

// GenerateRandomHex 生成指定长度的随机十六进制串
func GenerateRandomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// LoginNode 调用远端面板登录接口获取 JWT Token
func LoginNode(host string, port int, secretPath, username, password string) (string, error) {
	loginURL := fmt.Sprintf("http://%s:%d/%s/api/auth/login", host, port, strings.Trim(secretPath, "/"))
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	body, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(loginURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("连接节点失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("远端登录失败 (状态码 %d): %s", resp.StatusCode, string(respBody))
	}

	var res struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("解析远端登录返回失败: %w", err)
	}
	if res.Token == "" {
		if res.Error != "" {
			return "", errors.New(res.Error)
		}
		return "", errors.New("远端返回的 Token 为空")
	}

	return res.Token, nil
}

// PingNode 测量节点延迟并更新其在线状态
func PingNode(node *model.Node) (int64, error) {
	if node.IsMaster {
		_ = model.UpdateNodeStatus(node.ID, "online", 1)
		return 1, nil
	}

	start := time.Now()
	testURL := fmt.Sprintf("http://%s:%d/%s/api/system/overview", node.Host, node.PanelPort, strings.Trim(node.SecretPath, "/"))
	req, err := http.NewRequest(http.MethodGet, testURL, nil)
	if err != nil {
		_ = model.UpdateNodeStatus(node.ID, "error", 0)
		return 0, err
	}

	req.Header.Set("Authorization", "Bearer "+node.APIToken)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)

	latency := time.Since(start).Milliseconds()

	// 若 Token 失效 (401)，尝试使用保存的加密密码重新登录刷新 Token
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		jwtSecret, _ := model.GetSetting("jwt_secret")
		rawPass, decErr := crypto.DecryptAESGCM(node.PasswordEncrypted, crypto.DeriveKey(jwtSecret))
		if decErr == nil && rawPass != "" {
			newToken, logErr := LoginNode(node.Host, node.PanelPort, node.SecretPath, node.Username, rawPass)
			if logErr == nil && newToken != "" {
				node.APIToken = newToken
				_ = model.UpdateNodeToken(node.ID, newToken)
				_ = model.UpdateNodeStatus(node.ID, "online", latency)
				return latency, nil
			}
		}
		_ = model.UpdateNodeStatus(node.ID, "error", latency)
		return latency, errors.New("节点认证失效且重试登录失败")
	}

	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		_ = model.UpdateNodeStatus(node.ID, "offline", 0)
		return 0, fmt.Errorf("节点不可达")
	}
	resp.Body.Close()

	_ = model.UpdateNodeStatus(node.ID, "online", latency)
	return latency, nil
}

// RunDeploySSH 在独立 Goroutine 中执行 SSH 自动化部署
func RunDeploySSH(task *DeployTask, req DeploySSHRequest) {
	defer func() {
		if r := recover(); r != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("Panic during deploy: %v", r)
			task.AddLog("[ERROR] " + task.Error)
		}
	}()

	// 1. 参数规范化与默认值补全
	if req.PanelPort <= 0 {
		req.PanelPort = 20888
	}
	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	if strings.TrimSpace(req.SSHUser) == "" {
		req.SSHUser = "root"
	}
	if strings.TrimSpace(req.AdminUsername) == "" {
		req.AdminUsername = "admin"
	}
	if strings.TrimSpace(req.AdminPassword) == "" {
		req.AdminPassword = GenerateRandomPassword(12)
	}
	if strings.TrimSpace(req.SecretPath) == "" {
		req.SecretPath = GenerateRandomHex(4)
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = req.Host
	}

	task.AddLog(fmt.Sprintf("开始连接远程主机 %s:%d (用户: %s)...", req.Host, req.SSHPort, req.SSHUser))

	// 2. 建立 SSH 认证配置
	var authMethods []ssh.AuthMethod
	if req.AuthType == "key" || (req.SSHPrivateKey != "" && req.SSHPassword == "") {
		signer, err := ssh.ParsePrivateKey([]byte(req.SSHPrivateKey))
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("解析 SSH 私钥失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		authMethods = append(authMethods, ssh.Password(req.SSHPassword))
	}

	sshConfig := &ssh.ClientConfig{
		User:            req.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(req.Host, strconv.Itoa(req.SSHPort))
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("SSH 握手连接失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	defer client.Close()
	task.AddLog("SSH 连接建立成功！")

	// 3. 目标机器环境与架构识别
	runSSHCmd := func(cmd string) (string, error) {
		session, err := client.NewSession()
		if err != nil {
			return "", err
		}
		defer session.Close()

		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr
		err = session.Run(cmd)
		if err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return strings.TrimSpace(stdout.String()), nil
	}

	unameS, err := runSSHCmd("uname -s")
	if err != nil || !strings.Contains(strings.ToLower(unameS), "linux") {
		task.Status = "failed"
		task.Error = fmt.Sprintf("不支持的目标操作系统类型: %s", unameS)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	unameM, _ := runSSHCmd("uname -m")
	var targetArch string
	switch unameM {
	case "x86_64", "amd64":
		targetArch = "amd64"
	case "aarch64", "arm64":
		targetArch = "arm64"
	case "armv7l", "arm":
		targetArch = "arm"
	default:
		targetArch = unameM
	}
	task.AddLog(fmt.Sprintf("远程系统环境识别: Linux (%s), 目标架构: %s", unameM, targetArch))

	// 4. 传输或安装二进制文件
	task.AddLog("正在准备部署 vps-panel 二进制文件...")
	_, _ = runSSHCmd("mkdir -p /usr/local/bin /etc/vps-auto-deploy /etc/systemd/system")

	// 若目标机器架构与当前机器一致且可读取当前二进制，优先直接推流（毫秒级自复制，无需远程网络下载）
	exePath, exeErr := os.Executable()
	useStream := false
	if exeErr == nil && runtime.GOARCH == targetArch {
		if fi, statErr := os.Stat(exePath); statErr == nil && fi.Size() > 1000000 {
			useStream = true
		}
	}

	if useStream {
		task.AddLog("目标架构与主控一致，采用内网直接推流部署 (Zero-External-Download)...")
		session, err := client.NewSession()
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("创建 SSH 文件传输会话失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}

		stdin, err := session.StdinPipe()
		if err != nil {
			session.Close()
			task.Status = "failed"
			task.Error = fmt.Sprintf("获取 SSH 标准输入流失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}

		file, err := os.Open(exePath)
		if err != nil {
			session.Close()
			task.Status = "failed"
			task.Error = fmt.Sprintf("读取本地二进制失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}

		cmd := "cat > /usr/local/bin/vps-panel.tmp && mv /usr/local/bin/vps-panel.tmp /usr/local/bin/vps-panel && chmod +x /usr/local/bin/vps-panel"
		if err := session.Start(cmd); err != nil {
			file.Close()
			session.Close()
			task.Status = "failed"
			task.Error = fmt.Sprintf("启动远端写入命令失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}

		_, copyErr := io.Copy(stdin, file)
		file.Close()
		stdin.Close()
		if copyErr != nil {
			session.Close()
			task.Status = "failed"
			task.Error = fmt.Sprintf("推流写入失败: %v", copyErr)
			task.AddLog("[ERROR] " + task.Error)
			return
		}

		if err := session.Wait(); err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("设置远端二进制权限失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		task.AddLog("二进制文件写入并授权完成 (/usr/local/bin/vps-panel)")
	} else {
		// 跨架构下载模式
		task.AddLog(fmt.Sprintf("跨架构部署 (%s -> %s)，从 GitHub Release 自动下载...", runtime.GOARCH, targetArch))
		downloadCmd := fmt.Sprintf(
			"curl -fsSL -o /usr/local/bin/vps-panel https://github.com/Jason-cq/vps-auto-deploy/releases/latest/download/vps-panel-linux-%s || wget -q -O /usr/local/bin/vps-panel https://github.com/Jason-cq/vps-auto-deploy/releases/latest/download/vps-panel-linux-%s && chmod +x /usr/local/bin/vps-panel",
			targetArch, targetArch,
		)
		if _, err := runSSHCmd(downloadCmd); err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("远端下载二进制失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		task.AddLog("跨架构二进制下载完成")
	}

	// 5. 初始化远端管理员账号、密码及安全路径
	task.AddLog("初始化被纳管节点的凭据与安全入口...")
	initCmd := fmt.Sprintf(
		"/usr/local/bin/vps-panel -reset-username '%s' -reset-password '%s' -reset-secret '%s'",
		strings.ReplaceAll(req.AdminUsername, "'", `'\''`),
		strings.ReplaceAll(req.AdminPassword, "'", `'\''`),
		strings.ReplaceAll(req.SecretPath, "'", `'\''`),
	)
	if out, err := runSSHCmd(initCmd); err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("初始化远端凭据失败: %v (%s)", err, out)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	task.AddLog(fmt.Sprintf("远端管理员账号已置为: %s, 安全入口: /%s/", req.AdminUsername, req.SecretPath))

	// 6. 配置并启动 Systemd 守护进程
	task.AddLog("配置 Systemd 守护进程 (vps-panel.service)...")
	serviceContent := fmt.Sprintf(`[Unit]
Description=VPS Auto Deploy Panel
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/vps-panel -port %d
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, req.PanelPort)

	writeServiceCmd := fmt.Sprintf("cat <<'EOF' > /etc/systemd/system/vps-panel.service\n%s\nEOF\n", serviceContent)
	if _, err := runSSHCmd(writeServiceCmd); err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("写入 Systemd 服务配置失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	startCmd := "systemctl daemon-reload && systemctl enable vps-panel && systemctl restart vps-panel"
	if _, err := runSSHCmd(startCmd); err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("启动 vps-panel.service 失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	task.AddLog("Systemd 服务已注册并启动成功！正在等待端口监听与服务就绪...")

	// 7. 远端服务健康检查与 Token 交换
	var token string
	var pingLatency int64
	maxWait := 15 * time.Second
	deadline := time.Now().Add(maxWait)
	authenticated := false

	for time.Now().Before(deadline) {
		time.Sleep(1500 * time.Millisecond)
		startPing := time.Now()
		tok, logErr := LoginNode(req.Host, req.PanelPort, req.SecretPath, req.AdminUsername, req.AdminPassword)
		if logErr == nil && tok != "" {
			token = tok
			pingLatency = time.Since(startPing).Milliseconds()
			authenticated = true
			break
		}
	}

	if !authenticated {
		task.Status = "failed"
		task.Error = "节点已启动但主控端在 15 秒内未能完成 HTTP 握手认证（请检查防火墙/端口是否开放）"
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	task.AddLog(fmt.Sprintf("远端握手成功！获取到集群通信 Token，网络延迟: %d ms", pingLatency))

	// 8. AES-256-GCM 加密被纳管节点的登录密码并持久化入库
	jwtSecret, _ := model.GetSetting("jwt_secret")
	key := crypto.DeriveKey(jwtSecret)
	encryptedPass, err := crypto.EncryptAESGCM(req.AdminPassword, key)
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("加密节点凭据失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	newNode := &model.Node{
		Name:              req.Name,
		Host:              req.Host,
		SSHPort:           req.SSHPort,
		PanelPort:         req.PanelPort,
		SecretPath:        req.SecretPath,
		Username:          req.AdminUsername,
		PasswordEncrypted: encryptedPass,
		APIToken:          token,
		IsMaster:          false,
		Status:            "online",
		OSInfo:            fmt.Sprintf("Linux (%s)", targetArch),
		LatencyMs:         pingLatency,
	}

	nodeID, err := model.CreateNode(newNode)
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("保存节点记录失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	task.NodeID = nodeID
	task.Status = "success"
	task.AddLog(fmt.Sprintf("[SUCCESS] 节点 %s (ID: %d) 成功加入主控集群！", req.Name, nodeID))
}
