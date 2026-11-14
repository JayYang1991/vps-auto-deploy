package cfpref

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
)

const (
	DefaultSubURL    = "https://sub.19910417.xyz/sub?host={host}&uuid={uuid}"
	DefaultUserAgent = "v2rayN/edgetunnel (https://github.com/cmliu/edgetunnel)"
)

var skipSSRFValidationForTest = false

type FetchResult struct {
	Count     int       `json:"count"`
	FetchedAt time.Time `json:"fetched_at"`
	Host      string    `json:"host"`
	UUID      string    `json:"uuid"`
}

// GetEffectiveConfig 获取当前优选 IP 的运行参数与默认值
func GetEffectiveConfig() (subURL, host, uuidStr, path string) {
	subURL, _ = model.GetSetting("cf_pref_sub_url")
	if strings.TrimSpace(subURL) == "" {
		subURL = DefaultSubURL
	}

	host, _ = model.GetSetting("cf_pref_host")
	host = strings.TrimSpace(host)

	uuidStr, _ = model.GetSetting("cf_pref_uuid")
	uuidStr = strings.TrimSpace(uuidStr)

	path, _ = model.GetSetting("cf_pref_path")
	if strings.TrimSpace(path) == "" {
		path = "/singbox-ws-path"
	}

	// 缺省的 host 与 uuid：严防将真实的成员 UUID 与私有域名泄露给外部第三方测速源
	if host == "" {
		host = "cloudflare.com"
	}

	if uuidStr == "" {
		uuidStr = "00000000-0000-0000-0000-000000000000"
	}

	return subURL, host, uuidStr, path
}

// FetchAndSavePreferredIPs 从目标 EDGETUNNEL_SUB_URL 获取优选 IP 列表并持久化保存
func FetchAndSavePreferredIPs() (*FetchResult, error) {
	return FetchAndSavePreferredIPsWithConfig("", "", "", false)
}

