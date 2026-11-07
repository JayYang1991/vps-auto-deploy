package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"vps-auto-deploy/internal/cloud/vultr"
	"vps-auto-deploy/internal/model"
)

// ProvisionVultrRequest 云实例弹性创建并纳管请求
type ProvisionVultrRequest struct {
	Name          string `json:"name"`
	Region        string `json:"region"`
	Plan          string `json:"plan"`
	OSID          int    `json:"os_id"`
	EnableIPv6    bool   `json:"enable_ipv6"`
	PanelPort     int    `json:"panel_port"`
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
	SecretPath    string `json:"secret_path"`
	EnableTLS     *bool  `json:"enable_tls"`
}

// EnsureMasterSSHKeyPair 确保主控端拥有专用的 ED25519 云部署 SSH 密钥对
func EnsureMasterSSHKeyPair() (string, string, error) {
	privPEM, errPriv := model.GetSetting("cloud_ssh_private_key")
	pubAuth, errPub := model.GetSetting("cloud_ssh_public_key")
	if errPriv == nil && errPub == nil && privPEM != "" && pubAuth != "" {
		return privPEM, pubAuth, nil
	}

	// 自动生成高强度 ED25519 密钥对
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("生成 ED25519 密钥对失败: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", "", fmt.Errorf("转换 SSH 公钥失败: %w", err)
	}
	pubAuth = string(ssh.MarshalAuthorizedKey(sshPub))

	privBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", "", fmt.Errorf("编码 SSH 私钥失败: %w", err)
	}
	privPEM = string(pem.EncodeToMemory(privBlock))

	_ = model.SetSetting("cloud_ssh_private_key", privPEM)
	_ = model.SetSetting("cloud_ssh_public_key", pubAuth)

	return privPEM, pubAuth, nil
}

