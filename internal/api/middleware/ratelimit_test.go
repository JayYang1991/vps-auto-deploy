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

func TestSubscriptionRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/sub/:token", SubscriptionRateLimitMiddleware(), func(c *gin.Context) {
		c.String(http.StatusOK, "subscription content")
	})

	testIP := "198.51.100.1"

	// 模拟请求直到达到上限
	for i := 1; i <= subMaxPerMinute; i++ {
		req := httptest.NewRequest(http.MethodGet, "/sub/test-token", nil)
		req.RemoteAddr = testIP + ":54321"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i, w.Code)
		}
	}

	// 超过上限应返回 429
	reqOver := httptest.NewRequest(http.MethodGet, "/sub/test-token", nil)
	reqOver.RemoteAddr = testIP + ":54321"
	wOver := httptest.NewRecorder()
	r.ServeHTTP(wOver, reqOver)
	if wOver.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exceeding limit, got %d", wOver.Code)
	}
}

func TestIsTrustedProxyAndCloudflareRealIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. 验证受信任反代识别
	trustedIPs := []string{
		"127.0.0.1",
		"::1",
		"10.0.0.5",
		"172.17.0.2",
		"192.168.1.100",
		"162.158.10.20", // Cloudflare 162.158.0.0/15
		"104.16.12.34",  // Cloudflare 104.16.0.0/13
		"173.245.48.1",  // Cloudflare 173.245.48.0/20
		"2606:4700::1",  // Cloudflare IPv6
	}
	for _, ip := range trustedIPs {
		if !IsTrustedProxy(ip) {
			t.Errorf("expected IP %s to be trusted proxy", ip)
		}
	}

	// 2. 验证非受信公网 IP
	untrustedIPs := []string{
		"203.0.113.88",
		"198.51.100.1",
		"8.8.8.8",
		"1.1.1.1",
	}
	for _, ip := range untrustedIPs {
		if IsTrustedProxy(ip) {
			t.Errorf("expected IP %s to NOT be trusted proxy", ip)
		}
	}

	// 3. 验证通过 Cloudflare 边缘节点代理时安全解析 CF-Connecting-IP
	r := gin.New()
	var extractedIP string
	r.GET("/test-ip", func(c *gin.Context) {
		extractedIP = GetRealClientIP(c)
		c.String(http.StatusOK, "ok")
	})

	// 模拟来自 Cloudflare 边缘节点的请求
	reqCF := httptest.NewRequest(http.MethodGet, "/test-ip", nil)
	reqCF.RemoteAddr = "162.158.12.34:44321"
	reqCF.Header.Set("CF-Connecting-IP", "114.248.55.66")
	wCF := httptest.NewRecorder()
	r.ServeHTTP(wCF, reqCF)
	if extractedIP != "114.248.55.66" {
		t.Errorf("expected CF-Connecting-IP 114.248.55.66 to be extracted, got %s", extractedIP)
	}

	// 4. 验证恶意直连客户端尝试伪造 CF-Connecting-IP 时被坚决忽略 (防伪造)
	reqAttacker := httptest.NewRequest(http.MethodGet, "/test-ip", nil)
	reqAttacker.RemoteAddr = "203.0.113.88:44321"
	reqAttacker.Header.Set("CF-Connecting-IP", "1.1.1.1") // 攻击者尝试伪装
	wAttacker := httptest.NewRecorder()
	r.ServeHTTP(wAttacker, reqAttacker)
	if extractedIP != "203.0.113.88" {
		t.Errorf("expected attacker's real IP 203.0.113.88 to be returned, got %s", extractedIP)
	}

	// 5. 验证 X-Forwarded-For 从右向左解析首个非受信任代理 IP，防左侧客户端伪造
	reqXFF := httptest.NewRequest(http.MethodGet, "/test-ip", nil)
	reqXFF.RemoteAddr = "127.0.0.1:44321" // 来自受信本机 Nginx 反代
	// 客户端伪造了 1.2.3.4，实际客户端是 198.51.100.99，中间反代是 10.0.0.2
	reqXFF.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.99, 10.0.0.2")
	wXFF := httptest.NewRecorder()
	r.ServeHTTP(wXFF, reqXFF)
	if extractedIP != "198.51.100.99" {
		t.Errorf("expected rightmost untrusted IP 198.51.100.99, got %s", extractedIP)
	}
}
