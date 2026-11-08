package configgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"vps-auto-deploy/internal/model"
)

func setupRelayTestDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "relay_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
}

func TestBuildRelayOutbound(t *testing.T) {
	setupRelayTestDB(t)

	targetNode := &model.Node{
		ID:   2,
		Name: "JP-Tokyo",
		Host: "192.168.100.20",
	}

	// 1. 测试 VLESS-Reality 中转生成
	realitySpec := model.VLESSRealitySpec{
		Flow:       "xtls-rprx-vision",
		ServerName: "dl.google.com",
		PublicKey:  "mockPublicKey1234567890=",
		ShortID:    "6ba855ea",
	}
	specBytes, _ := json.Marshal(realitySpec)

	ibReality := &model.Inbound{
		ID:       10,
		Tag:      "vless-in-443",
		Protocol: "vless-reality",
		Port:     443,
		SpecJSON: string(specBytes),
	}

	ob, err := BuildRelayOutbound(targetNode, ibReality, "", "ip", "test-uuid-1111")
	if err != nil {
		t.Fatalf("BuildRelayOutbound failed: %v", err)
	}

	if ob.Type != "vless" {
		t.Errorf("expected type vless, got %s", ob.Type)
	}
	if ob.Server != "192.168.100.20" || ob.ServerPort != 443 {
		t.Errorf("expected server 192.168.100.20:443, got %s:%d", ob.Server, ob.ServerPort)
	}
	if ob.Tag != "relay_jp-tokyo_vless_reality_443" {
		t.Errorf("expected tag relay_jp-tokyo_vless_reality_443, got %s", ob.Tag)
	}

	var parsedSpec map[string]interface{}
	if err := json.Unmarshal([]byte(ob.SpecJSON), &parsedSpec); err != nil {
		t.Fatalf("failed to parse spec_json: %v", err)
	}
	if parsedSpec["uuid"] != "test-uuid-1111" || parsedSpec["flow"] != "xtls-rprx-vision" {
		t.Errorf("parsedSpec mismatch: %+v", parsedSpec)
	}

	// 2. 测试 VLESS-WS 使用隧道域名生成
	wsSpec := model.VLESSWSSpec{
		Path: "/ws-relay",
		Host: "cf.example.com",
	}
	wsSpecBytes, _ := json.Marshal(wsSpec)
	ibWS := &model.Inbound{
		ID:       11,
		Tag:      "vless-ws-in",
		Protocol: "vless-ws",
		Port:     8080,
		SpecJSON: string(wsSpecBytes),
	}

	obWS, err := BuildRelayOutbound(targetNode, ibWS, "jp.tunnel.com", "tunnel", "test-uuid-2222")
	if err != nil {
		t.Fatalf("BuildRelayOutbound for WS failed: %v", err)
	}
	if obWS.Server != "jp.tunnel.com" || obWS.ServerPort != 443 {
		t.Errorf("expected tunnel server jp.tunnel.com:443, got %s:%d", obWS.Server, obWS.ServerPort)
	}

	// 3. 测试 Hysteria2 中转生成
	hy2Spec := model.Hysteria2InboundSpec{
		Password:   "hy2-secret-pass",
		ServerName: "hy2.example.com",
	}
	hy2SpecBytes, _ := json.Marshal(hy2Spec)
	ibHy2 := &model.Inbound{
		ID:       12,
		Tag:      "hy2-in",
		Protocol: "hysteria2",
		Port:     8443,
		SpecJSON: string(hy2SpecBytes),
	}
	obHy2, err := BuildRelayOutbound(targetNode, ibHy2, "", "ip", "")
	if err != nil {
		t.Fatalf("BuildRelayOutbound for Hy2 failed: %v", err)
	}
	if obHy2.Type != "hysteria2" || obHy2.ServerPort != 8443 {
		t.Errorf("hy2 outbound mismatch: %+v", obHy2)
	}
}

