package handler

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/api/middleware"
	"vps-auto-deploy/internal/model"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type UpdateSettingsRequest struct {
	SecretPath string `json:"secret_path"`
	PublicIP   string `json:"public_ip"`
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

	token, err := middleware.GenerateJWT(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":    token,
		"username": req.Username,
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
	c.JSON(http.StatusOK, gin.H{
		"secret_path": secretPath,
		"public_ip":   publicIP,
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

	c.JSON(http.StatusOK, gin.H{"message": "settings saved"})
}

