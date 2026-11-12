package crypto

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTLSGenerationAndPinning(t *testing.T) {
	_ = os.Setenv("ALLOW_PRIVATE_IP", "1")
	defer os.Unsetenv("ALLOW_PRIVATE_IP")

	tmpDir, err := os.MkdirTemp("", "tls_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// 1. 生成自签证书
	fp, err := GenerateSelfSignedCert(certPath, keyPath, []string{"127.0.0.1", "localhost"})
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}
	if len(fp) < 20 || !strings.Contains(fp, ":") {
		t.Fatalf("Invalid fingerprint format: %s", fp)
	}

	// 2. 测试从文件重新读取指纹
	fpFromFile, err := GetCertFingerprintFromFile(certPath)
	if err != nil {
		t.Fatalf("GetCertFingerprintFromFile failed: %v", err)
	}
	if fpFromFile != fp {
		t.Fatalf("Fingerprint mismatch: %s != %s", fpFromFile, fp)
	}

	// 3. 启动本地 HTTPS 测试服务器
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadX509KeyPair failed: %v", err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	// 4. 使用正确指纹请求 -> 必须成功
	goodClient := CreatePinnedHTTPClient(fp, 5*time.Second)
	resp, err := goodClient.Get(server.URL)
	if err != nil {
		t.Fatalf("Expected success with matching pinned fingerprint, got err: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}

	// 5. 使用伪造/错误的指纹请求 -> 必须被安全拦截
	fakeFP := "11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00"
	badClient := CreatePinnedHTTPClient(fakeFP, 5*time.Second)
	_, err = badClient.Get(server.URL)
	if err == nil {
		t.Fatalf("Expected error with mismatched pinned fingerprint, got nil!")
	}
	if !strings.Contains(err.Error(), "安全拦截") && !strings.Contains(err.Error(), "中间人") {
		t.Fatalf("Expected MITM security intercept message, got: %v", err)
	}

	// 6. 使用空指纹请求 -> 必须被安全拦截，严防无验证放行
	emptyClient := CreatePinnedHTTPClient("", 5*time.Second)
	_, err = emptyClient.Get(server.URL)
	if err == nil {
		t.Fatalf("Expected error with empty pinned fingerprint, got nil!")
	}
	if !strings.Contains(err.Error(), "安全拦截") && !strings.Contains(err.Error(), "指纹未配置") {
		t.Fatalf("Expected empty fingerprint security error, got: %v", err)
	}

	// 7. SSRF 拦截验证：当 ALLOW_PRIVATE_IP 未设置时，Pinned Client 请求本地回环/私网地址必须被拦截
	_ = os.Unsetenv("ALLOW_PRIVATE_IP")
	ssrfClient := CreatePinnedHTTPClient(fp, 5*time.Second)
	_, ssrfErr := ssrfClient.Get(server.URL)
	if ssrfErr == nil {
		t.Fatalf("Expected SSRF error for loopback address without ALLOW_PRIVATE_IP, got nil!")
	}
	if !strings.Contains(ssrfErr.Error(), "SSRF") {
		t.Fatalf("Expected SSRF error message, got: %v", ssrfErr)
	}
}

