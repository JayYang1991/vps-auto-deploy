package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
)

type OutboundRequest struct {
	Tag        string          `json:"tag" binding:"required"`
	Type       string          `json:"type" binding:"required"` // direct | block | socks | http | shadowsocks | vless | custom
	Server     string          `json:"server"`
	ServerPort int             `json:"server_port"`
	Spec       json.RawMessage `json:"spec"`
	Priority   int             `json:"priority"`
	Enabled    bool            `json:"enabled"`
}

// ListOutbounds 查询所有出站规则
func ListOutbounds(c *gin.Context) {
	rows, err := model.DB.Query("SELECT id, tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at FROM outbounds ORDER BY priority ASC, id ASC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var list []model.Outbound
	for rows.Next() {
		var ob model.Outbound
		var enabledInt, isSystemInt int
		var createdAtRaw, updatedAtRaw any
		if err := rows.Scan(&ob.ID, &ob.Tag, &ob.Type, &ob.Server, &ob.ServerPort, &ob.SpecJSON, &ob.Priority, &enabledInt, &isSystemInt, &createdAtRaw, &updatedAtRaw); err == nil {
			ob.Enabled = enabledInt == 1
			ob.IsSystem = isSystemInt == 1
			ob.CreatedAt = model.ParseTime(createdAtRaw)
			ob.UpdatedAt = model.ParseTime(updatedAtRaw)
			list = append(list, ob)
		}
	}
	if list == nil {
		list = []model.Outbound{}
	}
	c.JSON(http.StatusOK, list)
}

// GetOutbound 获取指定出站详情
func GetOutbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid outbound id"})
		return
	}

	var ob model.Outbound
	var enabledInt, isSystemInt int
	var createdAtRaw, updatedAtRaw any
	err = model.DB.QueryRow("SELECT id, tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at FROM outbounds WHERE id = ?", id).
		Scan(&ob.ID, &ob.Tag, &ob.Type, &ob.Server, &ob.ServerPort, &ob.SpecJSON, &ob.Priority, &enabledInt, &isSystemInt, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "outbound not found"})
		return
	}
	ob.Enabled = enabledInt == 1
	ob.IsSystem = isSystemInt == 1
	ob.CreatedAt = model.ParseTime(createdAtRaw)
	ob.UpdatedAt = model.ParseTime(updatedAtRaw)
	c.JSON(http.StatusOK, ob)
}

// CreateOutbound 新建出站规则
func CreateOutbound(c *gin.Context) {
	var req OutboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	specStr := string(req.Spec)
	if specStr == "" || specStr == "null" {
		specStr = "{}"
	}

	priority := req.Priority
	if priority <= 0 {
		priority = 10
	}

	now := time.Now()
	res, err := model.DB.Exec(
		"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?)",
		req.Tag, req.Type, req.Server, req.ServerPort, specStr, priority, req.Enabled, now, now,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "创建出站规则失败: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// 重新生成并应用 sing-box 配置
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 出站配置生效失败，回滚 DB 插入
		_, _ = model.DB.Exec("DELETE FROM outbounds WHERE id = ?", id)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "出站规则生效失败，已自动回滚: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "message": "出站规则已创建并生效"})
}

// UpdateOutbound 更新出站规则
func UpdateOutbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid outbound id"})
		return
	}

	var req OutboundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查并备份当前出站记录
	var oldOb model.Outbound
	var oldEnabledInt, oldIsSystemInt int
	var oldCreatedAtRaw, oldUpdatedAtRaw any
	err = model.DB.QueryRow("SELECT id, tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at FROM outbounds WHERE id = ?", id).
		Scan(&oldOb.ID, &oldOb.Tag, &oldOb.Type, &oldOb.Server, &oldOb.ServerPort, &oldOb.SpecJSON, &oldOb.Priority, &oldEnabledInt, &oldIsSystemInt, &oldCreatedAtRaw, &oldUpdatedAtRaw)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "outbound not found"})
		return
	}
	oldOb.Enabled = oldEnabledInt == 1
	oldOb.IsSystem = oldIsSystemInt == 1
	oldOb.CreatedAt = model.ParseTime(oldCreatedAtRaw)
	oldOb.UpdatedAt = model.ParseTime(oldUpdatedAtRaw)

	specStr := string(req.Spec)
	if specStr == "" || specStr == "null" {
		specStr = "{}"
	}

	priority := req.Priority
	if priority <= 0 {
		priority = 10
	}

	now := time.Now()
	// 若是系统预置规则，禁止篡改核心 type
	if oldOb.IsSystem {
		_, err = model.DB.Exec(
			"UPDATE outbounds SET priority = ?, enabled = ?, updated_at = ? WHERE id = ?",
			priority, req.Enabled, now, id,
		)
	} else {
		_, err = model.DB.Exec(
			"UPDATE outbounds SET tag = ?, type = ?, server = ?, server_port = ?, spec_json = ?, priority = ?, enabled = ?, updated_at = ? WHERE id = ?",
			req.Tag, req.Type, req.Server, req.ServerPort, specStr, priority, req.Enabled, now, id,
		)
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "更新出站规则失败: " + err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 回滚出站数据库记录
		_, _ = model.DB.Exec(
			"UPDATE outbounds SET tag = ?, type = ?, server = ?, server_port = ?, spec_json = ?, priority = ?, enabled = ?, updated_at = ? WHERE id = ?",
			oldOb.Tag, oldOb.Type, oldOb.Server, oldOb.ServerPort, oldOb.SpecJSON, oldOb.Priority, oldOb.Enabled, oldOb.UpdatedAt, id,
		)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "出站规则更新后生效失败，已自动回滚: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "出站规则已更新并热重载生效"})
}

