package netutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// GetPublicIP 获取主机的公网 IPv4
func GetPublicIP() string {
	endpoints := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
		"https://checkip.amazonaws.com",
	}

	client := &http.Client{
		Timeout: 4 * time.Second,
	}

	for _, ep := range endpoints {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		req, err := http.NewRequestWithContext(ctx, "GET", ep, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()

		if err == nil {
			ip := strings.TrimSpace(string(body))
			if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
				return ip
			}
		}
	}
	return "127.0.0.1"
}

// IsPortAvailable 检测端口是否可用
func IsPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

