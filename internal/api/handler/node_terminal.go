package handler

import (
	"log"
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

// isKnownPanelHost 校验目标 Host/域名是否属于当前受管面板资产（公网IP/域名、集群节点Host、隧道绑定的自定义域名）
func isKnownPanelHost(host string) bool {
	if host == "" {
		return false
	}
	host = strings.TrimSpace(host)

	// 1. 匹配 public_ip 配置
	if pubHost, _ := model.GetSetting("public_ip"); pubHost != "" {
		if strings.EqualFold(host, strings.TrimSpace(pubHost)) {
			return true
		}
	}

	// 2. 匹配集群节点 Host 与隧道域名
	if model.DB != nil {
		var count int
		_ = model.DB.QueryRow("SELECT COUNT(1) FROM nodes WHERE host = ? LIMIT 1", host).Scan(&count)
		if count > 0 {
			return true
		}

		_ = model.DB.QueryRow("SELECT COUNT(1) FROM tunnels WHERE domain = ? LIMIT 1", host).Scan(&count)
		if count > 0 {
			return true
		}
	}

	return false
}

// isAllowedTerminalOrigin 智能校验 WebSocket Origin 合法性 (强力防御 CSWSH 跨源劫持)
func isAllowedTerminalOrigin(r *http.Request, allowedHost string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // 允许非浏览器或内联反代客户端连接
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	origHost := u.Hostname()

	// 1. 票据绑定来源主机校验 (最高优先级与安全性)：
	// 确保当前发起 WebSocket 的 Origin 与管理员刚刚通过二次密码鉴权签发 Ticket 的页面 Origin 完全一致！
	// 无论经过 Cloudflare CDN、Cloudflare Tunnel、Nginx 还是多级反代，均能 100% 信任匹配。
	if allowedHost != "" && strings.EqualFold(origHost, allowedHost) {
		return true
	}

	// 2. 直连请求：校验 Host / Hostname 是否匹配
	if strings.EqualFold(u.Host, r.Host) || strings.EqualFold(origHost, r.Host) {
		return true
	}
	reqHostOnly, _, _ := net.SplitHostPort(r.Host)
	if reqHostOnly != "" && strings.EqualFold(origHost, reqHostOnly) {
		return true
	}

	// 3. 反向代理匹配 X-Forwarded-Host 或 X-Original-Host
	for _, hdr := range []string{"X-Forwarded-Host", "X-Original-Host"} {
		if fwdHost := r.Header.Get(hdr); fwdHost != "" {
			if strings.EqualFold(u.Host, fwdHost) || strings.EqualFold(origHost, fwdHost) {
				return true
			}
			fwdHostOnly, _, _ := net.SplitHostPort(fwdHost)
			if fwdHostOnly != "" && strings.EqualFold(origHost, fwdHostOnly) {
				return true
			}
		}
	}

	// 4. 本地回环或局域网管理：若 Origin 本身是 localhost、127.0.0.1 或私有内网 IP，放行
	if origHost == "localhost" || node.IsLoopbackOrPrivate(origHost) {
		return true
	}

	// 5. 面板资产匹配：若 Origin 对应配置的 public_ip、集群纳管节点 Host 或已配置的隧道域名，放行
	if isKnownPanelHost(origHost) {
		return true
	}

	/* #nosec G706 - origin and host are sanitized against CRLF injection via cleanLogString */
	log.Printf("[WebSocket CheckOrigin] 拦截非法跨源请求 (CSWSH 防护): origin=%s, allowedHost=%s, host=%s, remoteIP=%s", cleanLogString(origin), cleanLogString(allowedHost), cleanLogString(r.Host), cleanLogString(r.RemoteAddr))
	return false
}

// cleanLogString 过滤换行符与截断超长字符串，杜绝日志注入 (CWE-117)
func cleanLogString(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	if len(s) > 128 {
		return s[:128] + "..."
	}
	return s
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedTerminalOrigin(r, "")
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

	// 3. 提取当前经二次鉴权合法客户端的来源主机名 (Origin / Referer / Host)
	allowedHost := ""
	if origin := c.GetHeader("Origin"); origin != "" {
		if u, err := url.Parse(origin); err == nil && u.Hostname() != "" {
			allowedHost = u.Hostname()
		}
	}
	if allowedHost == "" {
		if ref := c.GetHeader("Referer"); ref != "" {
			if u, err := url.Parse(ref); err == nil && u.Hostname() != "" {
				allowedHost = u.Hostname()
			}
		}
	}
	if allowedHost == "" {
		if h, _, err := net.SplitHostPort(c.Request.Host); err == nil && h != "" {
			allowedHost = h
		} else {
			allowedHost = c.Request.Host
		}
	}

	// 4. 签发 30 秒有效的一次性 Ticket，并绑定临时凭据载荷与合法来源主机
	clientIP := middleware.GetRealClientIP(c)
	ticket := node.IssueTerminalTicket(target.ID, usernameStr, clientIP, allowedHost, req.TempUser, req.TempPass, req.TempKey, 30*time.Second)

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
	clientIP := middleware.GetRealClientIP(c)
	ticketInfo, ok := node.ValidateAndConsumeTicket(ticket, id, clientIP)
	if !ok || ticketInfo == nil {
		log.Printf("[NodeTerminalWS] 终端票据校验失败或已过期: nodeID=%d, clientIP=%s, ticketLen=%d", id, cleanLogString(clientIP), len(ticket))
		c.String(http.StatusUnauthorized, "invalid or expired terminal ticket")
		return
	}

	target, err := model.GetNode(id)
	if err != nil {
		log.Printf("[NodeTerminalWS] 目标节点未找到: nodeID=%d", id)
		c.String(http.StatusNotFound, "target node not found")
		return
	}

	cols, _ := strconv.Atoi(c.DefaultQuery("cols", "110"))
	rows, _ := strconv.Atoi(c.DefaultQuery("rows", "32"))
	if cols < 10 || cols > 1000 {
		cols = 110
	}
	if rows < 5 || rows > 1000 {
		rows = 32
	}

	wsUpgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return isAllowedTerminalOrigin(r, ticketInfo.AllowedHost)
		},
	}

	ws, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[NodeTerminalWS] WebSocket 协议升级失败: nodeID=%d, err=%v", id, err)
		return
	}

	node.HandleTerminalSession(ws, target, cols, rows, ticketInfo)
}
