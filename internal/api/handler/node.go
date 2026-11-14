package handler

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/api/middleware"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
	"vps-auto-deploy/internal/notifier"
	"vps-auto-deploy/internal/pkg/crypto"
)

// ListNodes 列出集群中所有节点
func ListNodes(c *gin.Context) {
	nodes, err := model.ListNodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点列表失败: " + err.Error()})
		return
	}
	if nodes == nil {
		nodes = []model.Node{}
	}
	c.JSON(http.StatusOK, nodes)
}

func validateTargetNodeHost(host string) error {
	cleanHost := strings.TrimSpace(host)
	if cleanHost == "" {
		return fmt.Errorf("目标服务器地址 (Host) 不能为空")
	}
	if strings.ContainsAny(cleanHost, " \t\r\n/\\;@") {
		return fmt.Errorf("目标服务器地址格式包含非法字符")
	}
	// 严禁将云厂商元数据地址作为纳管目标 (防 SSRF)
	if cleanHost == "169.254.169.254" || strings.HasPrefix(cleanHost, "169.254.") {
		return fmt.Errorf("【安全拦截】禁止将链路本地或云元数据地址作为目标主机")
	}
	if os.Getenv("ALLOW_PRIVATE_IP") != "1" {
		lower := strings.ToLower(cleanHost)
		if lower == "localhost" || lower == "127.0.0.1" || lower == "::1" {
			return fmt.Errorf("【安全拦截】禁止将主控本机回环地址作为远程受管节点")
		}
	}
	return nil
}

// StartDeployNode 开启异步 SSH 部署任务
func StartDeployNode(c *gin.Context) {
	var req node.DeploySSHRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	if err := validateTargetNodeHost(req.Host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task := node.CreateDeployTask(req.Name, req.Host)
	go node.RunDeploySSH(task, req)

	c.JSON(http.StatusOK, gin.H{
		"task_id": task.ID,
		"message": "SSH 部署任务已启动",
	})
}

// StartUpgradeNode 开启异步节点软件推流升级任务 (支持 API 免密升级与 SSH 救援升级)
func StartUpgradeNode(c *gin.Context) {
	var req node.UpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	taskName := ""
	if req.NodeID > 0 {
		if n, err := model.GetNode(req.NodeID); err == nil && n != nil {
			taskName = "升级节点: " + n.Name
			if strings.TrimSpace(req.Host) == "" {
				req.Host = n.Host
			}
			if req.SSHPort <= 0 && n.SSHPort > 0 {
				req.SSHPort = n.SSHPort
			}
		}
	}

	if strings.TrimSpace(req.Host) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "目标服务器地址 (Host) 不能为空"})
		return
	}

	if taskName == "" {
		taskName = "升级节点: " + req.Host
	}

	task := node.CreateDeployTask(taskName, req.Host)
	task.NodeID = req.NodeID

	// 模式判定：默认为 API 免密升级；若显式指定 mode == "ssh"，走 SSH 通道
	if req.Mode == "ssh" {
		go node.RunUpgradeSSH(task, req)
		c.JSON(http.StatusOK, gin.H{
			"task_id": task.ID,
			"message": "SSH 节点软件推流升级任务已启动",
		})
	} else {
		req.Mode = "api"
		go node.RunUpgradeAPI(task, req)
		c.JSON(http.StatusOK, gin.H{
			"task_id": task.ID,
			"message": "节点免密平滑升级任务已启动",
		})
	}
}

// GetDeployStatus 查询部署任务当前状态 (并发安全读取快照，防 Data Race)
func GetDeployStatus(c *gin.Context) {
	taskID := c.Query("task_id")
	task, err := node.GetDeployTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	status, errMsg, logs := task.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"task_id":           task.ID,
		"node_name":         task.NodeName,
		"host":              task.Host,
		"status":            status,
		"error":             errMsg,
		"node_id":           task.NodeID,
		"cloud_provider":    task.CloudProvider,
		"cloud_instance_id": task.CloudInstanceID,
		"cloud_region":      task.CloudRegion,
		"cloud_plan":        task.CloudPlan,
		"started_at":        task.StartedAt,
		"logs":              logs,
	})
}

