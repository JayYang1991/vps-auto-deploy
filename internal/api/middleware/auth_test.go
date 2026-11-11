package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
)

func setupAuthTestDB(t *testing.T) string {
	tmpDB, err := os.CreateTemp("", "auth_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	dbPath := tmpDB.Name()
	tmpDB.Close()

	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	model.DB = db
	_ = model.SetSetting("jwt_secret", "test_jwt_secret_key_123456789012")
	return dbPath
}

func TestJWTAuthMiddlewareAndRevocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := setupAuthTestDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.Use(JWTAuthMiddleware())
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user": c.GetString("username")})
	})

	// 1. 正常生成 Token 并请求受保护资源
	token, err := GenerateJWTWithDuration("admin", 2*time.Hour)
	if err != nil {
		t.Fatalf("generate jwt failed: %v", err)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// 2. 模拟管理员执行全局注销 (设定的最小生效时刻在 Token 签发之后)
	revokeEpoch := time.Now().Add(1 * time.Second).Unix()
	_ = model.SetSetting("auth_min_issued_at", strconv.FormatInt(revokeEpoch, 10))

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized after revocation, got %d: %s", w2.Code, w2.Body.String())
	}

	// 3. 撤销后重新登录签发的 Token 能够正常通行
	time.Sleep(1100 * time.Millisecond) // 保证新 Token 的 IssuedAt > revokeEpoch
	newToken, err := GenerateJWTWithDuration("admin", 2*time.Hour)
	if err != nil {
		t.Fatalf("generate new jwt failed: %v", err)
	}

	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req3.Header.Set("Authorization", "Bearer "+newToken)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for new token, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestJWTAuthQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := setupAuthTestDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.Use(JWTAuthMiddleware())
	r.GET("/protected-sse", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user": c.GetString("username")})
	})

	token, _ := GenerateJWTWithDuration("admin", 1*time.Hour)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/protected-sse?token=%s", token), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK via query token, got %d", w.Code)
	}

	// 2. 常规 REST 端点禁止通过 URL Query 传递 Token
	r.GET("/protected-api/nodes", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	wRest := httptest.NewRecorder()
	reqRest, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/protected-api/nodes?token=%s", token), nil)
	r.ServeHTTP(wRest, reqRest)
	if wRest.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for REST endpoint with query token, got %d", wRest.Code)
	}

	// 3. 单次有效的 Stream Ticket 能够成功鉴权并防重放
	ticket := IssueStreamTicket("stream_user", 10*time.Second)
	wTicket := httptest.NewRecorder()
	reqTicket, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/protected-sse?ticket=%s", ticket), nil)
	r.ServeHTTP(wTicket, reqTicket)
	if wTicket.Code != http.StatusOK {
		t.Fatalf("expected 200 OK via stream ticket, got %d", wTicket.Code)
	}

	// 再次消费已作废的 ticket 必须失败 (防重放)
	wReplay := httptest.NewRecorder()
	reqReplay, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/protected-sse?ticket=%s", ticket), nil)
	r.ServeHTTP(wReplay, reqReplay)
	if wReplay.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for replayed stream ticket, got %d", wReplay.Code)
	}
}

func TestSecretPathMiddlewareAndCSP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := setupAuthTestDB(t)
	defer os.Remove(dbPath)

	_ = model.SetSetting("secret_path", "sec_portal_888")

	r := gin.New()
	r.Use(SecurityHeadersMiddleware())
	sec := r.Group("/:secret")
	sec.Use(SecretPathMiddleware())
	sec.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	// 1. 正确的安全路径 -> 200 OK + CSP 标头
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/sec_portal_888/ping", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w1.Code)
	}
	csp := w1.Header().Get("Content-Security-Policy")
	if csp == "" || !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("expected strict CSP header, got: %s", csp)
	}

	// 2. 错误的安全路径 -> 404 Not Found
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/wrong_secret_123/ping", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for wrong secret path, got %d", w2.Code)
	}
}

func TestSubscriptionRateLimitWithRealClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(SubscriptionRateLimitMiddleware())
	r.GET("/sub/:token", func(c *gin.Context) {
		c.String(http.StatusOK, "sub content")
	})

	// 模拟来自反向代理 (127.0.0.1) 但携带不同真实客户端 IP 的请求
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/sub/tok1", nil)
	req1.RemoteAddr = "127.0.0.1:54321"
	req1.Header.Set("CF-Connecting-IP", "203.0.113.199")
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for subscriber 1, got %d", w1.Code)
	}

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/sub/tok2", nil)
	req2.RemoteAddr = "127.0.0.1:54321"
	req2.Header.Set("CF-Connecting-IP", "203.0.113.200")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for subscriber 2, got %d", w2.Code)
	}
}

