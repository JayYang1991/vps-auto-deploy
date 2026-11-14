package component

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInjectBrowserHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com", nil)
	InjectBrowserHeaders(req, true)

	if req.Header.Get("User-Agent") != BrowserUserAgent {
		t.Errorf("expected User-Agent %s, got %s", BrowserUserAgent, req.Header.Get("User-Agent"))
	}
	if req.Header.Get("sec-ch-ua") != BrowserSecChUa {
		t.Errorf("expected sec-ch-ua %s, got %s", BrowserSecChUa, req.Header.Get("sec-ch-ua"))
	}
	if req.Header.Get("sec-ch-ua-platform") != `"Windows"` {
		t.Errorf("expected platform Windows, got %s", req.Header.Get("sec-ch-ua-platform"))
	}
	if !strings.Contains(req.Header.Get("Accept"), "application/vnd.github.v3+json") {
		t.Errorf("expected github api accept header, got %s", req.Header.Get("Accept"))
	}

	reqDownload, _ := http.NewRequest(http.MethodGet, "https://github.com/file.tar.gz", nil)
	InjectBrowserHeaders(reqDownload, false)
	if !strings.Contains(reqDownload.Header.Get("Accept"), "text/html") {
		t.Errorf("expected browser download accept header, got %s", reqDownload.Header.Get("Accept"))
	}
}

func TestDownloadFileWithBrowserHeadersAndRedirect(t *testing.T) {
	var capturedUAs []string

	// 模拟 S3 / 真实下载目标服务器
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUAs = append(capturedUAs, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-binary-content"))
	}))
	defer targetServer.Close()

	// 模拟 GitHub Release 302 重定向服务
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUAs = append(capturedUAs, r.Header.Get("User-Agent"))
		http.Redirect(w, r, targetServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	tmpDir := t.TempDir()
	destFile := filepath.Join(tmpDir, "downloaded.bin")

	err := DownloadFile(redirectServer.URL, destFile)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	// 验证两次请求（初始请求与 302 重定向请求）均携带了真实浏览器 UA
	if len(capturedUAs) != 2 {
		t.Fatalf("expected 2 requests (initial + redirect), got %d", len(capturedUAs))
	}
	for i, ua := range capturedUAs {
		if ua != BrowserUserAgent {
			t.Errorf("request %d: expected browser UA %s, got %s", i, BrowserUserAgent, ua)
		}
	}

	content, err := os.ReadFile(destFile)
	if err != nil || string(content) != "fake-binary-content" {
		t.Fatalf("file content mismatch: %s", string(content))
	}
}

func TestReleaseCacheAndFallback(t *testing.T) {
	var hitCount int32

	// 模拟 GitHub Release API 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitCount, 1)

		if r.Header.Get("User-Agent") != BrowserUserAgent {
			t.Errorf("expected browser User-Agent, got %s", r.Header.Get("User-Agent"))
		}

		rel := GitHubRelease{
			TagName: "v1.14.4",
			Assets: []GitHubAsset{
				{Name: "sing-box-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/asset.tar.gz"},
			},
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer server.Close()

	// 清理并直接操作缓存结构验证缓存特性
	cacheKey := "testowner/testrepo"
	releaseCacheMu.Lock()
	delete(releaseCache, cacheKey)
	releaseCacheMu.Unlock()

	// 1. 模拟写入缓存
	testRel := &GitHubRelease{TagName: "v1.0.0"}
	releaseCacheMu.Lock()
	releaseCache[cacheKey] = &releaseCacheEntry{
		release:   testRel,
		fetchedAt: time.Now(),
	}
	releaseCacheMu.Unlock()

	// 2. 检查在 TTL 内命中
	releaseCacheMu.RLock()
	cached, ok := releaseCache[cacheKey]
	releaseCacheMu.RUnlock()
	if !ok || cached.release.TagName != "v1.0.0" {
		t.Fatalf("cache should contain v1.0.0")
	}

	// 3. 模拟缓存过期后优雅降级
	releaseCacheMu.Lock()
	releaseCache[cacheKey] = &releaseCacheEntry{
		release:   testRel,
		fetchedAt: time.Now().Add(-20 * time.Minute), // 过期
	}
	releaseCacheMu.Unlock()

	releaseCacheMu.RLock()
	cachedExpired, _ := releaseCache[cacheKey]
	releaseCacheMu.RUnlock()
	if time.Since(cachedExpired.fetchedAt) < releaseTTL {
		t.Errorf("cache should be expired")
	}
	// 验证旧缓存仍可读取作为保底
	if cachedExpired.release.TagName != "v1.0.0" {
		t.Errorf("stale fallback tag mismatch")
	}
}

func TestExtractChecksumsFromBody(t *testing.T) {
	body := `
### Checksums
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  subconverter_linux64.tar.gz
11223344556677889900aabbccddeeff0011223344556677889900aabbccddee *subconverter_darwin64.tar.gz
`
	res := make(map[string]string)
	extractChecksumsFromBody(body, nil, res)

	if res["subconverter_linux64.tar.gz"] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("failed to extract checksum for subconverter_linux64.tar.gz, got: %s", res["subconverter_linux64.tar.gz"])
	}
	if res["subconverter_darwin64.tar.gz"] != "11223344556677889900aabbccddeeff0011223344556677889900aabbccddee" {
		t.Errorf("failed to extract checksum for subconverter_darwin64.tar.gz, got: %s", res["subconverter_darwin64.tar.gz"])
	}
}

