package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
)

type testResponseWriter struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func (tr *testResponseWriter) CloseNotify() <-chan bool {
	return tr.closed
}

func newTestRecorder() *testResponseWriter {
	return &testResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		closed:           make(chan bool, 1),
	}
}

func TestNodeProxyWithTLSFingerprint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir, err := os.MkdirTemp("", "proxy_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	_ = model.SetSetting("secret_path", "mastersecret")

	// 1. 创建受管端 TLS HTTPS Mock 服务器
	certPath := filepath.Join(tmpDir, "remote_cert.pem")
	keyPath := filepath.Join(tmpDir, "remote_key.pem")
	fp, err := crypto.GenerateSelfSignedCert(certPath, keyPath, []string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadX509KeyPair failed: %v", err)
	}

	remoteReceivedAuth := ""
	mockRemoteServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteReceivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok_from_remote"}`))
	}))
	mockRemoteServer.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	mockRemoteServer.StartTLS()
	defer mockRemoteServer.Close()

	remoteURL, _ := url.Parse(mockRemoteServer.URL)
	port := remoteURL.Port()
	var portInt int
	for _, c := range port {
		portInt = portInt*10 + int(c-'0')
	}

	// 2. 将此节点入库，配置 UseTLS=true 及正确的 CertFingerprint
	nodeID, err := model.CreateNode(&model.Node{
		Name:            "Secure-Node-1",
		Host:            "127.0.0.1",
		SSHPort:         22,
		PanelPort:       portInt,
		SecretPath:      "remotesecret",
		Username:        "admin",
		APIToken:        "mock-token-xyz",
		IsMaster:        false,
		UseTLS:          true,
		CertFingerprint: fp,
		Status:          "online",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// 3. 构建主控路由并挂载 NodeProxyMiddleware
	r := gin.New()
	sec := r.Group("/:secret")
	sec.Use(NodeProxyMiddleware())
	sec.GET("/api/test-endpoint", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"msg": "from_master"})
	})

	// 测试 A：携带正确的目标节点 ID -> 代理成功并注入 Token
	{
		req, _ := http.NewRequest(http.MethodGet, "/mastersecret/api/test-endpoint", nil)
		req.Header.Set("X-Target-Node-ID", "2")
		w := newTestRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK from proxy, got %d: %s", w.Code, w.Body.String())
		}
		if remoteReceivedAuth != "Bearer mock-token-xyz" {
			t.Fatalf("Expected injected token, got: %s", remoteReceivedAuth)
		}
	}

	// 测试 B：篡改指纹为伪造指纹 -> 触发中间人安全拦截 (502 Bad Gateway)
	{
		fakeFP := "99:88:77:66:55:44:33:22:11:00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66"
		_ = model.UpdateNode(&model.Node{
			ID:              nodeID,
			Name:            "Secure-Node-1",
			Host:            "127.0.0.1",
			SSHPort:         22,
			PanelPort:       portInt,
			SecretPath:      "remotesecret",
			Username:        "admin",
			APIToken:        "mock-token-xyz",
			UseTLS:          true,
			CertFingerprint: fakeFP, // 错误的指纹
			Status:          "online",
		})

		req, _ := http.NewRequest(http.MethodGet, "/mastersecret/api/test-endpoint", nil)
		req.Header.Set("X-Target-Node-ID", "2")
		w := newTestRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadGateway {
			t.Fatalf("Expected 502 Bad Gateway due to MITM fingerprint mismatch, got %d: %s", w.Code, w.Body.String())
		}
	}
}
