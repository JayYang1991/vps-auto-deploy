package configgen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
)

// RelayCandidateInbound 集群中可用于作为中转目标的入站
type RelayCandidateInbound struct {
	ID           int64  `json:"id"`
	Tag          string `json:"tag"`
	Protocol     string `json:"protocol"`
	Port         int    `json:"port"`
	SpecJSON     string `json:"spec_json"`
	TunnelDomain string `json:"tunnel_domain,omitempty"`
	Summary      string `json:"summary"`
}

// RelayCandidateNode 集群中可用的中转目标节点
type RelayCandidateNode struct {
	NodeID   int64                   `json:"node_id"`
	NodeName string                  `json:"node_name"`
	Host     string                  `json:"host"`
	Inbounds []RelayCandidateInbound `json:"inbounds"`
}

// GetRelayCandidates 获取当前操作节点之外的全集群可用入站清单（供配置中转 Outbound 使用）
func GetRelayCandidates(excludeNodeID int64) ([]RelayCandidateNode, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("list nodes failed: %w", err)
	}

	var candidates []RelayCandidateNode

	for _, n := range nodes {
		// 严防自回环死锁：严格排除当前节点自身
		if n.ID == excludeNodeID {
			continue
		}

		// 离线节点跳过
		if !n.IsMaster && n.Status != "online" && n.Status != "" {
			continue
		}

		var candidateInbounds []RelayCandidateInbound

		if n.IsMaster {
			// 查询主控本地入站
			rows, err := model.DB.Query("SELECT id, tag, protocol, port, spec_json FROM inbounds WHERE enabled = 1 ORDER BY id ASC")
			if err == nil {
				// 获取主控本地 tunnels
				tunnelsMap := make(map[int64]string)
				tRows, tErr := model.DB.Query("SELECT id, domain FROM tunnels WHERE enabled = 1 AND domain != ''")
				if tErr == nil {
					for tRows.Next() {
						var tID int64
						var tDomain string
						if err := tRows.Scan(&tID, &tDomain); err == nil {
							tunnelsMap[tID] = tDomain
						}
					}
					tRows.Close()
				}

				for rows.Next() {
					var ib model.Inbound
					if err := rows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Port, &ib.SpecJSON); err == nil {
						tunnelDomain := extractTunnelDomain(ib.Protocol, ib.SpecJSON, tunnelsMap)
						candidateInbounds = append(candidateInbounds, RelayCandidateInbound{
							ID:           ib.ID,
							Tag:          ib.Tag,
							Protocol:     ib.Protocol,
							Port:         ib.Port,
							SpecJSON:     ib.SpecJSON,
							TunnelDomain: tunnelDomain,
							Summary:      formatInboundSummary(ib.Protocol, ib.Port, ib.SpecJSON, tunnelDomain),
						})
					}
				}
				rows.Close()
			}
		} else {
			// 远程节点通过安全信道抓取 inventory
			remoteInbounds, remoteTunnels, err := node.FetchRemoteNodeInventory(&n)
			if err == nil {
				for _, ib := range remoteInbounds {
					if !ib.Enabled {
						continue
					}
					tunnelDomain := extractTunnelDomain(ib.Protocol, ib.SpecJSON, remoteTunnels)
					candidateInbounds = append(candidateInbounds, RelayCandidateInbound{
						ID:           ib.ID,
						Tag:          ib.Tag,
						Protocol:     ib.Protocol,
						Port:         ib.Port,
						SpecJSON:     ib.SpecJSON,
						TunnelDomain: tunnelDomain,
						Summary:      formatInboundSummary(ib.Protocol, ib.Port, ib.SpecJSON, tunnelDomain),
					})
				}
			}
		}

		if len(candidateInbounds) > 0 {
			candidates = append(candidates, RelayCandidateNode{
				NodeID:   n.ID,
				NodeName: n.Name,
				Host:     n.Host,
				Inbounds: candidateInbounds,
			})
		}
	}

	return candidates, nil
}

