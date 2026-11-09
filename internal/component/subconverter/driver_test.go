package subconverter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestDriverMetadata(t *testing.T) {
	d := NewDriver()
	if d.Name() != "subconverter" {
		t.Fatalf("expected name 'subconverter', got '%s'", d.Name())
	}
	if d.Category() != "converter" {
		t.Fatalf("expected category 'converter', got '%s'", d.Category())
	}

	st, err := d.Status()
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if st.CategoryLabel != "订阅与规则" {
		t.Fatalf("expected CategoryLabel '订阅与规则', got '%s'", st.CategoryLabel)
	}
	if st.Order != 3 {
		t.Fatalf("expected Order 3, got %d", st.Order)
	}
}

func TestExtractTarGzToDir(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	destDir := filepath.Join(tmpDir, "extracted")

	// 构造包含 subconverter/subconverter 和 subconverter/pref.example.ini 的测试压缩包
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	files := []struct {
		name string
		body string
	}{
		{"subconverter/subconverter", "#!/bin/sh\necho ok"},
		{"subconverter/pref.example.ini", "[common]\nlisten=0.0.0.0\nport=25500\n"},
		{"subconverter/config/ACL4SSR_Online.ini", "[custom]\nrule=direct\n"},
	}

	for _, f := range files {
		hdr := &tar.Header{
			Name: f.name,
			Mode: 0755,
			Size: int64(len(f.body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	tw.Close()
	gw.Close()

	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write test archive: %v", err)
	}

	if err := extractTarGzToDir(archivePath, destDir); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	// 验证 subconverter 根前缀已被自动剥离
	binFile := filepath.Join(destDir, "subconverter")
	if _, err := os.Stat(binFile); err != nil {
		t.Fatalf("expected subconverter extracted at root of destDir: %v", err)
	}

	configFile := filepath.Join(destDir, "config", "ACL4SSR_Online.ini")
	if _, err := os.Stat(configFile); err != nil {
		t.Fatalf("expected config/ACL4SSR_Online.ini extracted: %v", err)
	}

	// 验证已有用户 pref.ini 与 custom_rules.ini 受到保护不被覆盖
	userPrefPath := filepath.Join(destDir, "pref.ini")
	_ = os.WriteFile(userPrefPath, []byte("[common]\nexclude_remarks=socks-in\n"), 0644)
	userRulesPath := filepath.Join(destDir, "config", "custom_rules.ini")
	_ = os.WriteFile(userRulesPath, []byte("[custom]\nmy_user_rule=true\n"), 0644)

	// 构造包含默认 pref.ini 和 custom_rules.ini 的新版本发布包
	var buf2 bytes.Buffer
	gw2 := gzip.NewWriter(&buf2)
	tw2 := tar.NewWriter(gw2)
	files2 := []struct {
		name string
		body string
	}{
		{"subconverter/pref.ini", "[common]\nlisten=0.0.0.0\n"},
		{"subconverter/config/custom_rules.ini", "[custom]\ndefault_release_rule=true\n"},
	}
	for _, f := range files2 {
		hdr := &tar.Header{Name: f.name, Mode: 0644, Size: int64(len(f.body))}
		_ = tw2.WriteHeader(hdr)
		_, _ = tw2.Write([]byte(f.body))
	}
	tw2.Close()
	gw2.Close()
	archivePath2 := filepath.Join(tmpDir, "upgrade.tar.gz")
	_ = os.WriteFile(archivePath2, buf2.Bytes(), 0644)

	// 执行升级解压
	if err := extractTarGzToDir(archivePath2, destDir); err != nil {
		t.Fatalf("upgrade extract failed: %v", err)
	}

	// 确认用户的 pref.ini 与 custom_rules.ini 依然保留
	prefData, _ := os.ReadFile(userPrefPath)
	if string(prefData) != "[common]\nexclude_remarks=socks-in\n" {
		t.Fatalf("expected user pref.ini preserved, got: %s", string(prefData))
	}
	rulesData, _ := os.ReadFile(userRulesPath)
	if string(rulesData) != "[custom]\nmy_user_rule=true\n" {
		t.Fatalf("expected user custom_rules.ini preserved, got: %s", string(rulesData))
	}
}

func TestIniKeyHelpers(t *testing.T) {
	ini := "[common]\nlisten=127.0.0.1\nport=25500\n"
	if GetSettingFromContent(ini, "listen") != "127.0.0.1" {
		t.Fatalf("expected listen=127.0.0.1")
	}

	updated := UpdateIniKey(ini, "exclude_remarks", "socks-in")
	if GetSettingFromContent(updated, "exclude_remarks") != "socks-in" {
		t.Fatalf("expected exclude_remarks=socks-in in updated ini: %s", updated)
	}

	// Update existing key
	updated2 := UpdateIniKey(updated, "port", "25501")
	if GetSettingFromContent(updated2, "port") != "25501" {
		t.Fatalf("expected port=25501, got: %s", updated2)
	}
}