// ToggleOutbound 快速切换出站启用状态
func ToggleOutbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid outbound id"})
		return
	}

	var oldEnabled bool
	err = model.DB.QueryRow("SELECT enabled FROM outbounds WHERE id = ?", id).Scan(&oldEnabled)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "outbound not found"})
		return
	}

	now := time.Now()
	newEnabled := !oldEnabled
	_, err = model.DB.Exec("UPDATE outbounds SET enabled = ?, updated_at = ? WHERE id = ?", newEnabled, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 回滚状态
		_, _ = model.DB.Exec("UPDATE outbounds SET enabled = ?, updated_at = ? WHERE id = ?", oldEnabled, now, id)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "出站状态切换后生效失败，已自动回滚: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "出站状态已更新"})
}

// DeleteOutbound 删除出站规则
func DeleteOutbound(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid outbound id"})
		return
	}

	var oldOb model.Outbound
	var oldEnabledInt, oldIsSystemInt int
	var oldCreatedAtRaw, oldUpdatedAtRaw any
	err = model.DB.QueryRow("SELECT id, tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at FROM outbounds WHERE id = ?", id).
		Scan(&oldOb.ID, &oldOb.Tag, &oldOb.Type, &oldOb.Server, &oldOb.ServerPort, &oldOb.SpecJSON, &oldOb.Priority, &oldEnabledInt, &oldIsSystemInt, &oldCreatedAtRaw, &oldUpdatedAtRaw)
	if err != nil {
		// 幂等性保障：若该出站规则已被删除或已不存在，确保触发一次配置对齐并直接返回成功
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusOK, gin.H{"message": "outbound already deleted"})
		return
	}
	oldOb.Enabled = oldEnabledInt == 1
	oldOb.IsSystem = oldIsSystemInt == 1
	oldOb.CreatedAt = model.ParseTime(oldCreatedAtRaw)
	oldOb.UpdatedAt = model.ParseTime(oldUpdatedAtRaw)
	if oldOb.IsSystem {
		c.JSON(http.StatusBadRequest, gin.H{"error": "系统核心预置出站规则不允许删除"})
		return
	}

	_, err = model.DB.Exec("DELETE FROM outbounds WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 恢复被删除的出站规则
		_, _ = model.DB.Exec(
			"INSERT INTO outbounds (id, tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			oldOb.ID, oldOb.Tag, oldOb.Type, oldOb.Server, oldOb.ServerPort, oldOb.SpecJSON, oldOb.Priority, oldOb.Enabled, oldOb.IsSystem, oldOb.CreatedAt, oldOb.UpdatedAt,
		)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "出站删除后配置生效失败，已自动恢复: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "出站规则已成功删除"})
}

