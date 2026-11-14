package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/api/middleware"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/notifier"
)

type LoginRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"remember_me"`
}

type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type UpdateSettingsRequest struct {
	SecretPath         string `json:"secret_path"`
	PublicIP           string `json:"public_ip"`
	SessionIdleTimeout *int   `json:"session_idle_timeout"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters"})
		return
	}

	actualUser, _ := model.GetSetting("admin_username")
	hashedPass, _ := model.GetSetting("admin_password_hash")

	// 常数时间比对用户名 (SHA-256 固定长度摘要比对，彻底消除用户名枚举与长度时序侧信道)
	hReqUser := sha256.Sum256([]byte(req.Username))
	hActUser := sha256.Sum256([]byte(actualUser))
	userMatches := subtle.ConstantTimeCompare(hReqUser[:], hActUser[:]) == 1

	// 无论用户名是否匹配，均计算一次 bcrypt，确保响应耗时恒定（~100ms）
	passErr := bcrypt.CompareHashAndPassword([]byte(hashedPass), []byte(req.Password))

	if !userMatches || passErr != nil {
		clientIP := middleware.GetRealClientIP(c)
		middleware.RecordFailedLogin(clientIP)
		notifier.Send(notifier.Event{
			Type:      notifier.EventLoginFailed,
			Level:     notifier.LevelWarn,
			Title:     "管理员登录失败",
			Message:   "检测到一次密码错误的后台登录尝试",
			ClientIP:  clientIP,
			Username:  req.Username,
			Timestamp: time.Now(),
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect username or password"})
		return
	}

	// 认证通过，重置该 IP 失败尝试记录
	clientIP := middleware.GetRealClientIP(c)
	middleware.ResetLoginAttempts(clientIP)
	notifier.Send(notifier.Event{
		Type:      notifier.EventLoginSuccess,
		Level:     notifier.LevelInfo,
		Title:     "管理员登录成功",
		Message:   "管理员已成功登录后台管理控制面板",
		ClientIP:  clientIP,
		Username:  req.Username,
		Timestamp: time.Now(),
	})

	// 若存在初始明文密码记录，首次登录成功后自动擦除，杜绝明文凭据常驻数据库
	if initPass, _ := model.GetSetting("initial_admin_password"); initPass != "" {
		_ = model.SetSetting("initial_admin_password", "")
	}

	// 根据是否勾选“记住我”决定 JWT 签发生命周期 (未勾选 24 小时，勾选 7 天)
	duration := 24 * time.Hour
	if req.RememberMe {
		duration = 7 * 24 * time.Hour
	}

	token, err := middleware.GenerateJWTWithDuration(req.Username, duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":       token,
		"username":    req.Username,
		"remember_me": req.RememberMe,
	})
}

func UpdatePassword(c *gin.Context) {
	var req UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters"})
		return
	}

	if req.NewPassword != strings.TrimSpace(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码首尾不能包含空格"})
		return
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码长度必须介于 8 ~ 128 位之间"})
		return
	}

	hashedPass, _ := model.GetSetting("admin_password_hash")
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPass), []byte(req.OldPassword)); err != nil {
		middleware.RecordFailedLogin(middleware.GetRealClientIP(c))
		c.JSON(http.StatusBadRequest, gin.H{"error": "old password is incorrect"})
		return
	}
	middleware.ResetLoginAttempts(middleware.GetRealClientIP(c))

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt password"})
		return
	}

	_ = model.SetSetting("admin_password_hash", string(newHash))
	_ = model.SetSetting("initial_admin_password", "")

	// 会话轮换 (Session Rotation)：废止此前签发的所有旧会话，消除同一秒边界竞态 (VULN-14)
	nextSecond := time.Now().Add(1 * time.Second).Truncate(time.Second)
	_ = model.SetSetting("auth_min_issued_at", strconv.FormatInt(nextSecond.Unix(), 10))

	// 为当前操作端签发轮换后的全新有效 Token
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr, _ = model.GetSetting("admin_username")
	}
	newToken, _ := middleware.GenerateJWTWithIssuedAt(usernameStr, 7*24*time.Hour, nextSecond)

	notifier.Send(notifier.Event{
		Type:      notifier.EventPasswordChanged,
		Level:     notifier.LevelCritical,
		Title:     "管理员密码已修改",
		Message:   "管理员后台登录密码已被修改，所有旧客户端会话已强制轮换失效",
		ClientIP:  middleware.GetRealClientIP(c),
		Username:  usernameStr,
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "password updated successfully, sessions rotated",
		"token":   newToken,
	})
}

func GetSettings(c *gin.Context) {
	secretPath, _ := model.GetSetting("secret_path")
	publicIP, _ := model.GetSetting("public_ip")
	idleTimeoutStr, err := model.GetSetting("session_idle_timeout")
	idleTimeout := 30 // 默认 30 分钟无操作自动退出
	if err == nil && idleTimeoutStr != "" {
		if v, err := strconv.Atoi(idleTimeoutStr); err == nil {
			idleTimeout = v
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"secret_path":          secretPath,
		"public_ip":            publicIP,
		"session_idle_timeout": idleTimeout,
	})
}

func UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid parameters"})
		return
	}

	if req.SecretPath != "" {
		trimmedSecret := strings.Trim(strings.TrimSpace(req.SecretPath), "/")
		if !regexp.MustCompile(`^[a-zA-Z0-9_\-]{4,64}$`).MatchString(trimmedSecret) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "安全入口路径格式不合法 (仅支持4-64位字母、数字、短横线与下划线)"})
			return
		}
		reserved := map[string]bool{
			"api": true, "sub": true, "assets": true, "static": true, "favicon.ico": true, "index.html": true,
		}
		if reserved[strings.ToLower(trimmedSecret)] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该安全入口路径为系统保留关键字，禁止使用"})
			return
		}
		_ = model.SetSetting("secret_path", trimmedSecret)
		// 安全路径变更，强制废止所有旧会话以防扫描缓存利用 (消除同一秒竞态)
		_ = model.SetSetting("auth_min_issued_at", strconv.FormatInt(time.Now().Unix()+1, 10))
	}
	if req.PublicIP != "" {
		trimmedIP := strings.TrimSpace(req.PublicIP)
		if net.ParseIP(trimmedIP) == nil {
			domainRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
			if !domainRegex.MatchString(trimmedIP) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "公网 IP/域名格式不合法，请输入合法的 IP 地址或域名"})
				return
			}
		}
		_ = model.SetSetting("public_ip", trimmedIP)
	}
	if req.SessionIdleTimeout != nil {
		if *req.SessionIdleTimeout >= 0 {
			_ = model.SetSetting("session_idle_timeout", strconv.Itoa(*req.SessionIdleTimeout))
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings saved"})
}

// RevokeAllSessions 记录全局会话失效时刻，强制注销所有设备上已签发的 Token
func RevokeAllSessions(c *gin.Context) {
	epoch := time.Now().Unix() + 1
	_ = model.SetSetting("auth_min_issued_at", strconv.FormatInt(epoch, 10))

	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr, _ = model.GetSetting("admin_username")
	}

	notifier.Send(notifier.Event{
		Type:      notifier.EventSessionRevoked,
		Level:     notifier.LevelCritical,
		Title:     "所有活跃会话被全局强制注销",
		Message:   "管理员触发了全局会话一键注销，所有设备已签发的凭据已立即全部作废",
		ClientIP:  middleware.GetRealClientIP(c),
		Username:  usernameStr,
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{"message": "所有会话已强制注销，请重新登录"})
}

