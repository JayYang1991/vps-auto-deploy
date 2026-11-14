package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/subconverter"
	"vps-auto-deploy/internal/model"
)

func TestBuildAttachmentHeader(t *testing.T) {
	// 1. 常规英文文件名
	h1 := buildAttachmentHeader("sub-test", ".txt")
	if !strings.Contains(h1, `filename="sub-test.txt"`) {
		t.Fatalf("expected valid ASCII filename, got %s", h1)
	}

	// 2. 包含 CRLF 标头注入与特殊字符的文件名
	malicious := "user\r\nSet-Cookie: evil=1\r\n"
	h2 := buildAttachmentHeader(malicious, ".txt")
	if strings.Contains(h2, "\r") || strings.Contains(h2, "\n") {
		t.Fatalf("CRLF must be stripped/escaped from Content-Disposition header, got %q", h2)
	}
	if strings.Contains(h2, "Set-Cookie") && !strings.Contains(h2, "Set-Cookie_") {
		// Should be sanitized as safe ASCII
		t.Logf("Sanitized header: %s", h2)
	}

	// 3. 包含中文等 Unicode 字符的文件名 (符合 RFC 6266 / RFC 5987)
	h3 := buildAttachmentHeader("sub-张三-节点", ".yaml")
	if !strings.Contains(h3, "filename*=") {
		t.Fatalf("expected filename*= for unicode filename, got %s", h3)
	}
	if !strings.Contains(h3, "UTF-8''") {
		t.Fatalf("expected UTF-8'' prefix in filename*, got %s", h3)
	}
}

func TestIsAllowedRemoteConfigHost(t *testing.T) {
	allowedURLs := []string{
		"https://raw.githubusercontent.com/JayYang1991/ACL4SSR/refs/heads/main/Clash/config/ACL4SSR_Online_Bespoke.ini",
		"https://gist.githubusercontent.com/user/12345/raw/config.ini",
		"https://cdn.jsdelivr.net/gh/user/repo/rules.ini",
		"https://fastly.jsdelivr.net/npm/rules/config.ini",
		"https://testingcf.jsdelivr.net/rules.ini",
		"https://ghproxy.net/https://raw.githubusercontent.com/rules.ini",
		"https://mirror.ghproxy.com/https://raw.githubusercontent.com/rules.ini",
	}
	for _, u := range allowedURLs {
		if !isAllowedRemoteConfigHost(u) {
			t.Errorf("expected %s to be allowed remote config host", u)
		}
	}

	disallowedURLs := []string{
		"http://evil-domain.com/malicious_rules.ini",
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:25500/version",
		"https://attacker.io/redirect-to-internal",
		"http://sub.domain.attacker.com/config.ini",
	}
	for _, u := range disallowedURLs {
		if isAllowedRemoteConfigHost(u) {
			t.Errorf("expected %s to be disallowed remote config host", u)
		}
	}
}

func TestServeSubscription_RemoteTemplateSecurity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_sub_sec.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/sub/:token", ServeSubscription)

	// 获取默认 member 的 sub_token
	var token string
	err = db.QueryRow("SELECT sub_token FROM members LIMIT 1").Scan(&token)
	if err != nil {
		t.Fatalf("query member token failed: %v", err)
	}

	// 插入一个可用入站以生成节点 URI
	_, _ = db.Exec(`
		INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES 
		('vless-in', 'vless-reality', '0.0.0.0', 443, '{"server_name":"example.com"}', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)

	// 1. 测试未授权/恶意域名作为 config 模板被拦截
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/sub/"+token+"?target=clash&config=https://evil.com/rule.ini", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for untrusted template URL, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "未列入受信任名单") {
		t.Errorf("expected error message about trusted list, got: %s", w.Body.String())
	}

	// 2. 测试本地穿越路径作为 config 模板被拦截
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/sub/"+token+"?target=clash&config=../../etc/passwd", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal template, got %d", w.Code)
	}

	// 3. 测试受信任且已缓存的合法远程模板通过安全预取
	testURL := "https://raw.githubusercontent.com/JayYang1991/ACL4SSR/refs/heads/main/Clash/config/ACL4SSR_Online_Bespoke.ini"
	compDir := subconverter.GetComponentDir()
	cacheDir := filepath.Join(compDir, "config", "cache")
	_ = os.MkdirAll(cacheDir, 0755)
	h := sha256Sum([]byte(testURL))
	mockCacheFile := filepath.Join(cacheDir, h+".ini")
	_ = os.WriteFile(mockCacheFile, []byte("[custom]\n"), 0600)
	defer os.Remove(mockCacheFile)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/sub/"+token+"?target=clash&config="+testURL, nil)
	r.ServeHTTP(w, req)
	// 无论本地 Subconverter 是否处于活跃状态，此时应通过安全校验并进入转换流程 (非 400 Invalid template)
	if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "Invalid template") {
		t.Errorf("expected valid remote trusted template to pass template validation, got: %s", w.Body.String())
	}
}

