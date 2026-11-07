package subgen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
)

// ResolveWSGroupDomain 按照优先级解析 WS 入站的分组域名
func ResolveWSGroupDomain(spec *model.VLESSWSSpec, defaultServerIP string) string {
	if spec != nil {
		if strings.TrimSpace(spec.GroupDomain) != "" {
			return strings.TrimSpace(spec.GroupDomain)
		}
		if strings.TrimSpace(spec.Host) != "" {
			return strings.TrimSpace(spec.Host)
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

// GenerateURIs 为指定成员生成所有节点的 vless:// 链接列表
func GenerateURIs(member *model.Member) ([]string, error) {
	serverIP, _ := model.GetSetting("public_ip")
	if serverIP == "" {
		serverIP = netutil.GetPublicIP()
	}

	rows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json FROM inbounds WHERE enabled = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uris []string

	for rows.Next() {
		var ib model.Inbound
		if err := rows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON); err != nil {
			continue
		}

		remark := url.QueryEscape(fmt.Sprintf("%s-%d", ib.Tag, ib.Port))

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

			// vless://<uuid>@<server>:<port>?encryption=none&flow=xtls-rprx-vision&security=reality&sni=<sni>&fp=chrome&pbk=<pbk>&sid=<sid>&type=tcp#<remark>
			q := url.Values{}
			q.Set("encryption", "none")
			q.Set("flow", flow)
			q.Set("security", "reality")
			q.Set("sni", sni)
			q.Set("fp", fp)
			q.Set("pbk", spec.PublicKey)
			q.Set("sid", spec.ShortID)
			q.Set("type", "tcp")

			u := fmt.Sprintf("vless://%s@%s:%d?%s#%s", member.UUID, serverIP, ib.Port, q.Encode(), remark)
			uris = append(uris, u)

		case "vless-ws":
			var spec model.VLESSWSSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			groupDomain := ResolveWSGroupDomain(&spec, serverIP)
			path := spec.Path
			if path == "" {
				path = "/ws"
			}

			// 查询已保存的 Cloudflare 优选 IP 列表
			prefIPs, err := model.ListCFPreferredIPs("")
			if err == nil && len(prefIPs) > 0 {
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
					nodeRemark := fmt.Sprintf("%s-%s", ib.Tag, nodeName)

					u := fmt.Sprintf("vless://%s@%s:%d?%s#%s", member.UUID, p.IP, targetPort, q.Encode(), url.QueryEscape(nodeRemark))
					uris = append(uris, u)
				}
			} else {
				// 兜底回退：若当前未配置或未抓取优选 IP，生成使用分组域名的节点
				q := url.Values{}
				q.Set("encryption", "none")
				q.Set("security", "tls")
				q.Set("sni", groupDomain)
				q.Set("type", "ws")
				q.Set("host", groupDomain)
				q.Set("path", path)

				u := fmt.Sprintf("vless://%s@%s:443?%s#%s", member.UUID, groupDomain, q.Encode(), remark)
				uris = append(uris, u)
			}
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

