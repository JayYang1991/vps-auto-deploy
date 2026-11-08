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

var (
	loginLimiterMu sync.Mutex
	loginAttempts  = make(map[string]*loginAttempt)
)

const (
	maxFailedAttempts = 5
	lockoutDuration   = 15 * time.Minute
)

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