// StreamDeployLogs 通过 SSE 实时推送部署日志流
func StreamDeployLogs(c *gin.Context) {
	taskID := c.Query("task_id")
	task, err := node.GetDeployTask(taskID)
	if err != nil {
		c.String(http.StatusNotFound, "任务不存在")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	ch := task.Subscribe()
	defer task.Unsubscribe(ch)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	notify := c.Request.Context().Done()
	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			// 发送 SSE 规范心跳注释，防止反向代理 (Nginx/Cloudflare) 或浏览器静默丢包超时
			_, err := c.Writer.Write([]byte(": keep-alive\n\n"))
			if err != nil {
				return
			}
			c.Writer.Flush()
		case line, ok := <-ch:
			if !ok {
				return
			}
			c.SSEvent("message", line)
			c.Writer.Flush()
			if strings.Contains(line, "[SUCCESS]") || strings.Contains(line, "[ERROR]") {
				// 给客户端 600ms 缓冲时间读取并关闭连接，避免服务端瞬间截断导致前端误触 onerror
				time.Sleep(600 * time.Millisecond)
				return
			}
		}
	}
}

// ImportNode 直接纳管已手动安装面板的服务器
func ImportNode(c *gin.Context) {
	var req struct {
		Name            string `json:"name"`
		SubPrefix       string `json:"sub_prefix"`
		Host            string `json:"host"`
		PanelPort       int    `json:"panel_port"`
		SecretPath      string `json:"secret_path"`
		Username        string `json:"username"`
		Password        string `json:"password"`
		UseTLS          bool   `json:"use_tls"`
		CertFingerprint string `json:"cert_fingerprint"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效: " + err.Error()})
		return
	}

	if req.PanelPort <= 0 {
		req.PanelPort = 20888
	}
	if err := validateTargetNodeHost(req.Host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Username == "" {
		req.Username = "admin"
	}
	if req.Name == "" {
		req.Name = req.Host
	}

	if req.UseTLS && strings.TrimSpace(req.CertFingerprint) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "启用 HTTPS 自签加密通信时，必须提供节点的 TLS 证书指纹 (防中间人劫持)"})
		return
	}

	// 远程验证账号密码并获取通信 Token (支持 TLS 指纹校验防 MITM)
	token, err := node.LoginNode(req.Host, req.PanelPort, req.SecretPath, req.Username, req.Password, req.UseTLS, req.CertFingerprint)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法连接或登录该节点: " + err.Error()})
		return
	}

	// 加密凭据 (使用专用的静态数据加密主密钥)
	key := model.GetEncryptionKey()
	encryptedPass, err := crypto.EncryptAESGCM(req.Password, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加密凭据失败: " + err.Error()})
		return
	}

	subPrefix := strings.TrimSpace(req.SubPrefix)
	if subPrefix == "" {
		subPrefix = req.Name
	}

	newNode := &model.Node{
		Name:              req.Name,
		SubPrefix:         subPrefix,
		Host:              req.Host,
		SSHPort:           22,
		PanelPort:         req.PanelPort,
		SecretPath:        req.SecretPath,
		Username:          req.Username,
		PasswordEncrypted: encryptedPass,
		APIToken:          token,
		IsMaster:          false,
		UseTLS:            req.UseTLS,
		CertFingerprint:   req.CertFingerprint,
		Status:            "online",
		OSInfo:            "Remote Linux",
		LatencyMs:         10,
	}

	nodeID, err := model.CreateNode(newNode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存节点失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "成功纳管节点",
		"node_id": nodeID,
	})
}

// PingNode 测速并更新节点在线状态
func PingNode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	target, err := model.GetNode(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "节点未找到"})
		return
	}

	latency, err := node.PingNode(target)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":     "offline",
			"latency_ms": 0,
			"error":      err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "online",
		"latency_ms": latency,
	})
}

// UpdateNode 更新节点基础信息（如展示名称与订阅节点前缀）
func UpdateNode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	target, err := model.GetNode(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标节点不存在"})
		return
	}

	var req struct {
		Name      string `json:"name"`
		SubPrefix string `json:"sub_prefix"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name != "" {
		target.Name = name
	}
	target.SubPrefix = strings.TrimSpace(req.SubPrefix)

	if err := model.UpdateNodePrefix(target.ID, target.Name, target.SubPrefix); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新节点失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "节点信息更新成功",
		"id":         target.ID,
		"name":       target.Name,
		"sub_prefix": target.SubPrefix,
	})
}

