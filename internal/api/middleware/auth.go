package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"vps-auto-deploy/internal/model"
)

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

// SecretPathMiddleware 验证 URL 第一级目录是否匹配系统安全入口路径
func SecretPathMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqSecret := c.Param("secret")
		actualSecret, err := model.GetSetting("secret_path")
		if err != nil || reqSecret != actualSecret {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Next()
	}
}

// JWTAuthMiddleware 验证 Bearer Token (支持 Authorization Header 或 URL Query 中的 token 参数)
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		// 若 Header 中无 Token，允许从 URL query 中的 token 参数获取 (针对 EventSource / SSE 场景)
		if tokenStr == "" {
			tokenStr = c.Query("token")
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header or token query required"})
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

// CORSMiddleware 规范化跨域处理
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Target-Node-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

