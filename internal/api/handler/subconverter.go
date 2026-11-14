package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/subconverter"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
	"vps-auto-deploy/internal/subgen"
)

type SubconverterConfigResponse struct {
	Installed     bool                   `json:"installed"`
	Active        bool                   `json:"active"`
	Version       string                 `json:"version"`
	ConfigPath    string                 `json:"config_path"`
	ComponentDir  string                 `json:"component_dir"`
	Listen        string                 `json:"listen"`
	Port          int                    `json:"port"`
	ApiMode       bool                   `json:"api_mode"`
	ApiToken      string                 `json:"api_token"`
	DefaultURL    string                 `json:"default_url"`
	ExcludeRemark string                 `json:"exclude_remarks"`
	IncludeRemark string                 `json:"include_remarks"`
	DefaultConfig string                 `json:"default_external_config"`
	RawContent    string                 `json:"raw_content"`
	Settings      map[string]string      `json:"settings"`
}

type SaveSubconverterConfigRequest struct {
	IsRaw         bool   `json:"is_raw"`
	RawContent    string `json:"raw_content"`
	Listen        string `json:"listen"`
	Port          int    `json:"port"`
	ApiToken      string `json:"api_token"`
	ExcludeRemark string `json:"exclude_remarks"`
	IncludeRemark string `json:"include_remarks"`
	DefaultConfig string `json:"default_external_config"`
}

type RuleTemplateItem struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Recommended bool   `json:"recommended"`
	IsLocal     bool   `json:"is_local"`
	ConfigValue string `json:"config_value"`
}

// GetSubconverterStatus 获取 Subconverter 服务运行状态
func GetSubconverterStatus(c *gin.Context) {
	drv := subconverter.NewDriver()
	st, err := drv.Status()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	isResponsive := false
	if st.Active {
		client := &http.Client{Timeout: 800 * time.Millisecond}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/version", subconverter.DefaultPort))
		if err == nil {
			resp.Body.Close()
			isResponsive = true
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"installed":         st.Installed,
		"installed_version": st.InstalledVersion,
		"latest_version":    st.LatestVersion,
		"has_update":        st.HasUpdate,
		"active":            st.Active,
		"running_status":    st.RunningStatus,
		"is_responsive":     isResponsive,
		"port":              subconverter.DefaultPort,
		"component_dir":     subconverter.GetComponentDir(),
		"config_path":       subconverter.GetConfigPath(),
	})
}

