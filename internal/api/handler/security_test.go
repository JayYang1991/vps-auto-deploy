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
