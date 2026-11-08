package netutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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

// ValidatePublicURL 验证 URL 是否为合法的公网 Web 地址，拦截私网/内网/回环/元数据等 SSRF 敏感地址
func ValidatePublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("无效的 URL 格式: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("仅支持 HTTP 或 HTTPS 协议")
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL 缺少主机名")
	}

	// 检查是否为常见敏感主机名
	lowerHost := strings.ToLower(hostname)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") || strings.HasSuffix(lowerHost, ".local") {
		return fmt.Errorf("禁止访问本地回环或局域网主机名: %s", hostname)
	}

	// 解析 IP 地址
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// 如果是直接填写的 IP 格式
		if ip := net.ParseIP(hostname); ip != nil {
			ips = []net.IP{ip}
		} else {
			return fmt.Errorf("无法解析目标主机名: %w", err)
		}
	}

	for _, ip := range ips {
		if ip.IsLoopback() {
			return fmt.Errorf("【SSRF拦截】禁止访问回环地址 (127.0.0.0/8, ::1)")
		}
		if ip.IsPrivate() {
			return fmt.Errorf("【SSRF拦截】禁止访问局域网/私有地址 (RFC 1918 / RFC 4193)")
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("【SSRF拦截】禁止访问链路本地/云元数据地址 (169.254.0.0/16)")
		}
		if ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("【SSRF拦截】禁止访问未指定或多播地址")
		}
	}

	return nil
}


