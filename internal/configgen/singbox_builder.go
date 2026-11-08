package configgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
)

// SingboxConfig 代表完整的 sing-box 服务端配置
type SingboxConfig struct {
	Log       *LogConfig               `json:"log,omitempty"`
	Inbounds  []InboundConfig          `json:"inbounds"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Route     *RouteConfig             `json:"route,omitempty"`
}

type LogConfig struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type InboundConfig struct {
	Type       string                 `json:"type"`
	Tag        string                 `json:"tag"`
	Listen     string                 `json:"listen,omitempty"`
	ListenPort int                    `json:"listen_port"`
	Users      []InboundUser          `json:"users,omitempty"`
	TLS        *InboundTLSConfig      `json:"tls,omitempty"`
	Transport  map[string]interface{} `json:"transport,omitempty"`
}

type InboundUser struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
	Flow string `json:"flow,omitempty"`
}

type InboundTLSConfig struct {
	Enabled    bool                 `json:"enabled"`
	ServerName string               `json:"server_name,omitempty"`
	Reality    *InboundRealityConfig `json:"reality,omitempty"`
}

type InboundRealityConfig struct {
	Enabled    bool              `json:"enabled"`
	Handshake  map[string]interface{} `json:"handshake"`
	PrivateKey string            `json:"private_key"`
	ShortID    []string          `json:"short_id,omitempty"`
}

type OutboundConfig struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type RouteConfig struct {
	Rules []map[string]interface{} `json:"rules,omitempty"`
}

// GenerateServerConfig 根据数据库中的数据构建 sing-box 完整配置
func GenerateServerConfig() (*SingboxConfig, error) {
	// 1. 获取启用的成员
	rows, err := model.DB.Query("SELECT name, uuid FROM members WHERE enabled = 1")
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	var members []model.Member
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.Name, &m.UUID); err == nil {
			members = append(members, m)
		}
	}

	// 2. 获取启用的入站
	inboundRows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json FROM inbounds WHERE enabled = 1")
	if err != nil {
		return nil, fmt.Errorf("query inbounds: %w", err)
	}
	defer inboundRows.Close()

	cfg := &SingboxConfig{
		Log: &LogConfig{
			Level:     "info",
			Timestamp: true,
		},
		Outbounds: []map[string]interface{}{},
	}

	// 读取启用的出站规则 (按 priority ASC, id ASC)
	outboundRows, err := model.DB.Query("SELECT tag, type, server, server_port, spec_json FROM outbounds WHERE enabled = 1 ORDER BY priority ASC, id ASC")
	if err == nil {
		defer outboundRows.Close()
		for outboundRows.Next() {
			var tag, obType, server, specJSON string
			var serverPort int
			if err := outboundRows.Scan(&tag, &obType, &server, &serverPort, &specJSON); err != nil {
				continue
			}

			obMap := make(map[string]interface{})
			if obType == "custom" && strings.TrimSpace(specJSON) != "" {
				if err := json.Unmarshal([]byte(specJSON), &obMap); err == nil {
					obMap["tag"] = tag
					cfg.Outbounds = append(cfg.Outbounds, obMap)
					continue
				}
			}

			obMap["type"] = obType
			obMap["tag"] = tag

			if server != "" {
				obMap["server"] = server
			}
			if serverPort > 0 {
				obMap["server_port"] = serverPort
			}

			// 合并 spec_json 中的额外参数 (如 password, method, uuid, tls 等)
			if strings.TrimSpace(specJSON) != "" && specJSON != "{}" {
				var extra map[string]interface{}
				if err := json.Unmarshal([]byte(specJSON), &extra); err == nil {
					for k, v := range extra {
						if _, exists := obMap[k]; !exists {
							obMap[k] = v
						}
					}
				}
			}

			cfg.Outbounds = append(cfg.Outbounds, obMap)
		}
	}

	// 保证至少有 direct 和 block 出站
	hasDirect := false
	hasBlock := false
	for _, ob := range cfg.Outbounds {
		if t, ok := ob["type"].(string); ok {
			if t == "direct" {
				hasDirect = true
			} else if t == "block" {
				hasBlock = true
			}
		}
	}
	if !hasDirect {
		cfg.Outbounds = append([]map[string]interface{}{{"type": "direct", "tag": "direct"}}, cfg.Outbounds...)
	}
	if !hasBlock {
		cfg.Outbounds = append(cfg.Outbounds, map[string]interface{}{"type": "block", "tag": "block"})
	}

	for inboundRows.Next() {
		var ib model.Inbound
		if err := inboundRows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON); err != nil {
			continue
		}

		inboundCfg := InboundConfig{
			Type:       "vless",
			Tag:        ib.Tag,
			Listen:     ib.Listen,
			ListenPort: ib.Port,
		}

		switch ib.Protocol {
		case "vless-reality":
			var spec model.VLESSRealitySpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			flow := spec.Flow
			if flow == "" {
				flow = "xtls-rprx-vision"
			}

			// 填充用户
			for _, m := range members {
				inboundCfg.Users = append(inboundCfg.Users, InboundUser{
					Name: m.Name,
					UUID: m.UUID,
					Flow: flow,
				})
			}

			hsPort := spec.HandshakePort
			if hsPort == 0 {
				hsPort = 443
			}

			serverName := spec.ServerName
			if serverName == "" {
				serverName = "dl.google.com"
			}
			handshakeServer := spec.HandshakeServer
			if handshakeServer == "" {
				handshakeServer = serverName
			}

			inboundCfg.TLS = &InboundTLSConfig{
				Enabled:    true,
				ServerName: serverName,
				Reality: &InboundRealityConfig{
					Enabled: true,
					Handshake: map[string]interface{}{
						"server":      handshakeServer,
						"server_port": hsPort,
					},
					PrivateKey: spec.PrivateKey,
					ShortID:    []string{spec.ShortID},
				},
			}

		case "vless-ws":
			var spec model.VLESSWSSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)

			for _, m := range members {
				inboundCfg.Users = append(inboundCfg.Users, InboundUser{
					Name: m.Name,
					UUID: m.UUID,
				})
			}

			path := spec.Path
			if path == "" {
				path = "/ws"
			}
			inboundCfg.Transport = map[string]interface{}{
				"type": "ws",
				"path": path,
			}
		}

		cfg.Inbounds = append(cfg.Inbounds, inboundCfg)
	}

	return cfg, nil
}

func probeSingboxHealth(driver *singbox.Driver) error {
	// 启动后探活检查：多次检测服务是否异常退出或进入 crash loop
	for i := 0; i < 6; i++ {
		time.Sleep(250 * time.Millisecond)
		st, err := driver.Status()
		if err != nil {
			continue
		}
		if st.RunningStatus == "failed" {
			return fmt.Errorf("服务进入失败状态 (failed)")
		}
		if !st.Active && !component.IsProcessRunning(singbox.DriverName) {
			return fmt.Errorf("服务进程未保持运行")
		}
	}
	return nil
}

func rollbackSingbox(destPath string, oldConfig []byte, driver *singbox.Driver, wasActive bool) {
	if len(oldConfig) > 0 {
		_ = os.WriteFile(destPath, oldConfig, 0644)
	} else {
		_ = os.Remove(destPath)
	}
	if wasActive {
		_ = driver.ReloadOrRestart()
	}
}

// ApplyAndReload 将生成的配置写入并重启/重载 sing-box 服务
// 若配置语法校验不通过或启动探活失败，自动回滚至上一次的配置并恢复服务
func ApplyAndReload(destPath string) error {
	cfg, err := GenerateServerConfig()
	if err != nil {
		return fmt.Errorf("生成服务端配置失败: %w", err)
	}

	rawJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	_ = os.MkdirAll(filepath.Dir(destPath), 0755)

	driver := singbox.NewDriver()
	installedVer, _ := driver.GetInstalledVersion()

	// 1. 若已安装 sing-box，先写临时文件进行语法与语义合法性预检
	if installedVer != "" {
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("sb_chk_%d.json", time.Now().UnixNano()))
		if err := os.WriteFile(tmpPath, rawJSON, 0644); err != nil {
			return fmt.Errorf("写入临时校验文件失败: %w", err)
		}
		defer os.Remove(tmpPath)

		if err := singbox.CheckConfig(tmpPath); err != nil {
			return fmt.Errorf("sing-box 配置语法预检失败: %w", err)
		}
	}

	// 2. 备份原配置与活跃状态
	oldConfig, _ := os.ReadFile(destPath)
	prevStatus, _ := driver.Status()
	wasActive := prevStatus.Active || component.IsProcessRunning(singbox.DriverName)

	// 3. 校验通过，写入目标配置文件
	if err := os.WriteFile(destPath, rawJSON, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 4. 重载服务并探活
	if installedVer != "" {
		if err := driver.ReloadOrRestart(); err != nil {
			rollbackSingbox(destPath, oldConfig, driver, wasActive)
			return fmt.Errorf("sing-box 重启/重载失败，已自动回滚原配置: %w", err)
		}

		// 若原先或当前期望活跃，执行健康探活
		if wasActive {
			if err := probeSingboxHealth(driver); err != nil {
				rollbackSingbox(destPath, oldConfig, driver, wasActive)
				return fmt.Errorf("sing-box 启动探活失败 (%s)，已自动回滚原配置", err.Error())
			}
		}
	}

	return nil
}

