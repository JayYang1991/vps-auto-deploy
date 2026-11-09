package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
	"vps-auto-deploy/internal/pkg/netutil"
	"vps-auto-deploy/internal/subgen"
)

// NodeInventoryItem 单个入站代理节点信息与客户端配置
type NodeInventoryItem struct {
	InboundID     int64  `json:"inbound_id"`
	Tag           string `json:"tag"`
	Protocol      string `json:"protocol"`
	ProtocolLabel string `json:"protocol_label"`
	Port          int    `json:"port"`
	Listen        string `json:"listen"`
	Enabled       bool   `json:"enabled"`
	ServerHost    string `json:"server_host"`
	ServerName    string `json:"server_name,omitempty"`
	Flow          string `json:"flow,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	Path          string `json:"path,omitempty"`
	TunnelDomain  string `json:"tunnel_domain,omitempty"`
	Link          string `json:"link"`
	ClashSnippet  string `json:"clash_snippet"`
}

// NodeInventoryGroup 按 VPS 宿主机节点分组的清单
type NodeInventoryGroup struct {
	NodeID        int64               `json:"node_id"`
	NodeName      string              `json:"node_name"`
	SubPrefix     string              `json:"sub_prefix"`
	Host          string              `json:"host"`
	IsMaster      bool                `json:"is_master"`
	Status        string              `json:"status"`
	Region        string              `json:"region"`
	CloudProvider string              `json:"cloud_provider"`
	LatencyMs     int64               `json:"latency_ms"`
	Inbounds      []NodeInventoryItem `json:"inbounds"`
	InboundCount  int                 `json:"inbound_count"`
	ErrorMessage  string              `json:"error_message,omitempty"`
}

// NodeInventoryResponse 节点清单接口综合响应
type NodeInventoryResponse struct {
	Member           *model.Member        `json:"member"`
	Groups           []NodeInventoryGroup `json:"groups"`
	TotalNodes       int                  `json:"total_nodes"`
	TotalInbounds    int                  `json:"total_inbounds"`
	PreferredIPCount int                  `json:"preferred_ip_count"`
}

// GetNodeInventory 按照 VPS 节点为分组，聚合获取全集群各节点的代理清单
func GetNodeInventory(c *gin.Context) {
	// 1. 确定关联成员凭据 (用于计算节点 UUID 及个性化链接)
	memberIDStr := c.Query("member_id")
	var member model.Member

	if memberIDStr != "" {
		if id, err := strconv.ParseInt(memberIDStr, 10, 64); err == nil {
			_ = model.DB.QueryRow("SELECT id, name, uuid, sub_token, enabled FROM members WHERE id = ?", id).
				Scan(&member.ID, &member.Name, &member.UUID, &member.SubToken, &member.Enabled)
		}
	}

	if member.UUID == "" {
		// 默认取第一个有效成员
		_ = model.DB.QueryRow("SELECT id, name, uuid, sub_token, enabled FROM members WHERE enabled = 1 ORDER BY id ASC LIMIT 1").
			Scan(&member.ID, &member.Name, &member.UUID, &member.SubToken, &member.Enabled)
	}

	// 2. 获取集群节点列表
	nodes, err := model.ListNodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点列表失败: " + err.Error()})
		return
	}

	var groups []NodeInventoryGroup
	totalInbounds := 0

	masterPublicIP, _ := model.GetSetting("public_ip")
	if masterPublicIP == "" {
		masterPublicIP = netutil.GetPublicIP()
	}

	// 3. 逐个节点构建清单
	for _, n := range nodes {
		group := NodeInventoryGroup{
			NodeID:        n.ID,
			NodeName:      n.Name,
			SubPrefix:     n.SubPrefix,
			Host:          n.Host,
			IsMaster:      n.IsMaster,
			Status:        n.Status,
			Region:        n.Region,
			CloudProvider: n.CloudProvider,
			LatencyMs:     n.LatencyMs,
			Inbounds:      []NodeInventoryItem{},
		}

		var inbounds []model.Inbound
		var tunnelsMap = make(map[int64]string)

		if n.IsMaster {
			// 主控节点直接读取本地 SQLite
			inboundRows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json, enabled FROM inbounds ORDER BY port ASC")
			if err == nil {
				for inboundRows.Next() {
					var ib model.Inbound
					if err := inboundRows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON, &ib.Enabled); err == nil {
						inbounds = append(inbounds, ib)
					}
				}
				inboundRows.Close()
			}

			// 查询主控本地已建立的 tunnels
			tRows, err := model.DB.Query("SELECT id, domain FROM tunnels WHERE enabled = 1 AND domain != ''")
			if err == nil {
				for tRows.Next() {
					var tID int64
					var domain string
					if err := tRows.Scan(&tID, &domain); err == nil {
						tunnelsMap[tID] = domain
					}
				}
				tRows.Close()
			}
		} else {
			// 远端被纳管节点：通过受管安全信道抓取
			if n.Status == "online" {
				remoteInbounds, remoteTunnels, fetchErr := node.FetchRemoteNodeInventory(&n)
				if fetchErr != nil {
					group.ErrorMessage = fmt.Sprintf("拉取远端入站配置失败: %v", fetchErr)
				} else {
					inbounds = remoteInbounds
					tunnelsMap = remoteTunnels
				}
			} else {
				group.ErrorMessage = "该节点当前处于离线状态"
			}
		}

		serverHost := n.Host
		if serverHost == "" || serverHost == "127.0.0.1" || serverHost == "localhost" {
			serverHost = masterPublicIP
		}

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

		for _, ib := range inbounds {
			item := buildInventoryItem(ib, prefix, serverHost, &member, tunnelsMap)
			group.Inbounds = append(group.Inbounds, item)
			if ib.Enabled {
				totalInbounds++
			}
		}

		group.InboundCount = len(group.Inbounds)
		groups = append(groups, group)
	}

	// 4. 统计 Cloudflare 优选 IP 数量
	prefCount, _ := model.CountCFPreferredIPs()

	c.JSON(http.StatusOK, NodeInventoryResponse{
		Member:           &member,
		Groups:           groups,
		TotalNodes:       len(nodes),
		TotalInbounds:    totalInbounds,
		PreferredIPCount: prefCount,
	})
}

// buildInventoryItem 解析入站参数并生成对应客户端链接与片段
func buildInventoryItem(ib model.Inbound, nodeName, serverHost string, member *model.Member, tunnelsMap map[int64]string) NodeInventoryItem {
	item := NodeInventoryItem{
		InboundID:  ib.ID,
		Tag:        ib.Tag,
		Protocol:   ib.Protocol,
		Port:       ib.Port,
		Listen:     ib.Listen,
		Enabled:    ib.Enabled,
		ServerHost: serverHost,
	}

	displayName := fmt.Sprintf("%s | %s-%d", nodeName, ib.Tag, ib.Port)
	uuid := member.UUID
	if uuid == "" {
		uuid = "c3a6027a-7d96-4018-a5d9-34c2e0fbee37"
	}

	switch ib.Protocol {
	case "vless-reality":
		item.ProtocolLabel = "VLESS Reality (Vision)"
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

		item.ServerName = sni
		item.Flow = flow
		item.Fingerprint = fp

		pbk := spec.PublicKey
		if pbk == "" && spec.PrivateKey != "" {
			pbk = singbox.DeriveRealityPublicKey(spec.PrivateKey)
		}

		// 生成 vless:// 链接
		q := url.Values{}
		q.Set("encryption", "none")
		q.Set("flow", flow)
		q.Set("security", "reality")
		q.Set("sni", sni)
		q.Set("fp", fp)
		q.Set("pbk", pbk)
		q.Set("sid", spec.ShortID)
		q.Set("type", "tcp")

		item.Link = fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuid, serverHost, ib.Port, q.Encode(), url.QueryEscape(displayName))

		// 生成 Clash Meta YAML 格式片段
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: vless
  server: "%s"
  port: %d
  uuid: "%s"
  network: tcp
  tls: true
  udp: true
  flow: "%s"
  servername: "%s"
  reality-opts:
    public-key: "%s"
    short-id: "%s"
  client-fingerprint: "%s"`, displayName, serverHost, ib.Port, uuid, flow, sni, pbk, spec.ShortID, fp)

	case "vless-ws":
		item.ProtocolLabel = "VLESS WebSocket (Argo)"
		var spec model.VLESSWSSpec
		_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

		path := spec.Path
		if path == "" {
			path = "/ws"
		}
		item.Path = path

		groupDomain := subgen.ResolveWSGroupDomain(&spec, serverHost, tunnelsMap)
		item.TunnelDomain = groupDomain

		wsDisplayName := fmt.Sprintf("%s | %s-Argo", nodeName, ib.Tag)

		// 生成 vless:// 链接
		q := url.Values{}
		q.Set("encryption", "none")
		q.Set("security", "tls")
		q.Set("sni", groupDomain)
		q.Set("type", "ws")
		q.Set("host", groupDomain)
		q.Set("path", path)

		item.Link = fmt.Sprintf("vless://%s@%s:443?%s#%s", uuid, groupDomain, q.Encode(), url.QueryEscape(wsDisplayName))

		// 生成 Clash Meta YAML 片段
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: vless
  server: "%s"
  port: 443
  uuid: "%s"
  network: ws
  tls: true
  udp: true
  servername: "%s"
  ws-opts:
    path: "%s"
    headers:
      Host: "%s"`, wsDisplayName, groupDomain, uuid, groupDomain, path, groupDomain)

	case "hysteria2":
		item.ProtocolLabel = "Hysteria 2"
		var spec model.Hysteria2InboundSpec
		_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
		pwd := spec.Password
		if pwd == "" {
			pwd = uuid
		}
		sni := spec.ServerName
		if sni == "" {
			sni = serverHost
		}
		item.ServerName = sni
		obfsPart := ""
		if spec.ObfsPassword != "" {
			obfsPart = fmt.Sprintf("&obfs=salamander&obfs-password=%s", url.QueryEscape(spec.ObfsPassword))
		}
		item.Link = fmt.Sprintf("hysteria2://%s@%s:%d?sni=%s&insecure=1%s#%s", url.QueryEscape(pwd), serverHost, ib.Port, url.QueryEscape(sni), obfsPart, url.QueryEscape(displayName))
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: hysteria2
  server: "%s"
  port: %d
  password: "%s"
  sni: "%s"
  skip-cert-verify: true`, displayName, serverHost, ib.Port, pwd, sni)

	case "shadowsocks":
		item.ProtocolLabel = "Shadowsocks"
		var spec model.ShadowsocksInboundSpec
		_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
		method := spec.Method
		if method == "" {
			method = "2022-blake3-aes-128-gcm"
		}
		userInfo := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", method, spec.Password)))
		item.Link = fmt.Sprintf("ss://%s@%s:%d#%s", userInfo, serverHost, ib.Port, url.QueryEscape(displayName))
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: ss
  server: "%s"
  port: %d
  cipher: %s
  password: "%s"`, displayName, serverHost, ib.Port, method, spec.Password)

	case "trojan":
		item.ProtocolLabel = "Trojan"
		var spec model.TrojanInboundSpec
		_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
		pwd := spec.Password
		if pwd == "" {
			pwd = uuid
		}
		sni := spec.ServerName
		if sni == "" {
			sni = serverHost
		}
		item.ServerName = sni
		item.Link = fmt.Sprintf("trojan://%s@%s:%d?sni=%s&security=tls#%s", url.QueryEscape(pwd), serverHost, ib.Port, url.QueryEscape(sni), url.QueryEscape(displayName))
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: trojan
  server: "%s"
  port: %d
  password: "%s"
  sni: "%s"
  skip-cert-verify: true`, displayName, serverHost, ib.Port, pwd, sni)

	case "vmess":
		item.ProtocolLabel = "VMess"
		var spec model.VMessInboundSpec
		_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
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
			"ps":   displayName,
			"add":  serverHost,
			"port": ib.Port,
			"id":   uuid,
			"aid":  0,
			"scy":  "auto",
			"net":  net,
			"type": "none",
			"host": host,
			"path": path,
			"tls":  "",
		}
		vmessBytes, _ := json.Marshal(vmessObj)
		item.Link = "vmess://" + base64.StdEncoding.EncodeToString(vmessBytes)
		if net == "ws" {
			item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: vmess
  server: "%s"
  port: %d
  uuid: "%s"
  alterId: 0
  cipher: auto
  network: ws
  ws-opts:
    path: "%s"`, displayName, serverHost, ib.Port, uuid, path)
		} else {
			item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: vmess
  server: "%s"
  port: %d
  uuid: "%s"
  alterId: 0
  cipher: auto`, displayName, serverHost, ib.Port, uuid)
		}

	case "tuic":
		item.ProtocolLabel = "TUIC v5"
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
		item.ServerName = sni
		item.Link = fmt.Sprintf("tuic://%s:%s@%s:%d?congestion_control=%s&sni=%s&allow_insecure=1#%s", uuid, url.QueryEscape(pwd), serverHost, ib.Port, cc, url.QueryEscape(sni), url.QueryEscape(displayName))
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: tuic
  server: "%s"
  port: %d
  uuid: "%s"
  password: "%s"
  congestion-controller: %s
  sni: "%s"
  skip-cert-verify: true`, displayName, serverHost, ib.Port, uuid, pwd, cc, sni)

	case "socks", "mixed", "http":
		item.ProtocolLabel = strings.ToUpper(ib.Protocol)
		var spec model.SocksSpec
		_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
		if spec.Username != "" && spec.Password != "" {
			item.Link = fmt.Sprintf("socks5://%s:%s@%s:%d#%s", url.QueryEscape(spec.Username), url.QueryEscape(spec.Password), serverHost, ib.Port, url.QueryEscape(displayName))
			item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: socks5
  server: "%s"
  port: %d
  username: "%s"
  password: "%s"`, displayName, serverHost, ib.Port, spec.Username, spec.Password)
		} else {
			item.Link = fmt.Sprintf("socks5://%s:%d#%s", serverHost, ib.Port, url.QueryEscape(displayName))
			item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: socks5
  server: "%s"
  port: %d`, displayName, serverHost, ib.Port)
		}

	default:
		item.ProtocolLabel = strings.ToUpper(ib.Protocol)
		item.Link = fmt.Sprintf("vless://%s@%s:%d#%s", uuid, serverHost, ib.Port, url.QueryEscape(displayName))
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: %s
  server: "%s"
  port: %d`, displayName, ib.Protocol, serverHost, ib.Port)
	}

	return item
}

