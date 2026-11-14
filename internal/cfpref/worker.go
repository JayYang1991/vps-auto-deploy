package cfpref

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
)

var lastDailyRunDate string

// StartWorker 启动 Cloudflare 优选 IP 后台定期巡检与自动拉取协程
func StartWorker() {
	go func() {
		// 启动后延时 5 秒避开系统服务初始化高峰
		time.Sleep(5 * time.Second)
		log.Println("[CFPrefWorker] Cloudflare 优选 IP 定时拉取协程已就绪。")

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			checkAndExecuteSchedule()
		}
	}()
}

func checkAndExecuteSchedule() {
	enabled, _ := model.GetSetting("cf_pref_cron_enabled")
	if enabled != "true" {
		return
	}

	mode, _ := model.GetSetting("cf_pref_cron_mode")
	if mode == "" {
		mode = "interval"
	}

	now := time.Now()

	if mode == "daily" {
		targetTime, _ := model.GetSetting("cf_pref_cron_time")
		if targetTime == "" {
			targetTime = "04:00"
		}
		currentTimeStr := now.Format("15:04")
		currentDateStr := now.Format("2006-01-02")

		if currentTimeStr == targetTime && lastDailyRunDate != currentDateStr {
			log.Printf("[CFPrefWorker] 触发每日定点 (%s) 优选 IP 自动同步...", targetTime)
			lastDailyRunDate = currentDateStr
			runScheduledFetch()
		}
		return
	}

	// 间隔模式 (interval)
	intervalStr, _ := model.GetSetting("cf_pref_cron_interval")
	if intervalStr == "" {
		intervalStr = "6h"
	}

	dur, err := time.ParseDuration(intervalStr)
	if err != nil || dur < 30*time.Minute {
		dur = 6 * time.Hour
	}

	lastFetchStr, _ := model.GetSetting("cf_pref_last_fetch_at")
	if lastFetchStr == "" {
		// 从未拉取过，立即执行首次同步
		log.Println("[CFPrefWorker] 首次启动且无历史记录，触发优选 IP 自动同步...")
		runScheduledFetch()
		return
	}

	lastFetchTime, err := time.Parse(time.RFC3339, lastFetchStr)
	if err != nil {
		runScheduledFetch()
		return
	}

	if now.Sub(lastFetchTime) >= dur {
		log.Printf("[CFPrefWorker] 达到同步周期 (%v)，触发优选 IP 自动同步...", dur)
		runScheduledFetch()
	}
}

func runScheduledFetch() {
	res, err := FetchAndSavePreferredIPs()
	if err != nil {
		log.Printf("[CFPrefWorker] 自动拉取优选 IP 失败: %v", err)
		return
	}
	log.Printf("[CFPrefWorker] 自动拉取优选 IP 成功，共持久化 %d 个节点 (Host: %s)", res.Count, res.Host)
}

// TriggerManualFetch 手动触发重新拉取并返回详细执行结果
func TriggerManualFetch() (*FetchResult, error) {
	return FetchAndSavePreferredIPs()
}

// TriggerManualFetchWithConfig 手动触发重新拉取并支持覆盖参数与自动保存
func TriggerManualFetchWithConfig(customSubURL, customHost, customUUID string, autoSave bool) (*FetchResult, error) {
	return FetchAndSavePreferredIPsWithConfig(customSubURL, customHost, customUUID, autoSave)
}

// GetStatusSummary 获取当前配置与同步摘要
func GetStatusSummary() (map[string]interface{}, error) {
	subURL, host, uuidStr, path := GetEffectiveConfig()

	cronEnabled, _ := model.GetSetting("cf_pref_cron_enabled")
	cronMode, _ := model.GetSetting("cf_pref_cron_mode")
	if cronMode == "" {
		cronMode = "interval"
	}
	cronInterval, _ := model.GetSetting("cf_pref_cron_interval")
	if cronInterval == "" {
		cronInterval = "6h"
	}
	cronTime, _ := model.GetSetting("cf_pref_cron_time")
	if cronTime == "" {
		cronTime = "04:00"
	}

	lastFetchAt, _ := model.GetSetting("cf_pref_last_fetch_at")
	lastFetchStatus, _ := model.GetSetting("cf_pref_last_fetch_status")
	lastFetchCount, _ := model.GetSetting("cf_pref_last_fetch_count")

	totalCount, _ := model.CountCFPreferredIPs()

	return map[string]interface{}{
		"sub_url":           subURL,
		"host":              host,
		"uuid":              uuidStr,
		"path":              path,
		"cron_enabled":      cronEnabled == "true",
		"cron_mode":         cronMode,
		"cron_interval":     cronInterval,
		"cron_time":         cronTime,
		"last_fetch_at":     lastFetchAt,
		"last_fetch_status": lastFetchStatus,
		"last_fetch_count":  lastFetchCount,
		"total_count":       totalCount,
	}, nil
}

// SaveSettings 保存优选 IP 相关的用户定制与调度设置
func SaveSettings(settings map[string]interface{}) error {
	for k, v := range settings {
		valStr := ""
		switch val := v.(type) {
		case string:
			valStr = strings.TrimSpace(val)
		case bool:
			if val {
				valStr = "true"
			} else {
				valStr = "false"
			}
		}

		switch k {
		case "sub_url":
			if valStr != "" {
				testURL := strings.ReplaceAll(valStr, "{host}", "cloudflare.com")
				testURL = strings.ReplaceAll(testURL, "{uuid}", "00000000-0000-0000-0000-000000000000")
				if err := netutil.ValidatePublicURL(testURL); err != nil {
					return fmt.Errorf("优选订阅源 URL 不合法或触发安全拦截: %w", err)
				}
			}
			_ = model.SetSetting("cf_pref_sub_url", valStr)
		case "host":
			_ = model.SetSetting("cf_pref_host", valStr)
		case "uuid":
			_ = model.SetSetting("cf_pref_uuid", valStr)
		case "path":
			_ = model.SetSetting("cf_pref_path", valStr)
		case "cron_enabled":
			_ = model.SetSetting("cf_pref_cron_enabled", valStr)
		case "cron_mode":
			_ = model.SetSetting("cf_pref_cron_mode", valStr)
		case "cron_interval":
			if valStr != "" {
				dur, err := time.ParseDuration(valStr)
				if err != nil || dur < 10*time.Minute {
					return fmt.Errorf("定时同步间隔不合法 (如 6h, 30m，且最小间隔不能低于 10 分钟)")
				}
			}
			_ = model.SetSetting("cf_pref_cron_interval", valStr)
		case "cron_time":
			if valStr != "" {
				if !regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`).MatchString(valStr) {
					return fmt.Errorf("定点同步时间格式不合法 (必须为 HH:MM 格式，如 04:00)")
				}
			}
			_ = model.SetSetting("cf_pref_cron_time", valStr)
		}
	}
	return nil
}

