package configgen

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
	"vps-auto-deploy/internal/pkg/netutil"
)

// SingboxConfig 代表完整的 sing-box 服务端配置 (符合 sing-box 1.14 最新语法)
type SingboxConfig struct {
	Log          *LogConfig               `json:"log,omitempty"`
	DNS          interface{}              `json:"dns,omitempty"`
	NTP          interface{}              `json:"ntp,omitempty"`
	Certificate  interface{}              `json:"certificate,omitempty"`
	HTTPClients  []HTTPClientConfig       `json:"http_clients,omitempty"`
	Inbounds     []InboundConfig          `json:"inbounds"`
	Outbounds    []map[string]interface{} `json:"outbounds"`
	Route        *RouteConfig             `json:"route,omitempty"`
	Experimental interface{}              `json:"experimental,omitempty"`
}

type HTTPClientConfig struct {
	Tag    string `json:"tag"`
	Detour string `json:"detour,omitempty"`
}

type LogConfig struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type InboundConfig struct {
	Type                  string                 `json:"type"`
	Tag                   string                 `json:"tag"`
	Listen                string                 `json:"listen,omitempty"`
	ListenPort            int                    `json:"listen_port"`
	Users                 []InboundUser          `json:"users,omitempty"`
	TLS                   *InboundTLSConfig      `json:"tls,omitempty"`
	Transport             map[string]interface{} `json:"transport,omitempty"`
	Method                string                 `json:"method,omitempty"`
	Password              string                 `json:"password,omitempty"`
	Network               string                 `json:"network,omitempty"`
	UpMbps                int                    `json:"up_mbps,omitempty"`
	DownMbps              int                    `json:"down_mbps,omitempty"`
	Obfs                  map[string]interface{} `json:"obfs,omitempty"`
	CongestionControl     string                 `json:"congestion_control,omitempty"`
	IgnoreClientBandwidth bool                   `json:"ignore_client_bandwidth,omitempty"`
}

