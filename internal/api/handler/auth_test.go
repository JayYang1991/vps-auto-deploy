package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"vps-auto-deploy/internal/model"
)

func setupHandlerAuthDB(t *testing.T) string {
	tmpDB, err := os.CreateTemp("", "handler_auth_test_*.db")
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
	_ = model.SetSetting("admin_username", "admin")
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123456"), bcrypt.DefaultCost)
	_ = model.SetSetting("admin_password_hash", string(hash))
	_ = model.SetSetting("jwt_secret", "test_jwt_secret_handler_123456789")
	return dbPath
}

func TestLoginAndSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := setupHandlerAuthDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.POST("/login", Login)
	r.GET("/settings", GetSettings)
	r.POST("/settings", UpdateSettings)
	r.POST("/revoke", RevokeAllSessions)

	// 1. 登录 (RememberMe = true)
	loginBody, _ := json.Marshal(map[string]interface{}{
		"username":    "admin",
		"password":    "admin123456",
		"remember_me": true,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	var loginResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &loginResp)
	if loginResp["token"] == "" || loginResp["remember_me"] != true {
		t.Fatalf("invalid login response: %v", loginResp)
	}

	// 2. 获取默认超时设置 (应为默认 30 分钟)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/settings", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	var settingsResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &settingsResp)
	if settingsResp["session_idle_timeout"] != float64(30) {
		t.Fatalf("expected default idle timeout 30, got %v", settingsResp["session_idle_timeout"])
	}

	// 3. 修改超时设置 (改为 15 分钟)
	newTimeout := 15
	updateBody, _ := json.Marshal(map[string]interface{}{
		"session_idle_timeout": newTimeout,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/settings", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on update, got %d", w.Code)
	}

	// 验证更新生效
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/settings", nil)
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &settingsResp)
	if settingsResp["session_idle_timeout"] != float64(15) {
		t.Fatalf("expected updated idle timeout 15, got %v", settingsResp["session_idle_timeout"])
	}

	// 4. 执行全量会话注销
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/revoke", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on revoke, got %d", w.Code)
	}
	minEpochStr, _ := model.GetSetting("auth_min_issued_at")
	if minEpochStr == "" {
		t.Fatalf("expected auth_min_issued_at to be set")
	}
}