func extractTunnelDomain(protocol, specJSON string, tunnelsMap map[int64]string) string {
	if protocol != "vless-ws" {
		return ""
	}
	var spec model.VLESSWSSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return ""
	}
	if spec.GroupDomain != "" {
		return spec.GroupDomain
	}
	if spec.TunnelID > 0 && tunnelsMap != nil {
		if d, ok := tunnelsMap[spec.TunnelID]; ok && d != "" {
			return d
		}
	}
	if spec.Host != "" {
		return spec.Host
	}
	return ""
}

func formatInboundSummary(protocol string, port int, specJSON, tunnelDomain string) string {
	switch protocol {
	case "vless-reality":
		var spec model.VLESSRealitySpec
		_ = json.Unmarshal([]byte(specJSON), &spec)
		sni := spec.ServerName
		if sni == "" {
			sni = "dl.google.com"
		}
		return fmt.Sprintf("VLESS Reality (端口: %d, 伪装: %s)", port, sni)
	case "vless-ws":
		var spec model.VLESSWSSpec
		_ = json.Unmarshal([]byte(specJSON), &spec)
		path := spec.Path
		if path == "" {
			path = "/ws"
		}
		if tunnelDomain != "" {
			return fmt.Sprintf("VLESS WS (隧道域名: %s, 路径: %s)", tunnelDomain, path)
		}
		return fmt.Sprintf("VLESS WS (端口: %d, 路径: %s)", port, path)
	case "hysteria2":
		var spec model.Hysteria2InboundSpec
		_ = json.Unmarshal([]byte(specJSON), &spec)
		sni := spec.ServerName
		if sni == "" {
			sni = "自签/自动"
		}
		return fmt.Sprintf("Hysteria 2 极速 (端口: %d, SNI: %s)", port, sni)
	case "trojan":
		return fmt.Sprintf("Trojan (端口: %d)", port)
	case "shadowsocks":
		var spec model.ShadowsocksInboundSpec
		_ = json.Unmarshal([]byte(specJSON), &spec)
		return fmt.Sprintf("Shadowsocks (端口: %d, 加密: %s)", port, spec.Method)
	case "socks":
		return fmt.Sprintf("SOCKS5 (端口: %d)", port)
	case "mixed":
		return fmt.Sprintf("Mixed SOCKS/HTTP (端口: %d)", port)
	case "vmess":
		return fmt.Sprintf("VMess (端口: %d)", port)
	case "tuic":
		return fmt.Sprintf("TUIC v5 (端口: %d)", port)
	default:
		return fmt.Sprintf("%s (端口: %d)", protocol, port)
	}
}

