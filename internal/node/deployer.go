package node

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
	"vps-auto-deploy/internal/pkg/upgrade"
)

var (
	validNameRegex   = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,32}$`)
	validSecretRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)
)

// DeploySSHRequest SSH 远程部署请求参数
type DeploySSHRequest struct {
	Name          string `json:"name"`
	SubPrefix     string `json:"sub_prefix"` // 订阅节点名称前缀 (如 "🇭🇰 香港主控", "🇯🇵 日本东京")
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
	EnableTLS     *bool  `json:"enable_tls"` // 默认开启自签 TLS + 指纹绑定 (防中间人劫持)
}

// UpgradeRequest 远程节点软件推流升级请求参数 (支持 API 免密或 SSH 救援)
type UpgradeRequest struct {
	NodeID        int64  `json:"node_id"`
	Mode          string `json:"mode"`       // "api" (默认，免密) | "ssh" (应急救援)
	PackageID     string `json:"package_id"` // 可选：指定已导入的安装包 ID
	Force         bool   `json:"force"`      // 是否强制覆盖已一致的二进制
	Host          string `json:"host"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	AuthType      string `json:"auth_type"` // "password" | "key"
	SSHPassword   string `json:"ssh_password"`
	SSHPrivateKey string `json:"ssh_private_key"`
}

// UpgradeSSHRequest 兼容旧版命名
type UpgradeSSHRequest = UpgradeRequest

// computeFileSHA256 计算本地指定文件的 SHA-256 十六进制哈希串
func computeFileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

