package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
	"vps-auto-deploy/internal/pkg/netutil"
)

const (
	DefaultTelegramAPI = "https://api.telegram.org"
)

// TelegramConfig 描述 Telegram 机器人的持久化配置
type TelegramConfig struct {
	Enabled     bool     `json:"enabled"`
	BotToken    string   `json:"bot_token,omitempty"`
	MaskedToken string   `json:"masked_token,omitempty"`
	ChatID      string   `json:"chat_id"`
	APIURL      string   `json:"api_url"`
	Events      []string `json:"events"`
	Silent      bool     `json:"silent"`
}

var (
	failedLoginThrottleMu sync.Mutex
	failedLoginThrottle   = make(map[string]time.Time)
)

// MaskBotToken 对 Telegram Bot Token 进行脱敏处理
func MaskBotToken(token string) string {
	token = strings.TrimSpace(token)
	parts := strings.SplitN(token, ":", 2)
	if len(parts) == 2 {
		idPart := parts[0]
		secretPart := parts[1]
		if len(secretPart) > 6 {
			return fmt.Sprintf("%s:••••••••%s", idPart, secretPart[len(secretPart)-4:])
		}
		return fmt.Sprintf("%s:••••••••", idPart)
	}
	if len(token) <= 8 {
		return "••••••••"
	}
	return token[:4] + "••••••••" + token[len(token)-4:]
}

// GetTelegramConfig 从数据库获取 Telegram 配置
func GetTelegramConfig() (*TelegramConfig, error) {
	enabledStr, _ := model.GetSetting("telegram_enabled")
	enabled := enabledStr == "true" || enabledStr == "1"

	chatID, _ := model.GetSetting("telegram_chat_id")
	apiURL, _ := model.GetSetting("telegram_api_url")
	if strings.TrimSpace(apiURL) == "" {
		apiURL = DefaultTelegramAPI
	}

	silentStr, _ := model.GetSetting("telegram_silent")
	silent := silentStr == "true" || silentStr == "1"

	eventsStr, _ := model.GetSetting("telegram_events")
	var events []string
	if strings.TrimSpace(eventsStr) != "" {
		for _, e := range strings.Split(eventsStr, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				events = append(events, e)
			}
		}
	} else {
		// 默认订阅关键安全事件
		events = []string{
			EventLoginFailed,
			EventIPLocked,
			EventCredsRevealed,
			EventPasswordChanged,
			EventSessionRevoked,
			EventTerminalSession,
			EventNodeDeleted,
			EventCloudDestroyed,
		}
	}

	var rawToken string
	encToken, err := model.GetSetting("telegram_bot_token_encrypted")
	if err == nil && encToken != "" {
		dec, decErr := model.DecryptSensitiveData(encToken)
		if decErr == nil {
			rawToken = dec
		}
	}

	masked := ""
	if rawToken != "" {
		masked = MaskBotToken(rawToken)
	}

	return &TelegramConfig{
		Enabled:     enabled,
		BotToken:    rawToken,
		MaskedToken: masked,
		ChatID:      chatID,
		APIURL:      apiURL,
		Events:      events,
		Silent:      silent,
	}, nil
}

// SaveTelegramConfig 加密保存 Telegram 机器人配置
func SaveTelegramConfig(cfg TelegramConfig) error {
	_ = model.SetSetting("telegram_enabled", fmt.Sprintf("%t", cfg.Enabled))
	_ = model.SetSetting("telegram_chat_id", strings.TrimSpace(cfg.ChatID))

	cleanAPI := strings.TrimRight(strings.TrimSpace(cfg.APIURL), "/")
	if cleanAPI == "" {
		cleanAPI = DefaultTelegramAPI
	} else {
		// 安全校验：防 SSRF 探测内部或云元数据地址
		if err := netutil.ValidatePublicURL(cleanAPI); err != nil {
			return fmt.Errorf("自定义 Telegram API 地址安全拦截: %w", err)
		}
	}
	_ = model.SetSetting("telegram_api_url", cleanAPI)
	_ = model.SetSetting("telegram_silent", fmt.Sprintf("%t", cfg.Silent))
	_ = model.SetSetting("telegram_events", strings.Join(cfg.Events, ","))

	// 若提供了非掩码的有效 Bot Token，加密存储
	if strings.TrimSpace(cfg.BotToken) != "" && !strings.Contains(cfg.BotToken, "••••") {
		key := model.GetEncryptionKey()
		enc, err := crypto.EncryptAESGCM(strings.TrimSpace(cfg.BotToken), key)
		if err != nil {
			return fmt.Errorf("加密 Bot Token 失败: %w", err)
		}
		_ = model.SetSetting("telegram_bot_token_encrypted", enc)
	}

	return nil
}

