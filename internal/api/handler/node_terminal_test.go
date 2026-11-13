package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
)

func TestTerminalTicketWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir, err := os.MkdirTemp("", "handler_terminal_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// 设置主控密码
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123456"), bcrypt.DefaultCost)
	_ = model.SetSetting("admin_password_hash", string(hash))
	_ = model.SetSetting("jwt_secret", "0123456789abcdef0123456789abcdef")

	// 创建测试节点
	nodeID, err := model.CreateNode(&model.Node{
		Name:      "Terminal Test Node",
		Host:      "192.168.1.100",
		SSHPort:   22,
		PanelPort: 20888,
		Status:    "online",
		IsMaster:  false,
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	r := gin.New()
	r.POST("/api/nodes/:id/terminal-ticket", RequestTerminalTicket)
	r.POST("/api/nodes/:id/ssh-credentials", SaveNodeSSHCredentials)

	// 1. 测试错误密码二次鉴权拦截
	badBody, _ := json.Marshal(gin.H{"master_password": "wrong"})
	reqBad, _ := http.NewRequest(http.MethodPost, "/api/nodes/1/terminal-ticket", bytes.NewReader(badBody))
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()
	r.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden on wrong password, got %d", wBad.Code)
	}

	// 2. 测试正确密码签发 Ticket (含临时凭据载荷)
	okBody, _ := json.Marshal(gin.H{
		"master_password": "admin123456",
		"temp_user":       "ubuntu",
		"temp_pass":       "temp_pwd_123",
		"temp_key":        "temp_key_content",
	})
	reqOK, _ := http.NewRequest(http.MethodPost, "/api/nodes/1/terminal-ticket", bytes.NewReader(okBody))
	reqOK.Header.Set("Content-Type", "application/json")
	wOK := httptest.NewRecorder()
	r.ServeHTTP(wOK, reqOK)
	if wOK.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", wOK.Code, wOK.Body.String())
	}

	var res struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expires_in"`
		IsMaster  bool   `json:"is_master"`
	}
	if err := json.Unmarshal(wOK.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal ticket response failed: %v", err)
	}
	if res.Ticket == "" {
		t.Errorf("ticket should not be empty")
	}

	// 3. 测试 Ticket 验证、临时凭据绑定与一次性消费
	ticketInfo, ok := node.ValidateAndConsumeTicket(res.Ticket, 1)
	if !ok || ticketInfo == nil {
		t.Fatalf("expected ticket to be valid on first consume")
	}
	if ticketInfo.TempUser != "ubuntu" || ticketInfo.TempPass != "temp_pwd_123" || ticketInfo.TempKey != "temp_key_content" {
		t.Errorf("unexpected ticket payload: %+v", ticketInfo)
	}
	if _, ok := node.ValidateAndConsumeTicket(res.Ticket, 1); ok {
		t.Errorf("ticket should be destroyed after first consume (anti-replay)")
	}

	// 4. 测试保存 SSH 凭据
	credBody, _ := json.Marshal(gin.H{
		"master_password": "admin123456",
		"ssh_user":        "root",
		"ssh_password":    "secret_root_pass",
	})
	reqCred, _ := http.NewRequest(http.MethodPost, "/api/nodes/1/ssh-credentials", bytes.NewReader(credBody))
	reqCred.Header.Set("Content-Type", "application/json")
	wCred := httptest.NewRecorder()
	r.ServeHTTP(wCred, reqCred)
	if wCred.Code != http.StatusOK {
		t.Fatalf("SaveNodeSSHCredentials failed: %d: %s", wCred.Code, wCred.Body.String())
	}

	// 验证节点已更新凭据
	updatedNode, _ := model.GetNode(1)
	if !updatedNode.HasSSHAuth {
		t.Errorf("expected updatedNode.HasSSHAuth to be true")
	}

	_ = nodeID
	_ = time.Second
}

func TestCheckOriginSecurity(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "check_origin_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// 配置 public_ip
	_ = model.SetSetting("public_ip", "my-pub-domain.com")

	tests := []struct {
		name       string
		origin     string
		host       string
		fwdHost    string
		remoteAddr string
		expected   bool
	}{
		{
			name:       "直连 Host 完全匹配",
			origin:     "http://1.2.3.4:20888",
			host:       "1.2.3.4:20888",
			remoteAddr: "114.248.1.1:54321",
			expected:   true,
		},
		{
			name:       "直连 Hostname 匹配 (忽略端口)",
			origin:     "https://1.2.3.4",
			host:       "1.2.3.4:20888",
			remoteAddr: "114.248.1.1:54321",
			expected:   true,
		},
		{
			name:       "反向代理 X-Forwarded-Host 匹配",
			origin:     "https://panel.example.com",
			host:       "127.0.0.1:20888",
			fwdHost:    "panel.example.com",
			remoteAddr: "127.0.0.1:54321",
			expected:   true,
		},
		{
			name:       "本地调试 localhost 放行",
			origin:     "http://localhost:5173",
			host:       "127.0.0.1:20888",
			remoteAddr: "127.0.0.1:54321",
			expected:   true,
		},
		{
			name:       "局域网私有 IP 放行",
			origin:     "http://192.168.1.50:20888",
			host:       "127.0.0.1:20888",
			remoteAddr: "127.0.0.1:54321",
			expected:   true,
		},
		{
			name:       "匹配已配置的面板公网资产 public_ip",
			origin:     "https://my-pub-domain.com",
			host:       "127.0.0.1:20888",
			remoteAddr: "127.0.0.1:54321",
			expected:   true,
		},
		{
			name:       "【安全测试】恶意第三方跨源网站哪怕经由本地反代也必须拦截 (防 CSWSH)",
			origin:     "https://evil-hacker.com",
			host:       "127.0.0.1:20888",
			fwdHost:    "",
			remoteAddr: "127.0.0.1:54321",
			expected:   false,
		},
		{
			name:       "【安全测试】恶意第三方跨源网站直连必须拦截",
			origin:     "https://attacker-origin.net",
			host:       "1.2.3.4:20888",
			fwdHost:    "",
			remoteAddr: "203.0.113.88:54321",
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/terminal", nil)
			if err != nil {
				t.Fatalf("NewRequest error: %v", err)
			}
			req.Host = tc.host
			req.RemoteAddr = tc.remoteAddr
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fwdHost != "" {
				req.Header.Set("X-Forwarded-Host", tc.fwdHost)
			}

			allowed := upgrader.CheckOrigin(req)
			if allowed != tc.expected {
				t.Errorf("[%s] origin=%s, host=%s, remote=%s: got %v, expected %v",
					tc.name, tc.origin, tc.host, tc.remoteAddr, allowed, tc.expected)
			}
		})
	}
}
