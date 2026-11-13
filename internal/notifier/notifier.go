package notifier

import (
	"log"
	"strings"
	"sync"
	"time"
)

var (
	eventQueue = make(chan Event, 200)
	initOnce   sync.Once
)

// InitNotifier 初始化并启动事件分发后台工作协程
func InitNotifier() {
	initOnce.Do(func() {
		go eventWorker()
	})
}

// Send 向通知器提交一个审计安全事件 (非阻塞，零延迟返回)
func Send(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	select {
	case eventQueue <- event:
	default:
		log.Printf("[Notifier] 事件队列已满，丢弃事件: %s (%s)", event.Type, event.Title)
	}
}

// IsEventEnabled 判断给定的事件类型是否包含在配置的启用列表中
func IsEventEnabled(cfg *TelegramConfig, eventType string) bool {
	if cfg == nil || !cfg.Enabled {
		return false
	}
	for _, e := range cfg.Events {
		if strings.EqualFold(strings.TrimSpace(e), eventType) {
			return true
		}
	}
	return false
}

func eventWorker() {
	for event := range eventQueue {
		handleEvent(event)
	}
}

func handleEvent(event Event) {
	cfg, err := GetTelegramConfig()
	if err != nil || cfg == nil || !cfg.Enabled {
		return
	}

	if cfg.BotToken == "" || cfg.ChatID == "" {
		return
	}

	if !IsEventEnabled(cfg, event.Type) {
		return
	}

	// 针对密码暴力破解导致的高频登录失败事件，按 ClientIP 进行 30 秒流控抑制
	if event.Type == EventLoginFailed && event.ClientIP != "" {
		failedLoginThrottleMu.Lock()
		lastSent, exists := failedLoginThrottle[event.ClientIP]
		now := time.Now()
		if exists && now.Sub(lastSent) < 30*time.Second {
			failedLoginThrottleMu.Unlock()
			return
		}
		failedLoginThrottle[event.ClientIP] = now

		// 定期清理过期的 IP 记录，防止内存泄漏
		if len(failedLoginThrottle) > 1000 {
			for ip, t := range failedLoginThrottle {
				if now.Sub(t) > 5*time.Minute {
					delete(failedLoginThrottle, ip)
				}
			}
		}
		failedLoginThrottleMu.Unlock()
	}

	msg := FormatTelegramHTML(event)
	if err := SendTelegramMessage(cfg.APIURL, cfg.BotToken, cfg.ChatID, msg, cfg.Silent); err != nil {
		log.Printf("[Notifier] 发送 Telegram 审计告警失败 (%s): %v", event.Type, err)
	}
}

