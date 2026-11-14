package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/subconverter"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
	"vps-auto-deploy/internal/subgen"
)

var safeHeaderFilenameRegex = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)

// isAllowedRemoteConfigHost 限制公开订阅接口动态传入的外部规则集域名，防止利用 302 重定向绕过 SSRF 检测
func isAllowedRemoteConfigHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	allowedDomains := []string{
		"raw.githubusercontent.com",
		"gist.githubusercontent.com",
		"cdn.jsdelivr.net",
		"fastly.jsdelivr.net",
		"testingcf.jsdelivr.net",
		"ghproxy.net",
		"mirror.ghproxy.com",
	}
	for _, d := range allowedDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	// 若管理员在系统设置中保存过自定义模板 URL，则允许该特定域名
	if savedCustom, _ := model.GetSetting("subconverter_pref_custom_url"); savedCustom != "" {
		if su, err := url.Parse(savedCustom); err == nil && strings.EqualFold(su.Hostname(), host) {
			return true
		}
	}
	return false
}

// buildAttachmentHeader 安全构建 Content-Disposition 标头，防御 CRLF 响应头注入并符合 RFC 6266 标准
func buildAttachmentHeader(baseName, ext string) string {
	cleanASCII := safeHeaderFilenameRegex.ReplaceAllString(baseName, "_")
	if cleanASCII == "" {
		cleanASCII = "sub"
	}
	cleanExt := safeHeaderFilenameRegex.ReplaceAllString(ext, "")
	if cleanExt != "" && !strings.HasPrefix(cleanExt, ".") {
		cleanExt = "." + cleanExt
	}
	encodedName := url.PathEscape(baseName)
	return fmt.Sprintf(`attachment; filename="%s%s"; filename*=UTF-8''%s%s`, cleanASCII, cleanExt, encodedName, cleanExt)
}

