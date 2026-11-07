package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/model"
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
	jwtSecret, _ := model.GetSetting("jwt_secret")
	key := crypto.DeriveKey(jwtSecret)
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
			Password   string `json:"password"`
			Username   string `json:"username"`
			SecretPath string `json:"secret_path"`
			LoginURL   string `json:"login_url"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.Password != "RemotePass@999" {
			t.Fatalf("Expected revealed password 'RemotePass@999', got '%s'", res.Password)
		}
		if res.Username != "admin" || res.SecretPath != "secret123" {
			t.Fatalf("Revealed fields mismatch: %+v", res)
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
