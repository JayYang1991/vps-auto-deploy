package configgen

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
)

type ImportStats struct {
	InboundsImported  int      `json:"inbounds_imported"`
	MembersImported   int      `json:"members_imported"`
	OutboundsImported int      `json:"outbounds_imported"`
	RoutesImported    int      `json:"routes_imported"`
	InboundTags       []string `json:"inbound_tags"`
	ConfigPath        string   `json:"config_path"`
}

type SyncStats struct {
	InboundsUpdated   int      `json:"inbounds_updated"`
	InboundsCreated   int      `json:"inbounds_created"`
	InboundsDeleted   int      `json:"inbounds_deleted"`
	OutboundsUpdated  int      `json:"outbounds_updated"`
	OutboundsCreated  int      `json:"outbounds_created"`
	OutboundsDeleted  int      `json:"outbounds_deleted"`
	RoutesCreated     int      `json:"routes_created"`
	MembersImported   int      `json:"members_imported"`
	InboundTags       []string `json:"inbound_tags"`
	OutboundTags      []string `json:"outbound_tags"`
	ConfigPath        string   `json:"config_path"`
}

type RawSingboxFile struct {
	Inbounds  []map[string]interface{} `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Route     *RawSingboxRoute          `json:"route"`
}

type RawSingboxRoute struct {
	Rules []map[string]interface{} `json:"rules"`
}

// ParseInboundMap 解析 sing-box 原始 inbound map 为面板数据结构
func ParseInboundMap(ib map[string]interface{}) (tag, protocol, listen string, port int, specJSON string, ok bool) {
	tag, _ = ib["tag"].(string)
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", "", "", 0, "", false
	}

	ibType, _ := ib["type"].(string)

	if pVal, okVal := ib["listen_port"].(float64); okVal {
		port = int(pVal)
	} else if pVal, okVal := ib["port"].(float64); okVal {
		port = int(pVal)
	}
	if port <= 0 {
		return "", "", "", 0, "", false
	}

	listen, _ = ib["listen"].(string)
	listen = strings.TrimSpace(listen)
	if listen == "" {
		listen = "::"
	}

	protocol = "vless"
	specMap := make(map[string]interface{})

	tlsMap, hasTLS := ib["tls"].(map[string]interface{})
	var realityMap map[string]interface{}
	if hasTLS {
		if rMap, okR := tlsMap["reality"].(map[string]interface{}); okR {
			if enabled, okE := rMap["enabled"].(bool); okE && enabled {
				realityMap = rMap
			}
		}
	}

	transportMap, hasTransport := ib["transport"].(map[string]interface{})

	if realityMap != nil {
		protocol = "vless-reality"
		serverName, _ := tlsMap["server_name"].(string)
		specMap["server_name"] = serverName

		if hs, okHS := realityMap["handshake"].(map[string]interface{}); okHS {
			if s, okS := hs["server"].(string); okS {
				specMap["handshake_server"] = s
			}
			if p, okP := hs["server_port"].(float64); okP {
				specMap["handshake_port"] = int(p)
			}
		}
		if priv, okPriv := realityMap["private_key"].(string); okPriv {
			specMap["private_key"] = priv
			if pub := singbox.DeriveRealityPublicKey(priv); pub != "" {
				specMap["public_key"] = pub
			}
		}
		if sids, okSids := realityMap["short_id"].([]interface{}); okSids && len(sids) > 0 {
			if sid, okSid := sids[0].(string); okSid {
				specMap["short_id"] = sid
			}
		}

		if usersRaw, okU := ib["users"].([]interface{}); okU && len(usersRaw) > 0 {
			if uMap, okUM := usersRaw[0].(map[string]interface{}); okUM {
				if f, okF := uMap["flow"].(string); okF && f != "" {
					specMap["flow"] = f
				}
			}
		}
		if _, okFlow := specMap["flow"]; !okFlow {
			specMap["flow"] = "xtls-rprx-vision"
		}
	} else if hasTransport && transportMap["type"] == "ws" {
		protocol = "vless-ws"
		path, _ := transportMap["path"].(string)
		if path == "" {
			path = "/ws"
		}
		specMap["path"] = path
	} else if ibType == "socks" || ibType == "mixed" || ibType == "http" {
		protocol = ibType
		if usersRaw, okU := ib["users"].([]interface{}); okU && len(usersRaw) > 0 {
			if uMap, okUM := usersRaw[0].(map[string]interface{}); okUM {
				if u, okUsr := uMap["username"].(string); okUsr && u != "" {
					specMap["username"] = u
				}
				if p, okPwd := uMap["password"].(string); okPwd && p != "" {
					specMap["password"] = p
				}
			}
		}
	} else if ibType == "shadowsocks" {
		protocol = "shadowsocks"
		if m, okM := ib["method"].(string); okM {
			specMap["method"] = m
		}
		if p, okP := ib["password"].(string); okP {
			specMap["password"] = p
		}
	} else if ibType == "trojan" {
		protocol = "trojan"
		if usersRaw, okU := ib["users"].([]interface{}); okU && len(usersRaw) > 0 {
			if uMap, okUM := usersRaw[0].(map[string]interface{}); okUM {
				if p, okP := uMap["password"].(string); okP {
					specMap["password"] = p
				}
			}
		}
		if tlsMap != nil {
			if sn, okSn := tlsMap["server_name"].(string); okSn {
				specMap["server_name"] = sn
			}
			if cp, okCp := tlsMap["certificate_path"].(string); okCp {
				specMap["certificate_path"] = cp
			}
			if kp, okKp := tlsMap["key_path"].(string); okKp {
				specMap["key_path"] = kp
			}
		}
	} else if ibType == "hysteria2" {
		protocol = "hysteria2"
		if usersRaw, okU := ib["users"].([]interface{}); okU && len(usersRaw) > 0 {
			if uMap, okUM := usersRaw[0].(map[string]interface{}); okUM {
				if p, okP := uMap["password"].(string); okP {
					specMap["password"] = p
				}
			}
		}
		if up, okUp := ib["up_mbps"].(float64); okUp {
			specMap["up_mbps"] = int(up)
		}
		if down, okDown := ib["down_mbps"].(float64); okDown {
			specMap["down_mbps"] = int(down)
		}
		if obfs, okObfs := ib["obfs"].(map[string]interface{}); okObfs {
			if pwd, okPwd := obfs["password"].(string); okPwd {
				specMap["obfs_password"] = pwd
			}
		}
		if tlsMap != nil {
			if sn, okSn := tlsMap["server_name"].(string); okSn {
				specMap["server_name"] = sn
			}
			if cp, okCp := tlsMap["certificate_path"].(string); okCp {
				specMap["certificate_path"] = cp
			}
			if kp, okKp := tlsMap["key_path"].(string); okKp {
				specMap["key_path"] = kp
			}
		}
	} else if ibType == "vmess" {
		protocol = "vmess"
		if hasTransport && transportMap["type"] == "ws" {
			specMap["network"] = "ws"
			path, _ := transportMap["path"].(string)
			specMap["path"] = path
		} else {
			specMap["network"] = "tcp"
		}
	} else if ibType == "tuic" {
		protocol = "tuic"
		if usersRaw, okU := ib["users"].([]interface{}); okU && len(usersRaw) > 0 {
			if uMap, okUM := usersRaw[0].(map[string]interface{}); okUM {
				if p, okP := uMap["password"].(string); okP {
					specMap["password"] = p
				}
			}
		}
		if cc, okCc := ib["congestion_control"].(string); okCc {
			specMap["congestion_control"] = cc
		}
		if tlsMap != nil {
			if sn, okSn := tlsMap["server_name"].(string); okSn {
				specMap["server_name"] = sn
			}
			if cp, okCp := tlsMap["certificate_path"].(string); okCp {
				specMap["certificate_path"] = cp
			}
			if kp, okKp := tlsMap["key_path"].(string); okKp {
				specMap["key_path"] = kp
			}
		}
	} else {
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
	return tag, protocol, listen, port, string(specBytes), true
}

// ParseOutboundMap 解析 sing-box 原始 outbound map 为面板数据结构
func ParseOutboundMap(ob map[string]interface{}) (tag, obType, server string, serverPort int, specJSON string, ok bool) {
	tag, _ = ob["tag"].(string)
	obType, _ = ob["type"].(string)
	tag = strings.TrimSpace(tag)
	obType = strings.TrimSpace(obType)
	if tag == "" || obType == "" {
		return "", "", "", 0, "", false
	}

	server, _ = ob["server"].(string)
	if pVal, okP := ob["server_port"].(float64); okP {
		serverPort = int(pVal)
	}

	specMap := make(map[string]interface{})
	for k, v := range ob {
		if k != "tag" && k != "type" && k != "server" && k != "server_port" {
			specMap[k] = v
		}
	}
	specBytes, _ := json.Marshal(specMap)
	return tag, obType, server, serverPort, string(specBytes), true
}

// ParseRouteRuleMap 解析 sing-box 原始 route.rule map 为面板 RouteRule 数据结构
func ParseRouteRuleMap(rule map[string]interface{}, idx int) (*model.RouteRule, bool) {
	if rule == nil {
		return nil, false
	}

	outboundTag, _ := rule["outbound"].(string)
	outboundTag = strings.TrimSpace(outboundTag)

	action, _ := rule["action"].(string)
	action = strings.TrimSpace(action)
	if action == "" {
		action = "route"
	}
	if outboundTag == "block" || action == "reject" {
		action = "reject"
		if outboundTag == "" {
			outboundTag = "block"
		}
	} else if action == "direct" {
		outboundTag = "direct"
	}

	if outboundTag == "" && action != "reject" {
		return nil, false
	}

	extractStringList := func(key string) string {
		v, exists := rule[key]
		if !exists || v == nil {
			return ""
		}
		switch val := v.(type) {
		case string:
			return strings.TrimSpace(val)
		case []interface{}:
			var list []string
			for _, item := range val {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					list = append(list, strings.TrimSpace(s))
				}
			}
			return strings.Join(list, ", ")
		case []string:
			return strings.Join(val, ", ")
		default:
			return ""
		}
	}

	extractPorts := func() (string, string) {
		var ports []string
		if pVal, exists := rule["port"]; exists && pVal != nil {
			switch p := pVal.(type) {
			case float64:
				ports = append(ports, fmt.Sprintf("%d", int(p)))
			case string:
				ports = append(ports, strings.TrimSpace(p))
			case []interface{}:
				for _, item := range p {
					if num, ok := item.(float64); ok {
						ports = append(ports, fmt.Sprintf("%d", int(num)))
					} else if str, ok := item.(string); ok {
						ports = append(ports, strings.TrimSpace(str))
					}
				}
			}
		}
		portStr := strings.Join(ports, ", ")
		portRangeStr := extractStringList("port_range")
		return portStr, portRangeStr
	}

	inboundTags := extractStringList("inbound")
	domains := extractStringList("domain")
	domainSuffix := extractStringList("domain_suffix")
	domainKeyword := extractStringList("domain_keyword")
	ipCIDR := extractStringList("ip_cidr")
	protocol := extractStringList("protocol")
	ruleSet := extractStringList("rule_set")
	portStr, portRangeStr := extractPorts()

	ipIsPrivate := false
	if privVal, exists := rule["ip_is_private"]; exists {
		if b, ok := privVal.(bool); ok {
			ipIsPrivate = b
		}
	}

	invert := false
	if invVal, exists := rule["invert"]; exists {
		if b, ok := invVal.(bool); ok {
			invert = b
		}
	}

	name := ""
	if nVal, exists := rule["name"].(string); exists && strings.TrimSpace(nVal) != "" {
		name = strings.TrimSpace(nVal)
	} else if inboundTags != "" && outboundTag != "" {
		name = fmt.Sprintf("入站直通: %s -> %s", inboundTags, outboundTag)
	} else if domainSuffix != "" {
		name = fmt.Sprintf("域名分流: %s -> %s", domainSuffix, outboundTag)
	} else if ruleSet != "" {
		name = fmt.Sprintf("规则集分流: %s -> %s", ruleSet, outboundTag)
	} else if ipIsPrivate {
		name = fmt.Sprintf("局域网私有IP -> %s", outboundTag)
	} else {
		name = fmt.Sprintf("路由分流规则 %d -> %s", idx+1, outboundTag)
	}

	r := &model.RouteRule{
		Name:          name,
		Action:        action,
		OutboundTag:   outboundTag,
		InboundTags:   inboundTags,
		Domains:       domains,
		DomainSuffix:  domainSuffix,
		DomainKeyword: domainKeyword,
		IPCIDR:        ipCIDR,
		IPIsPrivate:   ipIsPrivate,
		Port:          portStr,
		PortRange:     portRangeStr,
		Protocol:      protocol,
		RuleSet:       ruleSet,
		Invert:        invert,
		Priority:      (idx + 1) * 10,
		Enabled:       true,
	}
	return r, true
}

// ImportExistingSingboxConfig 增量导入（不覆盖已有 Tag）
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

	// 0. 保存非受管全局块 (如 dns, experimental, ntp, certificate, 自定义 log 等)
	var fullDiskMap map[string]interface{}
	if err := json.Unmarshal(data, &fullDiskMap); err == nil {
		unmanaged := make(map[string]interface{})
		for k, v := range fullDiskMap {
			if k != "inbounds" && k != "outbounds" && k != "route" {
				unmanaged[k] = v
			}
		}
		if len(unmanaged) > 0 {
			unBytes, _ := json.Marshal(unmanaged)
			_ = model.SetSetting("singbox_unmanaged_json", string(unBytes))
		}
	}

	// 1. 同步 members
	syncMembersFromInbounds(raw.Inbounds, now, false, &stats.MembersImported)

	// 2. 增量导入 inbounds
	for _, ib := range raw.Inbounds {
		tag, protocol, listen, port, specJSON, ok := ParseInboundMap(ib)
		if !ok {
			continue
		}

		var existingID int64
		err := model.DB.QueryRow("SELECT id FROM inbounds WHERE tag = ?", tag).Scan(&existingID)
		if err != nil {
			_, err = model.DB.Exec(
				"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)",
				tag, protocol, listen, port, specJSON, now, now,
			)
			if err == nil {
				stats.InboundsImported++
				stats.InboundTags = append(stats.InboundTags, tag)
			}
		}
	}

	// 3. 增量导入 outbounds
	for _, ob := range raw.Outbounds {
		tag, obType, server, serverPort, specJSON, ok := ParseOutboundMap(ob)
		if !ok || obType == "direct" || obType == "block" {
			continue
		}

		var existingID int64
		err := model.DB.QueryRow("SELECT id FROM outbounds WHERE tag = ?", tag).Scan(&existingID)
		if err != nil {
			_, err = model.DB.Exec(
				"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 10, 1, 0, ?, ?)",
				tag, obType, server, serverPort, specJSON, now, now,
			)
			if err == nil {
				stats.OutboundsImported++
			}
		}
	}

	// 4. 增量导入路由分流规则 (当数据库仅有默认规则或无规则时)
	if raw.Route != nil && len(raw.Route.Rules) > 0 {
		var routeCount int
		_ = model.DB.QueryRow("SELECT COUNT(*) FROM routes").Scan(&routeCount)
		if routeCount <= 2 {
			_, _ = model.DB.Exec("DELETE FROM routes")
			for idx, rMap := range raw.Route.Rules {
				r, ok := ParseRouteRuleMap(rMap, idx)
				if !ok {
					continue
				}
				ipPrivInt := 0
				if r.IPIsPrivate {
					ipPrivInt = 1
				}
				invInt := 0
				if r.Invert {
					invInt = 1
				}
				_, err := model.DB.Exec(`
					INSERT INTO routes (
						name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword,
						ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls,
						invert, priority, enabled, created_at, updated_at
					) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, 1, ?, ?)
				`, r.Name, r.Action, r.OutboundTag, r.InboundTags, r.Domains, r.DomainSuffix, r.DomainKeyword,
					r.IPCIDR, ipPrivInt, r.Port, r.PortRange, r.Protocol, r.RuleSet,
					invInt, r.Priority, now, now)
				if err == nil {
					stats.RoutesImported++
				}
			}
		}
	}

	return stats, nil
}

// ForceSyncSingboxConfig 从磁盘配置文件强制加载并 100% 覆盖面板数据库记录
func ForceSyncSingboxConfig(cfgPath string) (*SyncStats, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var raw RawSingboxFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析 JSON 配置失败: %w", err)
	}

	stats := &SyncStats{ConfigPath: cfgPath}
	now := time.Now()

	// 0. 保存非受管全局块 (如 dns, experimental, ntp, certificate, 自定义 log 等)
	var fullDiskMap map[string]interface{}
	if err := json.Unmarshal(data, &fullDiskMap); err == nil {
		unmanaged := make(map[string]interface{})
		for k, v := range fullDiskMap {
			if k != "inbounds" && k != "outbounds" && k != "route" {
				unmanaged[k] = v
			}
		}
		if len(unmanaged) > 0 {
			unBytes, _ := json.Marshal(unmanaged)
			_ = model.SetSetting("singbox_unmanaged_json", string(unBytes))
		}
	}

	tx, err := model.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("开启数据库事务失败: %w", err)
	}
	defer tx.Rollback()

	// 1. 同步用户（members）
	for _, ib := range raw.Inbounds {
		usersRaw, ok := ib["users"].([]interface{})
		if !ok {
			continue
		}
		for _, uItem := range usersRaw {
			uMap, okUM := uItem.(map[string]interface{})
			if !okUM {
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

			var existingID int64
			err := tx.QueryRow("SELECT id FROM members WHERE uuid = ?", uuidStr).Scan(&existingID)
			if err == sql.ErrNoRows {
				b := make([]byte, 8)
				_, _ = rand.Read(b)
				subToken := hex.EncodeToString(b)
				_, err = tx.Exec(
					"INSERT INTO members (name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
					nameStr, uuidStr, subToken, now, now,
				)
				if err == nil {
					stats.MembersImported++
				}
			} else if err == nil {
				_, _ = tx.Exec("UPDATE members SET name = ?, updated_at = ? WHERE id = ?", nameStr, now, existingID)
			}
		}
	}

	// 2. 强制覆盖入站节点（inbounds）
	diskInboundTags := make(map[string]bool)
	for _, ib := range raw.Inbounds {
		tag, protocol, listen, port, specJSON, ok := ParseInboundMap(ib)
		if !ok {
			continue
		}
		diskInboundTags[tag] = true
		stats.InboundTags = append(stats.InboundTags, tag)

		var existingID int64
		err := tx.QueryRow("SELECT id FROM inbounds WHERE tag = ?", tag).Scan(&existingID)
		if err == sql.ErrNoRows {
			_, err = tx.Exec(
				"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)",
				tag, protocol, listen, port, specJSON, now, now,
			)
			if err == nil {
				stats.InboundsCreated++
			}
		} else if err == nil {
			_, err = tx.Exec(
				"UPDATE inbounds SET protocol = ?, listen = ?, port = ?, spec_json = ?, enabled = 1, updated_at = ? WHERE id = ?",
				protocol, listen, port, specJSON, now, existingID,
			)
			if err == nil {
				stats.InboundsUpdated++
			}
		}
	}

	// 清理数据库中多余的、已被磁盘删除的入站
	ibRows, err := tx.Query("SELECT id, tag FROM inbounds")
	if err == nil {
		var toDeleteIbIDs []int64
		for ibRows.Next() {
			var id int64
			var tag string
			if err := ibRows.Scan(&id, &tag); err == nil {
				if !diskInboundTags[tag] {
					toDeleteIbIDs = append(toDeleteIbIDs, id)
				}
			}
		}
		ibRows.Close()

		for _, id := range toDeleteIbIDs {
			if _, err := tx.Exec("DELETE FROM inbounds WHERE id = ?", id); err == nil {
				stats.InboundsDeleted++
			}
		}
	}

	// 3. 强制覆盖出站规则（outbounds）
	diskOutboundTags := make(map[string]bool)
	priorityIndex := 10

	for _, ob := range raw.Outbounds {
		tag, obType, server, serverPort, specJSON, ok := ParseOutboundMap(ob)
		if !ok {
			continue
		}
		diskOutboundTags[tag] = true
		stats.OutboundTags = append(stats.OutboundTags, tag)

		var existingID int64
		var isSystem int
		err := tx.QueryRow("SELECT id, is_system FROM outbounds WHERE tag = ?", tag).Scan(&existingID, &isSystem)
		if err == sql.ErrNoRows {
			isSys := 0
			if tag == "direct" || tag == "block" {
				isSys = 1
			}
			_, err = tx.Exec(
				"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)",
				tag, obType, server, serverPort, specJSON, priorityIndex, isSys, now, now,
			)
			if err == nil {
				stats.OutboundsCreated++
			}
		} else if err == nil {
			if isSystem == 1 {
				// 系统出站仅更新参数与启用状态，保留系统标识
				_, err = tx.Exec(
					"UPDATE outbounds SET spec_json = ?, enabled = 1, updated_at = ? WHERE id = ?",
					specJSON, now, existingID,
				)
			} else {
				_, err = tx.Exec(
					"UPDATE outbounds SET type = ?, server = ?, server_port = ?, spec_json = ?, enabled = 1, updated_at = ? WHERE id = ?",
					obType, server, serverPort, specJSON, now, existingID,
				)
			}
			if err == nil {
				stats.OutboundsUpdated++
			}
		}
		priorityIndex += 10
	}

	// 清理数据库中已被磁盘删除的非系统出站
	obRows, err := tx.Query("SELECT id, tag FROM outbounds WHERE is_system = 0")
	if err == nil {
		var toDeleteObIDs []int64
		for obRows.Next() {
			var id int64
			var tag string
			if err := obRows.Scan(&id, &tag); err == nil {
				if !diskOutboundTags[tag] {
					toDeleteObIDs = append(toDeleteObIDs, id)
				}
			}
		}
		obRows.Close()

		for _, id := range toDeleteObIDs {
			if _, err := tx.Exec("DELETE FROM outbounds WHERE id = ?", id); err == nil {
				stats.OutboundsDeleted++
			}
		}
	}

	// 确保基础 direct 和 block 规则在数据库中依然存在
	for _, sysTag := range []string{"direct", "block"} {
		var cnt int
		_ = tx.QueryRow("SELECT COUNT(*) FROM outbounds WHERE tag = ?", sysTag).Scan(&cnt)
		if cnt == 0 {
			prio := 1
			if sysTag == "block" {
				prio = 99
			}
			_, _ = tx.Exec(
				"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, '', 0, '{}', ?, 1, 1, ?, ?)",
				sysTag, sysTag, prio, now, now,
			)
		}
	}

	// 4. 强制覆盖路由分流规则 (routes)
	if raw.Route != nil && len(raw.Route.Rules) > 0 {
		_, _ = tx.Exec("DELETE FROM routes")
		for idx, rMap := range raw.Route.Rules {
			r, ok := ParseRouteRuleMap(rMap, idx)
			if !ok {
				continue
			}
			ipPrivInt := 0
			if r.IPIsPrivate {
				ipPrivInt = 1
			}
			invInt := 0
			if r.Invert {
				invInt = 1
			}
			_, err := tx.Exec(`
				INSERT INTO routes (
					name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword,
					ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls,
					invert, priority, enabled, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, 1, ?, ?)
			`, r.Name, r.Action, r.OutboundTag, r.InboundTags, r.Domains, r.DomainSuffix, r.DomainKeyword,
				r.IPCIDR, ipPrivInt, r.Port, r.PortRange, r.Protocol, r.RuleSet,
				invInt, r.Priority, now, now)
			if err == nil {
				stats.RoutesCreated++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交数据库事务失败: %w", err)
	}

	return stats, nil
}

func syncMembersFromInbounds(inbounds []map[string]interface{}, now time.Time, overwrite bool, importedCount *int) {
	for _, ib := range inbounds {
		usersRaw, ok := ib["users"].([]interface{})
		if !ok {
			continue
		}
		for _, uItem := range usersRaw {
			uMap, okUM := uItem.(map[string]interface{})
			if !okUM {
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

			var existingID int64
			err := model.DB.QueryRow("SELECT id FROM members WHERE uuid = ?", uuidStr).Scan(&existingID)
			if err != nil {
				b := make([]byte, 8)
				_, _ = rand.Read(b)
				subToken := hex.EncodeToString(b)

				_, err = model.DB.Exec(
					"INSERT INTO members (name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
					nameStr, uuidStr, subToken, now, now,
				)
				if err == nil && importedCount != nil {
					*importedCount++
				}
			} else if overwrite {
				_, _ = model.DB.Exec("UPDATE members SET name = ?, updated_at = ? WHERE id = ?", nameStr, now, existingID)
			}
		}
	}
}
