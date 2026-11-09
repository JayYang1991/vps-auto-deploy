package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/subconverter"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
	"vps-auto-deploy/internal/subgen"
)

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

	filteredURIs := subgen.FilterURIsByRemark(uris, excludeRemark, includeRemark)

	// 2. 若请求的是通用 Base64 / URI 原始链接
	if clientTarget == "base64" || clientTarget == "uri" || clientTarget == "v2ray" {
		b64Content := base64.StdEncoding.EncodeToString([]byte(strings.Join(filteredURIs, "\n")))
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"sub-%s.txt\"", member.Name))
		c.Header("Profile-Update-Interval", "24")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(b64Content))
		return
	}

	// 3. 优先使用 Subconverter 引擎生成最终客户端配置文件
	// 获取配置模板：优先请求参数，次选配置中心设置的自定义模板，最后使用 ACL4SSR 在线精炼模板
	configTemplate := strings.TrimSpace(config)
	if configTemplate != "" {
		// 安全防护：校验外部传入的 config 模板 URL，拦截私网/内网/元数据等 SSRF 敏感地址
		if err := netutil.ValidatePublicURL(configTemplate); err != nil {
			c.String(http.StatusBadRequest, "Invalid config template: %v", err)
			return
		}
	} else {
		if customTpl, _ := model.GetSetting("subconverter_pref_custom_url"); strings.TrimSpace(customTpl) != "" {
			configTemplate = strings.TrimSpace(customTpl)
		} else if defaultExt, _ := model.GetSetting("subconverter_default_external_config"); strings.TrimSpace(defaultExt) != "" {
			configTemplate = strings.TrimSpace(defaultExt)
		} else {
			configTemplate = "https://raw.githubusercontent.com/JayYang1991/ACL4SSR/refs/heads/main/Clash/config/ACL4SSR_Online_Bespoke.ini"
		}
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
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"sub-%s-%s.txt\"", token[:8], clientTarget))
		c.Header("Profile-Update-Interval", "24")
		c.Data(http.StatusOK, contentType, body)
		return
	}

	// 4. 容灾兜底：若 Subconverter 服务未启动或异常，且未强制要求 engine=subconverter，则优雅降级为内置生成器
	if engine != "subconverter" {
		res, genErr := subgen.GenerateSubscription(token, clientTarget, ua)
		if genErr == nil && res != nil {
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", res.Filename))
			c.Header("Profile-Update-Interval", "24")
			c.Data(http.StatusOK, res.ContentType, []byte(res.Content))
			return
		}
	}

	c.String(http.StatusBadGateway, fmt.Sprintf("Subconverter 转换失败: %v", convErr))
}
