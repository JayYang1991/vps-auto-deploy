package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/netutil"
)

func isValidListenAddress(addr string) bool {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" || trimmed == "::" || trimmed == "0.0.0.0" || trimmed == "localhost" {
		return true
	}
	return net.ParseIP(trimmed) != nil
}

type InboundCreateRequest struct {
	Tag      string          `json:"tag" binding:"required"`
	Protocol string          `json:"protocol" binding:"required"` // "vless-reality" | "vless-ws" | "socks"
	Listen   string          `json:"listen"`
	Port     int             `json:"port" binding:"required"`
	Spec     json.RawMessage `json:"spec"`
	Enabled  bool            `json:"enabled"`
}

func ListInbounds(c *gin.Context) {
	rows, err := model.DB.Query("SELECT id, tag, protocol, listen, port, spec_json, enabled, created_at, updated_at FROM inbounds ORDER BY id DESC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var list []model.Inbound
	for rows.Next() {
		var ib model.Inbound
		var enabledInt int
		var createdAtRaw, updatedAtRaw any
		if err := rows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON, &enabledInt, &createdAtRaw, &updatedAtRaw); err == nil {
			ib.Enabled = enabledInt == 1
			ib.CreatedAt = model.ParseTime(createdAtRaw)
			ib.UpdatedAt = model.ParseTime(updatedAtRaw)
			list = append(list, ib)
		}
	}
	if list == nil {
		list = []model.Inbound{}
	}
	c.JSON(http.StatusOK, list)
}

func GetInbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var ib model.Inbound
	var enabledInt int
	var createdAtRaw, updatedAtRaw any
	err = model.DB.QueryRow("SELECT id, tag, protocol, listen, port, spec_json, enabled, created_at, updated_at FROM inbounds WHERE id = ?", id).
		Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON, &enabledInt, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inbound not found"})
		return
	}
	ib.Enabled = enabledInt == 1
	ib.CreatedAt = model.ParseTime(createdAtRaw)
	ib.UpdatedAt = model.ParseTime(updatedAtRaw)

	c.JSON(http.StatusOK, ib)
}

func ToggleInbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var enabled bool
	var port int
	var protocol, listen string
	err = model.DB.QueryRow("SELECT enabled, port, protocol, listen FROM inbounds WHERE id = ?", id).Scan(&enabled, &port, &protocol, &listen)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inbound not found"})
		return
	}

	newStatus := !enabled
	_, err = model.DB.Exec("UPDATE inbounds SET enabled = ?, updated_at = ? WHERE id = ?", newStatus, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 回滚数据库记录并重刷配置
		_, _ = model.DB.Exec("UPDATE inbounds SET enabled = ?, updated_at = ? WHERE id = ?", enabled, time.Now(), id)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站状态切换失败，已自动回滚: " + err.Error()})
		return
	}

	if newStatus {
		go func() {
			_ = netutil.AllowInboundPort(port, protocol, listen)
		}()
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "enabled": newStatus, "message": "入站节点状态已切换"})
}

func GenerateRealityKeys(c *gin.Context) {
	priv, pub, sid, err := singbox.GenerateRealityKeypair()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"private_key": priv,
		"public_key":  pub,
		"short_id":    sid,
	})
}

// DeriveRealityKey 接收 X25519 私钥并实时推导出对应的 Base64URL 客户端公钥
func DeriveRealityKey(c *gin.Context) {
	var req struct {
		PrivateKey string `json:"private_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "私钥不能为空"})
		return
	}
	pub := singbox.DeriveRealityPublicKey(req.PrivateKey)
	if pub == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 X25519 私钥，必须是 32 字节 Base64 编码"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"public_key": pub})
}

func enrichInboundSpec(protocol, specStr string) string {
	if protocol == "vless-reality" {
		var rSpec model.VLESSRealitySpec
		if err := json.Unmarshal([]byte(specStr), &rSpec); err == nil {
			if strings.TrimSpace(rSpec.PublicKey) == "" && strings.TrimSpace(rSpec.PrivateKey) != "" {
				if pub := singbox.DeriveRealityPublicKey(rSpec.PrivateKey); pub != "" {
					rSpec.PublicKey = pub
					if updatedBytes, err := json.Marshal(rSpec); err == nil {
						return string(updatedBytes)
					}
				}
			}
		}
	}
	return specStr
}

func GenerateTLSCert(c *gin.Context) {
	certPath, keyPath, err := configgen.EnsureDefaultTLSCert()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成自签名证书失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"certificate_path": certPath,
		"key_path":         keyPath,
		"message":          "自签名证书已就绪",
	})
}

func CreateInbound(c *gin.Context) {
	var req InboundCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Port < 1 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站端口必须在 1 ~ 65535 之间"})
		return
	}
	req.Tag = strings.TrimSpace(req.Tag)
	if !regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,64}$`).MatchString(req.Tag) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站 Tag 格式不合法 (仅支持1-64位字母、数字、点号、短横线与下划线)"})
		return
	}
	if req.Listen != "" && !isValidListenAddress(req.Listen) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "监听地址格式不合法，请输入合法的 IP 地址（如 ::, 0.0.0.0, 127.0.0.1）"})
		return
	}

	listen := req.Listen
	if listen == "" {
		if req.Protocol == "vless-ws" || req.Protocol == "socks" || req.Protocol == "mixed" || req.Protocol == "http" {
			listen = "127.0.0.1"
		} else {
			listen = "::"
		}
	}

	specStr := strings.TrimSpace(string(req.Spec))
	if specStr == "" || specStr == "null" {
		specStr = "{}"
	}
	specStr = enrichInboundSpec(req.Protocol, specStr)

	now := time.Now()
	res, err := model.DB.Exec(
		"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		req.Tag, req.Protocol, listen, req.Port, specStr, req.Enabled, now, now,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to insert inbound: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// 自动触发一次配置生成与生效探活
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 回滚入站记录
		_, _ = model.DB.Exec("DELETE FROM inbounds WHERE id = ?", id)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站配置生效失败，已自动回滚: " + err.Error()})
		return
	}

	if req.Enabled {
		go func() {
			_ = netutil.AllowInboundPort(req.Port, req.Protocol, listen)
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      id,
		"message": "inbound created and sing-box updated",
	})
}

func UpdateInbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req InboundCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Port < 1 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站端口必须在 1 ~ 65535 之间"})
		return
	}
	req.Tag = strings.TrimSpace(req.Tag)
	if !regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,64}$`).MatchString(req.Tag) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站 Tag 格式不合法 (仅支持1-64位字母、数字、点号、短横线与下划线)"})
		return
	}
	if req.Listen != "" && !isValidListenAddress(req.Listen) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "监听地址格式不合法，请输入合法的 IP 地址（如 ::, 0.0.0.0, 127.0.0.1）"})
		return
	}

	var oldIb model.Inbound
	var oldEnabledInt int
	var oldCreatedAtRaw, oldUpdatedAtRaw any
	err = model.DB.QueryRow("SELECT id, tag, protocol, listen, port, spec_json, enabled, created_at, updated_at FROM inbounds WHERE id = ?", id).
		Scan(&oldIb.ID, &oldIb.Tag, &oldIb.Protocol, &oldIb.Listen, &oldIb.Port, &oldIb.SpecJSON, &oldEnabledInt, &oldCreatedAtRaw, &oldUpdatedAtRaw)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inbound not found"})
		return
	}
	oldIb.Enabled = oldEnabledInt == 1
	oldIb.CreatedAt = model.ParseTime(oldCreatedAtRaw)
	oldIb.UpdatedAt = model.ParseTime(oldUpdatedAtRaw)

	listen := req.Listen
	if listen == "" {
		if req.Protocol == "vless-ws" || req.Protocol == "socks" || req.Protocol == "mixed" || req.Protocol == "http" {
			listen = "127.0.0.1"
		} else {
			listen = "::"
		}
	}

	specStr := strings.TrimSpace(string(req.Spec))
	if specStr == "" || specStr == "null" {
		specStr = "{}"
	}
	specStr = enrichInboundSpec(req.Protocol, specStr)

	now := time.Now()
	_, err = model.DB.Exec(
		"UPDATE inbounds SET tag = ?, protocol = ?, listen = ?, port = ?, spec_json = ?, enabled = ?, updated_at = ? WHERE id = ?",
		req.Tag, req.Protocol, listen, req.Port, specStr, req.Enabled, now, id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 若入站 tag 发生变更，级联自动同步更新关联路由规则中的 inbound_tags
	if oldIb.Tag != req.Tag {
		routeRows, rErr := model.DB.Query("SELECT id, inbound_tags FROM routes")
		if rErr == nil {
			defer routeRows.Close()
			for routeRows.Next() {
				var rID int64
				var inTags string
				if err := routeRows.Scan(&rID, &inTags); err == nil && inTags != "" {
					parts := strings.Split(inTags, ",")
					changed := false
					for i, part := range parts {
						if strings.TrimSpace(part) == oldIb.Tag {
							parts[i] = req.Tag
							changed = true
						}
					}
					if changed {
						newInTags := strings.Join(parts, ", ")
						_, _ = model.DB.Exec("UPDATE routes SET inbound_tags = ?, updated_at = ? WHERE id = ?", newInTags, now, rID)
					}
				}
			}
		}
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 回滚数据库记录
		_, _ = model.DB.Exec(
			"UPDATE inbounds SET tag = ?, protocol = ?, listen = ?, port = ?, spec_json = ?, enabled = ?, updated_at = ? WHERE id = ?",
			oldIb.Tag, oldIb.Protocol, oldIb.Listen, oldIb.Port, oldIb.SpecJSON, oldIb.Enabled, oldIb.UpdatedAt, id,
		)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站更新后配置生效失败，已自动回滚: " + err.Error()})
		return
	}

	if req.Enabled {
		go func() {
			_ = netutil.AllowInboundPort(req.Port, req.Protocol, listen)
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "inbound updated"})
}

func DeleteInbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var oldIb model.Inbound
	var oldEnabledInt int
	var oldCreatedAtRaw, oldUpdatedAtRaw any
	err = model.DB.QueryRow("SELECT id, tag, protocol, listen, port, spec_json, enabled, created_at, updated_at FROM inbounds WHERE id = ?", id).
		Scan(&oldIb.ID, &oldIb.Tag, &oldIb.Protocol, &oldIb.Listen, &oldIb.Port, &oldIb.SpecJSON, &oldEnabledInt, &oldCreatedAtRaw, &oldUpdatedAtRaw)
	if err != nil {
		// 幂等性保障：若该入站已被删除或已不存在，确保触发一次配置对齐并直接返回成功
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusOK, gin.H{"message": "inbound already deleted"})
		return
	}
	oldIb.Enabled = oldEnabledInt == 1
	oldIb.CreatedAt = model.ParseTime(oldCreatedAtRaw)
	oldIb.UpdatedAt = model.ParseTime(oldUpdatedAtRaw)

	// 依赖预检：检查是否有路由规则限定引用了该入站节点
	routeRows, rErr := model.DB.Query("SELECT name, inbound_tags FROM routes")
	if rErr == nil {
		defer routeRows.Close()
		var referencingRoutes []string
		for routeRows.Next() {
			var rName, inTags string
			if err := routeRows.Scan(&rName, &inTags); err == nil && inTags != "" {
				for _, part := range strings.Split(inTags, ",") {
					if strings.TrimSpace(part) == oldIb.Tag {
						referencingRoutes = append(referencingRoutes, rName)
						break
					}
				}
			}
		}
		if len(referencingRoutes) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("无法删除入站节点 [%s]：该节点正被以下路由规则限定引用：%s。请先修改或解除对应路由规则的入站绑定。", oldIb.Tag, strings.Join(referencingRoutes, "、")),
			})
			return
		}
	}

	_, err = model.DB.Exec("DELETE FROM inbounds WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 回滚恢复被删除的记录
		_, _ = model.DB.Exec(
			"INSERT INTO inbounds (id, tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			oldIb.ID, oldIb.Tag, oldIb.Protocol, oldIb.Listen, oldIb.Port, oldIb.SpecJSON, oldIb.Enabled, oldIb.CreatedAt, oldIb.UpdatedAt,
		)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "入站删除后配置生效失败，已自动恢复: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "inbound deleted"})
}

func PreviewSingboxConfig(c *gin.Context) {
	cfg, err := configgen.GenerateServerConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func ApplySingboxConfig(c *gin.Context) {
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sing-box 配置生效失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sing-box configuration verified and reloaded successfully"})
}

// isAllowedSingboxConfigPath 校验 sing-box 配置文件路径是否安全合法，防止任意系统敏感文件探测或破坏性覆盖
func isAllowedSingboxConfigPath(p string) bool {
	clean := filepath.Clean(strings.TrimSpace(p))
	if clean == "" || strings.Contains(clean, "..") {
		return false
	}
	if strings.ToLower(filepath.Ext(clean)) != ".json" {
		return false
	}
	defaultPath := filepath.Clean(singbox.GetConfigPath())
	if clean == defaultPath {
		return true
	}

	allowedDirs := []string{
		"/etc/vps-auto-deploy",
		"/etc/sing-box",
		"/usr/local/etc/sing-box",
		filepath.Dir(defaultPath),
	}
	// 生产环境严禁将 /tmp 作为合法配置路径，仅在明确指定测试环境变量时允许包含
	if os.Getenv("ALLOW_TEST_CONFIG_PATH") == "1" {
		allowedDirs = append(allowedDirs, os.TempDir())
	}
	if home := os.Getenv("HOME"); home != "" {
		allowedDirs = append(allowedDirs, filepath.Join(home, ".config/vps-auto-deploy"))
		allowedDirs = append(allowedDirs, filepath.Join(home, ".config/sing-box"))
	}
	for _, dir := range allowedDirs {
		cleanDir := filepath.Clean(dir)
		if clean == cleanDir || strings.HasPrefix(clean, cleanDir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ImportExistingInbounds 从宿主机已有 sing-box 配置文件一键导入节点与用户
func ImportExistingInbounds(c *gin.Context) {
	cfgPath := singbox.GetConfigPath()
	var req struct {
		Path string `json:"path"`
	}
	_ = c.ShouldBindJSON(&req)
	if customPath := strings.TrimSpace(req.Path); customPath != "" {
		if !isAllowedSingboxConfigPath(customPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "非法配置文件路径，仅支持受管目录下的 .json 配置文件"})
			return
		}
		cfgPath = customPath
	}

	if _, err := os.Stat(cfgPath); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到配置文件: " + cfgPath})
		return
	}

	stats, err := configgen.ImportExistingSingboxConfig(cfgPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "导入已有配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            fmt.Sprintf("导入成功！已导入 %d 个入站节点、%d 个团队成员、%d 个出站规则", stats.InboundsImported, stats.MembersImported, stats.OutboundsImported),
		"inbounds_imported":  stats.InboundsImported,
		"members_imported":   stats.MembersImported,
		"outbounds_imported": stats.OutboundsImported,
		"tags":               stats.InboundTags,
		"config_path":        cfgPath,
	})
}

