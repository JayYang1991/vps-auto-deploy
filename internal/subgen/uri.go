package subgen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
	"vps-auto-deploy/internal/pkg/netutil"
)

// ClusterInboundItem 聚合全集群单节点入站与元数据
type ClusterInboundItem struct {
	Inbound    model.Inbound
	Node       model.Node
	NodePrefix string
	ServerHost string
	TunnelsMap map[int64]string
}

// ResolveWSGroupDomain 按照优先级解析 WS 入站的分组域名
func ResolveWSGroupDomain(spec *model.VLESSWSSpec, defaultServerIP string, optionalTunnels ...map[int64]string) string {
	if spec != nil {
		if strings.TrimSpace(spec.GroupDomain) != "" {
			return strings.TrimSpace(spec.GroupDomain)
		}
		if strings.TrimSpace(spec.Host) != "" {
			return strings.TrimSpace(spec.Host)
		}
		if spec.TunnelID > 0 && len(optionalTunnels) > 0 && optionalTunnels[0] != nil {
			if d, ok := optionalTunnels[0][spec.TunnelID]; ok && strings.TrimSpace(d) != "" {
				return strings.TrimSpace(d)
			}
		}
		if spec.TunnelID > 0 {
			var domain string
			_ = model.DB.QueryRow("SELECT domain FROM tunnels WHERE id = ?", spec.TunnelID).Scan(&domain)
			if strings.TrimSpace(domain) != "" {
				return strings.TrimSpace(domain)
			}
		}
	}

	// 检查全局设置 cf_pref_host
	prefHost, _ := model.GetSetting("cf_pref_host")
	if strings.TrimSpace(prefHost) != "" {
		return strings.TrimSpace(prefHost)
	}

	// 检查是否有任何启用的有效隧道域名
	var anyDomain string
	_ = model.DB.QueryRow("SELECT domain FROM tunnels WHERE domain != '' AND enabled = 1 LIMIT 1").Scan(&anyDomain)
	if strings.TrimSpace(anyDomain) != "" {
		return strings.TrimSpace(anyDomain)
	}

	return defaultServerIP
}

// FetchAllClusterInbounds 收集全集群（主控本机 + 在线纳管端）的所有启用入站
func FetchAllClusterInbounds() ([]ClusterInboundItem, error) {
	nodes, err := model.ListNodes()
	if err != nil {
		return nil, err
	}

	masterPublicIP, _ := model.GetSetting("public_ip")
	if masterPublicIP == "" {
		masterPublicIP = netutil.GetPublicIP()
	}

	var items []ClusterInboundItem

	// 若没有任何节点记录（极端情况兜底），仅查本机
	if len(nodes) == 0 {
		nodes = []model.Node{
			{ID: 1, Name: "🏠 本机 (Master)", SubPrefix: "🇭🇰 本地主控", IsMaster: true, Status: "online"},
		}
	}

	for _, n := range nodes {
		prefix := strings.TrimSpace(n.SubPrefix)
		if prefix == "" {
			prefix = strings.TrimSpace(n.Name)
		}
		if prefix == "" {
			if n.IsMaster {
				prefix = "🇭🇰 本地主控"
			} else {
				prefix = "节点"
			}
		}

		serverHost := strings.TrimSpace(n.Host)
		if serverHost == "" || serverHost == "127.0.0.1" || serverHost == "localhost" {
			serverHost = masterPublicIP
		}

		if n.IsMaster {
			inboundRows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json FROM inbounds WHERE enabled = 1 ORDER BY port ASC")
			if err != nil {
				continue
			}
			tunnelsMap := make(map[int64]string)
			tRows, tErr := model.DB.Query("SELECT id, domain FROM tunnels WHERE enabled = 1 AND domain != ''")
			if tErr == nil {
				for tRows.Next() {
					var tID int64
					var domain string
					if err := tRows.Scan(&tID, &domain); err == nil {
						tunnelsMap[tID] = domain
					}
				}
				tRows.Close()
			}

			for inboundRows.Next() {
				var ib model.Inbound
				if err := inboundRows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON); err == nil {
					ib.Enabled = true
					items = append(items, ClusterInboundItem{
						Inbound:    ib,
						Node:       n,
						NodePrefix: prefix,
						ServerHost: serverHost,
						TunnelsMap: tunnelsMap,
					})
				}
			}
			inboundRows.Close()
		} else {
			// 远程节点：仅在 online 状态下探测
			if n.Status != "online" {
				continue
			}
			remoteInbounds, remoteTunnels, fetchErr := node.FetchRemoteNodeInventory(&n)
			if fetchErr != nil {
				log.Printf("[SubGen] 抓取远端节点 %s (%s) 入站失败: %v", n.Name, n.Host, fetchErr)
				continue
			}
			for _, ib := range remoteInbounds {
				if ib.Enabled {
					items = append(items, ClusterInboundItem{
						Inbound:    ib,
						Node:       n,
						NodePrefix: prefix,
						ServerHost: serverHost,
						TunnelsMap: remoteTunnels,
					})
				}
			}
		}
	}

	return items, nil
}

