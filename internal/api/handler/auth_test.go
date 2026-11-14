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

	// 5. 密码长度校验测试
	r.POST("/password", UpdatePassword)
	// 短密码应被拦截 (400)
	shortPassBody, _ := json.Marshal(map[string]string{
		"old_password": "admin123456",
		"new_password": "short",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/password", bytes.NewReader(shortPassBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for short password, got %d", w.Code)
	}

	// 5b. 密码首尾包含空格应被拦截 (VULN-16)
	spacePassBody, _ := json.Marshal(map[string]string{
		"old_password": "admin123456",
		"new_password": "  spaces_in_pwd_888  ",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/password", bytes.NewReader(spacePassBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for password with leading/trailing spaces, got %d", w.Code)
	}

	// 合法密码通过 (200)
	validPassBody, _ := json.Marshal(map[string]string{
		"old_password": "admin123456",
		"new_password": "newValidPassword888",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/password", bytes.NewReader(validPassBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid password, got %d: %s", w.Code, w.Body.String())
	}

	// 6. 安全路径与公网 IP 校验测试
	// 短安全路径应被拦截
	badPathBody, _ := json.Marshal(map[string]string{
		"secret_path": "a",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/settings", bytes.NewReader(badPathBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short secret_path, got %d", w.Code)
	}

	// 非法公网 IP 应被拦截
	badIPBody, _ := json.Marshal(map[string]string{
		"public_ip": "not an ip or domain!; rm -rf",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/settings", bytes.NewReader(badIPBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid public_ip, got %d", w.Code)
	}

	// 合法公网 IP 通过
	goodIPBody, _ := json.Marshal(map[string]string{
		"public_ip":   "198.51.100.1",
		"secret_path": "secure-admin-entry",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/settings", bytes.NewReader(goodIPBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid settings, got %d: %s", w.Code, w.Body.String())
	}
}

