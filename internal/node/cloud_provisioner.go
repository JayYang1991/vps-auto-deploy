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
	InstanceID    string `json:"instance_id,omitempty"` // 若指定，则跳过创建，直接纳管已有实例
	Name          string `json:"name"`
	Region        string `json:"region"`
	Plan          string `json:"plan"`
	OSID          int    `json:"os_id"`
	EnableIPv6    bool   `json:"enable_ipv6"`
	AuthType      string `json:"auth_type,omitempty"` // "master_key" | "password" | "key"
	SSHPassword   string `json:"ssh_password,omitempty"`
	SSHPrivateKey string `json:"ssh_private_key,omitempty"`
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

	// 2. 获取已有实例或提交创建云实例请求
	var inst *vultr.Instance
	if req.InstanceID != "" {
		task.AddLog(fmt.Sprintf("【阶段 2/6】指定纳管已有 Vultr 实例 (UUID: %s)，正在读取云端配置...", req.InstanceID))
		existingInst, err := client.GetInstance(req.InstanceID)
		if err != nil {
			task.Status = "failed"
			task.Error = fmt.Sprintf("获取已有 Vultr 实例信息失败: %v", err)
			task.AddLog("[ERROR] " + task.Error)
			return
		}
		inst = existingInst
		if req.Name == "" {
			req.Name = inst.Label
		}
		if req.Region == "" {
			req.Region = inst.Region
		}
		if req.Plan == "" {
			req.Plan = inst.Plan
		}
		task.AddLog(fmt.Sprintf("已关联已有实例: %s (IP: %s, 区域: %s, 规格: %s)", inst.Label, inst.MainIP, inst.Region, inst.Plan))
	} else {
		task.AddLog(fmt.Sprintf("【阶段 2/6】向 Vultr 提交云主机创建请求 (区域: %s, 规格: %s, OS ID: %d)...", req.Region, req.Plan, req.OSID))
		label := req.Name
		if strings.TrimSpace(label) == "" {
			label = fmt.Sprintf("vps-%s-%s", req.Region, req.Plan)
		}

		var sshKeyIDs []string
		if vultrKeyID != "" {
			sshKeyIDs = []string{vultrKeyID}
		}

		createdInst, err := client.CreateInstance(vultr.CreateInstanceRequest{
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
		inst = createdInst
		task.AddLog(fmt.Sprintf("实例创建请求已成功受理！实例 UUID: %s", inst.ID))
	}

	task.CloudProvider = "vultr"
	task.CloudInstanceID = inst.ID
	task.CloudRegion = req.Region
	task.CloudPlan = req.Plan

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
		task.Host = polled.MainIP
		if polled.Status == "active" && polled.MainIP != "" && polled.MainIP != "0.0.0.0" {
			activeInst = polled
			if polled.ServerStatus == "ok" {
				break
			}
			task.AddLog(fmt.Sprintf("已分配 IP %s，系统镜像正在引导初始化 (server_status=%s, power=%s)...", polled.MainIP, polled.ServerStatus, polled.PowerStatus))
		} else {
			task.AddLog(fmt.Sprintf("等待实例就绪: status=%s, power=%s, server=%s...", polled.Status, polled.PowerStatus, polled.ServerStatus))
		}
	}

	if activeInst == nil {
		task.Status = "failed"
		task.Error = "等待 Vultr 实例开机超时（超过 5 分钟未就绪）"
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	task.Host = activeInst.MainIP
	task.AddLog(fmt.Sprintf("云主机已成功开机！分配公网 IP: %s (IPv6: %s)", activeInst.MainIP, activeInst.V6MainIP))
	if activeInst.DefaultPassword != "" {
		task.AddLog("已同步实例 root 初始安全凭据")
	}

	// 4. 等待网络完全打通与 SSH 握手就绪（包含 IP 路由连通与 cloud-init 密钥写入）
	task.AddLog(fmt.Sprintf("【阶段 4/6】正在等待云主机 %s 网络路由通畅与 SSH 握手就绪 (通常需 30~90 秒)...", activeInst.MainIP))

	// 准备探测所用的 SSH Auth 方法
	var probeAuthMethods []ssh.AuthMethod
	if req.AuthType == "password" && req.SSHPassword != "" {
		probeAuthMethods = append(probeAuthMethods, ssh.Password(req.SSHPassword))
	} else if req.AuthType == "key" && req.SSHPrivateKey != "" {
		if signer, err := ssh.ParsePrivateKey([]byte(req.SSHPrivateKey)); err == nil {
			probeAuthMethods = append(probeAuthMethods, ssh.PublicKeys(signer))
		}
	} else {
		// 默认主控部署公钥 + root 密码备用
		if masterSigner, err := ssh.ParsePrivateKey([]byte(privKeyPEM)); err == nil {
			probeAuthMethods = append(probeAuthMethods, ssh.PublicKeys(masterSigner))
		}
		if req.SSHPassword != "" {
			probeAuthMethods = append(probeAuthMethods, ssh.Password(req.SSHPassword))
		} else if activeInst.DefaultPassword != "" {
			probeAuthMethods = append(probeAuthMethods, ssh.Password(activeInst.DefaultPassword))
		}
	}

	sshReady := false
	startTime := time.Now()
	sshDeadline := time.Now().Add(5 * time.Minute)
	lastLogTime := time.Time{}

	for time.Now().Before(sshDeadline) {
		// 先探测 TCP 端口连通
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(activeInst.MainIP, "22"), 3*time.Second)
		if err != nil {
			elapsed := int(time.Since(startTime).Seconds())
			if time.Since(lastLogTime) >= 12*time.Second {
				task.AddLog(fmt.Sprintf("等待公网 IP 路由与端口响应中 (已等待 %d 秒)...", elapsed))
				lastLogTime = time.Now()
			}
			time.Sleep(4 * time.Second)
			continue
		}
		conn.Close()

		// TCP 端口通了，现在尝试实际 SSH 握手探测（确保 cloud-init 已写入 key 且 sshd 正常响应）
		sshConfig := &ssh.ClientConfig{
			User:            "root",
			Auth:            probeAuthMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		}
		sshClient, sshErr := ssh.Dial("tcp", net.JoinHostPort(activeInst.MainIP, "22"), sshConfig)
		if sshErr == nil {
			sshClient.Close()
			sshReady = true
			task.AddLog(fmt.Sprintf("[OK] 目标云主机网络打通且 SSH 认证成功 (耗时 %d 秒)！", int(time.Since(startTime).Seconds())))
			break
		}

		elapsed := int(time.Since(startTime).Seconds())
		if time.Since(lastLogTime) >= 10*time.Second {
			task.AddLog(fmt.Sprintf("SSH 端口已通，系统 cloud-init 正在配置授权公钥与环境，持续等待就绪 (%d 秒)...", elapsed))
			lastLogTime = time.Now()
		}
		time.Sleep(4 * time.Second)
	}

	if !sshReady {
		task.Status = "failed"
		task.Error = fmt.Sprintf("云主机 IP %s 已开机，但 SSH 认证在 5 分钟内未能就绪 (可能系统初始化较慢，您可点击「重试纳管」重新连接)", activeInst.MainIP)
		task.AddLog("[ERROR] " + task.Error)
		return
	}

	// 5. 调用已有 SSH 极速安装与纳管管道
	task.AddLog("【阶段 5/6】通过 SSH 安全信道执行面板自复制推流部署、TLS 指纹生成与守护进程注册...")
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

	if req.AuthType == "password" && req.SSHPassword != "" {
		deployReq.AuthType = "password"
		deployReq.SSHPassword = req.SSHPassword
	} else if req.AuthType == "key" && req.SSHPrivateKey != "" {
		deployReq.AuthType = "key"
		deployReq.SSHPrivateKey = req.SSHPrivateKey
	} else if req.SSHPassword != "" {
		deployReq.SSHPassword = req.SSHPassword
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

