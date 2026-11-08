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
}
