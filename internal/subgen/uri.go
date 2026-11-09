package subgen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
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
			remark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			obfsPart := ""
			if spec.ObfsPassword != "" {
				obfsPart = fmt.Sprintf("&obfs=salamander&obfs-password=%s", url.QueryEscape(spec.ObfsPassword))
			}
			u := fmt.Sprintf("hysteria2://%s@%s:%d?sni=%s&insecure=1%s#%s", url.QueryEscape(pwd), serverHost, ib.Port, url.QueryEscape(sni), obfsPart, url.QueryEscape(remark))
			uris = append(uris, u)

		case "shadowsocks":
			var spec model.ShadowsocksInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			method := spec.Method
			if method == "" {
				method = "2022-blake3-aes-128-gcm"
			}
			remark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			userInfo := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", method, spec.Password)))
			u := fmt.Sprintf("ss://%s@%s:%d#%s", userInfo, serverHost, ib.Port, url.QueryEscape(remark))
			uris = append(uris, u)

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
			remark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			u := fmt.Sprintf("trojan://%s@%s:%d?sni=%s&security=tls#%s", url.QueryEscape(pwd), serverHost, ib.Port, url.QueryEscape(sni), url.QueryEscape(remark))
			uris = append(uris, u)

		case "vmess":
			var spec model.VMessInboundSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			remark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			net := "tcp"
			path := ""
			host := ""
			if spec.Network == "ws" {
				net = "ws"
				path = spec.Path
				if path == "" {
					path = "/vmess"
				}
				host = spec.GroupDomain
			}
			vmessObj := map[string]interface{}{
				"v":    "2",
				"ps":   remark,
				"add":  serverHost,
				"port": ib.Port,
				"id":   member.UUID,
				"aid":  0,
				"scy":  "auto",
				"net":  net,
				"type": "none",
				"host": host,
				"path": path,
				"tls":  "",
			}
			b, _ := json.Marshal(vmessObj)
			uris = append(uris, "vmess://"+base64.StdEncoding.EncodeToString(b))

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
			remark := fmt.Sprintf("%s | %s-%d", prefix, ib.Tag, ib.Port)
			u := fmt.Sprintf("tuic://%s:%s@%s:%d?congestion_control=%s&sni=%s&allow_insecure=1#%s", member.UUID, url.QueryEscape(pwd), serverHost, ib.Port, cc, url.QueryEscape(sni), url.QueryEscape(remark))
			uris = append(uris, u)

		case "socks", "mixed", "http":
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

// AdaptSingleURIForSubconverter 将单个节点链接转换为 Subconverter 能严格识别的规范格式
// 特别说明：
// Subconverter 原生解析器不支持 socks5:// 或 socks:// 伪 Scheme，但原生支持 Telegram 代理规范：
//   tg://socks?server=HOST&port=PORT[&user=USER&pass=PASS]&remark=REMARK
// 以及 HTTP 代理规范：
//   tg://http?server=HOST&port=PORT[&user=USER&pass=PASS]&remark=REMARK
func AdaptSingleURIForSubconverter(rawURI string) string {
	rawURI = strings.TrimSpace(rawURI)
	if strings.HasPrefix(rawURI, "socks5://") || strings.HasPrefix(rawURI, "socks://") {
		parsed, err := url.Parse(rawURI)
		if err == nil && parsed.Hostname() != "" {
			q := url.Values{}
			q.Set("server", parsed.Hostname())
			port := parsed.Port()
			if port == "" {
				port = "1080"
			}
			q.Set("port", port)
			if parsed.User != nil {
				if user := parsed.User.Username(); user != "" {
					q.Set("user", user)
				}
				if pass, hasPass := parsed.User.Password(); hasPass && pass != "" {
					q.Set("pass", pass)
				}
			}
			remark := parsed.Fragment
			if unescapedRemark, err := url.QueryUnescape(remark); err == nil && unescapedRemark != "" {
				remark = unescapedRemark
			}
			if remark != "" {
				// Subconverter C++ 源码 (subparser.cpp) 严格读取 "remarks" (复数带 s)，未读到则兜底为 server:port
				q.Set("remarks", remark)
				q.Set("remark", remark)
			}
			return "tg://socks?" + q.Encode()
		}
	} else if strings.HasPrefix(rawURI, "http://") && strings.Contains(rawURI, "#") {
		parsed, err := url.Parse(rawURI)
		if err == nil && parsed.Hostname() != "" {
			q := url.Values{}
			q.Set("server", parsed.Hostname())
			port := parsed.Port()
			if port == "" {
				port = "80"
			}
			q.Set("port", port)
			if parsed.User != nil {
				if user := parsed.User.Username(); user != "" {
					q.Set("user", user)
				}
				if pass, hasPass := parsed.User.Password(); hasPass && pass != "" {
					q.Set("pass", pass)
				}
			}
			remark := parsed.Fragment
			if unescapedRemark, err := url.QueryUnescape(remark); err == nil && unescapedRemark != "" {
				remark = unescapedRemark
			}
			if remark != "" {
				// Subconverter C++ 源码 (subparser.cpp) 严格读取 "remarks"
				q.Set("remarks", remark)
				q.Set("remark", remark)
			}
			return "tg://http?" + q.Encode()
		}
	}
	return rawURI
}

