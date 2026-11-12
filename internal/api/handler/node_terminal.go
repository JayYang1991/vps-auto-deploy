package handler

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/api/middleware"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
	"vps-auto-deploy/internal/pkg/crypto"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // 允许非浏览器或内联反代客户端连接
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}

		// 1. 直连请求：校验 Host 是否完全匹配 (防跨源 WebSocket 劫持 CSWSH)
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}

		// 2. 反向代理/隧道模式：若请求来自本地回环，允许匹配 X-Forwarded-Host 或已配置的 public_ip
		remoteIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
			remoteIP = host
		}
		if remoteIP == "127.0.0.1" || remoteIP == "::1" || strings.HasPrefix(remoteIP, "127.") {
			if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
				if strings.EqualFold(u.Host, fwdHost) {
					return true
				}
			}
			if pubHost, _ := model.GetSetting("public_ip"); pubHost != "" {
				if strings.EqualFold(u.Hostname(), pubHost) {
					return true
				}
			}
		}

		return false
	},
}

// RequestTerminalTicket 请求签发一次性 SSH 控制台连接票据 (需二次密码校验)
func RequestTerminalTicket(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	var req struct {
		MasterPassword string `json:"master_password"`
		TempUser       string `json:"temp_user"`
		TempPass       string `json:"temp_pass"`
		TempKey        string `json:"temp_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供主控端管理员密码进行二次安全校验"})
		return
	}

	// 1. 二次鉴权验证主控密码 (带防爆破失败计数)
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

	// 2. 验证目标节点是否存在
	target, err := model.GetNode(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标节点不存在"})
		return
	}

	usernameVal, _ := c.Get("username")
	usernameStr, _ := usernameVal.(string)
	if usernameStr == "" {
		usernameStr = "admin"
	}

	// 3. 签发 30 秒有效的一次性 Ticket，并绑定内存临时凭据载荷
	ticket := node.IssueTerminalTicket(target.ID, usernameStr, c.ClientIP(), req.TempUser, req.TempPass, req.TempKey, 30*time.Second)

	c.JSON(http.StatusOK, gin.H{
		"ticket":       ticket,
		"expires_in":   30,
		"is_master":    target.IsMaster,
		"has_ssh_auth": target.HasSSHAuth,
		"ssh_user":     target.SSHUser,
		"host":         target.Host,
		"ssh_port":     target.SSHPort,
	})
}

// SaveNodeSSHCredentials 保存/更新目标节点的持久化加密 SSH 凭据
func SaveNodeSSHCredentials(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	var req struct {
		MasterPassword string `json:"master_password"`
		SSHUser        string `json:"ssh_user"`
		SSHPassword    string `json:"ssh_password"`
		SSHPrivateKey  string `json:"ssh_private_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	// 二次鉴权 (防爆破)
	masterPassHash, _ := model.GetSetting("admin_password_hash")
	if err := bcrypt.CompareHashAndPassword([]byte(masterPassHash), []byte(req.MasterPassword)); err != nil {
		middleware.RecordFailedLogin(middleware.GetRealClientIP(c))
		c.JSON(http.StatusForbidden, gin.H{"error": "主控管理员密码错误，二次验证失败！"})
		return
	}
	middleware.ResetLoginAttempts(middleware.GetRealClientIP(c))

	target, err := model.GetNode(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标节点不存在"})
		return
	}

	key := model.GetEncryptionKey()

	var passEnc, keyEnc string
	if req.SSHPassword != "" {
		passEnc, _ = crypto.EncryptAESGCM(req.SSHPassword, key)
	}
	if req.SSHPrivateKey != "" {
		keyEnc, _ = crypto.EncryptAESGCM(req.SSHPrivateKey, key)
	}
	sshUser := strings.TrimSpace(req.SSHUser)
	if sshUser == "" {
		sshUser = "root"
	}

	if err := model.UpdateNodeSSHCredentials(target.ID, sshUser, passEnc, keyEnc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新凭据失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SSH 凭据已加密保存"})
}

// NodeTerminalWS 建立与目标节点 SSH / PTY 的全双工 WebSocket 连接
func NodeTerminalWS(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid node id")
		return
	}

	ticket := c.Query("ticket")
	ticketInfo, ok := node.ValidateAndConsumeTicket(ticket, id)
	if !ok || ticketInfo == nil {
		c.String(http.StatusUnauthorized, "invalid or expired terminal ticket")
		return
	}

	target, err := model.GetNode(id)
	if err != nil {
		c.String(http.StatusNotFound, "target node not found")
		return
	}

	cols, _ := strconv.Atoi(c.DefaultQuery("cols", "110"))
	rows, _ := strconv.Atoi(c.DefaultQuery("rows", "32"))

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	node.HandleTerminalSession(ws, target, cols, rows, ticketInfo)
}
