package netutil

import (
	"testing"
)

func TestValidatePublicURL(t *testing.T) {
	// 1. 合法公网地址应该通过
	validURLs := []string{
		"https://raw.githubusercontent.com/JayYang1991/ACL4SSR/refs/heads/main/Clash/config/ACL4SSR_Online_Bespoke.ini",
		"http://1.1.1.1/test",
		"https://8.8.8.8/test",
	}
	for _, u := range validURLs {
		if err := ValidatePublicURL(u); err != nil {
			t.Errorf("Expected valid for %s, got: %v", u, err)
		}
	}

	// 2. 内网、回环、元数据、非法协议、CGNAT、保留段应该被拦截
	invalidURLs := []string{
		"http://127.0.0.1:20888/api",
		"http://localhost:25500/sub",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1:8080/admin",
		"http://192.168.1.1/",
		"http://172.16.0.1/status",
		"http://100.64.0.1/cgnat",
		"http://198.18.0.1/benchmark",
		"http://0.0.0.0/test",
		"http://[::1]/test",
		"http://[fc00::1]/test",
		"http://[fe80::1]/test",
		"http://[::ffff:127.0.0.1]/test",
		"ftp://example.com/test",
		"file:///etc/passwd",
		"://bad-url",
	}
	for _, u := range invalidURLs {
		if err := ValidatePublicURL(u); err == nil {
			t.Errorf("Expected SSRF block for %s, but passed!", u)
		}
	}
}

