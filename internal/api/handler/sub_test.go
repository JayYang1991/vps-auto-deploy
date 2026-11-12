package handler

import (
	"strings"
	"testing"
)

func TestBuildAttachmentHeader(t *testing.T) {
	// 1. 常规英文文件名
	h1 := buildAttachmentHeader("sub-test", ".txt")
	if !strings.Contains(h1, `filename="sub-test.txt"`) {
		t.Fatalf("expected valid ASCII filename, got %s", h1)
	}

	// 2. 包含 CRLF 标头注入与特殊字符的文件名
	malicious := "user\r\nSet-Cookie: evil=1\r\n"
	h2 := buildAttachmentHeader(malicious, ".txt")
	if strings.Contains(h2, "\r") || strings.Contains(h2, "\n") {
		t.Fatalf("CRLF must be stripped/escaped from Content-Disposition header, got %q", h2)
	}
	if strings.Contains(h2, "Set-Cookie") && !strings.Contains(h2, "Set-Cookie_") {
		// Should be sanitized as safe ASCII
		t.Logf("Sanitized header: %s", h2)
	}

	// 3. 包含中文等 Unicode 字符的文件名 (符合 RFC 6266 / RFC 5987)
	h3 := buildAttachmentHeader("sub-张三-节点", ".yaml")
	if !strings.Contains(h3, "filename*=") {
		t.Fatalf("expected filename*= for unicode filename, got %s", h3)
	}
	if !strings.Contains(h3, "UTF-8''") {
		t.Fatalf("expected UTF-8'' prefix in filename*, got %s", h3)
	}
}