// BuildRelayOutbound 根据目标节点、目标入站与配置偏好，自动生成标准的 Sing-box Outbound
func BuildRelayOutbound(targetNode *model.Node, inbound *model.Inbound, tunnelDomain string, addrType string, memberUUID string) (*model.Outbound, error) {
	if targetNode == nil || inbound == nil {
		return nil, fmt.Errorf("target node or inbound is nil")
	}

	// 1. 获取认证 UUID（优先使用传入的成员 UUID，其次主控库中首个启用的用户，兜底默认 UUID）
	if memberUUID == "" {
		var u string
		_ = model.DB.QueryRow("SELECT uuid FROM members WHERE enabled = 1 ORDER BY id ASC LIMIT 1").Scan(&u)
		if u != "" {
			memberUUID = u
		} else {
			memberUUID = "c3a6027a-7d96-4018-a5d9-34c2e0fbee37"
		}
	}

	// 2. 确定连接地址与端口
	server := targetNode.Host
	serverPort := inbound.Port
	if addrType == "tunnel" && tunnelDomain != "" {
		server = tunnelDomain
		serverPort = 443
	}

	cleanNodeName := sanitizeTagName(targetNode.Name)
	cleanProtocol := strings.ReplaceAll(inbound.Protocol, "-", "_")
	suggestedTag := fmt.Sprintf("relay_%s_%s_%d", cleanNodeName, cleanProtocol, serverPort)

	var obType string
	specMap := make(map[string]interface{})

	switch inbound.Protocol {
	case "vless-reality":
		obType = "vless"
		var spec model.VLESSRealitySpec
		_ = json.Unmarshal([]byte(inbound.SpecJSON), &spec)

		pubKey := spec.PublicKey
		if pubKey == "" && spec.PrivateKey != "" {
			pubKey = singbox.DeriveRealityPublicKey(spec.PrivateKey)
		}

		flow := spec.Flow
		if flow == "" {
			flow = "xtls-rprx-vision"
		}
		sni := spec.ServerName
		if sni == "" {
			sni = "dl.google.com"
		}
		fp := spec.Fingerprint
		if fp == "" {
			fp = "chrome"
		}

		specMap["uuid"] = memberUUID
		specMap["flow"] = flow
		specMap["tls"] = map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": pubKey,
				"short_id":   spec.ShortID,
			},
		}

	case "vless-ws":
		obType = "vless"
		var spec model.VLESSWSSpec
		_ = json.Unmarshal([]byte(inbound.SpecJSON), &spec)

		path := spec.Path
		if path == "" {
			path = "/ws"
		}
		host := server
		if spec.Host != "" {
			host = spec.Host
		} else if tunnelDomain != "" {
			host = tunnelDomain
		}

		specMap["uuid"] = memberUUID
		specMap["transport"] = map[string]interface{}{
			"type": "ws",
			"path": path,
			"headers": map[string]interface{}{
				"Host": host,
			},
		}
		specMap["tls"] = map[string]interface{}{
			"enabled":     true,
			"server_name": host,
		}

	case "hysteria2":
		obType = "hysteria2"
		var spec model.Hysteria2InboundSpec
		_ = json.Unmarshal([]byte(inbound.SpecJSON), &spec)

		pwd := spec.Password
		if pwd == "" {
			pwd = memberUUID
		}
		sni := spec.ServerName
		if sni == "" {
			sni = server
		}

		specMap["password"] = pwd
		specMap["tls"] = map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"insecure":    true,
		}
		if spec.ObfsPassword != "" {
			specMap["obfs"] = map[string]interface{}{
				"type":     "salamander",
				"password": spec.ObfsPassword,
			}
		}

	case "trojan":
		obType = "trojan"
		var spec model.TrojanInboundSpec
		_ = json.Unmarshal([]byte(inbound.SpecJSON), &spec)

		pwd := spec.Password
		if pwd == "" {
			pwd = memberUUID
		}
		sni := spec.ServerName
		if sni == "" {
			sni = server
		}

		specMap["password"] = pwd
		specMap["tls"] = map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"insecure":    true,
		}

	case "shadowsocks":
		obType = "shadowsocks"
		var spec model.ShadowsocksInboundSpec
		_ = json.Unmarshal([]byte(inbound.SpecJSON), &spec)

		method := spec.Method
		if method == "" {
			method = "2022-blake3-aes-128-gcm"
		}
		specMap["method"] = method
		specMap["password"] = spec.Password

	case "socks", "mixed":
		obType = "socks"
		var spec model.SocksSpec
		_ = json.Unmarshal([]byte(inbound.SpecJSON), &spec)

		if spec.Username != "" && spec.Password != "" {
			specMap["username"] = spec.Username
			specMap["password"] = spec.Password
		}

	case "vmess":
		obType = "vmess"
		specMap["uuid"] = memberUUID
		specMap["security"] = "auto"

	case "tuic":
		obType = "tuic"
		var spec model.TUICInboundSpec
		_ = json.Unmarshal([]byte(inbound.SpecJSON), &spec)

		pwd := spec.Password
		if pwd == "" {
			pwd = memberUUID
		}
		sni := spec.ServerName
		if sni == "" {
			sni = server
		}
		cc := spec.CongestionControl
		if cc == "" {
			cc = "bbr"
		}

		specMap["uuid"] = memberUUID
		specMap["password"] = pwd
		specMap["congestion_control"] = cc
		specMap["tls"] = map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"insecure":    true,
		}

	default:
		obType = inbound.Protocol
	}

	specBytes, _ := json.MarshalIndent(specMap, "", "  ")

	return &model.Outbound{
		Tag:        suggestedTag,
		Type:       obType,
		Server:     server,
		ServerPort: serverPort,
		SpecJSON:   string(specBytes),
		Priority:   10,
		Enabled:    true,
		IsSystem:   false,
	}, nil
}

func sanitizeTagName(name string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	cleaned := reg.ReplaceAllString(name, "")
	if cleaned == "" {
		cleaned = "node"
	}
	return strings.ToLower(cleaned)
}
