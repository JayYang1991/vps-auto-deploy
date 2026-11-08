package subgen

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"vps-auto-deploy/internal/model"
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

// GenerateClashMetaConfig 为指定成员生成 Clash Meta (Mihomo) YAML 配置 (聚合全集群各节点并附带定制前缀)
func GenerateClashMetaConfig(member *model.Member) (string, error) {
	clusterInbounds, err := FetchAllClusterInbounds()
	if err != nil {
		return "", err
	}

	var proxies []map[string]interface{}
	var proxyNames []string
	prefIPs, _ := model.ListCFPreferredIPs("")

	for _, item := range clusterInbounds {
		ib := item.Inbound
		prefix := item.NodePrefix
		serverHost := item.ServerHost

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

			name := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)

			proxy := map[string]interface{}{
				"name":               name,
				"type":               "vless",
				"server":             serverHost,
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

			groupDomain := ResolveWSGroupDomain(&spec, serverHost, item.TunnelsMap)
			path := spec.Path
			if path == "" {
				path = "/ws"
			}

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
					proxyName := fmt.Sprintf("%s | %s-%s", prefix, ib.Tag, nodeName)

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
				name := fmt.Sprintf("%s | %s-Argo", prefix, ib.Tag)
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
	_ = allGroupProxies

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

	yamlBytes, err := yaml.Marshal(&cfg)
	if err != nil {
		return "", err
	}

	// 添加头部提示注释
	header := fmt.Sprintf("# VPS Auto Deploy - Clash.Meta Profile\n# User: %s\n# Generated at: %s\n\n",
		member.Name, strings.Repeat("-", 30))

	return header + string(yamlBytes), nil
}
