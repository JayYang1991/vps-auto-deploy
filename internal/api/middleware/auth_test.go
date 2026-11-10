package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
}

