package configgen

import (
	"os"
	"path/filepath"
	"testing"

	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
)

func TestRollbackSingbox(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	oldContent := []byte(`{"log":{"level":"info"}}`)
	newContent := []byte(`{"log":{"level":"debug"},"invalid": true}`)

	// 1. 模拟写入新配置
	if err := os.WriteFile(cfgPath, newContent, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	driver := singbox.NewDriver()
	// 2. 执行回滚 (wasActive = false，避免没有 systemd 时调用报错)
	rollbackSingbox(cfgPath, oldContent, driver, false)

	// 3. 验证内容已恢复为 oldContent
	restored, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read restored config: %v", err)
	}
	if string(restored) != string(oldContent) {
		t.Errorf("expected restored content %s, got %s", string(oldContent), string(restored))
	}

	// 4. 测试原配置为空时的回滚 (应删除目标文件)
	rollbackSingbox(cfgPath, nil, driver, false)
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted when oldConfig is empty")
	}
}

func TestGenerateServerConfig(t *testing.T) {
	// 初始化 SQLite DB
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	cfg, err := GenerateServerConfig()
	if err != nil {
		t.Fatalf("GenerateServerConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil SingboxConfig")
	}
	// 验证默认必有 direct 和 block 出站
	hasDirect := false
	hasBlock := false
	for _, ob := range cfg.Outbounds {
		if t, ok := ob["type"].(string); ok {
			if t == "direct" {
				hasDirect = true
			} else if t == "block" {
				hasBlock = true
			}
		}
	}
	if !hasDirect || !hasBlock {
		t.Errorf("expected both direct and block outbounds, got %v", cfg.Outbounds)
	}

	// 验证 SOCKS 入站配置生成
	_, err = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"socks-anon", "socks", "127.0.0.1", 1080, "{}", 1)
	if err != nil {
		t.Fatalf("insert socks-anon failed: %v", err)
	}
	_, err = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"socks-auth", "socks", "127.0.0.1", 1081, `{"username":"testuser","password":"testpass"}`, 1)
	if err != nil {
		t.Fatalf("insert socks-auth failed: %v", err)
	}

	cfgWithSocks, err := GenerateServerConfig()
	if err != nil {
		t.Fatalf("GenerateServerConfig with socks failed: %v", err)
	}

	var foundAnon, foundAuth bool
	for _, ib := range cfgWithSocks.Inbounds {
		if ib.Tag == "socks-anon" {
			foundAnon = true
			if ib.Type != "socks" {
				t.Errorf("expected type socks, got %s", ib.Type)
			}
			if len(ib.Users) != 0 {
				t.Errorf("expected 0 users for anon socks, got %d", len(ib.Users))
			}
		}
		if ib.Tag == "socks-auth" {
			foundAuth = true
			if ib.Type != "socks" {
				t.Errorf("expected type socks, got %s", ib.Type)
			}
			if len(ib.Users) != 1 || ib.Users[0].Username != "testuser" || ib.Users[0].Password != "testpass" {
				t.Errorf("expected user testuser/testpass, got %+v", ib.Users)
			}
		}
	}
	if !foundAnon || !foundAuth {
		t.Errorf("expected both socks-anon and socks-auth in inbounds, got %+v", cfgWithSocks.Inbounds)
	}

	// 验证拓展协议入站生成 (Shadowsocks, Trojan, Hysteria 2, VMess, TUIC, Mixed)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"ss-test", "shadowsocks", "::", 8388, `{"method":"aes-256-gcm","password":"sspassword"}`, 1)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"trojan-test", "trojan", "::", 10443, `{"password":"trojanpass","server_name":"example.com"}`, 1)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"hy2-test", "hysteria2", "::", 8443, `{"password":"hy2pass","server_name":"example.com","up_mbps":100,"down_mbps":200}`, 1)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"vmess-test", "vmess", "127.0.0.1", 10087, `{"network":"ws","path":"/vmess"}`, 1)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"tuic-test", "tuic", "::", 8444, `{"password":"tuicpass","congestion_control":"bbr","server_name":"example.com"}`, 1)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"mixed-test", "mixed", "127.0.0.1", 2080, `{"username":"mixeduser","password":"mixedpass"}`, 1)

	fullCfg, err := GenerateServerConfig()
	if err != nil {
		t.Fatalf("GenerateServerConfig with all protocols failed: %v", err)
	}

	tagsFound := make(map[string]bool)
	for _, ib := range fullCfg.Inbounds {
		tagsFound[ib.Tag] = true
		switch ib.Tag {
		case "ss-test":
			if ib.Type != "shadowsocks" || ib.Method != "aes-256-gcm" || ib.Password != "sspassword" {
				t.Errorf("unexpected ss inbound: %+v", ib)
			}
		case "trojan-test":
			if ib.Type != "trojan" || ib.TLS == nil || ib.TLS.ServerName != "example.com" {
				t.Errorf("unexpected trojan inbound: %+v", ib)
			}
		case "hy2-test":
			if ib.Type != "hysteria2" || ib.UpMbps != 100 || ib.DownMbps != 200 {
				t.Errorf("unexpected hy2 inbound: %+v", ib)
			}
		case "vmess-test":
			if ib.Type != "vmess" || ib.Transport == nil || ib.Transport["path"] != "/vmess" {
				t.Errorf("unexpected vmess inbound: %+v", ib)
			}
		case "tuic-test":
			if ib.Type != "tuic" || ib.CongestionControl != "bbr" {
				t.Errorf("unexpected tuic inbound: %+v", ib)
			}
		case "mixed-test":
			if ib.Type != "mixed" || len(ib.Users) != 1 {
				t.Errorf("unexpected mixed inbound: %+v", ib)
			}
		}
	}

	expectedTags := []string{"ss-test", "trojan-test", "hy2-test", "vmess-test", "tuic-test", "mixed-test"}
	for _, tag := range expectedTags {
		if !tagsFound[tag] {
			t.Errorf("expected inbound tag %s was not found in config", tag)
		}
	}
}