// RunProvisionVultr 执行 Vultr 实例创建与 6 阶段自动化纳管流水线
func RunProvisionVultr(task *DeployTask, req ProvisionVultrRequest) {
	defer func() {
		if r := recover(); r != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("Panic during vultr provisioning: %v", r)
			task.AddLog("[ERROR] " + task.Error)
		}
	}()

	// 1. 读取并校验 Vultr API Key
	task.AddLog("【阶段 1/6】正在校验 Vultr 凭据并同步主控端部署密钥...")
	apiKey, err := model.GetVultrAPIKey()
	if err != nil || strings.TrimSpace(apiKey) == "" {
		task.Status = "failed"
		task.Error = "未配置 Vultr API Token，请先在面板设置中绑定 Vultr API 凭据"
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	client := vultr.NewClient(apiKey)
	_, err = client.GetAccount()
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("Vultr 账户校验失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	// 确保主控 SSH 公钥存在并注册到 Vultr
	privKeyPEM, pubKeyAuth, err := EnsureMasterSSHKeyPair()
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("准备主控部署公钥失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	vultrKeyID, err := client.EnsureSSHKey("vps-panel-master-deploy-key", pubKeyAuth)
	if err != nil {
		task.AddLog(fmt.Sprintf("警告：同步 SSH 公钥到 Vultr 失败: %v，将使用密码回退机制", err))
	} else {
		task.AddLog(fmt.Sprintf("主控 ED25519 部署公钥已绑定至 Vultr (Key ID: %s)", vultrKeyID))
	}

	// 2. 提交创建云实例请求
	task.AddLog(fmt.Sprintf("【阶段 2/6】向 Vultr 提交云主机创建请求 (区域: %s, 规格: %s, OS ID: %d)...", req.Region, req.Plan, req.OSID))
	label := req.Name
	if strings.TrimSpace(label) == "" {
		label = fmt.Sprintf("vps-%s-%s", req.Region, req.Plan)
	}

	var sshKeyIDs []string
	if vultrKeyID != "" {
		sshKeyIDs = []string{vultrKeyID}
	}

	inst, err := client.CreateInstance(vultr.CreateInstanceRequest{
		Region:     req.Region,
		Plan:       req.Plan,
		OSID:       req.OSID,
		Label:      label,
		SSHKeyIDs:  sshKeyIDs,
		EnableIPv6: req.EnableIPv6,
	})
	if err != nil {
		task.Status = "failed"
		task.Error = fmt.Sprintf("创建 Vultr 实例失败: %v", err)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	task.AddLog(fmt.Sprintf("实例创建请求已成功受理！实例 UUID: %s", inst.ID))

	// 3. 轮询等待实例开机与分配公网 IP
	task.AddLog("【阶段 3/6】正在等待云主机开机并分配公网 IPv4 地址 (通常需要 20~45 秒)...")
	var activeInst *vultr.Instance
	pollDeadline := time.Now().Add(5 * time.Minute)

	for time.Now().Before(pollDeadline) {
		time.Sleep(4 * time.Second)
		polled, err := client.GetInstance(inst.ID)
		if err != nil {
			continue
		}
		if polled.Status == "active" && polled.MainIP != "" && polled.MainIP != "0.0.0.0" {
			activeInst = polled
			break
		}
		task.AddLog(fmt.Sprintf("实例状态: status=%s, power=%s, server=%s...", polled.Status, polled.PowerStatus, polled.ServerStatus))
	}

	if activeInst == nil {
		task.Status = "failed"
		task.Error = "等待 Vultr 实例开机超时（超过 5 分钟未就绪）"
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	task.AddLog(fmt.Sprintf("云主机已成功开机！分配公网 IP: %s (IPv6: %s)", activeInst.MainIP, activeInst.V6MainIP))
	if activeInst.DefaultPassword != "" {
		task.AddLog("已成功同步实例初始 root 密码保护凭据")
	}

	// 4. 探测目标主机 SSH 端口 22 是否启动就绪
	task.AddLog(fmt.Sprintf("【阶段 4/6】正在等待 %s:22 SSH 服务就绪...", activeInst.MainIP))
	sshReady := false
	sshDeadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(sshDeadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(activeInst.MainIP, "22"), 3*time.Second)
		if err == nil {
			conn.Close()
			sshReady = true
			break
		}
		time.Sleep(3 * time.Second)
	}

	if !sshReady {
		task.Status = "failed"
		task.Error = "目标云主机已开机，但 SSH 端口在 3 分钟内未能响应连通"
		task.AddLog("[ERROR] " + task.Error)
		return
	}
	task.AddLog(fmt.Sprintf("目标云主机 SSH 端口已就绪！"))

	// 5. 调用已有 SSH 极速安装与纳管管道
	task.AddLog("【阶段 5/6】通过 SSH 安全信道执行面板自复制推流部署、TLS 指纹生成与守护进程注册...")
	// 组装 SSH 请求
	deployReq := DeploySSHRequest{
		Name:          req.Name,
		Host:          activeInst.MainIP,
		SSHPort:       22,
		SSHUser:       "root",
		AuthType:      "key",
		SSHPrivateKey: privKeyPEM,
		SSHPassword:   activeInst.DefaultPassword,
		PanelPort:     req.PanelPort,
		AdminUsername: req.AdminUsername,
		AdminPassword: req.AdminPassword,
		SecretPath:    req.SecretPath,
		EnableTLS:     req.EnableTLS,
	}
	if vultrKeyID == "" && activeInst.DefaultPassword != "" {
		deployReq.AuthType = "password"
	}

	// 执行标准安装流程
	RunDeploySSH(task, deployReq)

	// 若安装成功，补充打上 Vultr 云主机专属元数据标签
	if task.Status == "success" && task.NodeID > 0 {
		targetNode, err := model.GetNode(task.NodeID)
		if err == nil && targetNode != nil {
			targetNode.CloudProvider = "vultr"
			targetNode.CloudInstanceID = activeInst.ID
			targetNode.Region = req.Region
			targetNode.Plan = req.Plan
			_ = model.UpdateNode(targetNode)
		}
		task.AddLog(fmt.Sprintf("【阶段 6/6】[SUCCESS] Vultr 云主机 %s (IP: %s) 弹性创建与集群纳管全部就绪！", req.Name, activeInst.MainIP))
	}
}

// DestroyVultrInstanceIfBound 辅助函数：当用户要求同时销毁云端实例时执行
func DestroyVultrInstanceIfBound(nodeID int64) error {
	node, err := model.GetNode(nodeID)
	if err != nil {
		return err
	}
	if node.CloudProvider != "vultr" || strings.TrimSpace(node.CloudInstanceID) == "" {
		return nil
	}

	apiKey, err := model.GetVultrAPIKey()
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return errors.New("未配置 Vultr API Token，无法调用云厂商接口销毁实例")
	}

	client := vultr.NewClient(apiKey)
	return client.DeleteInstance(node.CloudInstanceID)
}