// GetSubconverterConfig 获取 pref.ini 配置与解析键值
func GetSubconverterConfig(c *gin.Context) {
	drv := subconverter.NewDriver()
	st, _ := drv.Status()

	configPath := filepath.Clean(subconverter.GetConfigPath())
	rawBytes, err := os.ReadFile(configPath)
	if err != nil && os.IsNotExist(err) {
		// 优先从 SQLite 数据库还原用户备份配置
		if dbRaw, _ := model.GetSetting("subconverter_raw_ini"); dbRaw != "" {
			rawBytes = []byte(dbRaw)
			_ = os.MkdirAll(filepath.Dir(configPath), 0750)
			_ = os.WriteFile(configPath, rawBytes, 0600)
			_ = os.Chmod(configPath, 0600)
		} else {
			// 尝试读取 example
			examplePath := filepath.Join(subconverter.GetComponentDir(), "pref.example.ini")
			/* #nosec G304 - reading example config file */
			rawBytes, _ = os.ReadFile(examplePath)
		}
	}

	rawContent := string(rawBytes)
	settings := parseIniKeyValues(rawContent)

	// 若 pref.ini 中缺失关键字段，尝试从数据库补充同步
	dirty := false
	if settings["exclude_remarks"] == "" {
		if dbExclude, _ := model.GetSetting("subconverter_exclude_remarks"); dbExclude != "" {
			settings["exclude_remarks"] = dbExclude
			rawContent = updateIniSettings(rawContent, map[string]string{"exclude_remarks": dbExclude})
			dirty = true
		}
	}
	if settings["include_remarks"] == "" {
		if dbInclude, _ := model.GetSetting("subconverter_include_remarks"); dbInclude != "" {
			settings["include_remarks"] = dbInclude
			rawContent = updateIniSettings(rawContent, map[string]string{"include_remarks": dbInclude})
			dirty = true
		}
	}
	if settings["default_external_config"] == "" {
		if dbConfig, _ := model.GetSetting("subconverter_default_external_config"); dbConfig != "" {
			settings["default_external_config"] = dbConfig
			rawContent = updateIniSettings(rawContent, map[string]string{"default_external_config": dbConfig})
			dirty = true
		}
	}
	if dirty {
		/* #nosec G703 */
		_ = os.WriteFile(configPath, []byte(rawContent), 0600)
		_ = os.Chmod(configPath, 0600)
	}

	port := subconverter.DefaultPort
	if pStr, ok := settings["port"]; ok {
		if pVal, err := strconv.Atoi(strings.TrimSpace(pStr)); err == nil && pVal > 0 {
			port = pVal
		}
	}

	listen := settings["listen"]
	if listen == "" {
		listen = "127.0.0.1"
	}

	resp := SubconverterConfigResponse{
		Installed:     st.Installed,
		Active:        st.Active,
		Version:       st.InstalledVersion,
		ConfigPath:    configPath,
		ComponentDir:  subconverter.GetComponentDir(),
		Listen:        listen,
		Port:          port,
		ApiMode:       settings["api_mode"] == "true",
		ApiToken:      settings["api_access_token"],
		DefaultURL:    settings["default_url"],
		ExcludeRemark: settings["exclude_remarks"],
		IncludeRemark: settings["include_remarks"],
		DefaultConfig: settings["default_external_config"],
		RawContent:    rawContent,
		Settings:      settings,
	}

	c.JSON(http.StatusOK, resp)
}

func rollbackSubconverter(configPath string, oldConfig []byte, drv *subconverter.Driver) {
	cleanPath := filepath.Clean(configPath)
	if len(oldConfig) > 0 {
		/* #nosec G703 */
		_ = os.WriteFile(cleanPath, oldConfig, 0600)
		_ = os.Chmod(cleanPath, 0600)
	} else {
		_ = os.Remove(cleanPath)
	}
	_ = drv.Restart()
}