// AdaptURIsForSubconverter 批量转换节点链接列表供 Subconverter 消费
func AdaptURIsForSubconverter(uris []string) []string {
	adapted := make([]string, 0, len(uris))
	for _, u := range uris {
		adapted = append(adapted, AdaptSingleURIForSubconverter(u))
	}
	return adapted
}

// AdaptSourceURLsForSubconverter 将以 '|' 分隔的复合节点字符串转换为 Subconverter 规范的方言
func AdaptSourceURLsForSubconverter(sourceURLs string) string {
	if strings.TrimSpace(sourceURLs) == "" {
		return ""
	}
	parts := strings.Split(sourceURLs, "|")
	adapted := make([]string, len(parts))
	for i, p := range parts {
		adapted[i] = AdaptSingleURIForSubconverter(p)
	}
	return strings.Join(adapted, "|")
}

// ExtractRemarkFromURI 从各协议标准 URI 中提取备注名称 (Remark / PS / Tag)
func ExtractRemarkFromURI(rawURI string) string {
	rawURI = strings.TrimSpace(rawURI)
	if strings.HasPrefix(rawURI, "tg://socks?") || strings.HasPrefix(rawURI, "tg://http?") {
		if parsed, err := url.Parse(rawURI); err == nil {
			if r := parsed.Query().Get("remarks"); r != "" {
				return r
			}
			if r := parsed.Query().Get("remark"); r != "" {
				return r
			}
		}
	}
	if strings.HasPrefix(rawURI, "vmess://") {
		b64 := strings.TrimPrefix(rawURI, "vmess://")
		if data, err := base64.StdEncoding.DecodeString(b64); err == nil {
			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err == nil {
				if ps, ok := m["ps"].(string); ok {
					return ps
				}
			}
		}
	}
	if idx := strings.Index(rawURI, "#"); idx != -1 {
		fragment := rawURI[idx+1:]
		if unescaped, err := url.QueryUnescape(fragment); err == nil && unescaped != "" {
			return unescaped
		}
		return fragment
	}
	return ""
}

// FilterURIsByRemark 根据排除 (excludeRegex) 与包含 (includeRegex) 正则表达式过滤节点链接列表
func FilterURIsByRemark(uris []string, excludeRegex, includeRegex string) []string {
	excludeRegex = strings.TrimSpace(excludeRegex)
	includeRegex = strings.TrimSpace(includeRegex)

	var excludeRE, includeRE *regexp.Regexp
	if excludeRegex != "" {
		excludeRE, _ = regexp.Compile(excludeRegex)
	}
	if includeRegex != "" {
		includeRE, _ = regexp.Compile(includeRegex)
	}
	if excludeRE == nil && includeRE == nil {
		return uris
	}

	filtered := make([]string, 0, len(uris))
	for _, u := range uris {
		rem := ExtractRemarkFromURI(u)
		if excludeRE != nil && excludeRE.MatchString(rem) {
			continue
		}
		if includeRE != nil && !includeRE.MatchString(rem) {
			continue
		}
		filtered = append(filtered, u)
	}
	return filtered
}
