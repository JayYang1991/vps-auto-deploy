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
}
