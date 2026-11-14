package netutil

import (
	"testing"
)

func TestIsLoopbackAddress(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"127.0.0.1", true},
		{"localhost", true},
		{"LocalHost", true},
		{"::1", true},
		{"", false},
		{"0.0.0.0", false},
		{"::", false},
		{"192.168.1.1", false},
		{"1.1.1.1", false},
	}

	for _, tt := range tests {
		got := IsLoopbackAddress(tt.addr)
		if got != tt.expected {
			t.Errorf("IsLoopbackAddress(%q) = %v; want %v", tt.addr, got, tt.expected)
		}
	}
}

func TestAllowPortValidation(t *testing.T) {
	// 边界与非法端口测试
	if err := AllowPort(0, "tcp"); err == nil {
		t.Errorf("AllowPort(0) expected error, got nil")
	}
	if err := AllowPort(-10, "tcp"); err == nil {
		t.Errorf("AllowPort(-10) expected error, got nil")
	}
	if err := AllowPort(70000, "tcp"); err == nil {
		t.Errorf("AllowPort(70000) expected error, got nil")
	}

	// 合法端口测试（在非 root 或无权限环境下应安全跳过并返回 nil）
	if err := AllowPort(443, "tcp"); err != nil {
		t.Errorf("AllowPort(443) unexpected error: %v", err)
	}

	// 本地回环跳过测试
	if err := AllowInboundPort(8080, "vless", "127.0.0.1"); err != nil {
		t.Errorf("AllowInboundPort for loopback unexpected error: %v", err)
	}

	// 批量放行测试
	EnsurePortsFirewall([]InboundPortInfo{
		{Port: 8080, Protocol: "http", Listen: "127.0.0.1"},
		{Port: 8443, Protocol: "vless-reality", Listen: "0.0.0.0"},
		{Port: 20443, Protocol: "hysteria2", Listen: "::"},
	})

	// AllowFirewallPort 测试
	if err := AllowFirewallPort(nil, 80, "invalid-proto"); err == nil {
		t.Errorf("AllowFirewallPort with invalid protocol expected error, got nil")
	}
	if err := AllowFirewallPort(nil, 80, "tcp"); err != nil {
		t.Errorf("AllowFirewallPort with tcp unexpected error: %v", err)
	}
}

