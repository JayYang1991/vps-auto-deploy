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

			// vless://<uuid>@<server>:<port>?encryption=none&flow=xtls-rprx-vision&security=reality&sni=<sni>&fp=chrome&pbk=<pbk>&sid=<sid>&type=tcp#<remark>
			q := url.Values{}
			q.Set("encryption", "none")
			q.Set("flow", flow)
			q.Set("security", "reality")
			q.Set("sni", spec.ServerName)
			q.Set("fp", fp)
			q.Set("pbk", spec.PublicKey)
			q.Set("sid", spec.ShortID)
			q.Set("type", "tcp")

			u := fmt.Sprintf("vless://%s@%s:%d?%s#%s", member.UUID, serverIP, ib.Port, q.Encode(), remark)
			uris = append(uris, u)

		case "vless-ws":
			var spec model.VLESSWSSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			var tunnelDomain string
			if spec.TunnelID > 0 {
				_ = model.DB.QueryRow("SELECT domain FROM tunnels WHERE id = ?", spec.TunnelID).Scan(&tunnelDomain)
			}
			if tunnelDomain == "" {
				tunnelDomain = serverIP
			}

			path := spec.Path
			if path == "" {
				path = "/ws"
			}

			q := url.Values{}
			q.Set("encryption", "none")
			q.Set("security", "tls")
			q.Set("sni", tunnelDomain)
			q.Set("type", "ws")
			q.Set("host", tunnelDomain)
			q.Set("path", path)

			u := fmt.Sprintf("vless://%s@%s:443?%s#%s", member.UUID, tunnelDomain, q.Encode(), remark)
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

