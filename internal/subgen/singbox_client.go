package subgen

import (
	"encoding/json"
	"fmt"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
)

type SingboxClientConfig struct {
	Outbounds []map[string]interface{} `json:"outbounds"`
}

// GenerateSingboxClientConfig 为指定成员生成 sing-box 客户端配置 JSON
func GenerateSingboxClientConfig(member *model.Member) (string, error) {
	serverIP, _ := model.GetSetting("public_ip")
	if serverIP == "" {
		serverIP = netutil.GetPublicIP()
	}

	rows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json FROM inbounds WHERE enabled = 1")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var outbounds []map[string]interface{}
	var outboundTags []string

	for rows.Next() {
		var ib model.Inbound
		if err := rows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON); err != nil {
			continue
		}

		tag := fmt.Sprintf("%s-%d", ib.Tag, ib.Port)

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

			outbound := map[string]interface{}{
				"type":        "vless",
				"tag":         tag,
				"server":      serverIP,
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
						"short_id":   spec.ShortID,
					},
				},
			}
			outbounds = append(outbounds, outbound)
			outboundTags = append(outboundTags, tag)

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
					nodeTag := fmt.Sprintf("%s-%s", ib.Tag, nodeName)

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