// SaveSubconverterConfig 保存 pref.ini 并进行合法性校验、探活与自动回滚
func SaveSubconverterConfig(c *gin.Context) {
	var req SaveSubconverterConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法: " + err.Error()})
		return
	}

	// 1. 语法与参数合法性预检
	listen := strings.TrimSpace(req.Listen)
	if listen != "" && listen != "0.0.0.0" && listen != "127.0.0.1" && listen != "::" && listen != "::1" {
		if net.ParseIP(listen) == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "绑定监听地址格式不合法: " + listen})
			return
		}
	}
	if req.Port < 0 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "监听端口必须在 1 ~ 65535 之间"})
		return
	}

	if defCfg := strings.TrimSpace(req.DefaultConfig); defCfg != "" {
		if strings.HasPrefix(defCfg, "http://") || strings.HasPrefix(defCfg, "https://") {
			if err := netutil.ValidatePublicURL(defCfg); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "外部规则配置 URL 安全校验失败: " + err.Error()})
				return
			}
		} else if !IsSafeLocalTemplate(defCfg) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法的受管规则模板路径: 仅支持受管本地模板 (如 config/custom_rules.ini) 或公网 URL"})
			return
		}
	}

	if strings.TrimSpace(req.RawContent) != "" {
		parsed := parseIniKeyValues(req.RawContent)
		if pStr, ok := parsed["port"]; ok {
			pVal, err := strconv.Atoi(strings.TrimSpace(pStr))
			if err != nil || pVal < 1 || pVal > 65535 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "pref.ini 中的 port 必须在 1 ~ 65535 之间"})
				return
			}
		}
		if lStr, ok := parsed["listen"]; ok {
			lStr = strings.TrimSpace(lStr)
			if lStr != "" && lStr != "0.0.0.0" && lStr != "127.0.0.1" && lStr != "::" && lStr != "::1" && net.ParseIP(lStr) == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "pref.ini 中的 listen 格式不合法: " + lStr})
				return
			}
		}
	}

	configPath := filepath.Clean(subconverter.GetConfigPath())
	_ = os.MkdirAll(filepath.Dir(configPath), 0750)

	// 2. 备份原配置
	oldConfig, _ := os.ReadFile(configPath)

	var contentToWrite string
	if req.IsRaw {
		contentToWrite = req.RawContent
		if strings.TrimSpace(contentToWrite) == "" {
			contentToWrite = string(oldConfig)
		}
	} else {
		baseContent := req.RawContent
		if strings.TrimSpace(baseContent) == "" {
			baseContent = string(oldConfig)
		}
		updates := map[string]string{
			"listen":                  req.Listen,
			"port":                    strconv.Itoa(req.Port),
			"api_access_token":        req.ApiToken,
			"exclude_remarks":         req.ExcludeRemark,
			"include_remarks":         req.IncludeRemark,
			"default_external_config": req.DefaultConfig,
		}
		contentToWrite = updateIniSettings(baseContent, updates)
	}

	// 确保基础容灾字段 skip_failed_links 存在
	if !strings.Contains(contentToWrite, "skip_failed_links") {
		if strings.Contains(contentToWrite, "[common]") {
			contentToWrite = strings.Replace(contentToWrite, "[common]", "[common]\nskip_failed_links=true", 1)
		}
	}

	// 3. 写入新配置到 pref.ini
	if err := os.WriteFile(configPath, []byte(contentToWrite), 0600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入配置文件失败: " + err.Error()})
		return
	}
	_ = os.Chmod(configPath, 0600)

	// 4. 解析生效设置并持久化到 SQLite 数据库（单一真相源，防重新升级或重启丢失配置）
	finalSettings := parseIniKeyValues(contentToWrite)
	_ = model.SetSetting("subconverter_exclude_remarks", finalSettings["exclude_remarks"])
	_ = model.SetSetting("subconverter_include_remarks", finalSettings["include_remarks"])
	_ = model.SetSetting("subconverter_default_external_config", finalSettings["default_external_config"])
	_ = model.SetSetting("subconverter_listen", finalSettings["listen"])
	_ = model.SetSetting("subconverter_port", finalSettings["port"])
	_ = model.SetSetting("subconverter_api_token", finalSettings["api_access_token"])
	_ = model.SetSetting("subconverter_raw_ini", contentToWrite)

	// 4. 计算目标探活端口
	targetPort := req.Port
	if targetPort <= 0 {
		s := parseIniKeyValues(contentToWrite)
		if pVal, err := strconv.Atoi(strings.TrimSpace(s["port"])); err == nil && pVal > 0 {
			targetPort = pVal
		}
	}
	if targetPort <= 0 {
		targetPort = subconverter.DefaultPort
	}

	// 5. 若服务已安装且运行中，重启并执行健康探活 (测试环境若设置 SKIP_SERVICE_RELOAD 则跳过实际 systemd 重启)
	drv := subconverter.NewDriver()
	st, _ := drv.Status()
	restarted := false
	if st.Installed && st.Active && os.Getenv("SKIP_SERVICE_RELOAD") != "1" {
		if err := drv.Restart(); err != nil {
			rollbackSubconverter(configPath, oldConfig, drv)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Subconverter 服务重启失败，已自动回滚原配置: " + err.Error()})
			return
		}
		restarted = true

		// 启动探活检测 (最多等待 3 秒)
		client := &http.Client{Timeout: 500 * time.Millisecond}
		probeURL := fmt.Sprintf("http://127.0.0.1:%d/version", targetPort)
		healthy := false
		for i := 0; i < 10; i++ {
			time.Sleep(300 * time.Millisecond)
			resp, err := client.Get(probeURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					healthy = true
					break
				}
			}
		}

		if !healthy {
			rollbackSubconverter(configPath, oldConfig, drv)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Subconverter 启动后健康探活失败 (端口 %d 无响应)，已自动回滚原配置", targetPort),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Subconverter 配置已成功保存并通过健康检查！",
		"restarted": restarted,
	})
}

