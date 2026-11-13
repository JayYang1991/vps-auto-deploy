package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/notifier"
)

type TelegramConfigRequest struct {
	Enabled  bool     `json:"enabled"`
	BotToken string   `json:"bot_token"`
	ChatID   string   `json:"chat_id"`
	APIURL   string   `json:"api_url"`
	Events   []string `json:"events"`
	Silent   bool     `json:"silent"`
}

type TelegramTestRequest struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	APIURL   string `json:"api_url"`
}

// GetTelegramConfig 获取 Telegram 告警配置 (Token 自动脱敏保护)
func GetTelegramConfig(c *gin.Context) {
	cfg, err := notifier.GetTelegramConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Telegram 配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"config": gin.H{
			"enabled":      cfg.Enabled,
			"masked_token": cfg.MaskedToken,
			"chat_id":      cfg.ChatID,
			"api_url":      cfg.APIURL,
			"events":       cfg.Events,
			"silent":       cfg.Silent,
		},
		"available_events": notifier.AllEventCategories(),
	})
}

// SaveTelegramConfig 保存 Telegram 告警配置 (AES-256-GCM 加密存储)
func SaveTelegramConfig(c *gin.Context) {
	var req TelegramConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的配置参数: " + err.Error()})
		return
	}

	if req.Enabled {
		if strings.TrimSpace(req.ChatID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "启用告警时，Chat ID 不能为空"})
			return
		}
	}

	cfg := notifier.TelegramConfig{
		Enabled:  req.Enabled,
		BotToken: strings.TrimSpace(req.BotToken),
		ChatID:   strings.TrimSpace(req.ChatID),
		APIURL:   strings.TrimSpace(req.APIURL),
		Events:   req.Events,
		Silent:   req.Silent,
	}

	if err := notifier.SaveTelegramConfig(cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "保存失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Telegram 告警配置已成功保存"})
}

// TestTelegram 发送 Telegram 测试消息验证机器人连通性
func TestTelegram(c *gin.Context) {
	var req TelegramTestRequest
	_ = c.ShouldBindJSON(&req)

	if err := notifier.TestTelegram(req.APIURL, req.BotToken, req.ChatID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "连通性测试失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "测试消息已成功推送，请前往 Telegram 查看机器人消息！"})
}

