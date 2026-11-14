package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
	"vps-auto-deploy/internal/pkg/crypto"
)

func setupTestDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "handler_node_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// Set known admin password
	hash, _ := bcrypt.GenerateFromPassword([]byte("master_pass_123"), bcrypt.DefaultCost)
	_ = model.SetSetting("admin_password_hash", string(hash))
	_ = model.SetSetting("jwt_secret", "test_jwt_secret_key_123456789012")
}

func TestNodeHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	// Insert remote test node with encrypted password
	key := model.GetEncryptionKey()
	encPass, _ := crypto.EncryptAESGCM("RemotePass@999", key)

	nodeID, err := model.CreateNode(&model.Node{
		Name:              "HK-Server-01",
		Host:              "192.168.10.20",
		SSHPort:           22,
		PanelPort:         20888,
		SecretPath:        "secret123",
		Username:          "admin",
		PasswordEncrypted: encPass,
		APIToken:          "remote-jwt-token",
		IsMaster:          false,
		UseTLS:            true,
		CertFingerprint:   "aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99",
		SSHHostKey:        "SHA256:mockSSHHostKeyFingerprint123456789",
		Status:            "online",
		OSInfo:            "linux/amd64",
		LatencyMs:         32,
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// 1. Test ListNodes
	{
		r := gin.New()
		r.GET("/api/nodes", ListNodes)
		req, _ := http.NewRequest(http.MethodGet, "/api/nodes", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("ListNodes returned %d", w.Code)
		}
		var nodes []model.Node
		_ = json.Unmarshal(w.Body.Bytes(), &nodes)
		if len(nodes) < 2 {
			t.Fatalf("Expected at least 2 nodes (master + remote), got %d", len(nodes))
		}
	}

	// 2. Test RevealNodeCredentials with WRONG master password -> 403 Forbidden
	{
		r := gin.New()
		r.POST("/api/nodes/:id/reveal-credentials", RevealNodeCredentials)

		body, _ := json.Marshal(map[string]string{
			"master_password": "wrong_password",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/nodes/2/reveal-credentials", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("Expected 403 Forbidden with wrong master password, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 3. Test RevealNodeCredentials with CORRECT master password -> 200 OK & decrypted password
	{
		r := gin.New()
		r.POST("/api/nodes/:id/reveal-credentials", RevealNodeCredentials)

		body, _ := json.Marshal(map[string]string{
			"master_password": "master_pass_123",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/nodes/2/reveal-credentials", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK with correct master password, got %d: %s", w.Code, w.Body.String())
		}

		var res struct {
			Password        string `json:"password"`
			Username        string `json:"username"`
			SecretPath      string `json:"secret_path"`
			LoginURL        string `json:"login_url"`
			UseTLS          bool   `json:"use_tls"`
			CertFingerprint string `json:"cert_fingerprint"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.Password != "RemotePass@999" {
			t.Fatalf("Expected revealed password 'RemotePass@999', got '%s'", res.Password)
		}
		if res.Username != "admin" || res.SecretPath != "secret123" || !res.UseTLS {
			t.Fatalf("Revealed fields mismatch: %+v", res)
		}
		if !strings.HasPrefix(res.LoginURL, "https://") {
			t.Fatalf("Expected https login url, got %s", res.LoginURL)
		}
	}

	// 3b. Test UpdateNode (customize name and sub_prefix)
	{
		r := gin.New()
		r.PUT("/api/nodes/:id", UpdateNode)

		payload := []byte(`{"name":"🇭🇰 香港BGP专线","sub_prefix":"🇭🇰 香港01"}`)
		req, _ := http.NewRequest(http.MethodPut, "/api/nodes/2", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 when updating node, got %d: %s", w.Code, w.Body.String())
		}

		updated, err := model.GetNode(2)
		if err != nil {
			t.Fatalf("Failed to fetch node 2: %v", err)
		}
		if updated.Name != "🇭🇰 香港BGP专线" {
			t.Fatalf("Expected updated name '🇭🇰 香港BGP专线', got '%s'", updated.Name)
		}
		if updated.SubPrefix != "🇭🇰 香港01" {
			t.Fatalf("Expected updated sub_prefix '🇭🇰 香港01', got '%s'", updated.SubPrefix)
		}
	}

	// 4. Test DeleteNode for master node (ID=1) -> 403 Forbidden
	{
		r := gin.New()
		r.DELETE("/api/nodes/:id", DeleteNode)
		req, _ := http.NewRequest(http.MethodDelete, "/api/nodes/1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("Expected 403 when deleting master node, got %d", w.Code)
		}
	}

	// 5. Test DeleteNode for remote node -> 200 OK
	{
		r := gin.New()
		r.DELETE("/api/nodes/:id", DeleteNode)
		req, _ := http.NewRequest(http.MethodDelete, "/api/nodes/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 when deleting remote node, got %d", w.Code)
		}

		_, err := model.GetNode(nodeID)
		if err == nil {
			t.Fatalf("Node %d should have been deleted", nodeID)
		}
	}
}

func TestStartUpgradeNode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	// 创建远程节点
	nodeID, err := model.CreateNode(&model.Node{
		Name:      "JP-Server-02",
		Host:      "192.168.20.30",
		SSHPort:   2222,
		Status:    "online",
		OSInfo:    "linux/amd64",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	r := gin.New()
	r.POST("/api/nodes/upgrade", StartUpgradeNode)

	// 1. 测试 host 为空，但提供了合法 node_id -> 应该自动从数据库解析 host，返回 200 并启动任务
	{
		body, _ := json.Marshal(map[string]interface{}{
			"node_id":   nodeID,
			"host":      "", // 空 host
			"auth_type": "password",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/nodes/upgrade", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 when node_id is provided even if host is empty, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["task_id"] == nil || resp["task_id"] == "" {
			t.Errorf("Expected valid task_id, got %v", resp["task_id"])
		}
	}

	// 2. 测试 host 为空且 node_id 无效 -> 应该报错 400 Host 不能为空
	{
		body, _ := json.Marshal(map[string]interface{}{
			"node_id":   99999,
			"host":      "",
			"auth_type": "password",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/nodes/upgrade", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("Expected 400 when host cannot be found, got %d", w.Code)
		}
	}
}

func TestStreamDeployLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	task := node.CreateDeployTask("Test Node", "127.0.0.1")

	r := gin.New()
	r.GET("/api/nodes/deploy-logs", StreamDeployLogs)

	go func() {
		// 模拟日志追加并结束任务
		time.Sleep(100 * time.Millisecond)
		task.AddLog("Step 1 done")
		time.Sleep(100 * time.Millisecond)
		task.AddLog("[SUCCESS] All done")
	}()

	req, _ := http.NewRequest(http.MethodGet, "/api/nodes/deploy-logs?task_id="+task.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Step 1 done") {
		t.Errorf("Expected body to contain 'Step 1 done', got: %s", body)
	}
	if !strings.Contains(body, "[SUCCESS]") {
		t.Errorf("Expected body to contain '[SUCCESS]', got: %s", body)
	}
}

func TestDeleteNodeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	r := gin.New()
	r.DELETE("/api/nodes/:id", DeleteNode)

	// Create a node to delete
	nodeID, _ := model.CreateNode(&model.Node{
		Name:     "Delete-Test-Node",
		Host:     "1.2.3.4",
		IsMaster: false,
	})

	// Case 1: Master node ID=1 cannot be deleted
	req, _ := http.NewRequest(http.MethodDelete, "/api/nodes/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when deleting master node, got %d", w.Code)
	}

	// Case 2: destroy_cloud=true but no master_password in body -> 400
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/nodes/%d?destroy_cloud=true", nodeID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when destroying cloud without password, got %d", w.Code)
	}

	// Case 3: master_password in query param only -> rejected with 400
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/nodes/%d?destroy_cloud=true&master_password=master_pass_123", nodeID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when password passed via query, got %d", w.Code)
	}

	// Case 4: wrong password in body -> 403
	body, _ := json.Marshal(map[string]any{"destroy_cloud": true, "master_password": "wrong_password"})
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/nodes/%d", nodeID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong master password, got %d", w.Code)
	}

	// Case 5: normal delete without cloud destroy -> 200
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/nodes/%d", nodeID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for normal delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestValidateTargetNodeHostSSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	r := gin.New()
	r.POST("/api/nodes/deploy", StartDeployNode)
	r.POST("/api/nodes/import", ImportNode)

	badHosts := []string{
		"169.254.169.254",
		"127.0.0.1",
		"localhost",
		"::1",
		"host with space",
		"host;evil",
	}

	for _, badHost := range badHosts {
		// Deploy
		depBody, _ := json.Marshal(map[string]any{
			"name":           "Evil Node",
			"host":           badHost,
			"ssh_port":       22,
			"ssh_user":       "root",
			"admin_username": "admin",
			"admin_password": "valid_pass_123",
			"secret_path":    "sec_path",
		})
		reqDep, _ := http.NewRequest(http.MethodPost, "/api/nodes/deploy", bytes.NewReader(depBody))
		reqDep.Header.Set("Content-Type", "application/json")
		wDep := httptest.NewRecorder()
		r.ServeHTTP(wDep, reqDep)
		if wDep.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for bad deploy host %q, got %d: %s", badHost, wDep.Code, wDep.Body.String())
		}

		// Import
		impBody, _ := json.Marshal(map[string]any{
			"name":        "Evil Node",
			"host":        badHost,
			"panel_port":  20888,
			"secret_path": "sec_path",
			"username":    "admin",
			"password":    "pass",
		})
		reqImp, _ := http.NewRequest(http.MethodPost, "/api/nodes/import", bytes.NewReader(impBody))
		reqImp.Header.Set("Content-Type", "application/json")
		wImp := httptest.NewRecorder()
		r.ServeHTTP(wImp, reqImp)
		if wImp.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for bad import host %q, got %d: %s", badHost, wImp.Code, wImp.Body.String())
		}
	}
}


