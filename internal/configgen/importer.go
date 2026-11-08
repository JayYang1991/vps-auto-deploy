package configgen

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"vps-auto-deploy/internal/model"
)

type ImportStats struct {
	InboundsImported  int      `json:"inbounds_imported"`
	MembersImported   int      `json:"members_imported"`
	OutboundsImported int      `json:"outbounds_imported"`
	InboundTags       []string `json:"inbound_tags"`
	ConfigPath        string   `json:"config_path"`
}

type RawSingboxFile struct {
	Inbounds  []map[string]interface{} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
}

// ImportExistingSingboxConfig 从给定的配置文件路径解析并导入入站、成员及出站配置到 SQLite 数据库
func ImportExistingSingboxConfig(cfgPath string) (*ImportStats, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var raw RawSingboxFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析 JSON 配置失败: %w", err)
	}

	stats := &ImportStats{ConfigPath: cfgPath}
	now := time.Now()

	// 1. 扫描与同步入站中的用户 (members)
	for _, ib := range raw.Inbounds {
		usersRaw, ok := ib["users"].([]interface{})
		if !ok {
			continue
		}
		for _, uItem := range usersRaw {
			uMap, ok := uItem.(map[string]interface{})
			if !ok {
				continue
			}
			uuidStr, _ := uMap["uuid"].(string)
			nameStr, _ := uMap["name"].(string)
			uuidStr = strings.TrimSpace(uuidStr)
			nameStr = strings.TrimSpace(nameStr)
			if uuidStr == "" {
				continue
			}
			if nameStr == "" {
				prefixLen := len(uuidStr)
				if prefixLen > 8 {
					prefixLen = 8
				}
				nameStr = "user-" + uuidStr[:prefixLen]
			}

			// 检查是否在 members 表中已存在
			var existingID int64
			err := model.DB.QueryRow("SELECT id FROM members WHERE uuid = ?", uuidStr).Scan(&existingID)
			if err != nil {
				// 生成随机 sub_token
				b := make([]byte, 8)
				_, _ = rand.Read(b)
				subToken := hex.EncodeToString(b)

				_, err = model.DB.Exec(
					"INSERT INTO members (name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
					nameStr, uuidStr, subToken, now, now,
				)
				if err == nil {
					stats.MembersImported++
				}
			}
		}
	}

	// 2. 扫描并同步入站节点 (inbounds)
	for _, ib := range raw.Inbounds {
		tag, _ := ib["tag"].(string)
		ibType, _ := ib["type"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}

		// 计算端口
		var port int
		if pVal, ok := ib["listen_port"].(float64); ok {
			port = int(pVal)
		} else if pVal, ok := ib["port"].(float64); ok {
			port = int(pVal)
		}
		if port <= 0 {
			continue
		}

		listen, _ := ib["listen"].(string)
		listen = strings.TrimSpace(listen)
		if listen == "" {
			listen = "::"
		}

		// 识别协议类型与参数
		protocol := "vless"
		specMap := make(map[string]interface{})

		// 检查 TLS & Reality
		tlsMap, hasTLS := ib["tls"].(map[string]interface{})
		var realityMap map[string]interface{}
		if hasTLS {
			if rMap, ok := tlsMap["reality"].(map[string]interface{}); ok {
				if enabled, ok := rMap["enabled"].(bool); ok && enabled {
					realityMap = rMap
				}
			}
		}

		// 检查 Transport
		transportMap, hasTransport := ib["transport"].(map[string]interface{})

		if realityMap != nil {
			protocol = "vless-reality"
			serverName, _ := tlsMap["server_name"].(string)
			specMap["server_name"] = serverName

			if hs, ok := realityMap["handshake"].(map[string]interface{}); ok {
				if s, ok := hs["server"].(string); ok {
					specMap["handshake_server"] = s
				}
				if p, ok := hs["server_port"].(float64); ok {
					specMap["handshake_port"] = int(p)
				}
			}
			if priv, ok := realityMap["private_key"].(string); ok {
				specMap["private_key"] = priv
			}
			if sids, ok := realityMap["short_id"].([]interface{}); ok && len(sids) > 0 {
				if sid, ok := sids[0].(string); ok {
					specMap["short_id"] = sid
				}
			}

			// 获取 flow
			if usersRaw, ok := ib["users"].([]interface{}); ok && len(usersRaw) > 0 {
				if uMap, ok := usersRaw[0].(map[string]interface{}); ok {
					if f, ok := uMap["flow"].(string); ok && f != "" {
						specMap["flow"] = f
					}
				}
			}
			if _, ok := specMap["flow"]; !ok {
				specMap["flow"] = "xtls-rprx-vision"
			}
		} else if hasTransport && transportMap["type"] == "ws" {
			protocol = "vless-ws"
			path, _ := transportMap["path"].(string)
			if path == "" {
				path = "/ws"
			}
			specMap["path"] = path
		} else {
			// 其他协议类型，完整保存至 specMap
			for k, v := range ib {
				if k != "type" && k != "tag" && k != "listen" && k != "listen_port" && k != "users" {
					specMap[k] = v
				}
			}
			if ibType != "" {
				protocol = ibType
			}
		}

		specBytes, _ := json.Marshal(specMap)

		// 检查是否在 inbounds 表中已存在
		var existingID int64
		err := model.DB.QueryRow("SELECT id FROM inbounds WHERE tag = ?", tag).Scan(&existingID)
		if err != nil {
			_, err = model.DB.Exec(
				"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)",
				tag, protocol, listen, port, string(specBytes), now, now,
			)
			if err == nil {
				stats.InboundsImported++
				stats.InboundTags = append(stats.InboundTags, tag)
			}
		}
	}

	// 3. 扫描并同步自定义出站规则 (outbounds)
	for _, ob := range raw.Outbounds {
		tag, _ := ob["tag"].(string)
		obType, _ := ob["type"].(string)
		tag = strings.TrimSpace(tag)
		obType = strings.TrimSpace(obType)
		if tag == "" || obType == "" || obType == "direct" || obType == "block" {
			continue
		}

		server, _ := ob["server"].(string)
		var serverPort int
		if pVal, ok := ob["server_port"].(float64); ok {
			serverPort = int(pVal)
		}

		specMap := make(map[string]interface{})
		for k, v := range ob {
			if k != "tag" && k != "type" && k != "server" && k != "server_port" {
				specMap[k] = v
			}
		}
		specBytes, _ := json.Marshal(specMap)

		var existingID int64
		err := model.DB.QueryRow("SELECT id FROM outbounds WHERE tag = ?", tag).Scan(&existingID)
		if err != nil {
			_, err = model.DB.Exec(
				"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 10, 1, 0, ?, ?)",
				tag, obType, server, serverPort, string(specBytes), now, now,
			)
			if err == nil {
				stats.OutboundsImported++
			}
		}
	}

	return stats, nil
}