// ListSubconverterTemplates 列出内置与常用的 ACL4SSR / 规则模板列表
func ListSubconverterTemplates(c *gin.Context) {
	compDir := subconverter.GetComponentDir()
	configDir := filepath.Join(compDir, "config")

	// 预设推荐模板池
	presets := []RuleTemplateItem{
		{
			Name:        "custom_rules.ini",
			Label:       "✏️ 本地专属自定义分流规则 (custom_rules.ini)",
			Description: "由您自定义维护的本地分流规则与策略组，支持离线即时转换生效",
			Recommended: false,
			IsLocal:     true,
			ConfigValue: "config/custom_rules.ini",
		},
		{
			Name:        "ACL4SSR_Online.ini",
			Label:       "ACL4SSR 经典在线分流版",
			Description: "默认推荐：自动测速节点池、国内直连、国外代理、主流流媒体分流",
			Recommended: true,
			IsLocal:     true,
			ConfigValue: "config/ACL4SSR_Online.ini",
		},
		{
			Name:        "ACL4SSR_Online_Full.ini",
			Label:       "ACL4SSR 全规则全分组完整版",
			Description: "包含 Apple、Google、微软、奈飞、油管、电报等数十项极细粒度策略组",
			Recommended: false,
			IsLocal:     true,
			ConfigValue: "config/ACL4SSR_Online_Full.ini",
		},
		{
			Name:        "ACL4SSR_Online_Mini.ini",
			Label:       "ACL4SSR 精简流媒体版",
			Description: "适合轻量用户，仅保留基础国内直连与常用国外流媒体分组",
			Recommended: false,
			IsLocal:     true,
			ConfigValue: "config/ACL4SSR_Online_Mini.ini",
		},
		{
			Name:        "ACL4SSR_Online_Full_AdblockPlus.ini",
			Label:       "ACL4SSR 完整分流 + 广告拦截版",
			Description: "在全分组基础上集成国内外广告反追踪规则集",
			Recommended: false,
			IsLocal:     true,
			ConfigValue: "config/ACL4SSR_Online_Full_AdblockPlus.ini",
		},
		{
			Name:        "ACL4SSR_Online_NoAuto.ini",
			Label:       "ACL4SSR 无自动测速版",
			Description: "所有节点组均采用手动选择，避免后台自动轮询测速流量",
			Recommended: false,
			IsLocal:     true,
			ConfigValue: "config/ACL4SSR_Online_NoAuto.ini",
		},
		{
			Name:        "ACL4SSR_Online_MultiCountry.ini",
			Label:       "ACL4SSR 按国家/地区分流版",
			Description: "按香港、台湾、日本、美国、新加坡等多国家/地区自动归类策略组",
			Recommended: false,
			IsLocal:     true,
			ConfigValue: "config/ACL4SSR_Online_MultiCountry.ini",
		},
	}

	// 扫描本地是否有已解压的更多 ini 配置
	if files, err := os.ReadDir(configDir); err == nil {
		existingMap := make(map[string]bool)
		for _, p := range presets {
			existingMap[p.Name] = true
		}
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".ini") {
				if !existingMap[f.Name()] {
					presets = append(presets, RuleTemplateItem{
						Name:        f.Name(),
						Label:       "本地规则: " + f.Name(),
						Description: "位于 " + filepath.Join(configDir, f.Name()),
						Recommended: false,
						IsLocal:     true,
						ConfigValue: "config/" + f.Name(),
					})
				}
			}
		}
	}

	c.JSON(http.StatusOK, presets)
}

type SubconverterPreferences struct {
	Target            string `json:"target"`
	Template          string `json:"template"`
	CustomTemplateURL string `json:"custom_template_url"`
	Emoji             bool   `json:"emoji"`
	UDP               bool   `json:"udp"`
	SCV               bool   `json:"scv"`
}