// GenerateURIs 为指定成员生成全集群所有启用节点的链接列表 (支持定制节点前缀与优选 IP 裂变)
func GenerateURIs(member *model.Member) ([]string, error) {
	clusterInbounds, err := FetchAllClusterInbounds()
	if err != nil {
		return nil, err
	}

	var uris []string

	// 查询已保存的 Cloudflare 优选 IP 列表
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

			sni := spec.ServerName
			if sni == "" {
				sni = "dl.google.com"
			}

			remark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)

			q := url.Values{}
			q.Set("encryption", "none")
			q.Set("flow", flow)
			q.Set("security", "reality")
			q.Set("sni", sni)
			q.Set("fp", fp)
			q.Set("pbk", spec.PublicKey)
			q.Set("sid", spec.ShortID)
			q.Set("type", "tcp")

			u := fmt.Sprintf("vless://%s@%s:%d?%s#%s", member.UUID, serverHost, ib.Port, q.Encode(), url.QueryEscape(remark))
			uris = append(uris, u)

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

					q := url.Values{}
					q.Set("encryption", "none")
					q.Set("security", "tls")
					q.Set("sni", groupDomain)
					q.Set("type", "ws")
					q.Set("host", groupDomain)
					q.Set("path", path)

					nodeName := p.Name
					if nodeName == "" {
						nodeName = fmt.Sprintf("%s-%s", p.IP, p.Colo)
					}
					nodeRemark := fmt.Sprintf("%s | %s-%s", prefix, ib.Tag, nodeName)

					u := fmt.Sprintf("vless://%s@%s:%d?%s#%s", member.UUID, p.IP, targetPort, q.Encode(), url.QueryEscape(nodeRemark))
					uris = append(uris, u)
				}
			} else {
				q := url.Values{}
				q.Set("encryption", "none")
				q.Set("security", "tls")
				q.Set("sni", groupDomain)
				q.Set("type", "ws")
				q.Set("host", groupDomain)
				q.Set("path", path)

				nodeRemark := fmt.Sprintf("%s | %s-Argo", prefix, ib.Tag)
				u := fmt.Sprintf("vless://%s@%s:443?%s#%s", member.UUID, groupDomain, q.Encode(), url.QueryEscape(nodeRemark))
				uris = append(uris, u)
			}

		case "hysteria2":
			nodeRemark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			u := fmt.Sprintf("hysteria2://%s@%s:%d?sni=%s&insecure=0#%s", member.UUID, serverHost, ib.Port, serverHost, url.QueryEscape(nodeRemark))
			uris = append(uris, u)

		case "socks":
			var spec model.SocksSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			remark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			var u string
			if spec.Username != "" && spec.Password != "" {
				u = fmt.Sprintf("socks5://%s:%s@%s:%d#%s", url.QueryEscape(spec.Username), url.QueryEscape(spec.Password), serverHost, ib.Port, url.QueryEscape(remark))
			} else {
				u = fmt.Sprintf("socks5://%s:%d#%s", serverHost, ib.Port, url.QueryEscape(remark))
			}
			uris = append(uris, u)

		default:
			nodeRemark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			u := fmt.Sprintf("vless://%s@%s:%d#%s", member.UUID, serverHost, ib.Port, url.QueryEscape(nodeRemark))
			uris = append(uris, u)
		}
	}

	return uris, nil
}

// GenerateBase64Sub 生成通用 Base64 订阅内容 (适配 Shadowrocket, v2rayN)
func GenerateBase64Sub(member *model.Member) (string, error) {
	uris, err := GenerateURIs(member)
	if err != nil {
		return "", err
	}
	joined := strings.Join(uris, "\n")
	return base64.StdEncoding.EncodeToString([]byte(joined)), nil
}
