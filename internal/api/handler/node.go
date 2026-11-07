package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
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

// StartDeployNode 开启异步 SSH 部署任务
func StartDeployNode(c *gin.Context) {
	var req node.DeploySSHRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	if strings.TrimSpace(req.Host) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "目标服务器地址 (Host) 不能为空"})
		return
	}

	task := node.CreateDeployTask(req.Name, req.Host)
	go node.RunDeploySSH(task, req)

	c.JSON(http.StatusOK, gin.H{
		"task_id": task.ID,
		"message": "SSH 部署任务已启动",
	})
}

// GetDeployStatus 查询部署任务当前状态
func GetDeployStatus(c *gin.Context) {
	taskID := c.Query("task_id")
	task, err := node.GetDeployTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_id":    task.ID,
		"node_name":  task.NodeName,
		"host":       task.Host,
		"status":     task.Status,
		"error":      task.Error,
		"node_id":    task.NodeID,
		"started_at": task.StartedAt,
		"logs":       task.Logs,
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

	notify := c.Request.Context().Done()
	for {
		select {
		case <-notify:
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			c.SSEvent("message", line)
			c.Writer.Flush()
			if strings.Contains(line, "[SUCCESS]") || strings.Contains(line, "[ERROR]") {
				return
			}
		}
	}
}

// ImportNode 直接纳管已手动安装面板的服务器
func ImportNode(c *gin.Context) {
	var req struct {
		Name       string `json:"name"`
		Host       string `json:"host"`
		PanelPort  int    `json:"panel_port"`
		SecretPath string `json:"secret_path"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效: " + err.Error()})
		return
	}

	if req.PanelPort <= 0 {
		req.PanelPort = 20888
	}
	if req.Username == "" {
		req.Username = "admin"
	}
	if req.Name == "" {
		req.Name = req.Host
	}

	// 远程验证账号密码并获取通信 Token
	token, err := node.LoginNode(req.Host, req.PanelPort, req.SecretPath, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法连接或登录该节点: " + err.Error()})
		return
	}

	// 加密凭据
	jwtSecret, _ := model.GetSetting("jwt_secret")
	key := crypto.DeriveKey(jwtSecret)
	encryptedPass, err := crypto.EncryptAESGCM(req.Password, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加密凭据失败: " + err.Error()})
		return
	}

	newNode := &model.Node{
		Name:              req.Name,
		Host:              req.Host,
		SSHPort:           22,
		PanelPort:         req.PanelPort,
		SecretPath:        req.SecretPath,
		Username:          req.Username,
		PasswordEncrypted: encryptedPass,
		APIToken:          token,
		IsMaster:          false,
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

// DeleteNode 删除指定纳管节点
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

	if err := model.DeleteNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "节点已移出集群"})
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
		c.JSON(http.StatusForbidden, gin.H{"error": "主控管理员密码错误，二次安全验证失败！"})
		return
	}

	// 2. 校验成功，查询目标节点
	target, err := model.GetNode(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标节点不存在"})
		return
	}

	if target.IsMaster {
		// 本机节点直接返回当前账号
		adminUser, _ := model.GetSetting("admin_username")
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
	jwtSecret, _ := model.GetSetting("jwt_secret")
	key := crypto.DeriveKey(jwtSecret)
	decryptedPass, err := crypto.DecryptAESGCM(target.PasswordEncrypted, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解密凭据失败: " + err.Error()})
		return
	}

	loginURL := fmt.Sprintf("http://%s:%d/%s/", target.Host, target.PanelPort, strings.Trim(target.SecretPath, "/"))

	c.JSON(http.StatusOK, gin.H{
		"id":          target.ID,
		"name":        target.Name,
		"username":    target.Username,
		"password":    decryptedPass,
		"secret_path": target.SecretPath,
		"panel_port":  target.PanelPort,
		"host":        target.Host,
		"login_url":   loginURL,
		"is_master":   false,
	})
}
