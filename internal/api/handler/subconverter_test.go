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

func TestCustomRulesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/subconverter/custom-rules", GetSubconverterCustomRules)
	r.POST("/subconverter/custom-rules", SaveSubconverterCustomRules)
	r.GET("/subconverter/templates", ListSubconverterTemplates)

	// 1. 获取默认自定义规则模板（此时文件可能尚未落盘，应返回预置范例）
	req := httptest.NewRequest(http.MethodGet, "/subconverter/custom-rules", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for GetSubconverterCustomRules, got %d", w.Code)
	}

	var getResp struct {
		Path    string `json:"path"`
		Exists  bool   `json:"exists"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal json failed: %v", err)
	}
	if getResp.Content == "" {
		t.Errorf("expected non-empty default custom rules content")
	}

	// 2. 保存自定义规则内容
	customContent := "[custom]\ncustom_group=测试策略组`select`.*"
	saveBody, _ := json.Marshal(map[string]string{"content": customContent})
	req = httptest.NewRequest(http.MethodPost, "/subconverter/custom-rules", bytes.NewReader(saveBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for SaveSubconverterCustomRules, got %d: %s", w.Code, w.Body.String())
	}

	// 3. 再次获取，验证已成功落盘
	req = httptest.NewRequest(http.MethodGet, "/subconverter/custom-rules", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &getResp)
	if !getResp.Exists || getResp.Content != customContent {
		t.Errorf("expected content '%s', got '%s'", customContent, getResp.Content)
	}

	// 4. 获取模板列表，验证 custom_rules.ini 处于模板选项中
	req = httptest.NewRequest(http.MethodGet, "/subconverter/templates", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var templates []RuleTemplateItem
	_ = json.Unmarshal(w.Body.Bytes(), &templates)
	foundCustom := false
	for _, tpl := range templates {
		if tpl.Name == "custom_rules.ini" && tpl.ConfigValue == "config/custom_rules.ini" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Errorf("expected custom_rules.ini found in templates list")
	}
}