// GetSubconverterPreferences 获取工作台默认偏好
func GetSubconverterPreferences(c *gin.Context) {
	target, _ := model.GetSetting("subconverter_pref_target")
	if target == "" {
		target = "clash"
	}

	template, _ := model.GetSetting("subconverter_pref_template")
	if template == "" {
		template = "config/ACL4SSR_Online.ini"
	}

	customURL, _ := model.GetSetting("subconverter_pref_custom_url")

	emojiStr, _ := model.GetSetting("subconverter_pref_emoji")
	emoji := emojiStr != "false"

	udpStr, _ := model.GetSetting("subconverter_pref_udp")
	udp := udpStr != "false"

	scvStr, _ := model.GetSetting("subconverter_pref_scv")
	scv := scvStr != "false"

	c.JSON(http.StatusOK, SubconverterPreferences{
		Target:            target,
		Template:          template,
		CustomTemplateURL: customURL,
		Emoji:             emoji,
		UDP:               udp,
		SCV:               scv,
	})
}

// SaveSubconverterPreferences 保存工作台选择为全局默认偏好
func SaveSubconverterPreferences(c *gin.Context) {
	var req SubconverterPreferences
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败: " + err.Error()})
		return
	}

	if req.Target == "" {
		req.Target = "clash"
	}
	if req.Template == "" {
		req.Template = "config/ACL4SSR_Online.ini"
	}

	// SSRF 防御校验：当选用自定义模板或外部 URL 时，必须验证目标为合法公网地址
	if req.Template == "custom" || strings.HasPrefix(req.Template, "http://") || strings.HasPrefix(req.Template, "https://") {
		targetURL := strings.TrimSpace(req.CustomTemplateURL)
		if req.Template != "custom" {
			targetURL = strings.TrimSpace(req.Template)
		}
		if targetURL != "" {
			if err := netutil.ValidatePublicURL(targetURL); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "自定义模板 URL 安全校验失败: " + err.Error()})
				return
			}
		}
	} else if !IsSafeLocalTemplate(req.Template) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法的受管规则模板路径: 仅支持受管本地模板 (如 config/custom_rules.ini) 或公网 URL"})
		return
	}

	_ = model.SetSetting("subconverter_pref_target", req.Target)
	_ = model.SetSetting("subconverter_pref_template", req.Template)
	_ = model.SetSetting("subconverter_pref_custom_url", req.CustomTemplateURL)
	_ = model.SetSetting("subconverter_pref_emoji", strconv.FormatBool(req.Emoji))
	_ = model.SetSetting("subconverter_pref_udp", strconv.FormatBool(req.UDP))
	_ = model.SetSetting("subconverter_pref_scv", strconv.FormatBool(req.SCV))

	// 同时将模板路径同步到 pref.ini 的 default_external_config
	configPath := filepath.Clean(subconverter.GetConfigPath())
	if existing, err := os.ReadFile(configPath); err == nil {
		effectiveTpl := req.Template
		if effectiveTpl == "custom" && req.CustomTemplateURL != "" {
			effectiveTpl = req.CustomTemplateURL
		}
		updated := updateIniSettings(string(existing), map[string]string{
			"default_external_config": effectiveTpl,
		})
		_ = os.WriteFile(configPath, []byte(updated), 0600)
		_ = os.Chmod(configPath, 0600)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "默认规则集与订阅偏好已成功持久化保存！",
		"data":    req,
	})
}

// 辅助函数：解析 INI 文件的 key=value
func parseIniKeyValues(content string) map[string]string {
	res := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			res[k] = v
		}
	}
	return res
}

