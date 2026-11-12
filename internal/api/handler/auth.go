package handler

import (
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

	// 常数时间比对用户名，消除用户名枚举时序侧信道
	userMatches := subtle.ConstantTimeCompare([]byte(req.Username), []byte(actualUser)) == 1

	// 无论用户名是否匹配，均计算一次 bcrypt，确保响应耗时恒定（~100ms）
	passErr := bcrypt.CompareHashAndPassword([]byte(hashedPass), []byte(req.Password))

	if !userMatches || passErr != nil {
		middleware.RecordFailedLogin(middleware.GetRealClientIP(c))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect username or password"})
		return
	}

	// 认证通过，重置该 IP 失败尝试记录
	middleware.ResetLoginAttempts(middleware.GetRealClientIP(c))

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

	newPwd := strings.TrimSpace(req.NewPassword)
	if len(newPwd) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码长度不能少于 8 位"})
		return
	}
	if len(newPwd) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码长度不能超过 128 位"})
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

	// 会话轮换 (Session Rotation)：废止此前签发的所有旧会话，防范旧凭据被盗用
	epoch := time.Now().Unix()
	_ = model.SetSetting("auth_min_issued_at", strconv.FormatInt(epoch, 10))

	// 为当前操作端签发轮换后的全新有效 Token
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)
	if usernameStr == "" {
		usernameStr, _ = model.GetSetting("admin_username")
	}
	newToken, _ := middleware.GenerateJWT(usernameStr)

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
		_ = model.SetSetting("secret_path", trimmedSecret)
		// 安全路径变更，强制废止所有旧会话以防扫描缓存利用
		_ = model.SetSetting("auth_min_issued_at", strconv.FormatInt(time.Now().Unix(), 10))
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
	epoch := time.Now().Unix()
	_ = model.SetSetting("auth_min_issued_at", strconv.FormatInt(epoch, 10))
	c.JSON(http.StatusOK, gin.H{"message": "所有会话已强制注销，请重新登录"})
}

