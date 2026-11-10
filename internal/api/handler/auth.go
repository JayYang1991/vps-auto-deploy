package handler

import (
	"crypto/subtle"
	"net/http"
	"strconv"
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
		middleware.RecordFailedLogin(c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect username or password"})
		return
	}

	// 认证通过，重置该 IP 失败尝试记录
	middleware.ResetLoginAttempts(c.ClientIP())

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

	hashedPass, _ := model.GetSetting("admin_password_hash")
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPass), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old password is incorrect"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt password"})
		return
	}

	_ = model.SetSetting("admin_password_hash", string(newHash))
	c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
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
		_ = model.SetSetting("secret_path", req.SecretPath)
	}
	if req.PublicIP != "" {
		_ = model.SetSetting("public_ip", req.PublicIP)
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

