package cfpref

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
)

const (
	DefaultSubURL   = "https://sub.19910417.xyz/sub?host={host}&uuid={uuid}"
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
	subURLTpl, host, uuidStr, _ := GetEffectiveConfig()

	reqURL := subURLTpl
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
	// 尝试 Base64 解码
	decodedText := rawText
	padded := rawText
	if m := len(padded) % 4; m != 0 {
		padded += strings.Repeat("=", 4-m)
	}
	if b, err := base64.StdEncoding.DecodeString(padded); err == nil {
		decodedText = string(b)
	} else if b, err := base64.URLEncoding.DecodeString(padded); err == nil {
		decodedText = string(b)
	}

	var parsedList []model.CFPreferredIP
	lines := strings.Split(decodedText, "\n")
	idx := 1

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "vless://") {
			continue
		}

		u, err := url.Parse(line)
		if err != nil {
			continue
		}

		ip := u.Hostname()
		portStr := u.Port()
		if portStr == "" {
			portStr = "443"
		}
		port, _ := strconv.Atoi(portStr)

		// 过滤无效本地或保留地址
		if ip == "" || ip == "example.com" || ip == "127.0.0.1" || ip == "localhost" {
			continue
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
			continue
		}

		cleanTag := strings.TrimSpace(strings.TrimPrefix(rawTag, "#"))
		name := fmt.Sprintf("CF-%s-%02d", cleanTag, idx)

		// 提取国家/地区或机房代码（例如前两位大写字母）
		colo := ""
		if len(cleanTag) >= 2 {
			prefix := cleanTag[:2]
			if isAllLetters(prefix) {
				colo = strings.ToUpper(prefix)
			}
		}

		parsedList = append(parsedList, model.CFPreferredIP{
			IP:        ip,
			Port:      port,
			Tag:       cleanTag,
			Name:      name,
			Colo:      colo,
			RawLink:   line,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		idx++
	}

	if len(parsedList) == 0 {
		recordFailure("未从订阅响应中解析出有效的优选 IP 节点")
		return nil, fmt.Errorf("no valid preferred nodes found in subscription response")
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
