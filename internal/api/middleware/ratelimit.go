package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/notifier"
)

type loginAttempt struct {
	count      int
	lastFailed time.Time
}

type subRequestTracker struct {
	count     int
	windowEnd time.Time
}

var (
	loginLimiterMu sync.Mutex
	loginAttempts  = make(map[string]*loginAttempt)

	subLimiterMu sync.Mutex
	subRequests  = make(map[string]*subRequestTracker)
)

const (
	maxFailedAttempts = 5
	lockoutDuration   = 15 * time.Minute
	maxLimiterEntries = 10000

	subMaxPerMinute = 60
)

func init() {
	// 启动后台定时垃圾回收协程，定期清理过期的限流条目，防止内存无界泄露 (Anti-DoS)
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			cleanupExpiredLimiters()
		}
	}()
}

func cleanupExpiredLimiters() {
	now := time.Now()

	loginLimiterMu.Lock()
	for ip, attempt := range loginAttempts {
		if now.Sub(attempt.lastFailed) > lockoutDuration {
			delete(loginAttempts, ip)
		}
	}
	loginLimiterMu.Unlock()

	subLimiterMu.Lock()
	for ip, tracker := range subRequests {
		if now.After(tracker.windowEnd) {
			delete(subRequests, ip)
		}
	}
	subLimiterMu.Unlock()
}

// GetRealClientIP 智能获取客户端真实 IP：
// 1. 若连接来自本机回环 (127.0.0.1, ::1)，说明前置存在反向代理 (Nginx/Cloudflare Tunnel/Caddy)，
//    优先解析 CF-Connecting-IP, X-Real-IP 或 X-Forwarded-For，精准识别真实来源，防止误封 127.0.0.1。
// 2. 否则严格使用 c.ClientIP() / RemoteAddr，杜绝外网客户端伪造 X-Forwarded-For。
func GetRealClientIP(c *gin.Context) string {
	remoteIP := c.ClientIP()
	if host, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil && host != "" {
		remoteIP = host
	}

	isLoopback := remoteIP == "127.0.0.1" || remoteIP == "::1" || strings.HasPrefix(remoteIP, "127.")
	if isLoopback {
		if cfIP := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); cfIP != "" && net.ParseIP(cfIP) != nil {
			return cfIP
		}
		if realIP := strings.TrimSpace(c.GetHeader("X-Real-IP")); realIP != "" && net.ParseIP(realIP) != nil {
			return realIP
		}
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				first := strings.TrimSpace(parts[0])
				if net.ParseIP(first) != nil {
					return first
				}
			}
		}
	}
	return c.ClientIP()
}

// LoginRateLimitMiddleware 针对登录接口的防暴力破解中间件（基于客户端真实 IP）
func LoginRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := GetRealClientIP(c)

		// 本地回环直接访问保护：若确系本机直连未携带反代头，豁免锁定，防止自断管理连接 (DoS)
		if ip == "127.0.0.1" || ip == "::1" {
			c.Next()
			return
		}

		loginLimiterMu.Lock()
		attempt, exists := loginAttempts[ip]
		if exists && attempt.count >= maxFailedAttempts {
			if time.Since(attempt.lastFailed) < lockoutDuration {
				remaining := int(lockoutDuration.Minutes() - time.Since(attempt.lastFailed).Minutes())
				if remaining <= 0 {
					remaining = 1
				}
				loginLimiterMu.Unlock()
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": fmt.Sprintf("登录失败尝试次数过多，该 IP 已被临时锁定，请在 %d 分钟后再试", remaining),
				})
				return
			}
			// 锁定时间已过，重置计数
			delete(loginAttempts, ip)
		}
		loginLimiterMu.Unlock()

		c.Next()
	}
}

// RecordFailedLogin 记录一次登录失败
func RecordFailedLogin(ip string) {
	// 回环地址豁免锁定，防止自断管理连接
	if ip == "127.0.0.1" || ip == "::1" {
		return
	}

	loginLimiterMu.Lock()
	defer loginLimiterMu.Unlock()

	// 防止恶意 IP 构造导致 map 无限膨胀
	if len(loginAttempts) >= maxLimiterEntries {
		now := time.Now()
		for k, v := range loginAttempts {
			if now.Sub(v.lastFailed) > lockoutDuration {
				delete(loginAttempts, k)
			}
		}
	}

	attempt, exists := loginAttempts[ip]
	if !exists {
		attempt = &loginAttempt{count: 1, lastFailed: time.Now()}
		loginAttempts[ip] = attempt
	} else {
		attempt.count++
		attempt.lastFailed = time.Now()
	}

	if attempt.count == maxFailedAttempts {
		notifier.Send(notifier.Event{
			Type:      notifier.EventIPLocked,
			Level:     notifier.LevelCritical,
			Title:     "IP 连续登录失败被系统锁定",
			Message:   fmt.Sprintf("来源 IP 连续 %d 次密码认证失败，已被系统强制锁定 %v", maxFailedAttempts, lockoutDuration),
			ClientIP:  ip,
			Timestamp: time.Now(),
			Metadata: map[string]string{
				"failed_attempts": fmt.Sprintf("%d", maxFailedAttempts),
				"lockout_period":  lockoutDuration.String(),
			},
		})
	}
}

// ResetLoginAttempts 登录成功时清除失败计数
func ResetLoginAttempts(ip string) {
	loginLimiterMu.Lock()
	defer loginLimiterMu.Unlock()
	delete(loginAttempts, ip)
}

// SubscriptionRateLimitMiddleware 针对公开订阅接口的防扫描与防压测限流中间件 (单真实 IP 每分钟最多 60 次)
func SubscriptionRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := GetRealClientIP(c)
		now := time.Now()

		subLimiterMu.Lock()
		if len(subRequests) >= maxLimiterEntries {
			for k, v := range subRequests {
				if now.After(v.windowEnd) {
					delete(subRequests, k)
				}
			}
		}

		tracker, exists := subRequests[ip]
		if !exists || now.After(tracker.windowEnd) {
			subRequests[ip] = &subRequestTracker{
				count:     1,
				windowEnd: now.Add(time.Minute),
			}
			subLimiterMu.Unlock()
			c.Next()
			return
		}

		if tracker.count >= subMaxPerMinute {
			subLimiterMu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "订阅请求过于频繁，请等待 1 分钟后再试",
			})
			return
		}

		tracker.count++
		subLimiterMu.Unlock()

		c.Next()
	}
}

