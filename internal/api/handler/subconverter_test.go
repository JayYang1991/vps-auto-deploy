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
	"vps-auto-deploy/internal/component/subconverter"
)

func TestRollbackSubconverter(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "pref.ini")

	oldContent := []byte("[common]\nport=25500\nlisten=127.0.0.1\n")
	newContent := []byte("[common]\nport=99999\nlisten=invalid\n")

	if err := os.WriteFile(cfgPath, newContent, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	drv := subconverter.NewDriver()
	rollbackSubconverter(cfgPath, oldContent, drv)

	restored, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read restored config: %v", err)
	}
	if string(restored) != string(oldContent) {
		t.Errorf("expected restored content %s, got %s", string(oldContent), string(restored))
	}
}

func TestSaveSubconverterConfigValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/subconverter/config", SaveSubconverterConfig)

	// 1. 测试非法端口 (>65535)
	badPortReq := SaveSubconverterConfigRequest{
		Port:   70000,
		Listen: "127.0.0.1",
	}
	body, _ := json.Marshal(badPortReq)
	req := httptest.NewRequest(http.MethodPost, "/subconverter/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for port > 65535, got %d", w.Code)
	}

	// 2. 测试非法 IP 地址
	badIPReq := SaveSubconverterConfigRequest{
		Port:   25500,
		Listen: "999.888.777.666",
	}
	body, _ = json.Marshal(badIPReq)
	req = httptest.NewRequest(http.MethodPost, "/subconverter/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid listen IP, got %d", w.Code)
	}

	// 3. 测试 raw_content 中包含非法端口
	badRawReq := SaveSubconverterConfigRequest{
		RawContent: "[common]\nport=88888\nlisten=127.0.0.1\n",
	}
	body, _ = json.Marshal(badRawReq)
	req = httptest.NewRequest(http.MethodPost, "/subconverter/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for raw_content with invalid port, got %d", w.Code)
	}
}
