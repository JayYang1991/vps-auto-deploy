package handler

import (
	"bufio"
	"fmt"
	"io"
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

	configPath := subconverter.GetConfigPath()
	rawBytes, err := os.ReadFile(configPath)
	if err != nil && os.IsNotExist(err) {
		// 尝试读取 example
		examplePath := filepath.Join(subconverter.GetComponentDir(), "pref.example.ini")
		rawBytes, _ = os.ReadFile(examplePath)
	}

	rawContent := string(rawBytes)
	settings := parseIniKeyValues(rawContent)

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

// SaveSubconverterConfig 保存 pref.ini 并按需重启服务
func SaveSubconverterConfig(c *gin.Context) {
	var req SaveSubconverterConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法: " + err.Error()})
		return
	}

	configPath := subconverter.GetConfigPath()
	_ = os.MkdirAll(filepath.Dir(configPath), 0755)

	contentToWrite := req.RawContent
	if strings.TrimSpace(contentToWrite) == "" {
		// 若未直接提交完整文本，根据表单字段更新或新建
		existing, _ := os.ReadFile(configPath)
		contentToWrite = updateIniSettings(string(existing), map[string]string{
			"listen":                  req.Listen,
			"port":                    strconv.Itoa(req.Port),
			"api_access_token":        req.ApiToken,
			"exclude_remarks":         req.ExcludeRemark,
			"include_remarks":         req.IncludeRemark,
			"default_external_config": req.DefaultConfig,
		})
	}

	if err := os.WriteFile(configPath, []byte(contentToWrite), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入配置文件失败: " + err.Error()})
		return
	}

	// 若服务当前处于活跃状态，自动执行重载/重启
	drv := subconverter.NewDriver()
	st, _ := drv.Status()
	restarted := false
	if st.Active {
		_ = drv.Restart()
		restarted = true
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Subconverter 配置已成功保存！",
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

	_ = model.SetSetting("subconverter_pref_target", req.Target)
	_ = model.SetSetting("subconverter_pref_template", req.Template)
	_ = model.SetSetting("subconverter_pref_custom_url", req.CustomTemplateURL)
	_ = model.SetSetting("subconverter_pref_emoji", strconv.FormatBool(req.Emoji))
	_ = model.SetSetting("subconverter_pref_udp", strconv.FormatBool(req.UDP))
	_ = model.SetSetting("subconverter_pref_scv", strconv.FormatBool(req.SCV))

	// 同时将模板路径同步到 pref.ini 的 default_external_config
	configPath := subconverter.GetConfigPath()
	if existing, err := os.ReadFile(configPath); err == nil {
		effectiveTpl := req.Template
		if effectiveTpl == "custom" && req.CustomTemplateURL != "" {
			effectiveTpl = req.CustomTemplateURL
		}
		updated := updateIniSettings(string(existing), map[string]string{
			"default_external_config": effectiveTpl,
		})
		_ = os.WriteFile(configPath, []byte(updated), 0644)
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

// ProxySubconverterConvert 在后端向本机 127.0.0.1:25500 发起请求并透传转换输出
func ProxySubconverterConvert(target, sourceSubURL, configTemplate string, extraParams map[string]string) ([]byte, string, error) {
	baseURL := fmt.Sprintf("http://127.0.0.1:%d/sub", subconverter.DefaultPort)
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, "", err
	}

	q := u.Query()
	q.Set("target", target)
	q.Set("url", sourceSubURL)
	if configTemplate != "" {
		q.Set("config", configTemplate)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("subconverter returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}

	return body, contentType, nil
}