// LoginNode 调用远端面板登录接口获取 JWT Token，支持 TLS 证书指纹校验防 MITM
func LoginNode(host string, port int, secretPath, username, password string, useTLS bool, certFingerprint string) (string, error) {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	loginURL := fmt.Sprintf("%s://%s:%d/%s/api/auth/login", scheme, host, port, strings.Trim(secretPath, "/"))
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	body, _ := json.Marshal(payload)

	var client *http.Client
	if useTLS {
		client = crypto.CreatePinnedHTTPClient(certFingerprint, 4*time.Second)
	} else {
		client = &http.Client{Timeout: 4 * time.Second}
	}

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

// PingNode 测量节点延迟并更新其在线状态（带证书指纹防篡改与 Token 失效自愈）
func PingNode(node *model.Node) (int64, error) {
	if node.IsMaster {
		_ = model.UpdateNodeStatus(node.ID, "online", 1)
		return 1, nil
	}

	scheme := "http"
	if node.UseTLS {
		scheme = "https"
	}

	start := time.Now()
	testURL := fmt.Sprintf("%s://%s:%d/%s/api/system/overview", scheme, node.Host, node.PanelPort, strings.Trim(node.SecretPath, "/"))
	req, err := http.NewRequest(http.MethodGet, testURL, nil)
	if err != nil {
		_ = model.UpdateNodeStatus(node.ID, "error", 0)
		return 0, err
	}

	req.Header.Set("Authorization", "Bearer "+node.APIToken)

	var client *http.Client
	if node.UseTLS {
		client = crypto.CreatePinnedHTTPClient(node.CertFingerprint, 8*time.Second)
	} else {
		client = &http.Client{Timeout: 8 * time.Second}
	}

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	// 若 Token 失效 (401)，尝试使用保存的加密密码重新登录刷新 Token
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		rawPass, decErr := model.DecryptSensitiveData(node.PasswordEncrypted)
		if decErr == nil && rawPass != "" {
			newToken, logErr := LoginNode(node.Host, node.PanelPort, node.SecretPath, node.Username, rawPass, node.UseTLS, node.CertFingerprint)
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
		if err != nil {
			return 0, fmt.Errorf("节点不可达: %w", err)
		}
		return 0, fmt.Errorf("节点返回异常状态码: %d", resp.StatusCode)
	}
	resp.Body.Close()

	_ = model.UpdateNodeStatus(node.ID, "online", latency)
	return latency, nil
}

// RunDeploySSH 在独立 Goroutine 中执行 SSH 自动化部署与指纹绑定
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

	// 白名单字符安全校验，防范命令行参数注入
	if !validNameRegex.MatchString(req.SSHUser) {
		task.Status = "failed"
		task.Error = fmt.Sprintf("SSH 用户名格式不合法: %q (仅允许 1-32 位字母、数字、点、下划线及横线)", req.SSHUser)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	if !validNameRegex.MatchString(req.AdminUsername) {
		task.Status = "failed"
		task.Error = fmt.Sprintf("管理员用户名格式不合法: %q (仅允许 1-32 位字母、数字、点、下划线及横线)", req.AdminUsername)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	if !validSecretRegex.MatchString(req.SecretPath) {
		task.Status = "failed"
		task.Error = fmt.Sprintf("安全路径格式不合法: %q (仅允许 1-64 位字母、数字、下划线及横线)", req.SecretPath)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	// 默认开启自签 TLS 防中间人劫持
	enableTLS := true
	if req.EnableTLS != nil {
		enableTLS = *req.EnableTLS
	}

	task.AddLog(fmt.Sprintf("开始连接远程主机 %s:%d (用户: %s)...", req.Host, req.SSHPort, req.SSHUser))

	// 2. 建立 SSH 认证配置并开启 TOFU 主机指纹捕获
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

	// 检查数据库中是否已存在该 Host 的历史指纹（防御二次连接遭遇中间人冒充或劫持）
	var existingSSHHostKey string
	_ = model.DB.QueryRow("SELECT ssh_host_key FROM nodes WHERE host = ? AND ssh_host_key != '' LIMIT 1", req.Host).Scan(&existingSSHHostKey)

	var remoteSSHHostKey string
	sshConfig := &ssh.ClientConfig{
		User: req.SSHUser,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			currentFP := ssh.FingerprintSHA256(key)
			if existingSSHHostKey != "" && currentFP != existingSSHHostKey {
				return fmt.Errorf("【安全拦截】SSH 主机指纹不匹配！已记录指纹: %s, 实际指纹: %s (防中间人冒充)", existingSSHHostKey, currentFP)
			}
			remoteSSHHostKey = currentFP
			return nil
		},
		Timeout: 15 * time.Second,
	}

	addr := net.JoinHostPort(req.Host, strconv.Itoa(req.SSHPort))
	var client *ssh.Client
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		client, err = ssh.Dial("tcp", addr, sshConfig)
		if err == nil {
			break
		}
		if attempt < 3 {
			task.AddLog(fmt.Sprintf("SSH 握手初次建立未成功 (%v)，正在重试 (%d/3)...", err, attempt))
			time.Sleep(3 * time.Second)
		}
	}
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("SSH 握手连接失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	defer client.Close()
	task.AddLog(fmt.Sprintf("SSH 连接建立成功！目标主机指纹 (TOFU): %s", remoteSSHHostKey))

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
	unameM = strings.TrimSpace(unameM)
	var targetArch string
	switch unameM {
	case "x86_64", "amd64":
		targetArch = "amd64"
	case "aarch64", "arm64":
		targetArch = "arm64"
	case "armv7l", "arm":
		targetArch = "arm"
	default:
		// 严防 shell 元字符注入，仅允许英文字母、数字和横线
		for _, r := range unameM {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
				task.Status = "failed"
				task.Error = fmt.Sprintf("异常的远程系统架构标识: %q", unameM)
				task.AddLog("[ERROR] " + task.Error)
				return
			}
		}
		targetArch = unameM
	}
	task.AddLog(fmt.Sprintf("远程系统环境识别: Linux (%s), 目标架构: %s", unameM, targetArch))

	// 4. 传输或安装二进制文件
	task.AddLog("正在准备部署 vps-panel 二进制文件...")
	_, _ = runSSHCmd("mkdir -p /usr/local/bin /etc/vps-auto-deploy /etc/systemd/system")

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
		task.AddLog(fmt.Sprintf("跨架构部署 (%s -> %s)，从 GitHub Release 自动下载...", runtime.GOARCH, targetArch))
		downloadCmd := fmt.Sprintf(
			"curl -fsSL -o /usr/local/bin/vps-panel https://github.com/JayYang1991/vps-auto-deploy/releases/latest/download/vps-panel-linux-%s || wget -q -O /usr/local/bin/vps-panel https://github.com/JayYang1991/vps-auto-deploy/releases/latest/download/vps-panel-linux-%s && chmod +x /usr/local/bin/vps-panel",
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

	// 5. 初始化远端管理员账号、密码及安全路径 (通过 Stdin 管道传输，严防 /proc 进程树暴露密码)
	task.AddLog("初始化被纳管节点的凭据与安全入口 (安全 Stdin 管道模式)...")
	credsJSON, _ := json.Marshal(map[string]string{
		"username":    req.AdminUsername,
		"password":    req.AdminPassword,
		"secret_path": req.SecretPath,
	})
	stdinInitCmd := fmt.Sprintf("cat <<'EOF' | /usr/local/bin/vps-panel -stdin-credentials\n%s\nEOF\n", string(credsJSON))
	out, err := runSSHCmd(stdinInitCmd)
	if err != nil {
		// 优雅向后兼容：若远端二进制版本过低不支持 -stdin-credentials，降级为参数模式
		task.AddLog("远端版本可能暂未启用 -stdin-credentials，降级兼容处理...")
		fallbackCmd := fmt.Sprintf(
			"/usr/local/bin/vps-panel -reset-username '%s' -reset-password '%s' -reset-secret '%s'",
			strings.ReplaceAll(req.AdminUsername, "'", `'\''`),
			strings.ReplaceAll(req.AdminPassword, "'", `'\''`),
			strings.ReplaceAll(req.SecretPath, "'", `'\''`),
		)
		if fbOut, fbErr := runSSHCmd(fallbackCmd); fbErr != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("初始化远端凭据失败: %v (%s / %s)", fbErr, out, fbOut)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
	}
	task.AddLog(fmt.Sprintf("远端管理员账号已置为: %s, 安全入口: /%s/", req.AdminUsername, req.SecretPath))

	// 6. 配置自签名 TLS 证书并提取证书指纹 (Phase 1 防中间人劫持)
	var certFingerprint string
	execArgs := fmt.Sprintf("-port %d", req.PanelPort)
	if enableTLS {
		task.AddLog("为被纳管节点生成专属 ECDSA TLS 证书并提取防劫持指纹...")
		fpOut, fpErr := runSSHCmd("/usr/local/bin/vps-panel -print-fingerprint")
		if fpErr != nil || fpOut == "" {
			task.Status = "failed"
			task.Error = fmt.Sprintf("提取远端 TLS 证书指纹失败: %v (%s)", fpErr, fpOut)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		certFingerprint = strings.TrimSpace(fpOut)
		task.AddLog(fmt.Sprintf("已捕获目标节点 TLS 锁定指纹: %s", certFingerprint))
		execArgs += " -tls"
	}

	// 7. 配置并启动 Systemd 守护进程
	task.AddLog("配置 Systemd 守护进程 (vps-panel.service)...")

	// 自动放行主机防火墙端口 (ufw, firewalld, iptables)
	firewallScript := fmt.Sprintf(`
if command -v ufw >/dev/null 2>&1; then
    ufw allow %d/tcp >/dev/null 2>&1 || true
fi
if command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld 2>/dev/null; then
    firewall-cmd --add-port=%d/tcp --permanent >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
fi
if command -v iptables >/dev/null 2>&1; then
    iptables -C INPUT -p tcp --dport %d -j ACCEPT >/dev/null 2>&1 || iptables -I INPUT -p tcp --dport %d -j ACCEPT >/dev/null 2>&1 || true
fi
`, req.PanelPort, req.PanelPort, req.PanelPort, req.PanelPort)
	_, _ = runSSHCmd(firewallScript)

	serviceContent := fmt.Sprintf(`[Unit]
Description=VPS Auto Deploy Panel
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/vps-panel %s
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, execArgs)

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
	task.AddLog("Systemd 服务已注册并启动成功！正在等待端口就绪与执行指纹锁定握手...")

	// 8. 远端服务健康检查与 Token 交换 (带 TLS 指纹比对强校验)
	var token string
	var pingLatency int64
	maxWait := 60 * time.Second
	deadline := time.Now().Add(maxWait)
	authenticated := false
	startTime := time.Now()
	var lastErr error

	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		startPing := time.Now()
		tok, logErr := LoginNode(req.Host, req.PanelPort, req.SecretPath, req.AdminUsername, req.AdminPassword, enableTLS, certFingerprint)
		if logErr == nil && tok != "" {
			token = tok
			pingLatency = time.Since(startPing).Milliseconds()
			authenticated = true
			break
		}
		lastErr = logErr
		elapsed := int(time.Since(startTime).Seconds())
		if elapsed%10 == 0 || elapsed == 4 {
			task.AddLog(fmt.Sprintf("正在等待远端服务端口监听与 TLS 握手建立 (%d/60秒)...", elapsed))
		}
	}

	if !authenticated {
		// 诊断远端 Systemd 服务状态与日志
		statusOut, _ := runSSHCmd("systemctl is-active vps-panel 2>&1")
		journalOut, _ := runSSHCmd("journalctl -u vps-panel -n 12 --no-pager 2>&1")

		task.Status = "failed"
		task.Error = fmt.Sprintf("主控端在 60 秒内未能完成安全握手认证 (错误: %v, 服务活跃状态: %s)", lastErr, strings.TrimSpace(statusOut))
		task.AddLog("[ERROR] " + task.Error)
		if strings.TrimSpace(journalOut) != "" {
			task.AddLog("远端服务最近输出:\n" + strings.TrimSpace(journalOut))
		}
		return
	}
	task.AddLog(fmt.Sprintf("远端握手成功！获取到集群通信 Token，网络延迟: %d ms", pingLatency))

	// 9. AES-256-GCM 加密被纳管节点的登录密码并持久化入库 (使用独立数据加密密钥)
	key := model.GetEncryptionKey()
	encryptedPass, err := crypto.EncryptAESGCM(req.AdminPassword, key)
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("加密节点凭据失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	var sshPassEnc, sshKeyEnc string
	if req.SSHPassword != "" {
		sshPassEnc, _ = crypto.EncryptAESGCM(req.SSHPassword, key)
	}
	if req.SSHPrivateKey != "" {
		sshKeyEnc, _ = crypto.EncryptAESGCM(req.SSHPrivateKey, key)
	}
	sshUser := strings.TrimSpace(req.SSHUser)
	if sshUser == "" {
		sshUser = "root"
	}

	subPrefix := strings.TrimSpace(req.SubPrefix)
	if subPrefix == "" {
		subPrefix = strings.TrimSpace(req.Name)
	}

	newNode := &model.Node{
		Name:                 req.Name,
		SubPrefix:            subPrefix,
		Host:                 req.Host,
		SSHPort:              req.SSHPort,
		PanelPort:            req.PanelPort,
		SecretPath:           req.SecretPath,
		Username:             req.AdminUsername,
		PasswordEncrypted:    encryptedPass,
		APIToken:             token,
		IsMaster:             false,
		UseTLS:               enableTLS,
		CertFingerprint:      certFingerprint,
		SSHHostKey:           remoteSSHHostKey,
		SSHUser:              sshUser,
		SSHPasswordEncrypted: sshPassEnc,
		SSHKeyEncrypted:      sshKeyEnc,
		Status:               "online",
		OSInfo:               fmt.Sprintf("Linux (%s)", targetArch),
		LatencyMs:            pingLatency,
	}

	nodeID, err := model.CreateNode(newNode)
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("保存节点记录失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	newNode.ID = nodeID
	task.NodeID = nodeID

	// 10. 自动将主控端所有成员身份与凭据同步至新纳管节点
	task.AddLog("正在同步全集群用户凭据至新节点...")
	if syncErr := SyncMembersToNode(newNode, nil); syncErr != nil {
		task.AddLog(fmt.Sprintf("[WARN] 初始用户凭据同步提示: %v", syncErr))
	} else {
		task.AddLog("已自动将主控端所有用户凭据同步至新节点！")
	}

	task.Status = "success"
	task.AddLog(fmt.Sprintf("[SUCCESS] 节点 %s (ID: %d) 成功加入集群并完成 TLS 指纹绑定！", req.Name, nodeID))
}

// RunUpgradeAPI 通过已有管理凭据 (API Token + Pinned TLS) 免密平滑升级纳管节点 (Zero-SSH)
func RunUpgradeAPI(task *DeployTask, req UpgradeRequest) {
	defer func() {
		if r := recover(); r != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("Panic during API upgrade: %v", r)
			task.AddLog("[ERROR] " + task.Error)
		}
	}()

	// 1. 获取目标节点元数据
	var targetNode *model.Node
	if req.NodeID > 0 {
		targetNode, _ = model.GetNode(req.NodeID)
	}
	if targetNode == nil && req.Host != "" {
		nodes, _ := model.ListNodes()
		for _, n := range nodes {
			if n.Host == req.Host {
				cpy := n
				targetNode = &cpy
				break
			}
		}
	}
	if targetNode == nil {
		task.Status = "failed"
		task.Error = "未找到目标纳管节点记录"
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	task.AddLog(fmt.Sprintf("🚀 开始通过管理凭据通道免密升级目标节点: %s (%s:%d)...", targetNode.Name, targetNode.Host, targetNode.PanelPort))

	// 2. 识别目标节点系统架构
	targetArch := "amd64"
	osInfoLower := strings.ToLower(targetNode.OSInfo)
	if strings.Contains(osInfoLower, "arm64") || strings.Contains(osInfoLower, "aarch64") {
		targetArch = "arm64"
	} else if strings.Contains(osInfoLower, "arm") {
		targetArch = "arm"
	} else if strings.Contains(osInfoLower, "386") {
		targetArch = "386"
	}

	// 3. 准备待推送的二进制升级程序
	var binPath string
	var localHash string
	if req.PackageID != "" {
		task.AddLog(fmt.Sprintf("使用指定导入的升级安装包: %s", req.PackageID))
		var err error
		binPath, err = upgrade.GetPackageBinaryPath(req.PackageID)
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("读取指定安装包失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		localHash, _ = upgrade.ComputeFileSHA256(binPath)
	} else {
		task.AddLog(fmt.Sprintf("正在为目标架构 %s 匹配最佳升级包 (Zero-External-Download)...", targetArch))
		var err error
		binPath, localHash, err = upgrade.FindBestPackageForArch(targetArch)
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			task.AddLog("[ERROR] " + task.Error)
			return
		}
	}

	prefixLen := 12
	if len(localHash) < 12 {
		prefixLen = len(localHash)
	}
	task.AddLog(fmt.Sprintf("升级包准备就绪: %s (架构: %s, 指纹: %s...)", filepath.Base(binPath), targetArch, localHash[:prefixLen]))

	// 4. 构建 API 客户端与远端状态握手
	scheme := "http"
	if targetNode.UseTLS {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s:%d/%s", scheme, targetNode.Host, targetNode.PanelPort, strings.Trim(targetNode.SecretPath, "/"))

	var client *http.Client
	if targetNode.UseTLS {
		client = crypto.CreatePinnedHTTPClient(targetNode.CertFingerprint, 25*time.Second)
	} else {
		client = &http.Client{Timeout: 25 * time.Second}
	}

	task.AddLog(fmt.Sprintf("连接目标节点管理通道 %s (TLS 指纹锁定保护)...", baseURL))
	needPush := true
	overviewURL := baseURL + "/api/system/overview"
	ovReq, _ := http.NewRequest(http.MethodGet, overviewURL, nil)
	ovReq.Header.Set("Authorization", "Bearer "+targetNode.APIToken)

	ovResp, ovErr := client.Do(ovReq)
	if ovErr == nil {
		if ovResp.StatusCode == http.StatusOK {
			var ovData struct {
				Version string `json:"version"`
				SHA256  string `json:"sha256"`
			}
			_ = json.NewDecoder(ovResp.Body).Decode(&ovData)
			ovResp.Body.Close()

			if ovData.SHA256 != "" && strings.EqualFold(ovData.SHA256, localHash) {
				if !req.Force {
					task.AddLog(fmt.Sprintf("✓ 远端节点程序指纹与升级包完全一致 (SHA-256: %s...): 已是最新版本！", localHash[:prefixLen]))
					task.AddLog("已自动跳过重复传输与重启，直接进入服务状态校验与全网凭据同步...")
					needPush = false
				} else {
					task.AddLog(fmt.Sprintf("远端节点程序指纹已是一致 (SHA-256: %s...)，但已指定 [强制覆盖]，继续推流写入...", localHash[:prefixLen]))
				}
			}
		} else if ovResp.StatusCode == http.StatusUnauthorized && targetNode.PasswordEncrypted != "" {
			ovResp.Body.Close()
			task.AddLog("目标节点 API Token 已过期，正在使用主控密钥解密凭据刷新 Token...")
			rawPass, decErr := model.DecryptSensitiveData(targetNode.PasswordEncrypted)
			if decErr == nil && rawPass != "" {
				newToken, logErr := LoginNode(targetNode.Host, targetNode.PanelPort, targetNode.SecretPath, targetNode.Username, rawPass, targetNode.UseTLS, targetNode.CertFingerprint)
				if logErr == nil && newToken != "" {
					targetNode.APIToken = newToken
					_ = model.UpdateNodeToken(targetNode.ID, newToken)
					task.AddLog("Token 自动刷新握手成功！")
				}
			}
		} else {
			ovResp.Body.Close()
		}
	}

	// 5. 推送二进制升级包流
	if needPush {
		task.AddLog(fmt.Sprintf("开始向远端节点推流最新版可执行程序 (%s)...", filepath.Base(binPath)))
		file, err := os.Open(binPath)
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("打开本地二进制失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		defer file.Close()

		fi, _ := file.Stat()
		task.AddLog(fmt.Sprintf("推流传输数据流大小: %.2f MB，免密写入远端...", float64(fi.Size())/1024/1024))

		upgradeURL := baseURL + "/api/system/self-upgrade"
		upReq, err := http.NewRequest(http.MethodPost, upgradeURL, file)
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("构建推送请求失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		upReq.Header.Set("Content-Type", "application/octet-stream")
		upReq.Header.Set("Authorization", "Bearer "+targetNode.APIToken)
		upReq.ContentLength = fi.Size()

		var pushClient *http.Client
		if targetNode.UseTLS {
			pushClient = crypto.CreatePinnedHTTPClient(targetNode.CertFingerprint, 180*time.Second)
		} else {
			pushClient = &http.Client{Timeout: 180 * time.Second}
		}

		upResp, err := pushClient.Do(upReq)
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("推流传输网络请求失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		defer upResp.Body.Close()

		respBody, _ := io.ReadAll(upResp.Body)
		if upResp.StatusCode != http.StatusOK {
			task.Status = "failed"
			task.Error = fmt.Sprintf("远端节点拒绝升级 (HTTP %d): %s", upResp.StatusCode, string(respBody))
			task.AddLog("[ERROR] " + task.Error)
			return
		}

		task.AddLog("✓ 远端节点已成功接收升级程序并完成原子替换！")
		task.AddLog("远端守护进程正在平滑重启，正在持续探活等待节点恢复上线...")
		time.Sleep(2 * time.Second)
	} else {
		task.AddLog("守护进程运行正常，保持在线状态...")
		time.Sleep(500 * time.Millisecond)
	}

	// 6. 持续健康探活 (最长 25 秒)
	task.AddLog("正在执行网络延迟测速与在线状态验证...")
	time.Sleep(1 * time.Second)
	online := false
	var pingLat int64
	for i := 0; i < 15; i++ {
		time.Sleep(1500 * time.Millisecond)
		lat, pingErr := PingNode(targetNode)
		if pingErr == nil {
			pingLat = lat
			online = true
			task.AddLog(fmt.Sprintf("✓ 远端节点健康探活成功！响应延迟: %d ms", pingLat))
			break
		}
	}
	if !online {
		task.AddLog("[WARN] 节点重启探活超时，可能正在重新编译或网络有波动，继续进行全网凭据同步...")
	}

	// 7. 全网凭据自动同步
	task.AddLog(fmt.Sprintf("正在向升级后的节点精准推送全量成员凭据 (目标: %s:%d)...", targetNode.Host, targetNode.PanelPort))
	if syncErr := SyncMembersToNode(targetNode, nil); syncErr != nil {
		task.AddLog(fmt.Sprintf("[WARN] 成员凭据自动同步提示: %v", syncErr))
	} else {
		task.AddLog("🎉 全网成员凭据自动同步成功！")
	}

	task.Status = "success"
	task.AddLog(fmt.Sprintf("[SUCCESS] 节点 %s (ID: %d) 免密平滑升级完成！已处于最新状态！", targetNode.Name, targetNode.ID))
}

// RunUpgradeSSH 执行对已有受管节点的推流升级 (保留其现有数据库与配置，仅更新二进制并重启)
func RunUpgradeSSH(task *DeployTask, req UpgradeSSHRequest) {
	defer func() {
		if r := recover(); r != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("Panic during upgrade: %v", r)
			task.AddLog("[ERROR] " + task.Error)
		}
	}()

	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	if strings.TrimSpace(req.SSHUser) == "" {
		req.SSHUser = "root"
	}

	if !validNameRegex.MatchString(req.SSHUser) {
		task.Status = "failed"
		task.Error = fmt.Sprintf("SSH 用户名格式不合法: %q (仅允许 1-32 位字母、数字、点、下划线及横线)", req.SSHUser)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	// 1. 获取目标节点元数据
	var targetNode *model.Node
	if req.NodeID > 0 {
		targetNode, _ = model.GetNode(req.NodeID)
	}
	if targetNode == nil && req.Host != "" {
		nodes, _ := model.ListNodes()
		for _, n := range nodes {
			if n.Host == req.Host {
				cpy := n
				targetNode = &cpy
				break
			}
		}
	}

	task.AddLog(fmt.Sprintf("开始连接远程主机 %s:%d 进行软件推流升级 (用户: %s)...", req.Host, req.SSHPort, req.SSHUser))

	// 2. 建立 SSH 认证配置并校验 TOFU 主机指纹
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

	var existingSSHHostKey string
	_ = model.DB.QueryRow("SELECT ssh_host_key FROM nodes WHERE host = ? AND ssh_host_key != '' LIMIT 1", req.Host).Scan(&existingSSHHostKey)

	var remoteSSHHostKey string
	sshConfig := &ssh.ClientConfig{
		User: req.SSHUser,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			currentFP := ssh.FingerprintSHA256(key)
			if existingSSHHostKey != "" && currentFP != existingSSHHostKey {
				return fmt.Errorf("【安全拦截】SSH 主机指纹不匹配！已记录指纹: %s, 实际指纹: %s", existingSSHHostKey, currentFP)
			}
			remoteSSHHostKey = currentFP
			return nil
		},
		Timeout: 15 * time.Second,
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
	task.AddLog(fmt.Sprintf("SSH 连接建立成功！目标主机指纹: %s", remoteSSHHostKey))

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
			return stdout.String() + stderr.String(), err
		}
		return stdout.String(), nil
	}

	// 3. 识别目标架构
	unameM, err := runSSHCmd("uname -m")
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("获取远程系统架构失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	unameM = strings.TrimSpace(unameM)

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
	task.AddLog(fmt.Sprintf("目标主机架构: %s (主控当前架构: %s)", targetArch, runtime.GOARCH))

	// 4. 比对版本指纹与推流传输新版 vps-panel 二进制
	exePath, exeErr := os.Executable()
	useStream := false
	if exeErr == nil && runtime.GOARCH == targetArch {
		if fi, statErr := os.Stat(exePath); statErr == nil && fi.Size() > 1000000 {
			useStream = true
		}
	}

	needRestart := true
	if useStream {
		task.AddLog("目标架构与主控一致，正在比对本地与远端程序版本指纹...")
		localHash, hashErr := computeFileSHA256(exePath)
		var remoteHash string
		if hashErr == nil {
			remoteSumOut, remoteErr := runSSHCmd("sha256sum /usr/local/bin/vps-panel 2>/dev/null || shasum -a 256 /usr/local/bin/vps-panel 2>/dev/null")
			if remoteErr == nil {
				fields := strings.Fields(remoteSumOut)
				if len(fields) > 0 {
					remoteHash = strings.ToLower(fields[0])
				}
			}
		}

		if hashErr == nil && remoteHash != "" && strings.EqualFold(localHash, remoteHash) {
			prefixLen := 12
			if len(localHash) < 12 {
				prefixLen = len(localHash)
			}
			if !req.Force {
				task.AddLog(fmt.Sprintf("✓ 远端节点程序指纹与主控完全一致 (SHA-256: %s...): 已是最新版本！", localHash[:prefixLen]))
				task.AddLog("已自动跳过重复推流传输与服务重启，直接进入服务状态校验与全网凭据同步...")
				needRestart = false
			} else {
				task.AddLog(fmt.Sprintf("远端节点程序指纹已是一致 (SHA-256: %s...)，但已指定 [强制覆盖]，继续推流写入...", localHash[:prefixLen]))
			}
		} else {
			if remoteHash != "" && len(remoteHash) >= 12 && len(localHash) >= 12 {
				task.AddLog(fmt.Sprintf("检测到版本更新 (远端: %s... -> 主控: %s...)，开始向远程主机推流传输最新版 vps-panel 二进制文件...", remoteHash[:12], localHash[:12]))
			} else {
				task.AddLog("开始向远程主机推流传输最新版 vps-panel 二进制文件...")
			}
			task.AddLog("采用内网直接推流部署 (Zero-External-Download)...")
		}

		if needRestart {
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
			task.AddLog("最新二进制文件写入并授权完成 (/usr/local/bin/vps-panel)")
		}
	} else {
		task.AddLog(fmt.Sprintf("跨架构升级 (%s -> %s)，从 GitHub Release 自动下载...", runtime.GOARCH, targetArch))
		downloadCmd := fmt.Sprintf(
			"curl -fsSL -o /usr/local/bin/vps-panel https://github.com/JayYang1991/vps-auto-deploy/releases/latest/download/vps-panel-linux-%s || wget -q -O /usr/local/bin/vps-panel https://github.com/JayYang1991/vps-auto-deploy/releases/latest/download/vps-panel-linux-%s && chmod +x /usr/local/bin/vps-panel",
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

	// 5. 重启远端守护进程
	if needRestart {
		task.AddLog("重启远程 vps-panel.service 守护进程...")
		restartCmd := "systemctl daemon-reload && systemctl restart vps-panel"
		if out, err := runSSHCmd(restartCmd); err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("重启远端守护进程失败: %v (%s)", err, out)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		task.AddLog("守护进程重启指令已下发，正在验证服务在线状态...")
		time.Sleep(2 * time.Second)
	} else {
		task.AddLog("守护进程运行正常，保持在线状态...")
		time.Sleep(500 * time.Millisecond)
	}

	// 6. 服务健康探活
	time.Sleep(2 * time.Second)
	if targetNode != nil {
		task.AddLog("正在执行网络延迟测速与 Token 安全握手...")
		lat, pingErr := PingNode(targetNode)
		if pingErr == nil {
			task.AddLog(fmt.Sprintf("远端节点服务在线，测速响应延迟: %d ms", lat))
		} else {
			task.AddLog(fmt.Sprintf("健康检查提示: %v", pingErr))
		}

		// 7. 自动触发全量成员凭据同步
		task.AddLog(fmt.Sprintf("正在向升级后的节点精准推送全量成员身份凭据 (目标: %s:%d)...", targetNode.Host, targetNode.PanelPort))
		if syncErr := SyncMembersToNode(targetNode, nil); syncErr != nil {
			task.AddLog(fmt.Sprintf("[WARN] 成员凭据自动同步提示: %v", syncErr))
		} else {
			task.AddLog("🎉 全网成员凭据自动同步成功！")
		}
	}

	task.Status = "success"
	if needRestart {
		task.AddLog("[SUCCESS] 节点软件升级完成！所有最新功能 (路由分流、全网凭据同步、Sing-box 1.14) 已就绪！")
	} else {
		task.AddLog("[SUCCESS] 节点已处于最新版本，全量成员凭据与连接状态已校准同步！")
	}
}