// 辅助函数：更新 INI 文件的部分 key 值
func updateIniSettings(existing string, updates map[string]string) string {
	if strings.TrimSpace(existing) == "" {
		var sb strings.Builder
		sb.WriteString("[common]\n")
		for k, v := range updates {
			if v != "" {
				sb.WriteString(fmt.Sprintf("%s=%s\n", k, v))
			}
		}
		return sb.String()
	}

	updatedKeys := make(map[string]bool)
	var outLines []string
	scanner := bufio.NewScanner(strings.NewReader(existing))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
			outLines = append(outLines, line)
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			if newVal, ok := updates[k]; ok {
				outLines = append(outLines, fmt.Sprintf("%s=%s", k, newVal))
				updatedKeys[k] = true
				continue
			}
		}
		outLines = append(outLines, line)
	}

	// 补全尚未存在的键
	for k, v := range updates {
		if !updatedKeys[k] && v != "" {
			outLines = append(outLines, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return strings.Join(outLines, "\n")
}

// ProxySubconverterConvert 在后端向本机 127.0.0.1 发起请求并透传转换输出 (动态感知端口与 API Token 鉴权)
func ProxySubconverterConvert(target, sourceSubURL, configTemplate string, extraParams map[string]string) ([]byte, string, error) {
	port := subconverter.DefaultPort
	if pStr, err := model.GetSetting("subconverter_port"); err == nil && pStr != "" {
		if pVal, err := strconv.Atoi(strings.TrimSpace(pStr)); err == nil && pVal > 0 {
			port = pVal
		}
	} else if pPref := subconverter.GetSettingFromPref("port"); pPref != "" {
		if pVal, err := strconv.Atoi(strings.TrimSpace(pPref)); err == nil && pVal > 0 {
			port = pVal
		}
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/sub", port)
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, "", err
	}

	// 自动将源节点链接方言转换为 Subconverter 严格支持的格式 (如将 socks5:// 转为 tg://socks?...)
	adaptedSource := subgen.AdaptSourceURLsForSubconverter(sourceSubURL)

	q := u.Query()
	q.Set("target", target)
	q.Set("url", adaptedSource)
	if configTemplate != "" {
		q.Set("config", configTemplate)
	}

	// 若启用了 api_access_token 鉴权保护，自动挂载访问令牌
	apiToken, _ := model.GetSetting("subconverter_api_token")
	if apiToken == "" {
		apiToken = subconverter.GetSettingFromPref("api_access_token")
	}
	if apiToken != "" {
		q.Set("token", strings.TrimSpace(apiToken))
	}

	for k, v := range extraParams {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "VPS-Auto-Deploy/Subconverter-Proxy")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("connect to local subconverter failed: %w", err)
	}
	defer resp.Body.Close()

	// 限制最大读取 10MB，防范超大响应引发 OOM
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("subconverter returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	if target == "singbox" && len(body) > 0 {
		body = PatchSingboxTLSAndTransportSNI(body)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}

	return body, contentType, nil
}

// PatchSingboxTLSAndTransportSNI 补全 Subconverter 生成的 sing-box 客户端配置缺失的 tls.server_name 与 transport Host
func PatchSingboxTLSAndTransportSNI(rawJSON []byte) []byte {
	var root map[string]interface{}
	if err := json.Unmarshal(rawJSON, &root); err != nil {
		return rawJSON
	}

	outbounds, ok := root["outbounds"].([]interface{})
	if !ok || len(outbounds) == 0 {
		return rawJSON
	}

	modified := false
	for _, obRaw := range outbounds {
		ob, ok := obRaw.(map[string]interface{})
		if !ok {
			continue
		}

		// 提取根级别或已有的 sni
		sni, _ := ob["sni"].(string)
		sni = strings.TrimSpace(sni)

		// 检查 tls 结构体
		if tlsRaw, ok := ob["tls"].(map[string]interface{}); ok {
			serverName, _ := tlsRaw["server_name"].(string)
			serverName = strings.TrimSpace(serverName)
			if serverName == "" && sni != "" {
				tlsRaw["server_name"] = sni
				modified = true
			}
		} else if sni != "" {
			// 若 tls 块不存在但有 tls 相关配置
			if obType, _ := ob["type"].(string); obType == "vless" || obType == "vmess" || obType == "trojan" {
				ob["tls"] = map[string]interface{}{
					"enabled":     true,
					"server_name": sni,
				}
				modified = true
			}
		}

		// 检查 transport (ws / grpc / httpupgrade) headers.Host
		if transRaw, ok := ob["transport"].(map[string]interface{}); ok {
			transType, _ := transRaw["type"].(string)
			if transType == "ws" || transType == "http" || transType == "httpupgrade" {
				headers, _ := transRaw["headers"].(map[string]interface{})
				if headers == nil {
					headers = make(map[string]interface{})
					transRaw["headers"] = headers
				}
				hostVal, _ := headers["Host"].(string)
				if strings.TrimSpace(hostVal) == "" && sni != "" {
					headers["Host"] = sni
					modified = true
				}
			}
		}
	}

	if !modified {
		return rawJSON
	}

	patched, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return rawJSON
	}
	return patched
}

