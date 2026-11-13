package notifier

import (
	"os"
	"strings"
	"testing"
	"time"

	"vps-auto-deploy/internal/model"
)

func setupTestDB(t *testing.T) string {
	tmpDB, err := os.CreateTemp("", "test_notifier_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	dbPath := tmpDB.Name()
	tmpDB.Close()

	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	model.DB = db
	_ = model.SetSetting("jwt_secret", "test_jwt_secret_for_notifier_encryption_32b")
	return dbPath
}

func TestMaskBotToken(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11", "123456:••••••••ew11"},
		{"123456:ABCDEF", "123456:••••••••"},
		{"short", "••••••••"},
		{"1234567890", "1234••••••••7890"},
		{"", "••••••••"},
	}

	for _, c := range cases {
		got := MaskBotToken(c.input)
		if got != c.expected {
			t.Errorf("MaskBotToken(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestFormatTelegramHTML(t *testing.T) {
	evt := Event{
		Type:      EventLoginFailed,
		Level:     LevelWarn,
		Title:     "登录失败 <test>",
		Message:   "IP & Password test: 1 < 2",
		ClientIP:  "192.168.1.1",
		Username:  "admin<script>",
		NodeName:  "Node & 1",
		Timestamp: time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC),
		Metadata: map[string]string{
			"param1": "val1 & <val2>",
		},
	}

	html := FormatTelegramHTML(evt)
	if !strings.Contains(html, "&lt;test&gt;") {
		t.Errorf("expected HTML escaping for title, got: %s", html)
	}
	if !strings.Contains(html, "admin&lt;script&gt;") {
		t.Errorf("expected HTML escaping for username, got: %s", html)
	}
	if !strings.Contains(html, "val1 &amp; &lt;val2&gt;") {
		t.Errorf("expected HTML escaping for metadata value, got: %s", html)
	}
	if !strings.Contains(html, "<b>安全警告</b>") {
		t.Errorf("expected level badge for LevelWarn, got: %s", html)
	}
}

func TestIsEventEnabled(t *testing.T) {
	cfg := &TelegramConfig{
		Enabled: true,
		Events:  []string{EventLoginFailed, EventIPLocked},
	}

	if !IsEventEnabled(cfg, EventLoginFailed) {
		t.Errorf("expected EventLoginFailed to be enabled")
	}
	if !IsEventEnabled(cfg, "login_failed") { // case insensitive
		t.Errorf("expected login_failed to be enabled (case-insensitive)")
	}
	if IsEventEnabled(cfg, EventLoginSuccess) {
		t.Errorf("expected EventLoginSuccess to be disabled")
	}

	cfgDisabled := &TelegramConfig{
		Enabled: false,
		Events:  []string{EventLoginFailed},
	}
	if IsEventEnabled(cfgDisabled, EventLoginFailed) {
		t.Errorf("expected EventLoginFailed to be disabled when cfg.Enabled is false")
	}
}

func TestTelegramConfigSaveAndLoad(t *testing.T) {
	dbPath := setupTestDB(t)
	defer os.Remove(dbPath)

	cfg := TelegramConfig{
		Enabled:  true,
		BotToken: "123456789:AAFakeTokenForTest_1234567890abcdef",
		ChatID:   "987654321",
		APIURL:   "https://api.telegram.org",
		Events:   []string{EventLoginFailed, EventIPLocked, EventCredsRevealed},
		Silent:   true,
	}

	if err := SaveTelegramConfig(cfg); err != nil {
		t.Fatalf("SaveTelegramConfig failed: %v", err)
	}

	loaded, err := GetTelegramConfig()
	if err != nil {
		t.Fatalf("GetTelegramConfig failed: %v", err)
	}

	if !loaded.Enabled {
		t.Errorf("expected Enabled true, got false")
	}
	if loaded.ChatID != "987654321" {
		t.Errorf("expected ChatID 987654321, got %q", loaded.ChatID)
	}
	if loaded.BotToken != cfg.BotToken {
		t.Errorf("decrypted BotToken mismatch: got %q, want %q", loaded.BotToken, cfg.BotToken)
	}
	if !strings.Contains(loaded.MaskedToken, "••••") {
		t.Errorf("expected MaskedToken to be masked, got %q", loaded.MaskedToken)
	}
	if !loaded.Silent {
		t.Errorf("expected Silent true, got false")
	}
	if len(loaded.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(loaded.Events))
	}
}

func TestTelegramConfigSSRFProtection(t *testing.T) {
	dbPath := setupTestDB(t)
	defer os.Remove(dbPath)

	ssrfTargets := []string{
		"http://127.0.0.1:8080",
		"http://localhost/test",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.1:80",
		"http://192.168.1.1:80",
	}

	for _, target := range ssrfTargets {
		cfg := TelegramConfig{
			Enabled:  true,
			BotToken: "123456:valid_token",
			ChatID:   "12345",
			APIURL:   target,
		}
		err := SaveTelegramConfig(cfg)
		if err == nil {
			t.Errorf("expected SSRF error for %s, got nil", target)
		}
	}
}

