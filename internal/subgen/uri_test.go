package subgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vps-auto-deploy/internal/model"
)

func TestClusterPrefixSubgen(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "subgen_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	_ = model.SetSetting("public_ip", "114.114.114.114")

	// Update master node prefix
	err = model.UpdateNodePrefix(1, "🏠 本机 (Master)", "🇭🇰 香港主控")
	if err != nil {
		t.Fatalf("UpdateNodePrefix failed: %v", err)
	}

	// Insert test inbound for master node
	_, err = model.DB.Exec(
		"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"vless-vision",
		"vless-reality",
		"0.0.0.0",
		443,
		`{"server_name":"dl.google.com","public_key":"mock-pub-key","short_id":"mock-sid","flow":"xtls-rprx-vision"}`,
	)
	if err != nil {
		t.Fatalf("Failed to insert inbound: %v", err)
	}

	member := &model.Member{
		ID:       1,
		Name:     "TestUser",
		UUID:     "12345678-1234-1234-1234-123456789abc",
		SubToken: "token123",
		Enabled:  true,
	}

	// 1. Test GenerateURIs
	uris, err := GenerateURIs(member)
	if err != nil {
		t.Fatalf("GenerateURIs failed: %v", err)
	}
	if len(uris) != 1 {
		t.Fatalf("Expected 1 URI, got %d", len(uris))
	}
	// Verify that URI fragment contains escaped prefix "🇭🇰 香港主控"
	expectedPart := "%F0%9F%87%AD%F0%9F%87%B0" // URL-encoded HK flag
	if !strings.Contains(uris[0], expectedPart) {
		t.Fatalf("URI should contain encoded HK flag prefix, got: %s", uris[0])
	}

	// 2. Test Clash Meta config
	clashYaml, err := GenerateClashMetaConfig(member)
	if err != nil {
		t.Fatalf("GenerateClashMetaConfig failed: %v", err)
	}
	if !strings.Contains(clashYaml, "香港主控 | vless-vision-443") {
		t.Fatalf("Clash YAML should contain customized node name, got:\n%s", clashYaml)
	}

	// 3. Test Singbox client config
	singboxJSON, err := GenerateSingboxClientConfig(member)
	if err != nil {
		t.Fatalf("GenerateSingboxClientConfig failed: %v", err)
	}
	if !strings.Contains(singboxJSON, "🇭🇰 香港主控 | vless-vision-443") {
		t.Fatalf("Singbox JSON should contain customized node tag, got:\n%s", singboxJSON)
	}

	// 4. Test SOCKS inbound support
	_, err = model.DB.Exec(
		"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"socks-local",
		"socks",
		"127.0.0.1",
		1080,
		`{"username":"myuser","password":"mypassword"}`,
	)
	if err != nil {
		t.Fatalf("Failed to insert socks inbound: %v", err)
	}

	socksURIs, err := GenerateURIs(member)
	if err != nil {
		t.Fatalf("GenerateURIs with socks failed: %v", err)
	}
	var foundSocksURI bool
	for _, u := range socksURIs {
		if strings.HasPrefix(u, "socks5://myuser:mypassword@") && strings.Contains(u, "socks-local-1080") {
			foundSocksURI = true
			break
		}
	}
	if !foundSocksURI {
		t.Fatalf("expected socks5 URI with auth in %v", socksURIs)
	}

	socksClash, err := GenerateClashMetaConfig(member)
	if err != nil {
		t.Fatalf("GenerateClashMetaConfig with socks failed: %v", err)
	}
	if !strings.Contains(socksClash, "type: socks5") || !strings.Contains(socksClash, "username: myuser") {
		t.Fatalf("expected socks5 in Clash Meta config: %s", socksClash)
	}

	socksSingbox, err := GenerateSingboxClientConfig(member)
	if err != nil {
		t.Fatalf("GenerateSingboxClientConfig with socks failed: %v", err)
	}
	if !strings.Contains(socksSingbox, `"type": "socks"`) || !strings.Contains(socksSingbox, `"username": "myuser"`) {
		t.Fatalf("expected socks in Singbox client config: %s", socksSingbox)
	}

	// 5. Test Extended Protocols (Shadowsocks, Trojan, Hysteria2, VMess, TUIC)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"ss-node", "shadowsocks", "::", 8388, `{"method":"aes-256-gcm","password":"secretpassword"}`)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"trojan-node", "trojan", "::", 4433, `{"password":"trojanpass","server_name":"example.com"}`)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"hy2-node", "hysteria2", "::", 8443, `{"password":"hy2pass","server_name":"example.com"}`)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"vmess-node", "vmess", "::", 10087, `{"network":"ws","path":"/vmess"}`)
	_, _ = model.DB.Exec("INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"tuic-node", "tuic", "::", 8444, `{"password":"tuicpass","congestion_control":"bbr","server_name":"example.com"}`)

	extURIs, err := GenerateURIs(member)
	if err != nil {
		t.Fatalf("GenerateURIs with extended protocols failed: %v", err)
	}

	uriMap := make(map[string]bool)
	for _, u := range extURIs {
		scheme := strings.Split(u, "://")[0]
		uriMap[scheme] = true
	}

	for _, s := range []string{"vless", "socks5", "ss", "trojan", "hysteria2", "vmess", "tuic"} {
		if !uriMap[s] {
			t.Errorf("expected URI scheme %s not found in generated URIs: %v", s, extURIs)
		}
	}

	extClash, err := GenerateClashMetaConfig(member)
	if err != nil {
		t.Fatalf("GenerateClashMetaConfig failed: %v", err)
	}
	for _, typeStr := range []string{"type: ss", "type: trojan", "type: hysteria2", "type: vmess", "type: tuic"} {
		if !strings.Contains(extClash, typeStr) {
			t.Errorf("expected %s in Clash config: %s", typeStr, extClash)
		}
	}

	extSingbox, err := GenerateSingboxClientConfig(member)
	if err != nil {
		t.Fatalf("GenerateSingboxClientConfig failed: %v", err)
	}
	for _, typeStr := range []string{`"type": "shadowsocks"`, `"type": "trojan"`, `"type": "hysteria2"`, `"type": "vmess"`, `"type": "tuic"`} {
		if !strings.Contains(extSingbox, typeStr) {
			t.Errorf("expected %s in Singbox config: %s", typeStr, extSingbox)
		}
	}

	// 6. Test Subconverter Adapter: SOCKS5 / HTTP URL transformation
	rawSocksNoAuth := "socks5://1.2.3.4:10086#US+%E7%BE%8E%E5%9B%BD%E8%8A%82%E7%82%B9+%7C+socks-in-10086"
	adaptedNoAuth := AdaptSingleURIForSubconverter(rawSocksNoAuth)
	if !strings.HasPrefix(adaptedNoAuth, "tg://socks?") {
		t.Fatalf("expected tg://socks? prefix, got: %s", adaptedNoAuth)
	}
	if !strings.Contains(adaptedNoAuth, "server=1.2.3.4") || !strings.Contains(adaptedNoAuth, "port=10086") || !strings.Contains(adaptedNoAuth, "remark=") {
		t.Fatalf("expected server, port, and remark in adapted URL: %s", adaptedNoAuth)
	}

	rawSocksAuth := "socks5://myuser:mypassword@1.2.3.4:1080#my-node"
	adaptedAuth := AdaptSingleURIForSubconverter(rawSocksAuth)
	if !strings.HasPrefix(adaptedAuth, "tg://socks?") || !strings.Contains(adaptedAuth, "user=myuser") || !strings.Contains(adaptedAuth, "pass=mypassword") {
		t.Fatalf("expected tg://socks with auth, got: %s", adaptedAuth)
	}

	rawHTTP := "http://admin:pwd@1.2.3.4:8080#http-node"
	adaptedHTTP := AdaptSingleURIForSubconverter(rawHTTP)
	if !strings.HasPrefix(adaptedHTTP, "tg://http?") || !strings.Contains(adaptedHTTP, "server=1.2.3.4") || !strings.Contains(adaptedHTTP, "port=8080") {
		t.Fatalf("expected tg://http, got: %s", adaptedHTTP)
	}

	rawVless := "vless://uuid@1.2.3.4:443?security=reality#my-vless"
	if AdaptSingleURIForSubconverter(rawVless) != rawVless {
		t.Fatalf("vless URL should remain unchanged")
	}

	// Test multi-node joined string adaptation
	multiRaw := rawSocksNoAuth + "|" + rawVless + "|" + rawSocksAuth
	adaptedMulti := AdaptSourceURLsForSubconverter(multiRaw)
	parts := strings.Split(adaptedMulti, "|")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got: %d", len(parts))
	}
	if !strings.HasPrefix(parts[0], "tg://socks?") || parts[1] != rawVless || !strings.HasPrefix(parts[2], "tg://socks?") {
		t.Fatalf("unexpected adapted multi string: %s", adaptedMulti)
	}
}
