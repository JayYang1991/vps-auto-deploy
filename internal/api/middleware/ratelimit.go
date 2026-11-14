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

	cloudflareCIDRs []*net.IPNet
)

const (
	maxFailedAttempts = 5
	lockoutDuration   = 15 * time.Minute
	maxLimiterEntries = 10000

	subMaxPerMinute = 60
)

func init() {
	cfRanges := []string{
		// Cloudflare 官方公布 IPv4 网段
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
		"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
		"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
		// Cloudflare 官方公布 IPv6 网段
		"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
		"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
	}
	for _, r := range cfRanges {
		_, cidr, err := net.ParseCIDR(r)
		if err == nil {
			cloudflareCIDRs = append(cloudflareCIDRs, cidr)
		}
	}

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

// IsTrustedProxy 校验连接对端 IP 是否属于受信任反代（本地回环、内网私有网段或 Cloudflare 官方边缘节点）
func IsTrustedProxy(ipStr string) bool {
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "127.0.0.1" || ipStr == "::1" || strings.HasPrefix(ipStr, "127.") || strings.HasPrefix(ipStr, "::ffff:127.") {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return true
	}
	for _, cidr := range cloudflareCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// ExtractClientIP 仅在请求来自受信任的反代网段时才解析 X-Forwarded-For，且从右向左取首个非受信任代理 IP (防伪造)
func ExtractClientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil {
		return remoteHost
	}

	isTrusted := false
	for _, cidr := range trustedProxies {
		if cidr.Contains(remoteIP) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		return remoteIP.String()
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ipStr := strings.TrimSpace(parts[i])
			ip := net.ParseIP(ipStr)
			if ip == nil {
				continue
			}
			trusted := false
			for _, cidr := range trustedProxies {
				if cidr.Contains(ip) {
					trusted = true
					break
				}
			}
			if !trusted {
				return ip.String()
			}
		}
	}
	return remoteIP.String()
}

// GetRealClientIP 智能获取客户端真实 IP：
// 1. 若连接来自受信任反代（本机回环、内网私有网段、Cloudflare 官方边缘节点），
//    安全提取 CF-Connecting-IP, X-Real-IP 或 X-Forwarded-For (从右向左取首个非受信节点，防左侧注入伪造)，
//    精准识别真实来源，防止误封代理节点或跨节点 IP 漂移。
// 2. 否则严格使用 c.ClientIP() / RemoteAddr，杜绝外网未授权客户端伪造 X-Forwarded-For。
func GetRealClientIP(c *gin.Context) string {
	remoteIP := c.ClientIP()
	if host, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil && host != "" {
		remoteIP = host
	}

	if IsTrustedProxy(remoteIP) {
		if cfIP := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); cfIP != "" && net.ParseIP(cfIP) != nil {
			return cfIP
		}
		if realIP := strings.TrimSpace(c.GetHeader("X-Real-IP")); realIP != "" && net.ParseIP(realIP) != nil {
			return realIP
		}
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				ipStr := strings.TrimSpace(parts[i])
				if ip := net.ParseIP(ipStr); ip != nil {
					if !IsTrustedProxy(ipStr) {
						return ipStr
					}
				}
			}
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