// GetSingboxFullConfig 查看当前 sing-box 服务端完整生成配置与磁盘状态
func GetSingboxFullConfig(c *gin.Context) {
	cfg, genErr := configgen.GenerateServerConfig()
	if genErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成配置失败: " + genErr.Error()})
		return
	}

	rawJSON, _ := json.MarshalIndent(cfg, "", "  ")

	cfgPath := singbox.GetConfigPath()
	diskBytes, fileErr := os.ReadFile(cfgPath)
	fileExists := fileErr == nil
	diskRawJSON := ""
	diskInboundCount := 0
	if fileExists {
		diskRawJSON = string(diskBytes)
		var diskRaw struct {
			Inbounds []map[string]interface{} `json:"inbounds"`
		}
		if err := json.Unmarshal(diskBytes, &diskRaw); err == nil {
			diskInboundCount = len(diskRaw.Inbounds)
		}
	}

	driver := singbox.NewDriver()
	st, _ := driver.Status()

	checkOK := true
	checkOutput := "配置语法检查通过"
	if st.Installed {
		tmpCheck := filepath.Join(os.TempDir(), fmt.Sprintf("sb_chk_%d.json", time.Now().UnixNano()))
		_ = os.WriteFile(tmpCheck, rawJSON, 0644)
		defer os.Remove(tmpCheck)

		if err := singbox.CheckConfig(tmpCheck); err != nil {
			checkOK = false
			checkOutput = err.Error()
		}
	} else {
		checkOutput = "sing-box 核心未安装，将在安装后自动校验生效"
	}

	// 语义级深度等价比对 (避免因空白、末尾换行或字段序列化排版差异导致的假性不同步)
	isSynced := false
	if fileExists {
		if strings.TrimSpace(diskRawJSON) == strings.TrimSpace(string(rawJSON)) {
			isSynced = true
		} else {
			var diskObj, genObj map[string]interface{}
			if err1 := json.Unmarshal([]byte(diskRawJSON), &diskObj); err1 == nil {
				if err2 := json.Unmarshal(rawJSON, &genObj); err2 == nil {
					isSynced = reflect.DeepEqual(diskObj, genObj)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"config":             cfg,
		"raw_json":           string(rawJSON),
		"disk_raw_json":      diskRawJSON,
		"config_path":        cfgPath,
		"file_exists":        fileExists,
		"is_synced":          isSynced,
		"check_ok":           checkOK,
		"check_output":       checkOutput,
		"running_status":     st.RunningStatus,
		"installed":          st.Installed,
		"disk_inbound_count": diskInboundCount,
	})
}

// ReloadSingboxService 强制重新应用并重启/重载 sing-box
func ReloadSingboxService(c *gin.Context) {
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "重载失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sing-box 服务已成功重新加载最新配置！"})
}

// LoadSingboxFromDisk 从磁盘配置文件强制加载并 100% 覆盖面板数据库记录，并闭环规范化回盘
func LoadSingboxFromDisk(c *gin.Context) {
	cfgPath := singbox.GetConfigPath()
	var req struct {
		Path string `json:"path"`
	}
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.Path) != "" {
		cfgPath = strings.TrimSpace(req.Path)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未找到磁盘配置文件: " + cfgPath})
		return
	}

	stats, err := configgen.ForceSyncSingboxConfig(cfgPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "从磁盘强制加载配置失败: " + err.Error()})
		return
	}

	// 核心闭环：将反向导入落盘的最新配置通过标准序列化规范化安全回盘，达成双向 100% 同步平衡态
	if applyErr := configgen.ApplyAndReload(cfgPath); applyErr != nil {
		fmt.Printf("[LoadSingboxFromDisk] 规范化回盘重载提示: %v\n", applyErr)
	}

	msg := fmt.Sprintf(
		"成功从磁盘加载并完成双向规范化对齐！已同步 %d 个入站节点（新增 %d/更新 %d/清理 %d）、%d 个出站规则（新增 %d/更新 %d/清理 %d）",
		len(stats.InboundTags), stats.InboundsCreated, stats.InboundsUpdated, stats.InboundsDeleted,
		len(stats.OutboundTags), stats.OutboundsCreated, stats.OutboundsUpdated, stats.OutboundsDeleted,
	)
	if stats.RoutesCreated > 0 {
		msg += fmt.Sprintf("、%d 条路由分流规则", stats.RoutesCreated)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": msg,
		"stats":   stats,
	})
}

