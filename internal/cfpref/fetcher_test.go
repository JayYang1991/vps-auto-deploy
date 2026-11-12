package cfpref

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"vps-auto-deploy/internal/model"
)

func TestParseAndSavePreferredIPs(t *testing.T) {
	// 初始化测试内存数据库
	_, err := model.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}

	rawLinks := `
vless://uuid-1@161.118.210.121:443?encryption=none&security=tls&sni=test.com#SG%E8%87%AA%E7%94%A8
vless://uuid-1@167.179.90.137:443?encryption=none&security=tls&sni=test.com#JP%E8%87%AA%E7%94%A8
vless://uuid-1@example.com:443?encryption=none#Invalid
vless://uuid-1@104.238.148.149:443?encryption=none#HK-Test
`
	b64 := base64.StdEncoding.EncodeToString([]byte(rawLinks))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(b64))
	}))
	defer ts.Close()

	_ = model.SetSetting("cf_pref_sub_url", ts.URL+"?host={host}&uuid={uuid}")
	_ = model.SetSetting("cf_pref_host", "test.com")
	_ = model.SetSetting("cf_pref_uuid", "uuid-1")

	// 1. 验证默认 SSRF 拦截生效 (ts.URL 为 127.0.0.1 回环地址)
	_, errSSRF := FetchAndSavePreferredIPs()
	if errSSRF == nil {
		t.Fatalf("expected SSRF block for %s, but passed", ts.URL)
	}

	// 2. 豁免 SSRF 后验证解析逻辑
	skipSSRFValidationForTest = true
	defer func() { skipSSRFValidationForTest = false }()

	res, err := FetchAndSavePreferredIPs()
	if err != nil {
		t.Fatalf("FetchAndSavePreferredIPs failed: %v", err)
	}

	if res.Count != 3 {
		t.Errorf("expected 3 valid nodes, got %d", res.Count)
	}

	ips, err := model.ListCFPreferredIPs("")
	if err != nil {
		t.Fatalf("ListCFPreferredIPs failed: %v", err)
	}
	if len(ips) != 3 {
		t.Errorf("expected 3 stored IPs, got %d", len(ips))
	}

	if ips[0].IP != "161.118.210.121" || ips[0].Colo != "SG" {
		t.Errorf("unexpected first ip: %+v", ips[0])
	}
}

func TestSaveSettingsValidation(t *testing.T) {
	_, err := model.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB error: %v", err)
	}

	// 1. 测试合法参数
	validSettings := map[string]interface{}{
		"cron_interval": "6h",
		"cron_time":     "04:30",
	}
	if err := SaveSettings(validSettings); err != nil {
		t.Fatalf("expected valid settings to pass, got error: %v", err)
	}

	// 2. 测试非法 interval (小于 10m 或格式错误)
	invalidInterval := map[string]interface{}{
		"cron_interval": "5m",
	}
	if err := SaveSettings(invalidInterval); err == nil {
		t.Fatalf("expected interval 5m to be rejected, but passed")
	}

	// 3. 测试非法 cron_time 格式
	invalidTime := map[string]interface{}{
		"cron_time": "25:00",
	}
	if err := SaveSettings(invalidTime); err == nil {
		t.Fatalf("expected time 25:00 to be rejected, but passed")
	}
}