// DeleteNode 删除指定纳管节点（支持联动彻底销毁云端实例）
func DeleteNode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	if id == 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "主控本机节点受保护，禁止删除"})
		return
	}

	targetNode, _ := model.GetNode(id)

	// 检查是否要求同时销毁云端实例
	var req struct {
		DestroyCloud   bool   `json:"destroy_cloud"`
		MasterPassword string `json:"master_password"`
	}
	_ = c.ShouldBindJSON(&req)

	destroyCloud := req.DestroyCloud || c.Query("destroy_cloud") == "true" || c.Query("destroy_cloud") == "1"
	if destroyCloud {
		masterPassword := strings.TrimSpace(req.MasterPassword)
		if masterPassword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "彻底销毁云端实例需在请求体中提供主控管理员密码进行二次安全校验"})
			return
		}

		masterPassHash, err := model.GetSetting("admin_password_hash")
		if err != nil || bcrypt.CompareHashAndPassword([]byte(masterPassHash), []byte(masterPassword)) != nil {
			middleware.RecordFailedLogin(middleware.GetRealClientIP(c))
			c.JSON(http.StatusForbidden, gin.H{"error": "主控管理员密码错误，二次安全验证失败！拒绝销毁云主机"})
			return
		}
		middleware.ResetLoginAttempts(middleware.GetRealClientIP(c))

		if err := node.DestroyVultrInstanceIfBound(id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "销毁云厂商主机失败: " + err.Error()})
			return
		}
	}

	if err := model.DeleteNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "从数据库删除节点失败: " + err.Error()})
		return
	}

	nodeName := "未知节点"
	nodeHost := ""
	if targetNode != nil {
		nodeName = targetNode.Name
		nodeHost = targetNode.Host
	}
	notifier.Send(notifier.Event{
		Type:      notifier.EventNodeDeleted,
		Level:     notifier.LevelWarn,
		Title:     "节点已移出集群",
		Message:   fmt.Sprintf("节点 [%s] (%s) 已被管理员从控制平面移出删除", nodeName, nodeHost),
		ClientIP:  middleware.GetRealClientIP(c),
		NodeName:  nodeName,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"node_id":       strconv.FormatInt(id, 10),
			"destroy_cloud": strconv.FormatBool(destroyCloud),
		},
	})

	msg := "节点已移出集群"
	if destroyCloud {
		msg = "节点已移出集群，且云端实例已成功向云厂商提交销毁请求并停止计费"
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

// RevealNodeCredentials 二次安全鉴权：验证主控密码后解密查看被纳管节点凭据
func RevealNodeCredentials(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	var req struct {
		MasterPassword string `json:"master_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供主控端管理员密码进行二次安全校验"})
		return
	}

	// 1. 二次鉴权：对比主控端密码哈希
	masterPassHash, err := model.GetSetting("admin_password_hash")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取主控安全配置失败"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(masterPassHash), []byte(req.MasterPassword)); err != nil {
		middleware.RecordFailedLogin(middleware.GetRealClientIP(c))
		c.JSON(http.StatusForbidden, gin.H{"error": "主控管理员密码错误，二次安全验证失败！"})
		return
	}
	middleware.ResetLoginAttempts(middleware.GetRealClientIP(c))

	// 2. 校验成功，查询目标节点
	target, err := model.GetNode(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标节点不存在"})
		return
	}

	if target.IsMaster {
		// 本机节点直接返回当前账号 (提示语非硬编码密码)
		adminUser, _ := model.GetSetting("admin_username")
		/* #nosec G101 */
		c.JSON(http.StatusOK, gin.H{
			"id":          target.ID,
			"name":        target.Name,
			"username":    adminUser,
			"password":    "(主控本机密码为当前登录密码)",
			"secret_path": target.SecretPath,
			"panel_port":  target.PanelPort,
			"is_master":   true,
		})
		return
	}

	// 3. 解密远程节点密码
	decryptedPass, err := model.DecryptSensitiveData(target.PasswordEncrypted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解密凭据失败: " + err.Error()})
		return
	}

	scheme := "http"
	if target.UseTLS {
		scheme = "https"
	}
	loginURL := fmt.Sprintf("%s://%s:%d/%s/", scheme, target.Host, target.PanelPort, strings.Trim(target.SecretPath, "/"))

	notifier.Send(notifier.Event{
		Type:      notifier.EventCredsRevealed,
		Level:     notifier.LevelWarn,
		Title:     "节点机密凭据被查阅",
		Message:   fmt.Sprintf("管理员通过二次鉴权查看了节点 [%s] (%s) 的登录密码与连接凭据", target.Name, target.Host),
		ClientIP:  middleware.GetRealClientIP(c),
		Username:  target.Username,
		NodeName:  target.Name,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"node_id":   strconv.FormatInt(target.ID, 10),
			"node_host": target.Host,
		},
	})

	c.JSON(http.StatusOK, gin.H{
		"id":               target.ID,
		"name":             target.Name,
		"username":         target.Username,
		"password":         decryptedPass,
		"secret_path":      target.SecretPath,
		"panel_port":       target.PanelPort,
		"host":             target.Host,
		"login_url":        loginURL,
		"is_master":        false,
		"use_tls":          target.UseTLS,
		"cert_fingerprint": target.CertFingerprint,
		"ssh_host_key":     target.SSHHostKey,
	})
}