func ServeSubscription(c *gin.Context) {
	token := c.Param("token")
	target := c.Query("target")
	engine := c.Query("engine")
	config := c.Query("config")
	ua := c.GetHeader("User-Agent")

	member, err := subgen.GetMemberByToken(token)
	if err != nil {
		c.String(http.StatusBadRequest, "Subscription Error: "+err.Error())
		return
	}

	// 1. 生成全部节点链接 (Reality 直连保持不变 + 每个 WS 节点裂变为优选 IP 列表)
	uris, err := subgen.GenerateURIs(member)
	if err != nil || len(uris) == 0 {
		c.String(http.StatusBadRequest, "当前无可用入站节点供生成订阅")
		return
	}

	// 识别目标客户端格式
	clientTarget := strings.ToLower(strings.TrimSpace(target))
	if clientTarget == "" || clientTarget == "auto" {
		lowerUA := strings.ToLower(ua)
		if strings.Contains(lowerUA, "clash") || strings.Contains(lowerUA, "mihomo") || strings.Contains(lowerUA, "stash") {
			clientTarget = "clash"
		} else if strings.Contains(lowerUA, "sing-box") || strings.Contains(lowerUA, "sfi") || strings.Contains(lowerUA, "sfa") {
			clientTarget = "singbox"
		} else if strings.Contains(lowerUA, "surge") {
			clientTarget = "surge"
		} else if strings.Contains(lowerUA, "shadowrocket") || strings.Contains(lowerUA, "v2rayn") {
			clientTarget = "base64"
		} else {
			clientTarget = "clash"
		}
	}

	// 提取排除与包含节点过滤正则
	// 优先级：URL 查询参数 > 数据库持久化配置 (settings) > pref.ini 物理配置文件
	excludeRemark := strings.TrimSpace(c.Query("exclude"))
	if excludeRemark == "" {
		excludeRemark, _ = model.GetSetting("subconverter_exclude_remarks")
		excludeRemark = strings.TrimSpace(excludeRemark)
	}
	if excludeRemark == "" {
		excludeRemark = strings.TrimSpace(subconverter.GetSettingFromPref("exclude_remarks"))
	}

	includeRemark := strings.TrimSpace(c.Query("include"))
	if includeRemark == "" {
		includeRemark, _ = model.GetSetting("subconverter_include_remarks")
		includeRemark = strings.TrimSpace(includeRemark)
	}
	if includeRemark == "" {
		includeRemark = strings.TrimSpace(subconverter.GetSettingFromPref("include_remarks"))
	}

	// 限制正则长度防 ReDoS
	if len(excludeRemark) > 256 || len(includeRemark) > 256 {
		c.String(http.StatusBadRequest, "Filter regex too long (max 256 chars)")
		return
	}

	filteredURIs := subgen.FilterURIsByRemark(uris, excludeRemark, includeRemark)

	// 2. 若请求的是通用 Base64 / URI 原始链接
	if clientTarget == "base64" || clientTarget == "uri" || clientTarget == "v2ray" {
		b64Content := base64.StdEncoding.EncodeToString([]byte(strings.Join(filteredURIs, "\n")))
		c.Header("Content-Disposition", buildAttachmentHeader(fmt.Sprintf("sub-%s", member.Name), ".txt"))
		c.Header("Profile-Update-Interval", "24")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(b64Content))
		return
	}

	// 3. 优先使用 Subconverter 引擎生成最终客户端配置文件
	// 安全防御策略 (VULN-02)：公开免认证订阅接口仅允许选用后台预置的受管本地规则模板 (防外部 URL SSRF 与注入放大)
	requestedTpl := strings.TrimSpace(c.Query("template"))
	if requestedTpl == "" {
		requestedTpl = strings.TrimSpace(config)
	}

	var configTemplate string
	if requestedTpl != "" {
		if IsSafeLocalTemplate(requestedTpl) {
			configTemplate = requestedTpl
		} else {
			c.String(http.StatusBadRequest, "Invalid template: 订阅端点仅允许选用受管本地规则模板 (如 config/custom_rules.ini)")
			return
		}
	} else {
		// 仅读取管理员在后台数据库中持久化保存的信任配置
		prefTpl, _ := model.GetSetting("subconverter_pref_template")
		prefTpl = strings.TrimSpace(prefTpl)
		if prefTpl == "custom" {
			if customURL, _ := model.GetSetting("subconverter_pref_custom_url"); strings.TrimSpace(customURL) != "" {
				configTemplate = strings.TrimSpace(customURL)
			}
		} else if prefTpl != "" {
			configTemplate = prefTpl
		} else if defaultExt, _ := model.GetSetting("subconverter_default_external_config"); strings.TrimSpace(defaultExt) != "" {
			configTemplate = strings.TrimSpace(defaultExt)
		} else {
			configTemplate = "https://raw.githubusercontent.com/JayYang1991/ACL4SSR/refs/heads/main/Clash/config/ACL4SSR_Online_Bespoke.ini"
		}
	}

	if configTemplate == "config/custom_rules.ini" {
		_ = EnsureCustomRulesFile()
	}

	if strings.HasPrefix(configTemplate, "http://") || strings.HasPrefix(configTemplate, "https://") {
		if err := netutil.ValidatePublicURL(configTemplate); err != nil {
			c.String(http.StatusBadRequest, "Invalid config template: %v", err)
			return
		}
		if !isAllowedRemoteConfigHost(configTemplate) {
			c.String(http.StatusForbidden, "Security Policy: 远程规则集域名未列入受信任名单")
			return
		}
	} else if !IsSafeLocalTemplate(configTemplate) {
		c.String(http.StatusBadRequest, "Invalid config template: 仅支持受管本地规则模板 (如 config/custom_rules.ini)")
		return
	}

	extraParams := make(map[string]string)
	if emoji := c.Query("emoji"); emoji != "" {
		extraParams["emoji"] = emoji
	} else if e, _ := model.GetSetting("subconverter_pref_emoji"); e != "" {
		extraParams["emoji"] = e
	}

	if udp := c.Query("udp"); udp != "" {
		extraParams["udp"] = udp
	} else if u, _ := model.GetSetting("subconverter_pref_udp"); u != "" {
		extraParams["udp"] = u
	}

	if scv := c.Query("scv"); scv != "" {
		extraParams["scv"] = scv
	} else if s, _ := model.GetSetting("subconverter_pref_scv"); s != "" {
		extraParams["scv"] = s
	}

	if excludeRemark != "" {
		extraParams["exclude"] = excludeRemark
	}
	if includeRemark != "" {
		extraParams["include"] = includeRemark
	}

	subconverterURIs := subgen.AdaptURIsForSubconverter(filteredURIs)
	sourceNodes := strings.Join(subconverterURIs, "|")
	body, contentType, convErr := ProxySubconverterConvert(clientTarget, sourceNodes, configTemplate, extraParams)
	if convErr == nil && len(body) > 0 {
		tokenPrefix := token
		if len(tokenPrefix) > 8 {
			tokenPrefix = tokenPrefix[:8]
		}
		cleanTarget := safeHeaderFilenameRegex.ReplaceAllString(clientTarget, "")
		c.Header("Content-Disposition", buildAttachmentHeader(fmt.Sprintf("sub-%s-%s", tokenPrefix, cleanTarget), ".txt"))
		c.Header("Profile-Update-Interval", "24")
		c.Data(http.StatusOK, contentType, body)
		return
	}

	// 4. 容灾兜底：若 Subconverter 服务未启动或异常，且未强制要求 engine=subconverter，则优雅降级为内置生成器
	if engine != "subconverter" {
		res, genErr := subgen.GenerateSubscription(token, clientTarget, ua)
		if genErr == nil && res != nil {
			c.Header("Content-Disposition", buildAttachmentHeader(strings.TrimSuffix(res.Filename, filepath.Ext(res.Filename)), filepath.Ext(res.Filename)))
			c.Header("Profile-Update-Interval", "24")
			c.Data(http.StatusOK, res.ContentType, []byte(res.Content))
			return
		}
	}

	c.String(http.StatusBadGateway, fmt.Sprintf("Subconverter 转换失败: %v", convErr))
}
