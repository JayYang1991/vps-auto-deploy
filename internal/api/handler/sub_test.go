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

func TestIsAllowedRemoteConfigHost(t *testing.T) {
	allowedURLs := []string{
		"https://raw.githubusercontent.com/JayYang1991/ACL4SSR/refs/heads/main/Clash/config/ACL4SSR_Online_Bespoke.ini",
		"https://gist.githubusercontent.com/user/12345/raw/config.ini",
		"https://cdn.jsdelivr.net/gh/user/repo/rules.ini",
		"https://fastly.jsdelivr.net/npm/rules/config.ini",
		"https://testingcf.jsdelivr.net/rules.ini",
		"https://ghproxy.net/https://raw.githubusercontent.com/rules.ini",
		"https://mirror.ghproxy.com/https://raw.githubusercontent.com/rules.ini",
	}
	for _, u := range allowedURLs {
		if !isAllowedRemoteConfigHost(u) {
			t.Errorf("expected %s to be allowed remote config host", u)
		}
	}

	disallowedURLs := []string{
		"http://evil-domain.com/malicious_rules.ini",
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:25500/version",
		"https://attacker.io/redirect-to-internal",
		"http://sub.domain.attacker.com/config.ini",
	}
	for _, u := range disallowedURLs {
		if isAllowedRemoteConfigHost(u) {
			t.Errorf("expected %s to be disallowed remote config host", u)
		}
	}
}