// FormatTelegramHTML 将事件渲染为结构化精美 HTML 消息
func FormatTelegramHTML(event Event) string {
	icon := "ℹ️"
	levelBadge := "普通通知"
	switch event.Level {
	case LevelCritical:
		icon = "🚨"
		levelBadge = "<b>严重告警</b>"
	case LevelWarn:
		icon = "⚠️"
		levelBadge = "<b>安全警告</b>"
	case LevelInfo:
		icon = "ℹ️"
		levelBadge = "系统通知"
	}

	ts := event.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	timeStr := ts.Format("2006-01-02 15:04:05 MST")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s <b>[VPS Auto Deploy 审计告警]</b>\n\n", icon))
	sb.WriteString(fmt.Sprintf("<b>事件类型</b>: %s\n", html.EscapeString(event.Title)))
	sb.WriteString(fmt.Sprintf("<b>告警级别</b>: %s\n", levelBadge))
	if event.ClientIP != "" {
		sb.WriteString(fmt.Sprintf("<b>来源 IP</b>: <code>%s</code>\n", html.EscapeString(event.ClientIP)))
	}
	if event.Username != "" {
		sb.WriteString(fmt.Sprintf("<b>操作账号</b>: <code>%s</code>\n", html.EscapeString(event.Username)))
	}
	if event.NodeName != "" {
		sb.WriteString(fmt.Sprintf("<b>涉及节点</b>: %s\n", html.EscapeString(event.NodeName)))
	}
	sb.WriteString(fmt.Sprintf("<b>发生时间</b>: <code>%s</code>\n", html.EscapeString(timeStr)))
	if event.Message != "" {
		sb.WriteString(fmt.Sprintf("\n<b>详细说明</b>:\n%s\n", html.EscapeString(event.Message)))
	}

	if len(event.Metadata) > 0 {
		sb.WriteString("\n<b>上下文参数</b>:\n")
		for k, v := range event.Metadata {
			sb.WriteString(fmt.Sprintf("• <i>%s</i>: <code>%s</code>\n", html.EscapeString(k), html.EscapeString(v)))
		}
	}

	return sb.String()
}

// SendTelegramMessage 底层向 Telegram Bot API 推送单条消息
func SendTelegramMessage(apiURL, botToken, chatID, text string, silent bool) error {
	cleanAPI := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if cleanAPI == "" {
		cleanAPI = DefaultTelegramAPI
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", cleanAPI, botToken)

	payload := map[string]interface{}{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
		"disable_notification":     silent,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := netutil.CreateSafeHTTPClient(10 * time.Second)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接 Telegram API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		var tgErr struct {
			Ok          bool   `json:"ok"`
			Description string `json:"description"`
		}
		_ = json.Unmarshal(respBody, &tgErr)
		if tgErr.Description != "" {
			return fmt.Errorf("Telegram 拒绝接收 (HTTP %d): %s", resp.StatusCode, tgErr.Description)
		}
		return fmt.Errorf("Telegram 返回异常状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// TestTelegram 发送一条 Telegram 测试连通性消息
func TestTelegram(apiURL, botToken, chatID string) error {
	cleanToken := strings.TrimSpace(botToken)
	if cleanToken == "" || strings.Contains(cleanToken, "••••") {
		// 若前端未修改 token 或传了掩码，从已持久化配置解密读取
		cfg, err := GetTelegramConfig()
		if err != nil || cfg.BotToken == "" {
			return fmt.Errorf("未配置有效的 Telegram Bot Token")
		}
		cleanToken = cfg.BotToken
	}

	cleanChatID := strings.TrimSpace(chatID)
	if cleanChatID == "" {
		cfg, _ := GetTelegramConfig()
		cleanChatID = cfg.ChatID
	}
	if cleanChatID == "" {
		return fmt.Errorf("Chat ID 不能为空")
	}

	cleanAPI := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if cleanAPI == "" {
		cfg, _ := GetTelegramConfig()
		cleanAPI = cfg.APIURL
	}
	if cleanAPI == "" {
		cleanAPI = DefaultTelegramAPI
	} else if cleanAPI != DefaultTelegramAPI {
		if err := netutil.ValidatePublicURL(cleanAPI); err != nil {
			return fmt.Errorf("自定义 Telegram API 地址安全拦截: %w", err)
		}
	}

	testEvent := Event{
		Type:      "TEST_MESSAGE",
		Level:     LevelInfo,
		Title:     "Telegram 审计机器人连通性测试",
		Message:   "恭喜！VPS Auto Deploy 审计告警通道已成功打通，后续特权操作、防爆破封禁与节点运维事件将实时推送至本会话。",
		ClientIP:  netutil.GetPublicIP(),
		Username:  "admin",
		Timestamp: time.Now(),
	}

	msg := FormatTelegramHTML(testEvent)
	return SendTelegramMessage(cleanAPI, cleanToken, cleanChatID, msg, false)
}

