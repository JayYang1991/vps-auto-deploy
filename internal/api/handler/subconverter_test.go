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
	"vps-auto-deploy/internal/model"
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
	_, _ = model.InitDB(":memory:")
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

func TestSaveAndGetSubconverterConfigPersistence(t *testing.T) {
	t.Setenv("SKIP_SERVICE_RELOAD", "1")
	// 初始化内存数据库
	dbPath := filepath.Join(t.TempDir(), "test_subconverter.db")
	_, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/subconverter/config", GetSubconverterConfig)
	r.POST("/subconverter/config", SaveSubconverterConfig)

	// 1. 保存结构化配置，包含 exclude_remarks="socks-in"
	saveReq := SaveSubconverterConfigRequest{
		IsRaw:         false,
		Listen:        "127.0.0.1",
		Port:          25500,
		ExcludeRemark: "socks-in",
		IncludeRemark: "HK",
		DefaultConfig: "config/ACL4SSR_Online.ini",
	}
	body, _ := json.Marshal(saveReq)
	req := httptest.NewRequest(http.MethodPost, "/subconverter/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for SaveSubconverterConfig, got %d: %s", w.Code, w.Body.String())
	}

	// 2. 验证数据库中已持久化配置
	dbExclude, err := model.GetSetting("subconverter_exclude_remarks")
	if err != nil || dbExclude != "socks-in" {
		t.Fatalf("expected db subconverter_exclude_remarks='socks-in', got: %s (err: %v)", dbExclude, err)
	}

	// 3. 验证通过 GET 读取返回的配置
	req = httptest.NewRequest(http.MethodGet, "/subconverter/config", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for GetSubconverterConfig, got %d", w.Code)
	}
	var resp SubconverterConfigResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ExcludeRemark != "socks-in" {
		t.Fatalf("expected exclude_remarks='socks-in', got: %s", resp.ExcludeRemark)
	}

	// 4. 模拟组件升级或文件被清理（删除磁盘 pref.ini）
	configPath := subconverter.GetConfigPath()
	_ = os.Remove(configPath)

	// 再次调用 GET /subconverter/config，验证自动从数据库兜底还原
	req = httptest.NewRequest(http.MethodGet, "/subconverter/config", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for GetSubconverterConfig after file removal, got %d", w.Code)
	}
	var restoredResp SubconverterConfigResponse
	_ = json.Unmarshal(w.Body.Bytes(), &restoredResp)
	if restoredResp.ExcludeRemark != "socks-in" {
		t.Fatalf("expected exclude_remarks restored from SQLite as 'socks-in', got: %s", restoredResp.ExcludeRemark)
	}

	// 验证磁盘上的 pref.ini 已被重新创建并包含 exclude_remarks
	recreatedData, err := os.ReadFile(configPath)
	if err != nil || !bytes.Contains(recreatedData, []byte("exclude_remarks=socks-in")) {
		t.Fatalf("expected recreated pref.ini on disk with exclude_remarks, got: %s (err: %v)", string(recreatedData), err)
	}
}

func TestIsSafeLocalTemplate(t *testing.T) {
	// 确保 custom_rules.ini 存在
	if err := EnsureCustomRulesFile(); err != nil {
		t.Fatalf("ensure custom rules file: %v", err)
	}

	// 合法模板
	if !IsSafeLocalTemplate("config/custom_rules.ini") {
		t.Errorf("expected config/custom_rules.ini to be safe")
	}

	// 非法穿越路径
	unsafePaths := []string{
		"../etc/passwd",
		"../../secret.key",
		"/etc/shadow",
		"config/../../etc/passwd",
		"http://evil.com",
		"https://evil.com",
		"config/not_exist_file_9999.ini",
		"config/illegal.sh",
		"",
	}
	for _, p := range unsafePaths {
		if IsSafeLocalTemplate(p) {
			t.Errorf("expected path '%s' to be unsafe", p)
		}
	}
}

func TestPatchSingboxTLSAndTransportSNI(t *testing.T) {
	rawJSON := []byte(`{
  "outbounds": [
    {
      "type": "vless",
      "tag": "test-vless",
      "server": "45.76.207.88",
      "server_port": 443,
      "sni": "argo.example.com",
      "tls": {
        "enabled": true
      },
      "transport": {
        "type": "ws",
        "path": "/ws",
        "headers": {}
      }
    }
  ]
}`)

	patched := PatchSingboxTLSAndTransportSNI(rawJSON)

	var root map[string]interface{}
	if err := json.Unmarshal(patched, &root); err != nil {
		t.Fatalf("unmarshal patched failed: %v", err)
	}

	outbounds := root["outbounds"].([]interface{})
	ob := outbounds[0].(map[string]interface{})
	tls := ob["tls"].(map[string]interface{})
	if tls["server_name"] != "argo.example.com" {
		t.Errorf("expected tls.server_name == 'argo.example.com', got '%v'", tls["server_name"])
	}

	transport := ob["transport"].(map[string]interface{})
	headers := transport["headers"].(map[string]interface{})
	if headers["Host"] != "argo.example.com" {
		t.Errorf("expected transport.headers.Host == 'argo.example.com', got '%v'", headers["Host"])
	}
}
