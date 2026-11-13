package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/notifier"
)

func setupTelegramHandlerDB(t *testing.T) string {
	tmpDB, err := os.CreateTemp("", "handler_tg_test_*.db")
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
	_ = model.SetSetting("jwt_secret", "test_jwt_secret_tg_handler_123456789")
	return dbPath
}

func TestTelegramHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := setupTelegramHandlerDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.GET("/settings/telegram", GetTelegramConfig)
	r.POST("/settings/telegram", SaveTelegramConfig)
	r.POST("/settings/telegram/test", TestTelegram)

	// 1. 获取初始配置
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/settings/telegram", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var getResp struct {
		Config          notifier.TelegramConfig `json:"config"`
		AvailableEvents []string                `json:"available_events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(getResp.AvailableEvents) == 0 {
		t.Errorf("expected available events to be non-empty")
	}

	// 2. 尝试保存缺少 Chat ID 的启用配置 (应返回 400 Bad Request)
	saveReqMissingChatID := TelegramConfigRequest{
		Enabled:  true,
		BotToken: "123456:valid_token_test",
		ChatID:   "",
	}
	body, _ := json.Marshal(saveReqMissingChatID)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/settings/telegram", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for missing ChatID, got %d", w.Code)
	}

	// 3. 正常保存配置
	saveReqValid := TelegramConfigRequest{
		Enabled:  true,
		BotToken: "123456:valid_token_test",
		ChatID:   "998877",
		APIURL:   "https://api.telegram.org",
		Events:   []string{notifier.EventLoginFailed, notifier.EventIPLocked},
		Silent:   false,
	}
	body, _ = json.Marshal(saveReqValid)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/settings/telegram", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid save, got %d: %s", w.Code, w.Body.String())
	}

	// 4. 再次获取，验证掩码脱敏
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/settings/telegram", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var getRespAfter struct {
		Config notifier.TelegramConfig `json:"config"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &getRespAfter)
	if !getRespAfter.Config.Enabled {
		t.Errorf("expected Enabled true")
	}
	if getRespAfter.Config.ChatID != "998877" {
		t.Errorf("expected ChatID 998877, got %s", getRespAfter.Config.ChatID)
	}
	if getRespAfter.Config.MaskedToken == "" || getRespAfter.Config.MaskedToken == "123456:valid_token_test" {
		t.Errorf("expected token to be masked, got: %s", getRespAfter.Config.MaskedToken)
	}
}

