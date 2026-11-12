package handler

import (
	"os"
	"path/filepath"
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
