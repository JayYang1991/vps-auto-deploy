package subgen

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"vps-auto-deploy/internal/component/singbox"
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

			pbk := spec.PublicKey
			if pbk == "" && spec.PrivateKey != "" {
				pbk = singbox.DeriveRealityPublicKey(spec.PrivateKey)
			}
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
					"public-key": pbk,
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

		case "socks", "mixed", "http":
			var spec model.SocksSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			name := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			proxy := map[string]interface{}{
				"name":   name,
				"type":   "socks5",
				"server": serverHost,
				"port":   ib.Port,
			}
			if spec.Username != "" && spec.Password != "" {
				proxy["username"] = spec.Username
				proxy["password"] = spec.Password
			}
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)

		case "shadowsocks":
			var spec model.ShadowsocksInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			method := spec.Method
			if method == "" {
				method = "2022-blake3-aes-128-gcm"
			}
			name := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			proxy := map[string]interface{}{
				"name":     name,
				"type":     "ss",
				"server":   serverHost,
				"port":     ib.Port,
				"cipher":   method,
				"password": spec.Password,
			}
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)

		case "trojan":
			var spec model.TrojanInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			pwd := spec.Password
			if pwd == "" {
				pwd = member.UUID
			}
			sni := spec.ServerName
			if sni == "" {
				sni = serverHost
			}
			name := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			proxy := map[string]interface{}{
				"name":             name,
				"type":             "trojan",
				"server":           serverHost,
				"port":             ib.Port,
				"password":         pwd,
				"sni":              sni,
				"skip-cert-verify": true,
			}
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)

		case "hysteria2":
			var spec model.Hysteria2InboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			pwd := spec.Password
			if pwd == "" {
				pwd = member.UUID
			}
			sni := spec.ServerName
			if sni == "" {
				sni = serverHost
			}
			name := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			proxy := map[string]interface{}{
				"name":             name,
				"type":             "hysteria2",
				"server":           serverHost,
				"port":             ib.Port,
				"password":         pwd,
				"sni":              sni,
				"skip-cert-verify": true,
			}
			if spec.ObfsPassword != "" {
				proxy["obfs"] = "salamander"
				proxy["obfs-password"] = spec.ObfsPassword
			}
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)

		case "vmess":
			var spec model.VMessInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			name := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			proxy := map[string]interface{}{
				"name":     name,
				"type":     "vmess",
				"server":   serverHost,
				"port":     ib.Port,
				"uuid":     member.UUID,
				"alterId":  0,
				"cipher":   "auto",
				"network":  "tcp",
			}
			if spec.Network == "ws" {
				path := spec.Path
				if path == "" {
					path = "/vmess"
				}
				proxy["network"] = "ws"
				proxy["ws-opts"] = map[string]interface{}{
					"path": path,
				}
			}
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)

		case "tuic":
			var spec model.TUICInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			pwd := spec.Password
			if pwd == "" {
				pwd = "tuic-password"
			}
			cc := spec.CongestionControl
			if cc == "" {
				cc = "bbr"
			}
			sni := spec.ServerName
			if sni == "" {
				sni = serverHost
			}
			name := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			proxy := map[string]interface{}{
				"name":                  name,
				"type":                  "tuic",
				"server":                serverHost,
				"port":                  ib.Port,
				"uuid":                  member.UUID,
				"password":              pwd,
				"congestion-controller": cc,
				"sni":                   sni,
				"skip-cert-verify":      true,
			}
			proxies = append(proxies, proxy)
			proxyNames = append(proxyNames, name)
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