// FetchAndSavePreferredIPsWithConfig 支持动态传入订阅源 URL 与节点参数，并可选择是否自动持久化到配置表中
func FetchAndSavePreferredIPsWithConfig(customSubURL, customHost, customUUID string, autoSave bool) (*FetchResult, error) {
	subURLTpl, host, uuidStr, path := GetEffectiveConfig()

	if strings.TrimSpace(customSubURL) != "" {
		subURLTpl = strings.TrimSpace(customSubURL)
		if autoSave {
			_ = model.SetSetting("cf_pref_sub_url", subURLTpl)
		}
	}
	if strings.TrimSpace(customHost) != "" {
		host = strings.TrimSpace(customHost)
		if autoSave {
			_ = model.SetSetting("cf_pref_host", host)
		}
	}
	if strings.TrimSpace(customUUID) != "" {
		uuidStr = strings.TrimSpace(customUUID)
		if autoSave {
			_ = model.SetSetting("cf_pref_uuid", uuidStr)
		}
	}

	reqURL := subURLTpl
	// 智能 URL 补全：若包含 /sub 且未声明 host= 查询参数或占位符，自动补全占位符
	if strings.Contains(reqURL, "/sub") && !strings.Contains(reqURL, "{host}") && !strings.Contains(reqURL, "host=") {
		if strings.Contains(reqURL, "?") {
			reqURL += "&host={host}&uuid={uuid}"
		} else {
			reqURL += "?host={host}&uuid={uuid}"
		}
	}

	reqURL = strings.ReplaceAll(reqURL, "{host}", url.QueryEscape(host))
	reqURL = strings.ReplaceAll(reqURL, "{uuid}", url.QueryEscape(uuidStr))

	// SSRF 防御拦截：校验是否为合法的公网地址
	if !skipSSRFValidationForTest {
		if err := netutil.ValidatePublicURL(reqURL); err != nil {
			recordFailure(fmt.Sprintf("优选 IP 订阅源 URL 安全拦截: %v", err))
			return nil, fmt.Errorf("SSRF 校验未通过: %w", err)
		}
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		recordFailure(fmt.Sprintf("构建请求失败: %v", err))
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	var client *http.Client
	if skipSSRFValidationForTest {
		client = &http.Client{Timeout: 15 * time.Second}
	} else {
		client = netutil.CreateSafeHTTPClient(15 * time.Second)
	}
	resp, err := client.Do(req)
	if err != nil {
		recordFailure(fmt.Sprintf("连接远端订阅源超时或失败: %v", err))
		return nil, fmt.Errorf("fetch preferred ips failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("远端订阅源返回 HTTP %d", resp.StatusCode)
		recordFailure(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	// 限制最大读取 10MB，防范外部恶意投毒或 OOM 炸弹
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		recordFailure(fmt.Sprintf("读取响应失败: %v", err))
		return nil, err
	}

	rawText := strings.TrimSpace(string(bodyBytes))
	parsedList := parsePreferredNodes(rawText, host, uuidStr, path)

	if len(parsedList) == 0 {
		recordFailure("未从订阅响应中解析出有效的优选 IP 节点")
		return nil, fmt.Errorf("未从订阅响应中解析出有效的优选 IP 节点 (支持 VLESS/Trojan 订阅及纯 IP/IP:PORT 列表)")
	}

	// 持久化保存到数据库
	if err := model.SaveCFPreferredIPs(parsedList); err != nil {
		recordFailure(fmt.Sprintf("保存到数据库失败: %v", err))
		return nil, fmt.Errorf("save to database failed: %w", err)
	}

	// 记录成功状态与数量
	now := time.Now()
	_ = model.SetSetting("cf_pref_last_fetch_at", now.Format(time.RFC3339))
	_ = model.SetSetting("cf_pref_last_fetch_status", "success")
	_ = model.SetSetting("cf_pref_last_fetch_count", strconv.Itoa(len(parsedList)))

	return &FetchResult{
		Count:     len(parsedList),
		FetchedAt: now,
		Host:      host,
		UUID:      uuidStr,
	}, nil
}

// tryDecodeBase64 安全尝试清洗空白字符并做 Base64 解码
func tryDecodeBase64(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)

	if len(cleaned) < 4 {
		return s
	}

	padded := cleaned
	if m := len(padded)%4; m != 0 {
		padded += strings.Repeat("=", 4-m)
	}

	if b, err := base64.StdEncoding.DecodeString(padded); err == nil && len(b) > 0 {
		return string(b)
	}
	if b, err := base64.URLEncoding.DecodeString(padded); err == nil && len(b) > 0 {
		return string(b)
	}
	return s
}

// parsePreferredNodes 统一多格式解析器：支持全量 Base64、逐行 Base64、VLESS/Trojan 链接、纯 IP、IP:PORT#TAG 与 CSV
func parsePreferredNodes(rawText, host, uuidStr, path string) []model.CFPreferredIP {
	// 1. 尝试全量 Base64 解码
	decodedText := tryDecodeBase64(rawText)

	var parsedList []model.CFPreferredIP
	lines := strings.Split(decodedText, "\n")
	idx := 1

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		// 2. 检查单行是否为独立 Base64
		if !strings.HasPrefix(line, "vless://") && !strings.HasPrefix(line, "trojan://") {
			if decodedLine := tryDecodeBase64(line); decodedLine != line {
				line = strings.TrimSpace(decodedLine)
			}
		}

		// 3. 处理 VLESS 或 Trojan 链接格式
		if strings.HasPrefix(line, "vless://") || strings.HasPrefix(line, "trojan://") {
			item, ok := parseVlessOrTrojanLink(line, idx)
			if ok {
				parsedList = append(parsedList, item)
				idx++
			}
			continue
		}

		// 4. 处理纯 IP 或 IP:PORT 或 CSV 格式 (例如 104.16.1.1:443#CM 或 104.16.1.1,443,CM)
		item, ok := parsePlainIPLine(line, host, uuidStr, path, idx)
		if ok {
			parsedList = append(parsedList, item)
			idx++
		}
	}

	return parsedList
}

// parseVlessOrTrojanLink 解析 vless:// 或 trojan:// 协议节点
func parseVlessOrTrojanLink(line string, idx int) (model.CFPreferredIP, bool) {
	u, err := url.Parse(line)
	if err != nil {
		return model.CFPreferredIP{}, false
	}

	ip := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port, _ := strconv.Atoi(portStr)

	// 过滤无效本地或保留地址
	if ip == "" || ip == "example.com" || ip == "127.0.0.1" || ip == "localhost" {
		return model.CFPreferredIP{}, false
	}

	rawTag := u.Fragment
	if unescaped, err := url.QueryUnescape(rawTag); err == nil && unescaped != "" {
		rawTag = unescaped
	}
	if rawTag == "" {
		rawTag = fmt.Sprintf("CF-%s", ip)
	}

	// 过滤失效或废弃节点提示
	if strings.Contains(rawTag, "不再支持旧版") || strings.Contains(rawTag, "更新至最新版本") {
		return model.CFPreferredIP{}, false
	}

	cleanTag := strings.TrimSpace(strings.TrimPrefix(rawTag, "#"))
	name := fmt.Sprintf("CF-%s-%02d", cleanTag, idx)
	colo := extractColo(cleanTag)

	now := time.Now()
	return model.CFPreferredIP{
		IP:        ip,
		Port:      port,
		Tag:       cleanTag,
		Name:      name,
		Colo:      colo,
		RawLink:   line,
		CreatedAt: now,
		UpdatedAt: now,
	}, true
}

// parsePlainIPLine 解析纯 IP、IP:PORT#TAG、IP#TAG 或 CSV 格式
func parsePlainIPLine(line, host, uuidStr, path string, idx int) (model.CFPreferredIP, bool) {
	tag := ""
	hostPortPart := line

	// 兼容 CSV 格式: 162.159.160.213,443,CM-Default 或 162.159.160.213,443
	if strings.Contains(line, ",") {
		parts := strings.Split(line, ",")
		ipStr := strings.TrimSpace(parts[0])
		parsedIP := net.ParseIP(ipStr)
		if parsedIP == nil || isPrivateOrSpecialIP(parsedIP) {
			return model.CFPreferredIP{}, false
		}
		port := 443
		if len(parts) >= 2 {
			if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && p > 0 && p <= 65535 {
				port = p
			}
		}
		if len(parts) >= 3 {
			tag = strings.TrimSpace(parts[2])
		}
		return buildPreferredIPItem(ipStr, port, tag, host, uuidStr, path, idx)
	}

	// 兼容 IP:PORT#TAG 或 IP#TAG
	if strings.Contains(line, "#") {
		parts := strings.SplitN(line, "#", 2)
		hostPortPart = strings.TrimSpace(parts[0])
		tag = strings.TrimSpace(parts[1])
	}

	var ipStr string
	port := 443

	if strings.Contains(hostPortPart, ":") {
		h, pStr, err := net.SplitHostPort(hostPortPart)
		if err == nil {
			ipStr = h
			if p, err := strconv.Atoi(pStr); err == nil && p > 0 && p <= 65535 {
				port = p
			}
		} else {
			// 可能为纯 IPv6 或带端口格式
			ipStr = strings.Trim(hostPortPart, "[]")
		}
	} else {
		ipStr = hostPortPart
	}

	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil || isPrivateOrSpecialIP(parsedIP) {
		return model.CFPreferredIP{}, false
	}

	return buildPreferredIPItem(ipStr, port, tag, host, uuidStr, path, idx)
}

func buildPreferredIPItem(ip string, port int, tag, host, uuidStr, path string, idx int) (model.CFPreferredIP, bool) {
	cleanTag := strings.TrimSpace(strings.TrimPrefix(tag, "#"))
	if cleanTag == "" {
		cleanTag = fmt.Sprintf("CF-%s", ip)
	}

	name := fmt.Sprintf("CF-%s-%02d", cleanTag, idx)
	colo := extractColo(cleanTag)

	// 自动生成标准 VLESS-WS 连接串，无缝适配客户端导入与订阅裂变
	rawLink := fmt.Sprintf("vless://%s@%s:%d?encryption=none&security=tls&sni=%s&fp=chrome&type=ws&host=%s&path=%s#%s",
		uuidStr, ip, port, host, host, url.QueryEscape(path), url.QueryEscape(cleanTag))

	now := time.Now()
	return model.CFPreferredIP{
		IP:        ip,
		Port:      port,
		Tag:       cleanTag,
		Name:      name,
		Colo:      colo,
		RawLink:   rawLink,
		CreatedAt: now,
		UpdatedAt: now,
	}, true
}

func isPrivateOrSpecialIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast()
}

// extractColo 提取国家/地区或运营商缩写
func extractColo(tag string) string {
	upperTag := strings.ToUpper(tag)

	// 中文常见运营商与地区智能映射
	switch {
	case strings.Contains(tag, "香港"):
		return "HK"
	case strings.Contains(tag, "日本"):
		return "JP"
	case strings.Contains(tag, "新加坡"):
		return "SG"
	case strings.Contains(tag, "美国"):
		return "US"
	case strings.Contains(tag, "韩国"):
		return "KR"
	case strings.Contains(tag, "台湾"):
		return "TW"
	case strings.Contains(tag, "移动"):
		return "CM"
	case strings.Contains(tag, "联通"):
		return "CU"
	case strings.Contains(tag, "电信"):
		return "CT"
	}

	if len(upperTag) >= 2 {
		prefix := upperTag[:2]
		if isAllLetters(prefix) {
			return prefix
		}
	}
	return ""
}

func recordFailure(msg string) {
	_ = model.SetSetting("cf_pref_last_fetch_at", time.Now().Format(time.RFC3339))
	_ = model.SetSetting("cf_pref_last_fetch_status", "error: "+msg)
}

func isAllLetters(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}
