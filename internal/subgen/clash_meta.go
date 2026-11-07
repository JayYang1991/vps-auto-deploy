package subgen

import (
	"encoding/json"
	"fmt"
	"strings"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
	"gopkg.in/yaml.v3"
)

type ClashConfig struct {
	Port               int                      `yaml:"port"`
	SocksPort          int                      `yaml:"socks-port"`
	MixedPort          int                      `yaml:"mixed-port"`
	AllowLan           bool                     `yaml:"allow-lan"`
	Mode               string                   `yaml:"mode"`
	LogLevel           string                   `yaml:"log-level"`
	IPv6               bool                     `yaml:"ipv6"`
	ExternalController string                   `yaml:"external-controller"`
	Proxies            []map[string]interface{} `yaml:"proxies"`
	ProxyGroups        []map[string]interface{} `yaml:"proxy-groups"`
	Rules              []string                 `yaml:"rules"`
}

// GenerateClashMetaConfig 为指定成员生成 Clash Meta (Mihomo) YAML 配置
func GenerateClashMetaConfig(member *model.Member) (string, error) {
	// 获取主机的公网 IP
	serverIP, _ := model.GetSetting("public_ip")
	if serverIP == "" {
		serverIP = netutil.GetPublicIP()
	}

	// 查询所有启用的入站
	rows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json FROM inbounds WHERE enabled = 1")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var proxies []map[string]interface{}
	var proxyNames []string

	for rows.Next() {
		var ib model.Inbound
		if err := rows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON); err != nil {
			continue
		}

		name := fmt.Sprintf("%s-%d", ib.Tag, ib.Port)

		switch ib.Protocol {
		case "vless-reality":
			var spec model.VLESSRealitySpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			flow := spec.Flow
			if flow == "" {
				flow = "xtls-rprx-vision"
			}
			fp := spec.Fingerprint
			if fp == "" {
				fp = "chrome"
			}

			serverName := spec.ServerName
			if serverName == "" {
				serverName = "dl.google.com"
			}

			proxy := map[string]interface{}{
				"name":               name,
				"type":               "vless",
				"server":             serverIP,
				"port":               ib.Port,
				"uuid":               member.UUID,
				"network":            "tcp",
				"tls":                true,
				"udp":                true,
				"flow":               flow,
				"servername":         serverName,
				"client-fingerprint": fp,
				"reality-opts": map[string]interface{}{
					"public-key": spec.PublicKey,
					"short-id":   spec.ShortID,
				},
			}
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)

		case "vless-ws":
			var spec model.VLESSWSSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			groupDomain := ResolveWSGroupDomain(&spec, serverIP)
			path := spec.Path
			if path == "" {
				path = "/ws"
			}

			prefIPs, _ := model.ListCFPreferredIPs("")
			if len(prefIPs) > 0 {
				for _, p := range prefIPs {
					targetPort := p.Port
					if targetPort <= 0 {
						targetPort = 443
					}

					nodeName := p.Name
					if nodeName == "" {
						nodeName = fmt.Sprintf("%s-%s", p.IP, p.Colo)
					}
					proxyName := fmt.Sprintf("%s-%s", ib.Tag, nodeName)

					proxy := map[string]interface{}{
						"name":       proxyName,
						"type":       "vless",
						"server":     p.IP,
						"port":       targetPort,
						"uuid":       member.UUID,
						"network":    "ws",
						"tls":        true,
						"udp":        true,
						"servername": groupDomain,
						"ws-opts": map[string]interface{}{
							"path": path,
							"headers": map[string]interface{}{
								"Host": groupDomain,
							},
						},
					}
					proxies = append(proxies, proxy)
					proxyNames = append(proxyNames, proxyName)
				}
			} else {
				proxy := map[string]interface{}{
					"name":       name,
					"type":       "vless",
					"server":     groupDomain,
					"port":       443,
					"uuid":       member.UUID,
					"network":    "ws",
					"tls":        true,
					"udp":        true,
					"servername": groupDomain,
					"ws-opts": map[string]interface{}{
						"path": path,
						"headers": map[string]interface{}{
							"Host": groupDomain,
						},
					},
				}
				proxies = append(proxies, proxy)
				proxyNames = append(proxyNames, name)
			}
		}
	}

	if len(proxies) == 0 {
		return "# No active inbounds available\n", nil
	}

	// 组装 Proxy Groups
	allGroupProxies := append([]string{"DIRECT"}, proxyNames...)
	proxyGroups := []map[string]interface{}{
		{
			"name":    "PROXY",
			"type":    "select",
			"proxies": proxyNames,
		},
		{
			"name":     "AUTO",
			"type":     "url-test",
			"url":      "https://www.gstatic.com/generate_204",
			"interval": 300,
			"proxies":  proxyNames,
		},
		{
			"name":    "FALLBACK",
			"type":    "fallback",
			"url":     "https://www.gstatic.com/generate_204",
			"interval": 300,
			"proxies":  proxyNames,
		},
	}

	rules := []string{
		"GEOIP,LAN,DIRECT,no-resolve",
		"GEOIP,CN,DIRECT",
		"MATCH,PROXY",
	}

	cfg := ClashConfig{
		Port:               7890,
		SocksPort:          7891,
		MixedPort:          7892,
		AllowLan:           true,
		Mode:               "rule",
		LogLevel:           "info",
		IPv6:               false,
		ExternalController: "127.0.0.1:9090",
		Proxies:            proxies,
		ProxyGroups:        proxyGroups,
		Rules:              rules,
	}

	// 补充全局通用组
	_ = allGroupProxies

	yamlBytes, err := yaml.Marshal(&cfg)
	if err != nil {
		return "", err
	}

	// 添加头部提示注释
	header := fmt.Sprintf("# VPS Auto Deploy - Clash.Meta Profile\n# User: %s\n# Generated at: %s\n\n",
		member.Name, strings.Repeat("-", 30))

	return header + string(yamlBytes), nil
}

