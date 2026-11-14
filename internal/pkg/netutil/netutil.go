package netutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
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
			body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
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

var (
	blockedCIDRs  []*net.IPNet
	benchmarkCIDR *net.IPNet
)

func init() {
	_, benchmarkCIDR, _ = net.ParseCIDR("198.18.0.0/15")

	rawCIDRs := []string{
		"0.0.0.0/8",          // Current network (RFC 1122)
		"10.0.0.0/8",         // Private network (RFC 1918)
		"100.64.0.0/10",      // Shared Address Space / CGNAT (RFC 6598)
		"127.0.0.0/8",        // Loopback (RFC 1122)
		"169.254.0.0/16",     // Link-Local / Cloud Metadata (RFC 3927)
		"172.16.0.0/12",      // Private network (RFC 1918)
		"192.0.0.0/24",       // IETF Protocol Assignments (RFC 6890)
		"192.0.2.0/24",       // Documentation TEST-NET-1 (RFC 5737)
		"192.168.0.0/16",     // Private network (RFC 1918)
		"198.18.0.0/15",      // Benchmarking (RFC 2544)
		"198.51.100.0/24",    // Documentation TEST-NET-2 (RFC 5737)
		"203.0.113.0/24",     // Documentation TEST-NET-3 (RFC 5737)
		"224.0.0.0/4",        // Multicast (RFC 5771)
		"240.0.0.0/4",        // Reserved / Class E (RFC 1112)
		"255.255.255.255/32", // Limited Broadcast (RFC 919)
		"::/128",             // Unspecified IPv6
		"::1/128",            // Loopback IPv6
		"100::/64",           // Discard-Only Address Block (RFC 6666)
		"2001:db8::/32",      // Documentation IPv6 (RFC 3849)
		"fc00::/7",           // Unique Local Address / ULA (RFC 4193)
		"fe80::/10",          // Link-Local IPv6 (RFC 4291)
		"ff00::/8",           // Multicast IPv6 (RFC 4291)
	}
	for _, cidr := range rawCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			blockedCIDRs = append(blockedCIDRs, ipNet)
		}
	}
}

// IsDisallowedIP 判断 IP 是否属于内网、环回、多播、链路本地或受限地址 (SSRF 防御)
func IsDisallowedIP(ip net.IP) bool {
	if os.Getenv("ALLOW_PRIVATE_IP") == "1" {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// IsSafePublicIP 检查单个 IP 是否为安全的公网 IP (拦截回环、私网、云元数据及受限保留地址)
func IsSafePublicIP(ip net.IP, isLiteralIP bool) error {
	if os.Getenv("ALLOW_PRIVATE_IP") == "1" {
		return nil
	}

	// IPv4-mapped IPv6 规整化 (例如 ::ffff:127.0.0.1 规整为 127.0.0.1)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	if ip.IsLoopback() {
		return fmt.Errorf("【SSRF拦截】禁止访问回环地址 (127.0.0.0/8, ::1)")
	}
	if ip.IsPrivate() {
		return fmt.Errorf("【SSRF拦截】禁止访问局域网/私有地址 (RFC 1918 / RFC 4193)")
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("【SSRF拦截】禁止访问链路本地/云元数据地址 (169.254.0.0/16, fe80::/10)")
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("【SSRF拦截】禁止访问未指定或多播地址")
	}

	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			// 针对 198.18.0.0/15 (RFC 2544)，在开发或生产环境使用透明代理 (如 Clash/sing-box Fake-IP 模式) 时，
			// DNS 会将所有公网域名解析为 198.18.x.x。
			// 为保障透明代理环境兼容性，仅在输入为显式字面量 IP (isLiteralIP) 时拦截，域名解析出的 Fake-IP 予以豁免。
			if benchmarkCIDR != nil && benchmarkCIDR.String() == cidr.String() && !isLiteralIP {
				continue
			}
			return fmt.Errorf("【SSRF拦截】目标 IP (%s) 命中了受限地址块 (%s)", ip.String(), cidr.String())
		}
	}
	return nil
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
	isLiteralIP := false
	var ips []net.IP
	if parsed := net.ParseIP(hostname); parsed != nil {
		isLiteralIP = true
		ips = []net.IP{parsed}
	} else {
		resolved, err := net.LookupIP(hostname)
		if err != nil {
			return fmt.Errorf("无法解析目标主机名: %w", err)
		}
		ips = resolved
	}

	for _, ip := range ips {
		if err := IsSafePublicIP(ip, isLiteralIP); err != nil {
			return err
		}
	}

	return nil
}

// CreateSafeHTTPTransport 创建原生免疫 DNS 重绑定 (Anti-DNS-Rebinding) 的安全 HTTP Transport：
// 在底层建立 TCP 握手的瞬时原子解析并校验目标 IP，杜绝 TOCTOU 时序差逃逸
func CreateSafeHTTPTransport(timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// 敏感主机名过滤
			lowerHost := strings.ToLower(host)
			if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") || strings.HasSuffix(lowerHost, ".local") {
				return nil, fmt.Errorf("【SSRF拦截】禁止访问本地主机: %s", host)
			}

			isLiteralIP := false
			var ips []net.IP
			if parsed := net.ParseIP(host); parsed != nil {
				isLiteralIP = true
				ips = []net.IP{parsed}
			} else {
				resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
				if err != nil {
					return nil, fmt.Errorf("解析域名失败: %w", err)
				}
				ips = resolved
			}

			if len(ips) == 0 {
				return nil, fmt.Errorf("未找到可用的 IP 地址")
			}

			// 遍历并连接首个经合规性验证通过的公网 IP (消灭 DNS 重绑定窗口)
			var dialErr error
			for _, ip := range ips {
				if err := IsSafePublicIP(ip, isLiteralIP); err != nil {
					return nil, fmt.Errorf("【SSRF DNS Rebinding拦截】%w", err)
				}
				targetAddr := net.JoinHostPort(ip.String(), port)
				conn, err := dialer.DialContext(ctx, network, targetAddr)
				if err == nil {
					return conn, nil
				}
				dialErr = err
			}
			return nil, dialErr
		},
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   10 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
	}
}

// CreateSafeHTTPClient 创建原生抵御 SSRF 与 DNS 重绑定的安全 HTTP 客户端
func CreateSafeHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: CreateSafeHTTPTransport(timeout),
		Timeout:   timeout,
	}
}

// NewSSRFSafeHTTPClient 创建防 SSRF 与防 DNS 重绑定的安全 HTTP Client (集成重定向深度与协议安全约束)
func NewSSRFSafeHTTPClient(timeout time.Duration) *http.Client {
	client := CreateSafeHTTPClient(timeout)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("unsupported redirect scheme: %s", req.URL.Scheme)
		}
		return nil
	}
	return client
}


