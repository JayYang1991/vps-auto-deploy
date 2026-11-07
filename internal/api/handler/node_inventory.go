package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
	"vps-auto-deploy/internal/pkg/netutil"
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
				remoteInbounds, remoteTunnels, fetchErr := fetchRemoteNodeInventory(&n)
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

		for _, ib := range inbounds {
			item := buildInventoryItem(ib, n.Name, serverHost, &member, tunnelsMap)
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

	displayName := fmt.Sprintf("%s-%s-%d", nodeName, ib.Tag, ib.Port)
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

		// 生成 vless:// 链接
		q := url.Values{}
		q.Set("encryption", "none")
		q.Set("flow", flow)
		q.Set("security", "reality")
		q.Set("sni", sni)
		q.Set("fp", fp)
		q.Set("pbk", spec.PublicKey)
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
  client-fingerprint: "%s"`, displayName, serverHost, ib.Port, uuid, flow, sni, spec.PublicKey, spec.ShortID, fp)

	case "vless-ws":
		item.ProtocolLabel = "VLESS WebSocket (Argo)"
		var spec model.VLESSWSSpec
		_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

		path := spec.Path
		if path == "" {
			path = "/ws"
		}
		item.Path = path

		tunnelDomain := ""
		if spec.TunnelID > 0 {
			tunnelDomain = tunnelsMap[spec.TunnelID]
		}
		if tunnelDomain == "" {
			tunnelDomain = serverHost
		}
		item.TunnelDomain = tunnelDomain

		// 生成 vless:// 链接
		q := url.Values{}
		q.Set("encryption", "none")
		q.Set("security", "tls")
		q.Set("sni", tunnelDomain)
		q.Set("type", "ws")
		q.Set("host", tunnelDomain)
		q.Set("path", path)

		item.Link = fmt.Sprintf("vless://%s@%s:443?%s#%s", uuid, tunnelDomain, q.Encode(), url.QueryEscape(displayName))

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
      Host: "%s"`, displayName, tunnelDomain, uuid, tunnelDomain, path, tunnelDomain)

	case "hysteria2":
		item.ProtocolLabel = "Hysteria 2"
		item.Link = fmt.Sprintf("hysteria2://%s@%s:%d?sni=%s&insecure=0#%s", uuid, serverHost, ib.Port, serverHost, url.QueryEscape(displayName))
		item.ClashSnippet = fmt.Sprintf(`- name: "%s"
  type: hysteria2
  server: "%s"
  port: %d
  password: "%s"
  sni: "%s"`, displayName, serverHost, ib.Port, uuid, serverHost)

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

// fetchRemoteNodeInventory 跨节点抓取受管端的 inbounds 与 tunnels 配置
func fetchRemoteNodeInventory(node *model.Node) ([]model.Inbound, map[int64]string, error) {
	scheme := "http"
	if node.UseTLS {
		scheme = "https"
	}

	var client *http.Client
	if node.UseTLS {
		client = crypto.CreatePinnedHTTPClient(node.CertFingerprint, 3*time.Second)
	} else {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	// 1. 抓取 inbounds
	ibURL := fmt.Sprintf("%s://%s:%d/%s/api/inbounds", scheme, node.Host, node.PanelPort, strings.Trim(node.SecretPath, "/"))
	req, err := http.NewRequest(http.MethodGet, ibURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+node.APIToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var inbounds []model.Inbound
	if err := json.NewDecoder(resp.Body).Decode(&inbounds); err != nil {
		return nil, nil, err
	}

	// 2. 抓取 tunnels
	tunnelsMap := make(map[int64]string)
	tnURL := fmt.Sprintf("%s://%s:%d/%s/api/tunnels", scheme, node.Host, node.PanelPort, strings.Trim(node.SecretPath, "/"))
	reqTn, err := http.NewRequest(http.MethodGet, tnURL, nil)
	if err == nil {
		reqTn.Header.Set("Authorization", "Bearer "+node.APIToken)
		if respTn, err := client.Do(reqTn); err == nil {
			defer respTn.Body.Close()
			if respTn.StatusCode == http.StatusOK {
				var tunnels []model.Tunnel
				if err := json.NewDecoder(respTn.Body).Decode(&tunnels); err == nil {
					for _, t := range tunnels {
						if t.Domain != "" {
							tunnelsMap[t.ID] = t.Domain
						}
					}
				}
			}
		}
	}

	return inbounds, tunnelsMap, nil
}

