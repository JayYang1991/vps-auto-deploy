package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsAllowedSingboxConfigPath_Security(t *testing.T) {
	// 确保默认环境下 /tmp 目录被严格拒绝
	os.Unsetenv("ALLOW_TEST_CONFIG_PATH")

	tmpFile := filepath.Join(os.TempDir(), "malicious.json")
	if isAllowedSingboxConfigPath(tmpFile) {
		t.Fatalf("expected tmp file %s to be rejected in production, but was allowed", tmpFile)
	}

	// 路径穿越测试
	traversal := "/etc/vps-auto-deploy/../shadow.json"
	if isAllowedSingboxConfigPath(traversal) {
		t.Fatalf("expected traversal path %s to be rejected, but was allowed", traversal)
	}

	// 非 json 文件测试
	nonJSON := "/etc/vps-auto-deploy/config.yaml"
	if isAllowedSingboxConfigPath(nonJSON) {
		t.Fatalf("expected non-json file %s to be rejected, but was allowed", nonJSON)
	}

	// 受管路径应允许
	validManaged := "/etc/vps-auto-deploy/config.json"
	if !isAllowedSingboxConfigPath(validManaged) {
		t.Fatalf("expected valid managed path %s to be allowed, but was rejected", validManaged)
	}

	// 开启测试环境变量后允许 /tmp
	os.Setenv("ALLOW_TEST_CONFIG_PATH", "1")
	defer os.Unsetenv("ALLOW_TEST_CONFIG_PATH")
	if !isAllowedSingboxConfigPath(tmpFile) {
		t.Fatalf("expected tmp file %s to be allowed when ALLOW_TEST_CONFIG_PATH=1, but was rejected", tmpFile)
	}
}

func TestIsValidListenAddress(t *testing.T) {
	valid := []string{"", "::", "0.0.0.0", "127.0.0.1", "localhost", "192.168.1.1", "2001:db8::1"}
	for _, v := range valid {
		if !isValidListenAddress(v) {
			t.Errorf("expected %q to be valid listen address, but got invalid", v)
		}
	}

	invalid := []string{"127.0.0.1; rm -rf /", "invalid_ip", "999.999.999.999", "1.2.3.4.5", "http://127.0.0.1"}
	for _, inv := range invalid {
		if isValidListenAddress(inv) {
			t.Errorf("expected %q to be invalid listen address, but got valid", inv)
		}
	}
}

func TestValidateIPCIDRAndPorts(t *testing.T) {
	validCIDRs := []string{"", "192.168.1.0/24", "10.0.0.0/8, 172.16.0.0/12", "1.1.1.1", "2001:db8::/32"}
	for _, c := range validCIDRs {
		if err := validateIPCIDR(c); err != nil {
			t.Errorf("expected %q to be valid CIDR, but got error: %v", c, err)
		}
	}

	invalidCIDRs := []string{"1.2.3.4/99", "invalid", "10.0.0.1; DROP TABLE", "999.999.999.999/24"}
	for _, c := range invalidCIDRs {
		if err := validateIPCIDR(c); err == nil {
			t.Errorf("expected %q to be invalid CIDR, but got nil error", c)
		}
	}

	validPorts := []string{"", "80", "443, 8080", "1, 65535"}
	for _, p := range validPorts {
		if err := validatePorts(p); err != nil {
			t.Errorf("expected %q to be valid port list, but got error: %v", p, err)
		}
	}

	invalidPorts := []string{"0", "-1", "65536", "abc", "80; rm"}
	for _, p := range invalidPorts {
		if err := validatePorts(p); err == nil {
			t.Errorf("expected %q to be invalid port list, but got nil error", p)
		}
	}
}

func TestMaskToken(t *testing.T) {
	if maskToken("short") != "******" {
		t.Fatalf("expected ****** for short token, got %s", maskToken("short"))
	}
	longToken := "eyJhGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abcdef"
	masked := maskToken(longToken)
	if !strings.HasPrefix(masked, "eyJh...") || !strings.HasSuffix(masked, "cdef") {
		t.Fatalf("unexpected masked token format: %s", masked)
	}
}
