package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoginRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.POST("/login", LoginRateLimitMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	testIP := "192.0.2.100"

	// 模拟前 4 次失败
	for i := 1; i <= 4; i++ {
		RecordFailedLogin(testIP)
	}

	// 尚未达到 5 次，应该能正常通过中间件
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = testIP + ":12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 before reaching limit, got %d", w.Code)
	}

	// 第 5 次失败，触发锁定
	RecordFailedLogin(testIP)

	reqLock := httptest.NewRequest(http.MethodPost, "/login", nil)
	reqLock.RemoteAddr = testIP + ":12345"
	wLock := httptest.NewRecorder()
	r.ServeHTTP(wLock, reqLock)
	if wLock.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", wLock.Code)
	}

	// 测试 ResetLoginAttempts
	ResetLoginAttempts(testIP)
	reqAfterReset := httptest.NewRequest(http.MethodPost, "/login", nil)
	reqAfterReset.RemoteAddr = testIP + ":12345"
	wAfterReset := httptest.NewRecorder()
	r.ServeHTTP(wAfterReset, reqAfterReset)
	if wAfterReset.Code != http.StatusOK {
		t.Fatalf("expected 200 after reset, got %d", wAfterReset.Code)
	}
}
