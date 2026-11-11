package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

// LoginRateLimitMiddleware 针对登录接口的防暴力破解中间件（基于客户端真实 IP）
func LoginRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

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
		loginAttempts[ip] = &loginAttempt{count: 1, lastFailed: time.Now()}
	} else {
		attempt.count++
		attempt.lastFailed = time.Now()
	}
}

// ResetLoginAttempts 登录成功时清除失败计数
func ResetLoginAttempts(ip string) {
	loginLimiterMu.Lock()
	defer loginLimiterMu.Unlock()
	delete(loginAttempts, ip)
}

// SubscriptionRateLimitMiddleware 针对公开订阅接口的防扫描与防压测限流中间件 (单 IP 每分钟最多 60 次)
func SubscriptionRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
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

