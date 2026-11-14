package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"vps-auto-deploy/internal/model"
)

type streamTicketInfo struct {
	Username  string
	ExpiresAt time.Time
}

var (
	streamTicketMu sync.RWMutex
	streamTickets  = make(map[string]streamTicketInfo)
)

// IssueStreamTicket 签发 30 秒单次有效的 SSE 实时日志流临时 Ticket (消除 URL 中的长期 JWT 泄露隐患)
func IssueStreamTicket(username string, ttl time.Duration) string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	ticket := hex.EncodeToString(b)

	streamTicketMu.Lock()
	defer streamTicketMu.Unlock()

	now := time.Now()
	for k, v := range streamTickets {
		if now.After(v.ExpiresAt) {
			delete(streamTickets, k)
		}
	}

	streamTickets[ticket] = streamTicketInfo{
		Username:  username,
		ExpiresAt: now.Add(ttl),
	}
	return ticket
}

// ValidateAndConsumeStreamTicket 验证并单次消费 SSE Stream Ticket (防重放)
func ValidateAndConsumeStreamTicket(ticket string) (string, bool) {
	if ticket == "" {
		return "", false
	}
	streamTicketMu.Lock()
	defer streamTicketMu.Unlock()

	info, exists := streamTickets[ticket]
	if !exists {
		return "", false
	}
	delete(streamTickets, ticket)

	if time.Now().After(info.ExpiresAt) {
		return "", false
	}
	return info.Username, true
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenerateJWT 生成默认 7 天有效期的 JWT Token (保留用于向后兼容)
func GenerateJWT(username string) (string, error) {
	return GenerateJWTWithDuration(username, 7*24*time.Hour)
}

// GenerateJWTWithDuration 生成指定有效期的 JWT Token
func GenerateJWTWithDuration(username string, duration time.Duration) (string, error) {
	secret, err := model.GetSetting("jwt_secret")
	if err != nil {
		return "", err
	}

	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// SecretPathMiddleware 验证 URL 第一级目录是否匹配系统安全入口路径 (常数时间比对，防时序侧信道攻击)
func SecretPathMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqSecret := c.Param("secret")
		actualSecret, err := model.GetSetting("secret_path")
		if err != nil || actualSecret == "" || subtle.ConstantTimeCompare([]byte(reqSecret), []byte(actualSecret)) != 1 {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Next()
	}
}

// JWTAuthMiddleware 验证 Bearer Token (支持 Authorization Header、一次性 Stream Ticket 或限流白名单 SSE Query Token)
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 优先校验单次有效短期的 Stream Ticket (针对 SSE 实时日志流，杜绝 URL 暴露长期 JWT)
		if ticket := c.Query("ticket"); ticket != "" {
			if username, ok := ValidateAndConsumeStreamTicket(ticket); ok {
				c.Set("username", username)
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired stream ticket"})
			return
		}

		// 2. 从 Authorization Header 提取 Bearer Token
		tokenStr := ""
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		// 3. 若 Header 中无 Token，仅严格限制在 SSE 实时流白名单端点允许读取 URL query 中的 token 参数作为向后兼容
		if tokenStr == "" {
			tokenQuery := c.Query("token")
			if tokenQuery != "" {
				cleanPath := path.Clean(c.Request.URL.Path)
				baseName := path.Base(cleanPath)
				isAllowedSSE := strings.Contains(baseName, "logs") || strings.Contains(baseName, "sse")
				if !isAllowedSSE {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"error": "安全拦截：禁止在常规 REST API 中通过 URL Query 传递 Token，请使用 Authorization 请求头或申请 Stream Ticket",
					})
					return
				}
				tokenStr = tokenQuery
			}
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header or stream ticket required"})
			return
		}

		secret, err := model.GetSetting("jwt_secret")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "jwt secret not initialized"})
			return
		}

		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			return
		}

		// 会话撤销阻断检查：若管理员执行了“强制注销所有会话”，废止在撤销时刻之前签发的所有 Token
		if minIssuedAtStr, err := model.GetSetting("auth_min_issued_at"); err == nil && minIssuedAtStr != "" {
			if minEpoch, err := strconv.ParseInt(minIssuedAtStr, 10, 64); err == nil {
				if claims.IssuedAt != nil && claims.IssuedAt.Unix() < minEpoch {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session revoked, please re-login"})
					return
				}
			}
		}

		c.Set("username", claims.Username)
		c.Next()
	}
}

// SecurityHeadersMiddleware 全局安全防御响应头中间件
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 抵御点击劫持 (Clickjacking)
		c.Writer.Header().Set("X-Frame-Options", "DENY")
		// 抵御 MIME 嗅探注入
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		// 启用基础 XSS 防护过滤
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
		// 严控 Referrer 泄露系统 Secret 路径
		c.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// 注入严苛内容安全策略 (CSP)，防范 XSS、未授权脚本与恶意跨站嵌入
		c.Writer.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none';")
		// 若为 HTTPS 连接，自动启用 HSTS
		if c.Request.TLS != nil {
			c.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// CORSMiddleware 安全跨域处理 (防御任意源反射与敏感凭据泄露，兼容反代与隧道)
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		host := c.Request.Host

		allowed := false
		if origin != "" {
			if u, err := url.Parse(origin); err == nil && u.Host != "" {
				if strings.EqualFold(u.Host, host) {
					allowed = true
				} else {
					// 安全严密校验：仅当物理套接字连接真正来自本地回环时，才信任反向代理/隧道头 (防 Host 请求头伪造绕过)
					remoteIP := c.ClientIP()
					if rip, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil && rip != "" {
						remoteIP = rip
					}
					isLoopback := remoteIP == "127.0.0.1" || remoteIP == "::1" || strings.HasPrefix(remoteIP, "127.")

					if isLoopback {
						if fwdHost := c.GetHeader("X-Forwarded-Host"); fwdHost != "" && strings.EqualFold(u.Host, fwdHost) {
							allowed = true
						} else if pubIP, _ := model.GetSetting("public_ip"); pubIP != "" {
							if strings.EqualFold(u.Hostname(), pubIP) || strings.EqualFold(u.Host, pubIP) {
								allowed = true
							}
						}
					}
				}

				if allowed {
					c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
					c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
		}

		if !allowed {
			// 未经允许的跨域来源，不赋予凭据权限
			c.Writer.Header().Set("Access-Control-Allow-Origin", "")
		}

		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Authorization, X-Requested-With, X-Target-Node-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

