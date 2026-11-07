package component_test

import (
	"os"
	"path/filepath"
	"testing"

	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/component/cloudflared"
	"vps-auto-deploy/internal/component/singbox"
)

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "sing-box version 1.11.0 (go1.23.0, linux/amd64)",
			expected: "1.11.0",
		},
		{
			input:    "sing-box version 1.14.4\nCore features enabled",
			expected: "1.14.4",
		},
		{
			input:    "cloudflared version 2024.10.0 (built 2024-10-15-1245 UTC)",
			expected: "2024.10.0",
		},
		{
			input:    "cloudflared version 2026.10.7",
			expected: "2026.10.7",
		},
		{
			input:    "v1.2.3",
			expected: "1.2.3",
		},
	}

	for _, tt := range tests {
		got := component.ExtractVersion(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractVersion(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestPreInstalledBinaryDetection(t *testing.T) {
	// 创建临时目录并加入 PATH 模拟预先安装的环境
	tmpDir, err := os.MkdirTemp("", "vps-test-bin-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)
	os.Setenv("PATH", tmpDir+":"+origPath)

	// 模拟已预装在 /usr/bin 等 PATH 路径中的 sing-box
	fakeSingbox := filepath.Join(tmpDir, "sing-box")
	script := `#!/bin/sh
echo "sing-box version 1.12.5 (custom-built)"
`
	if err := os.WriteFile(fakeSingbox, []byte(script), 0755); err != nil {
		t.Fatalf("write fake sing-box: %v", err)
	}

	// 模拟已预装在 PATH 中的 cloudflared
	fakeCloudflared := filepath.Join(tmpDir, "cloudflared")
	cfScript := `#!/bin/sh
echo "cloudflared version 2026.5.1"
`
	if err := os.WriteFile(fakeCloudflared, []byte(cfScript), 0755); err != nil {
		t.Fatalf("write fake cloudflared: %v", err)
	}

	// 测试 sing-box 驱动检测
	sbDriver := singbox.NewDriver()
	sbVer, err := sbDriver.GetInstalledVersion()
	if err != nil {
		t.Fatalf("singbox.GetInstalledVersion() error: %v", err)
	}
	if sbVer != "1.12.5" {
		t.Errorf("expected sing-box version 1.12.5, got %q", sbVer)
	}

	sbStatus, err := sbDriver.Status()
	if err != nil {
		t.Fatalf("singbox.Status() error: %v", err)
	}
	if !sbStatus.Installed {
		t.Errorf("expected sing-box to be reported as Installed=true")
	}

	// 测试 cloudflared 驱动检测
	cfDriver := cloudflared.NewDriver()
	cfVer, err := cfDriver.GetInstalledVersion()
	if err != nil {
		t.Fatalf("cloudflared.GetInstalledVersion() error: %v", err)
	}
	if cfVer != "2026.5.1" {
		t.Errorf("expected cloudflared version 2026.5.1, got %q", cfVer)
	}

	cfStatus, err := cfDriver.Status()
	if err != nil {
		t.Fatalf("cloudflared.Status() error: %v", err)
	}
	if !cfStatus.Installed {
		t.Errorf("expected cloudflared to be reported as Installed=true")
	}
}

func TestDefaultPaths(t *testing.T) {
	installDir := component.GetDefaultInstallDir()
	if installDir == "" {
		t.Fatalf("expected non-empty installDir")
	}
	cfgPath := component.GetDefaultConfigPath("sing-box", "config.json")
	if cfgPath == "" || !filepath.IsAbs(cfgPath) {
		t.Fatalf("expected valid absolute config path, got %q", cfgPath)
	}
}