// DefaultCustomRulesContent 预置的标准自定义规则模板范例
const DefaultCustomRulesContent = `[custom]
; ==============================================================================
; 🚀 VPS Auto Deploy - 本地自定义分流规则模板 (custom_rules.ini)
; 本文件供 Clash / Surge / Loon / Quantumult X 等客户端一键应用自定义策略组与分流规则
; ==============================================================================

; 启用规则生成器并覆盖默认策略
enable_rule_generator=true
overwrite_original_rules=true

; --- 1. 策略组定义 (Proxy Groups) ---
; 语法: custom_proxy_group=策略组名称` + "`" + `select|url-test|fallback` + "`" + `规则1` + "`" + `规则2` + "`" + `...` + "`" + `节点正则
custom_proxy_group=🚀 节点选择` + "`" + `select` + "`" + `[]♻️ 自动选择` + "`" + `[]🎯 全球直连` + "`" + `.*
custom_proxy_group=♻️ 自动选择` + "`" + `url-test` + "`" + `.*` + "`" + `http://www.gstatic.com/generate_204` + "`" + `300,,50
custom_proxy_group=🎬 国外流媒体` + "`" + `select` + "`" + `[]🚀 节点选择` + "`" + `[]♻️ 自动选择` + "`" + `[]🎯 全球直连` + "`" + `.*
custom_proxy_group=🤖 人工智能` + "`" + `select` + "`" + `[]🚀 节点选择` + "`" + `[]♻️ 自动选择` + "`" + `.*
custom_proxy_group=🛑 广告拦截` + "`" + `select` + "`" + `[]🍃 全球拦截` + "`" + `[]🎯 全球直连
custom_proxy_group=🎯 全球直连` + "`" + `select` + "`" + `[]DIRECT
custom_proxy_group=🍃 全球拦截` + "`" + `select` + "`" + `[]REJECT

; --- 2. 外部规则集引用 (Rulesets) ---
; 语法: ruleset=策略组名称,规则集URL或本地路径
ruleset=🛑 广告拦截,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/BanAD.list
ruleset=🤖 人工智能,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/Ruleset/OpenAi.list
ruleset=🎬 国外流媒体,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/Ruleset/Netflix.list
ruleset=🎬 国外流媒体,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/Ruleset/YouTube.list
ruleset=🎯 全球直连,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/ChinaDomain.list
ruleset=🎯 全球直连,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/ChinaCompanyIp.list
ruleset=🎯 全球直连,[]GEOIP,CN
ruleset=🚀 节点选择,[]FINAL
`

// EnsureCustomRulesFile 确保本地 custom_rules.ini 存在，若不存在则从数据库或默认范例落地到磁盘
func EnsureCustomRulesFile() error {
	compDir := subconverter.GetComponentDir()
	rulesPath := filepath.Join(compDir, "config", "custom_rules.ini")
	if _, err := os.Stat(rulesPath); err == nil {
		return nil
	}
	content := DefaultCustomRulesContent
	if model.DB != nil {
		if dbRules, _ := model.GetSetting("subconverter_custom_rules"); strings.TrimSpace(dbRules) != "" {
			content = dbRules
		}
	}
	if err := os.MkdirAll(filepath.Dir(rulesPath), 0750); err != nil {
		return err
	}
	return os.WriteFile(rulesPath, []byte(content), 0600)
}

