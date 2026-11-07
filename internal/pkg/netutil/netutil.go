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

// GetPublicIP 并发竞速获取主机的公网 IPv4 地址（首个返回即响应，最长等待 2 秒）
func GetPublicIP() string {
	endpoints := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
		"https://checkip.amazonaws.com",
	}

	resultCh := make(chan string, len(endpoints))
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	for _, ep := range endpoints {
		go func(url string) {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return
			}
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				ip := strings.TrimSpace(string(body))
				if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
					select {
					case resultCh <- ip:
					case <-ctx.Done():
					}
				}
			}
		}(ep)
	}

	select {
	case ip := <-resultCh:
		return ip
	case <-time.After(2 * time.Second):
		return "127.0.0.1"
	}
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

