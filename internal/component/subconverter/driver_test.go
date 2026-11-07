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
}