// IsSafeLocalTemplate 检查指定的模板路径是否为安全合法的本地相对模板文件 (防目录穿越 LFI 攻击)
func IsSafeLocalTemplate(tpl string) bool {
	clean := filepath.Clean(strings.TrimSpace(tpl))
	if clean == "" || filepath.IsAbs(clean) || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "\\") {
		return false
	}
	if strings.Contains(clean, "..") || strings.HasPrefix(clean, ".") {
		return false
	}

	// 必须限定在受管组件专属子目录下
	if !strings.HasPrefix(clean, "config/") && !strings.HasPrefix(clean, "base/") && !strings.HasPrefix(clean, "snippets/") {
		return false
	}

	// 限制文件扩展名
	ext := strings.ToLower(filepath.Ext(clean))
	switch ext {
	case ".ini", ".toml", ".yml", ".yaml", ".conf", ".tpl", ".txt", ".list":
		// 允许的格式
	default:
		return false
	}

	compDir := subconverter.GetComponentDir()
	fullPath := filepath.Join(compDir, clean)
	// 二次防穿越校验：确保绝对路径完全限定在 compDir 内部 (严防兄弟前缀目录逃逸)
	rel, err := filepath.Rel(filepath.Clean(compDir), filepath.Clean(fullPath))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	// 官方受管预设规则模板集合（即使用户尚未安装组件到宿主机磁盘，业务层亦可安全引用）
	knownPresets := map[string]bool{
		"config/custom_rules.ini":                    true,
		"config/ACL4SSR_Online.ini":                  true,
		"config/ACL4SSR_Online_Full.ini":             true,
		"config/ACL4SSR_Online_Mini.ini":             true,
		"config/ACL4SSR_Online_Full_AdblockPlus.ini": true,
		"config/ACL4SSR_Online_NoAuto.ini":           true,
		"config/ACL4SSR_Online_MultiCountry.ini":     true,
	}
	if knownPresets[clean] {
		if clean == "config/custom_rules.ini" {
			_ = EnsureCustomRulesFile()
		}
		return true
	}

	fi, err := os.Stat(fullPath)
	if err != nil || fi.IsDir() {
		return false
	}
	return true
}

// GetSubconverterCustomRules 获取本地自定义规则 custom_rules.ini 内容
func GetSubconverterCustomRules(c *gin.Context) {
	compDir := subconverter.GetComponentDir()
	rulesPath := filepath.Join(compDir, "config", "custom_rules.ini")

	/* #nosec G304 - reading internal custom_rules.ini */
	data, err := os.ReadFile(rulesPath)
	exists := true
	content := string(data)

	if err != nil && os.IsNotExist(err) {
		// 优先从 SQLite 数据库还原用户保存的自定义规则
		if dbRules, _ := model.GetSetting("subconverter_custom_rules"); dbRules != "" {
			_ = os.MkdirAll(filepath.Dir(rulesPath), 0750)
			_ = os.WriteFile(rulesPath, []byte(dbRules), 0600)
			exists = true
			content = dbRules
		} else {
			exists = false
			content = DefaultCustomRulesContent
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取自定义规则失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":    rulesPath,
		"exists":  exists,
		"content": content,
	})
}

// SaveSubconverterCustomRules 保存本地自定义规则 custom_rules.ini 内容
func SaveSubconverterCustomRules(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = DefaultCustomRulesContent
	}

	compDir := subconverter.GetComponentDir()
	configDir := filepath.Join(compDir, "config")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建规则目录失败: " + err.Error()})
		return
	}

	rulesPath := filepath.Join(configDir, "custom_rules.ini")
	if err := os.WriteFile(rulesPath, []byte(content), 0600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入自定义规则文件失败: " + err.Error()})
		return
	}

	// 持久化保存至 SQLite 数据库（单一真相源）
	_ = model.SetSetting("subconverter_custom_rules", content)

	c.JSON(http.StatusOK, gin.H{
		"message": "本地自定义规则模板 (custom_rules.ini) 已成功保存并立即生效！",
		"path":    rulesPath,
	})
}

