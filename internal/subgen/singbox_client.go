package subgen

import (
	"encoding/json"
	"fmt"

	"vps-auto-deploy/internal/model"
)

type SingboxClientConfig struct {
	Outbounds []map[string]interface{} `json:"outbounds"`
}

// GenerateSingboxClientConfig 为指定成员生成 sing-box 客户端配置 JSON (聚合全集群各节点并附带定制前缀)
func GenerateSingboxClientConfig(member *model.Member) (string, error) {
	clusterInbounds, err := FetchAllClusterInbounds()
	if err != nil {
		return "", err
	}

	var outbounds []map[string]interface{}
	var outboundTags []string
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

			tag := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)

			outbound := map[string]interface{}{
				"type":        "vless",
				"tag":         tag,
				"server":      serverHost,
				"server_port": ib.Port,
				"uuid":        member.UUID,
				"flow":        flow,
				"tls": map[string]interface{}{
					"enabled":     true,
					"server_name": serverName,
					"utls": map[string]interface{}{
						"enabled":     true,
						"fingerprint": fp,
					},
					"reality": map[string]interface{}{
						"enabled":    true,
						"public_key": spec.PublicKey,
						"short-id":   spec.ShortID,
					},
				},
			}
			outbounds = append(outbounds, outbound)
			outboundTags = append(outboundTags, tag)

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
					nodeTag := fmt.Sprintf("%s | %s-%s", prefix, ib.Tag, nodeName)

					outbound := map[string]interface{}{
						"type":        "vless",
						"tag":         nodeTag,
						"server":      p.IP,
						"server_port": targetPort,
						"uuid":        member.UUID,
						"tls": map[string]interface{}{
							"enabled":     true,
							"server_name": groupDomain,
						},
						"transport": map[string]interface{}{
							"type": "ws",
							"path": path,
							"headers": map[string]string{
								"Host": groupDomain,
							},
						},
					}
					outbounds = append(outbounds, outbound)
					outboundTags = append(outboundTags, nodeTag)
				}
			} else {
				tag := fmt.Sprintf("%s | %s-Argo", prefix, ib.Tag)
				outbound := map[string]interface{}{
					"type":        "vless",
					"tag":         tag,
					"server":      groupDomain,
					"server_port": 443,
					"uuid":        member.UUID,
					"tls": map[string]interface{}{
						"enabled":     true,
						"server_name": groupDomain,
					},
					"transport": map[string]interface{}{
						"type": "ws",
						"path": path,
						"headers": map[string]string{
							"Host": groupDomain,
						},
					},
				}
				outbounds = append(outbounds, outbound)
				outboundTags = append(outboundTags, tag)
			}

		case "socks":
			var spec model.SocksSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			tag := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			outbound := map[string]interface{}{
				"type":        "socks",
				"tag":         tag,
				"server":      serverHost,
				"server_port": ib.Port,
			}
			if spec.Username != "" && spec.Password != "" {
				outbound["username"] = spec.Username
				outbound["password"] = spec.Password
			}
			outbounds = append(outbounds, outbound)
			outboundTags = append(outboundTags, tag)
		}
	}

	// 增加 selector 策略组
	if len(outboundTags) > 0 {
		selector := map[string]interface{}{
			"type":      "selector",
			"tag":       "proxy",
			"outbounds": outboundTags,
			"default":   outboundTags[0],
		}
		outbounds = append([]map[string]interface{}{selector}, outbounds...)
	}

	outbounds = append(outbounds,
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	)

	cfg := SingboxClientConfig{
		Outbounds: outbounds,
	}

	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