type InboundUser struct {
	Name     string `json:"name,omitempty"`
	UUID     string `json:"uuid,omitempty"`
	Flow     string `json:"flow,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	AlterId  int    `json:"alterId,omitempty"`
}

type InboundTLSConfig struct {
	Enabled         bool                  `json:"enabled"`
	ServerName      string                `json:"server_name,omitempty"`
	CertificatePath string                `json:"certificate_path,omitempty"`
	KeyPath         string                `json:"key_path,omitempty"`
	Reality         *InboundRealityConfig `json:"reality,omitempty"`
}

type InboundRealityConfig struct {
	Enabled    bool              `json:"enabled"`
	Handshake  map[string]interface{} `json:"handshake"`
	PrivateKey string            `json:"private_key"`
	ShortID    []string          `json:"short_id,omitempty"`
}

type OutboundConfig struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type RouteConfig struct {
	DefaultHTTPClient   string                   `json:"default_http_client,omitempty"`
	RuleSet             []RuleSetConfig          `json:"rule_set,omitempty"`
	Rules               []map[string]interface{} `json:"rules,omitempty"`
	AutoDetectInterface bool                     `json:"auto_detect_interface"`
	Final               string                   `json:"final,omitempty"`
}

type RuleSetConfig struct {
	Tag            string `json:"tag"`
	Type           string `json:"type"`                      // "remote"
	Format         string `json:"format"`                    // "binary"
	URL            string `json:"url"`
	UpdateInterval string `json:"update_interval,omitempty"` // "1d"
}

var defaultRuleSetURLs = map[string]string{
	"geosite-category-ads-all": "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
	"geosite-cn":               "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs",
	"geoip-cn":                 "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs",
	"geosite-geolocation-!cn":  "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-geolocation-!cn.srs",
	"geosite-openai":           "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-openai.srs",
	"geosite-netflix":          "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-netflix.srs",
	"geosite-youtube":          "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-youtube.srs",
	"geosite-google":           "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-google.srs",
	"geosite-telegram":         "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-telegram.srs",
	"geosite-github":           "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-github.srs",
	"geosite-bilibili":         "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-bilibili.srs",
}

func getRuleSetURL(tag string, custom map[string]string) string {
	if custom != nil {
		if u, ok := custom[tag]; ok && strings.TrimSpace(u) != "" {
			trimmedURL := strings.TrimSpace(u)
			if err := netutil.ValidatePublicURL(trimmedURL); err == nil {
				return trimmedURL
			}
		}
	}
	if u, ok := defaultRuleSetURLs[tag]; ok {
		return u
	}
	if strings.HasPrefix(tag, "geoip-") {
		return "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/" + tag + ".srs"
	}
	if strings.HasPrefix(tag, "geosite-") {
		return "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/" + tag + ".srs"
	}
	return "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-" + tag + ".srs"
}

func splitAndTrim(s string) []string {
	var result []string
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	for _, f := range fields {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// GenerateServerConfig 根据数据库中的数据构建 sing-box 完整配置
func GenerateServerConfig() (*SingboxConfig, error) {
	// 1. 获取启用的成员
	rows, err := model.DB.Query("SELECT name, uuid FROM members WHERE enabled = 1")
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	var members []model.Member
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.Name, &m.UUID); err == nil {
			members = append(members, m)
		}
	}

	// 2. 获取启用的入站
	inboundRows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json FROM inbounds WHERE enabled = 1")
	if err != nil {
		return nil, fmt.Errorf("query inbounds: %w", err)
	}
	defer inboundRows.Close()

	cfg := &SingboxConfig{
		Log: &LogConfig{
			Level:     "info",
			Timestamp: true,
		},
		Outbounds: []map[string]interface{}{},
	}

	// 读取启用的出站规则 (按 priority ASC, id ASC)
	outboundRows, err := model.DB.Query("SELECT tag, type, server, server_port, spec_json FROM outbounds WHERE enabled = 1 ORDER BY priority ASC, id ASC")
	if err == nil {
		defer outboundRows.Close()
		for outboundRows.Next() {
			var tag, obType, server, specJSON string
			var serverPort int
			if err := outboundRows.Scan(&tag, &obType, &server, &serverPort, &specJSON); err != nil {
				continue
			}

			obMap := make(map[string]interface{})
			if obType == "custom" && strings.TrimSpace(specJSON) != "" {
				if err := json.Unmarshal([]byte(specJSON), &obMap); err == nil {
					customType, _ := obMap["type"].(string)
					if customType == "" || customType == "direct" || customType == "block" {
						continue // 禁止通过 custom 冒充或覆盖系统保留出站类型
					}
					obMap["tag"] = tag
					cfg.Outbounds = append(cfg.Outbounds, obMap)
					continue
				}
			}

			obMap["type"] = obType
			obMap["tag"] = tag

			if server != "" {
				obMap["server"] = server
			}
			if serverPort > 0 {
				obMap["server_port"] = serverPort
			}

			// 合并 spec_json 中的额外参数 (如 password, method, uuid, tls 等)
			if strings.TrimSpace(specJSON) != "" && specJSON != "{}" {
				var extra map[string]interface{}
				if err := json.Unmarshal([]byte(specJSON), &extra); err == nil {
					for k, v := range extra {
						if _, exists := obMap[k]; !exists {
							obMap[k] = v
						}
					}
				}
			}

			cfg.Outbounds = append(cfg.Outbounds, obMap)
		}
	}

	// 保证至少有 direct 和 block 出站
	hasDirect := false
	hasBlock := false
	for _, ob := range cfg.Outbounds {
		if t, ok := ob["type"].(string); ok {
			if t == "direct" {
				hasDirect = true
			} else if t == "block" {
				hasBlock = true
			}
		}
	}
	if !hasDirect {
		cfg.Outbounds = append([]map[string]interface{}{{"type": "direct", "tag": "direct"}}, cfg.Outbounds...)
	}
	if !hasBlock {
		cfg.Outbounds = append(cfg.Outbounds, map[string]interface{}{"type": "block", "tag": "block"})
	}

	for inboundRows.Next() {
		var ib model.Inbound
		if err := inboundRows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON); err != nil {
			continue
		}

		inboundCfg := InboundConfig{
			Type:       "vless",
			Tag:        ib.Tag,
			Listen:     ib.Listen,
			ListenPort: ib.Port,
		}

		switch ib.Protocol {
		case "vless-reality":
			var spec model.VLESSRealitySpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			flow := spec.Flow
			if flow == "" {
				flow = "xtls-rprx-vision"
			}

			// 填充用户
			for _, m := range members {
				inboundCfg.Users = append(inboundCfg.Users, InboundUser{
					Name: m.Name,
					UUID: m.UUID,
					Flow: flow,
				})
			}

			hsPort := spec.HandshakePort
			if hsPort == 0 {
				hsPort = 443
			}

			serverName := spec.ServerName
			if serverName == "" {
				serverName = "dl.google.com"
			}
			handshakeServer := spec.HandshakeServer
			if handshakeServer == "" {
				handshakeServer = serverName
			}

			inboundCfg.TLS = &InboundTLSConfig{
				Enabled:    true,
				ServerName: serverName,
				Reality: &InboundRealityConfig{
					Enabled: true,
					Handshake: map[string]interface{}{
						"server":      handshakeServer,
						"server_port": hsPort,
					},
					PrivateKey: spec.PrivateKey,
					ShortID:    []string{spec.ShortID},
				},
			}

		case "vless-ws":
			var spec model.VLESSWSSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			for _, m := range members {
				inboundCfg.Users = append(inboundCfg.Users, InboundUser{
					Name: m.Name,
					UUID: m.UUID,
				})
			}

			path := spec.Path
			if path == "" {
				path = "/ws"
			}
			inboundCfg.Transport = map[string]interface{}{
				"type": "ws",
				"path": path,
			}

		case "socks":
			inboundCfg.Type = "socks"
			var spec model.SocksSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			if strings.TrimSpace(spec.Username) != "" && strings.TrimSpace(spec.Password) != "" {
				inboundCfg.Users = []InboundUser{
					{
						Username: strings.TrimSpace(spec.Username),
						Password: strings.TrimSpace(spec.Password),
					},
				}
			}

		case "mixed", "http":
			inboundCfg.Type = "mixed"
			if ib.Protocol == "http" {
				inboundCfg.Type = "http"
			}
			var spec model.MixedSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			if strings.TrimSpace(spec.Username) != "" && strings.TrimSpace(spec.Password) != "" {
				inboundCfg.Users = []InboundUser{
					{
						Username: strings.TrimSpace(spec.Username),
						Password: strings.TrimSpace(spec.Password),
					},
				}
			}

		case "shadowsocks":
			inboundCfg.Type = "shadowsocks"
			var spec model.ShadowsocksInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			method := strings.TrimSpace(spec.Method)
			if method == "" {
				method = "2022-blake3-aes-128-gcm"
			}
			inboundCfg.Method = method
			inboundCfg.Password = strings.TrimSpace(spec.Password)

		case "trojan":
			inboundCfg.Type = "trojan"
			var spec model.TrojanInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			pwd := strings.TrimSpace(spec.Password)
			if pwd != "" {
				inboundCfg.Users = []InboundUser{
					{
						Password: pwd,
					},
				}
			} else {
				for _, m := range members {
					inboundCfg.Users = append(inboundCfg.Users, InboundUser{
						Name:     m.Name,
						Password: m.UUID,
					})
				}
			}

			certPath := strings.TrimSpace(spec.CertificatePath)
			keyPath := strings.TrimSpace(spec.KeyPath)
			if certPath == "" || keyPath == "" {
				if defCert, defKey, err := EnsureDefaultTLSCert(); err == nil {
					certPath = defCert
					keyPath = defKey
				}
			}

			inboundCfg.TLS = &InboundTLSConfig{
				Enabled:         true,
				ServerName:      spec.ServerName,
				CertificatePath: certPath,
				KeyPath:         keyPath,
			}

		case "hysteria2":
			inboundCfg.Type = "hysteria2"
			var spec model.Hysteria2InboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			pwd := strings.TrimSpace(spec.Password)
			if pwd != "" {
				inboundCfg.Users = []InboundUser{
					{
						Password: pwd,
					},
				}
			} else {
				for _, m := range members {
					inboundCfg.Users = append(inboundCfg.Users, InboundUser{
						Name:     m.Name,
						Password: m.UUID,
					})
				}
			}

			if spec.UpMbps > 0 {
				inboundCfg.UpMbps = spec.UpMbps
			}
			if spec.DownMbps > 0 {
				inboundCfg.DownMbps = spec.DownMbps
			}
			if spec.IgnoreClientBandwidth {
				inboundCfg.IgnoreClientBandwidth = true
			}
			if strings.TrimSpace(spec.ObfsPassword) != "" {
				inboundCfg.Obfs = map[string]interface{}{
					"type":     "salamander",
					"password": strings.TrimSpace(spec.ObfsPassword),
				}
			}

			certPath := strings.TrimSpace(spec.CertificatePath)
			keyPath := strings.TrimSpace(spec.KeyPath)
			if certPath == "" || keyPath == "" {
				if defCert, defKey, err := EnsureDefaultTLSCert(); err == nil {
					certPath = defCert
					keyPath = defKey
				}
			}

			inboundCfg.TLS = &InboundTLSConfig{
				Enabled:         true,
				ServerName:      spec.ServerName,
				CertificatePath: certPath,
				KeyPath:         keyPath,
			}

		case "vmess":
			inboundCfg.Type = "vmess"
			var spec model.VMessInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			for _, m := range members {
				inboundCfg.Users = append(inboundCfg.Users, InboundUser{
					Name: m.Name,
					UUID: m.UUID,
				})
			}

			if spec.Network == "ws" {
				path := spec.Path
				if path == "" {
					path = "/vmess"
				}
				inboundCfg.Transport = map[string]interface{}{
					"type": "ws",
					"path": path,
				}
			}

		case "tuic":
			inboundCfg.Type = "tuic"
			var spec model.TUICInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			pwd := strings.TrimSpace(spec.Password)
			if pwd == "" {
				pwd = "tuic-password"
			}
			for _, m := range members {
				inboundCfg.Users = append(inboundCfg.Users, InboundUser{
					Name:     m.Name,
					UUID:     m.UUID,
					Password: pwd,
				})
			}

			cc := strings.TrimSpace(spec.CongestionControl)
			if cc == "" {
				cc = "bbr"
			}
			inboundCfg.CongestionControl = cc

			certPath := strings.TrimSpace(spec.CertificatePath)
			keyPath := strings.TrimSpace(spec.KeyPath)
			if certPath == "" || keyPath == "" {
				if defCert, defKey, err := EnsureDefaultTLSCert(); err == nil {
					certPath = defCert
					keyPath = defKey
				}
			}

			inboundCfg.TLS = &InboundTLSConfig{
				Enabled:         true,
				ServerName:      spec.ServerName,
				CertificatePath: certPath,
				KeyPath:         keyPath,
			}
		}

		cfg.Inbounds = append(cfg.Inbounds, inboundCfg)
	}

	// 4. 构建符合 sing-box 1.14 最新语法的路由分流 (Route Rules & RuleSet)
	routeRules, err := model.ListRouteRules()
	if err == nil {
		routeCfg := &RouteConfig{
			Rules:               []map[string]interface{}{},
			AutoDetectInterface: true,
			Final:               "direct",
		}

		neededRuleSets := make(map[string]bool)
		customRuleSetURLs := make(map[string]string)

		for _, r := range routeRules {
			if !r.Enabled {
				continue
			}

			// 解析自定义规则集 URL (每行 tag=url)
			if strings.TrimSpace(r.RuleSetURLs) != "" {
				lines := strings.Split(r.RuleSetURLs, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if idx := strings.Index(line, "="); idx > 0 {
						k := strings.TrimSpace(line[:idx])
						v := strings.TrimSpace(line[idx+1:])
						if k != "" && v != "" {
							customRuleSetURLs[k] = v
						}
					}
				}
			}

			ruleMap := make(map[string]interface{})

			// 条件 1: 私有局域网 IP (sing-box 1.14 原生字段替代废弃的 geoip: private)
			if r.IPIsPrivate {
				ruleMap["ip_is_private"] = true
			}

			// 条件 2: 协议嗅探 (如 bittorrent)
			if strings.TrimSpace(r.Protocol) != "" {
				protos := splitAndTrim(r.Protocol)
				if len(protos) > 0 {
					ruleMap["protocol"] = protos
				}
			}

			// 条件 3: 域名规则 (完全匹配、后缀匹配、关键词匹配)
			if strings.TrimSpace(r.Domains) != "" {
				domains := splitAndTrim(r.Domains)
				if len(domains) > 0 {
					ruleMap["domain"] = domains
				}
			}
			if strings.TrimSpace(r.DomainSuffix) != "" {
				suffixes := splitAndTrim(r.DomainSuffix)
				if len(suffixes) > 0 {
					ruleMap["domain_suffix"] = suffixes
				}
			}
			if strings.TrimSpace(r.DomainKeyword) != "" {
				kw := splitAndTrim(r.DomainKeyword)
				if len(kw) > 0 {
					ruleMap["domain_keyword"] = kw
				}
			}

			// 条件 4: IP / CIDR 网段
			if strings.TrimSpace(r.IPCIDR) != "" {
				cidrs := splitAndTrim(r.IPCIDR)
				if len(cidrs) > 0 {
					ruleMap["ip_cidr"] = cidrs
				}
			}

			// 条件 5: 端口 / 端口范围
			if strings.TrimSpace(r.Port) != "" {
				var ports []int
				for _, pStr := range splitAndTrim(r.Port) {
					var pInt int
					if _, err := fmt.Sscanf(pStr, "%d", &pInt); err == nil && pInt > 0 {
						ports = append(ports, pInt)
					}
				}
				if len(ports) > 0 {
					ruleMap["port"] = ports
				}
			}
			if strings.TrimSpace(r.PortRange) != "" {
				ranges := splitAndTrim(r.PortRange)
				if len(ranges) > 0 {
					ruleMap["port_range"] = ranges
				}
			}

			// 条件 6: 入站限定
			if strings.TrimSpace(r.InboundTags) != "" {
				inbounds := splitAndTrim(r.InboundTags)
				if len(inbounds) > 0 {
					ruleMap["inbound"] = inbounds
				}
			}

			// 条件 7: sing-box 1.14 标准规则集引用 (rule_set)
			if strings.TrimSpace(r.RuleSet) != "" {
				rsList := splitAndTrim(r.RuleSet)
				if len(rsList) > 0 {
					ruleMap["rule_set"] = rsList
					for _, rs := range rsList {
						neededRuleSets[rs] = true
					}
				}
			}

			// 条件 8: 反向匹配
			if r.Invert {
				ruleMap["invert"] = true
			}

			// 出站动作
			targetOutbound := strings.TrimSpace(r.OutboundTag)
			if targetOutbound == "" {
				targetOutbound = "direct"
			}
			ruleMap["outbound"] = targetOutbound

			if r.Action == "reject" || targetOutbound == "block" {
				ruleMap["action"] = "reject"
			}

			routeCfg.Rules = append(routeCfg.Rules, ruleMap)
		}

		// 为所有被引用的 rule_set 注入远程定义 (sing-box 1.14 规范)
		if len(neededRuleSets) > 0 {
			var ruleSetList []RuleSetConfig
			for rsTag := range neededRuleSets {
				url := getRuleSetURL(rsTag, customRuleSetURLs)
				ruleSetList = append(ruleSetList, RuleSetConfig{
					Tag:            rsTag,
					Type:           "remote",
					Format:         "binary",
					URL:            url,
					UpdateInterval: "1d",
				})
			}
			routeCfg.RuleSet = ruleSetList

			// sing-box 1.14: 根级定义 http_clients，route 中引用 default_http_client
			cfg.HTTPClients = []HTTPClientConfig{
				{Tag: "default-client"},
			}
			routeCfg.DefaultHTTPClient = "default-client"
		}

		cfg.Route = routeCfg
	}

	// 5. 还原导入的非受管全局块 (如 dns, experimental, ntp, certificate，实现无损保留)
	if unmanagedJSON, err := model.GetSetting("singbox_unmanaged_json"); err == nil && strings.TrimSpace(unmanagedJSON) != "" {
		var unmanaged map[string]interface{}
		if err := json.Unmarshal([]byte(unmanagedJSON), &unmanaged); err == nil {
			if dnsRaw, exists := unmanaged["dns"]; exists && cfg.DNS == nil {
				cfg.DNS = dnsRaw
			}
			if ntpRaw, exists := unmanaged["ntp"]; exists && cfg.NTP == nil {
				cfg.NTP = ntpRaw
			}
			if certRaw, exists := unmanaged["certificate"]; exists && cfg.Certificate == nil {
				cfg.Certificate = certRaw
			}
			if expRaw, exists := unmanaged["experimental"]; exists && cfg.Experimental == nil {
				cfg.Experimental = expRaw
			}
			if logRaw, ok := unmanaged["log"].(map[string]interface{}); ok && cfg.Log != nil {
				if lvl, okL := logRaw["level"].(string); okL && lvl != "" {
					cfg.Log.Level = lvl
				}
				if ts, okT := logRaw["timestamp"].(bool); okT {
					cfg.Log.Timestamp = ts
				}
			}
		}
	}

	return cfg, nil
}

// EnsureDefaultTLSCert 确保 sing-box 所需的自签名 TLS 证书文件已存在于宿主机磁盘
func EnsureDefaultTLSCert() (string, string, error) {
	cfgPath := singbox.GetConfigPath()
	dir := filepath.Dir(cfgPath)
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return certPath, keyPath, nil
		}
	}

	pubIP, _ := model.GetSetting("public_ip")
	hosts := []string{"localhost", "127.0.0.1"}
	if pubIP != "" {
		hosts = append(hosts, pubIP)
	}

	_, err := crypto.GenerateSelfSignedCert(certPath, keyPath, hosts)
	if err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

func probeSingboxHealth(driver *singbox.Driver) error {
	// 启动/重载后探活检查：多次检测服务是否异常退出或进入 crash loop
	for i := 0; i < 3; i++ {
		time.Sleep(150 * time.Millisecond)
		st, err := driver.Status()
		if err != nil {
			continue
		}
		if st.RunningStatus == "failed" {
			return fmt.Errorf("服务进入失败状态 (failed)")
		}
		if !st.Active && !component.IsProcessRunning(singbox.DriverName) {
			return fmt.Errorf("服务进程未保持运行")
		}
	}
	return nil
}

func rollbackSingbox(destPath string, oldConfig []byte, driver *singbox.Driver, wasActive bool) {
	cleanPath := filepath.Clean(destPath)
	if len(oldConfig) > 0 {
		/* #nosec G703 */
		_ = os.WriteFile(cleanPath, oldConfig, 0600)
		_ = os.Chmod(cleanPath, 0600)
	} else {
		_ = os.Remove(cleanPath)
	}
	if wasActive {
		_ = driver.ReloadOrRestart()
	}
}

// ApplyAndReload 将生成的配置写入并重启/重载 sing-box 服务
// 若配置语法校验不通过或启动探活失败，自动回滚至上一次的配置并恢复服务
func ApplyAndReload(destPath string) error {
	if destPath == "" {
		destPath = singbox.GetConfigPath()
	}

	cfg, err := GenerateServerConfig()
	if err != nil {
		return fmt.Errorf("生成服务端配置失败: %w", err)
	}

	rawJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	destPath = filepath.Clean(destPath)
	_ = os.MkdirAll(filepath.Dir(destPath), 0700)

	driver := singbox.NewDriver()
	installedVer, _ := driver.GetInstalledVersion()

	// 1. 若已安装 sing-box，使用安全临时文件 (0600 权限) 进行语法与语义合法性预检
	if installedVer != "" {
		tmpFile, err := os.CreateTemp("", "sb_chk_*.json")
		if err != nil {
			return fmt.Errorf("创建临时校验文件失败: %w", err)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(rawJSON); err != nil {
			tmpFile.Close()
			return fmt.Errorf("写入临时校验文件失败: %w", err)
		}
		_ = tmpFile.Close()

		if err := singbox.CheckConfig(tmpPath); err != nil {
			return fmt.Errorf("sing-box 配置语法预检失败: %w", err)
		}
	}

	// 2. 备份原配置与活跃状态
	oldConfig, _ := os.ReadFile(destPath)
	prevStatus, _ := driver.Status()
	wasActive := prevStatus.Active || component.IsProcessRunning(singbox.DriverName)

	// 3. 校验通过，写入目标配置文件 (0600: 仅所有者读写，严防本地低权限进程窥视 Reality 私钥与 UUID)
	if err := os.WriteFile(destPath, rawJSON, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	_ = os.Chmod(destPath, 0600)

	// 4. 重载服务并探活 (测试环境若设置 SKIP_SERVICE_RELOAD 则跳过 systemd 启停，但始终保留上一步的 sing-box check 语法预检)
	if installedVer != "" && os.Getenv("SKIP_SERVICE_RELOAD") != "1" {
		if err := driver.ReloadOrRestart(); err != nil {
			rollbackSingbox(destPath, oldConfig, driver, wasActive)
			return fmt.Errorf("sing-box 重启/重载失败，已自动回滚原配置: %w", err)
		}

		// 若原先或当前期望活跃，执行健康探活
		if wasActive {
			if err := probeSingboxHealth(driver); err != nil {
				rollbackSingbox(destPath, oldConfig, driver, wasActive)
				return fmt.Errorf("sing-box 启动探活失败 (%s)，已自动回滚原配置", err.Error())
			}
		}
	}

	// 5. 自动确保已启用入站节点的防火墙端口放行 (ufw / firewalld / iptables)
	currentDB := model.DB
	go EnsureFirewallForActiveInboundsWithDB(currentDB)

	return nil
}

// EnsureFirewallForActiveInbounds 自动同步并放行数据库中所有已启用入站节点的公网防火墙端口
func EnsureFirewallForActiveInbounds() {
	EnsureFirewallForActiveInboundsWithDB(model.DB)
}

// EnsureFirewallForActiveInboundsWithDB 使用传入的 DB 实例同步放行防火墙端口 (防全局变量并发 Data Race)
func EnsureFirewallForActiveInboundsWithDB(db *sql.DB) {
	if db == nil {
		return
	}
	rows, err := db.Query("SELECT port, protocol, listen FROM inbounds WHERE enabled = 1")
	if err != nil {
		return
	}
	defer rows.Close()

	var ports []netutil.InboundPortInfo
	for rows.Next() {
		var p netutil.InboundPortInfo
		if err := rows.Scan(&p.Port, &p.Protocol, &p.Listen); err == nil && p.Port > 0 {
			ports = append(ports, p)
		}
	}
	if len(ports) > 0 {
		netutil.EnsurePortsFirewall(ports)
	}
}

