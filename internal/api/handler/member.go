package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
)

type MemberCreateRequest struct {
	Name    string `json:"name" binding:"required"`
	Enabled bool   `json:"enabled"`
}

type MemberUpdateRequest struct {
	Name    string `json:"name"`
	UUID    string `json:"uuid"`
	Enabled *bool  `json:"enabled"`
}

func cleanMemberName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\t", " ")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("成员名称不能为空")
	}
	if len(name) > 64 {
		return "", errors.New("成员名称长度不能超过 64 个字符")
	}
	return name, nil
}

func ListMembers(c *gin.Context) {
	list, err := model.ListMembers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Member{}
	}
	c.JSON(http.StatusOK, list)
}

func CreateMember(c *gin.Context) {
	var req MemberCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	name, err := cleanMemberName(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	subBytes := make([]byte, 8)
	_, _ = rand.Read(subBytes)
	subToken := hex.EncodeToString(subBytes)

	now := time.Now()
	newUUID := uuid.New().String()

	res, err := model.DB.Exec(
		"INSERT INTO members (name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		name, newUUID, subToken, req.Enabled, now, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// 重新生成 sing-box 配置
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		_, _ = model.DB.Exec("DELETE FROM members WHERE id = ?", id)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "成员创建后配置生效失败，已自动回滚: " + err.Error()})
		return
	}

	// 异步全网广播同步至各受管节点
	go node.BroadcastMembersSync()

	c.JSON(http.StatusOK, gin.H{
		"id":        id,
		"uuid":      newUUID,
		"sub_token": subToken,
		"message":   "member created",
	})
}

func ResetMemberToken(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	subBytes := make([]byte, 8)
	_, _ = rand.Read(subBytes)
	newToken := hex.EncodeToString(subBytes)

	_, err := model.DB.Exec("UPDATE members SET sub_token = ?, updated_at = ? WHERE id = ?", newToken, time.Now(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	go node.BroadcastMembersSync()

	c.JSON(http.StatusOK, gin.H{
		"sub_token": newToken,
		"message":   "token reset successfully",
	})
}

func UpdateMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member id"})
		return
	}

	var req MemberUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existing model.Member
	var enabledInt int
	err = model.DB.QueryRow("SELECT id, name, uuid, enabled FROM members WHERE id = ?", id).
		Scan(&existing.ID, &existing.Name, &existing.UUID, &enabledInt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	existing.Enabled = enabledInt == 1

	name := existing.Name
	if strings.TrimSpace(req.Name) != "" {
		cleaned, err := cleanMemberName(req.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		name = cleaned
	}

	u := existing.UUID
	if strings.TrimSpace(req.UUID) != "" {
		if _, err := uuid.Parse(req.UUID); err == nil {
			u = req.UUID
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid uuid format"})
			return
		}
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	_, err = model.DB.Exec(
		"UPDATE members SET name = ?, uuid = ?, enabled = ?, updated_at = ? WHERE id = ?",
		name, u, enabled, time.Now(), id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 重新生成 sing-box 配置以更新 inbounds 中的 users[].name
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		_, _ = model.DB.Exec(
			"UPDATE members SET name = ?, uuid = ?, enabled = ?, updated_at = ? WHERE id = ?",
			existing.Name, existing.UUID, existing.Enabled, existing.UpdatedAt, id,
		)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "成员更新后配置生效失败，已自动回滚: " + err.Error()})
		return
	}

	go node.BroadcastMembersSync()

	c.JSON(http.StatusOK, gin.H{
		"message": "member updated successfully",
		"id":      id,
		"name":    name,
		"uuid":    u,
		"enabled": enabled,
	})
}

func DeleteMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var oldM model.Member
	var enabledInt int
	var createdAtRaw, updatedAtRaw any
	err = model.DB.QueryRow("SELECT id, name, uuid, sub_token, enabled, created_at, updated_at FROM members WHERE id = ?", id).
		Scan(&oldM.ID, &oldM.Name, &oldM.UUID, &oldM.SubToken, &enabledInt, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	oldM.Enabled = enabledInt == 1
	oldM.CreatedAt = model.ParseTime(createdAtRaw)
	oldM.UpdatedAt = model.ParseTime(updatedAtRaw)

	_, err = model.DB.Exec("DELETE FROM members WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		_, _ = model.DB.Exec(
			"INSERT INTO members (id, name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			oldM.ID, oldM.Name, oldM.UUID, oldM.SubToken, oldM.Enabled, oldM.CreatedAt, oldM.UpdatedAt,
		)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": "成员删除后配置生效失败，已自动恢复: " + err.Error()})
		return
	}

	go node.BroadcastMembersSync()

	c.JSON(http.StatusOK, gin.H{"message": "member deleted"})
}

// SyncMembers 被纳管节点接收来自主控端的成员身份同步（支持 mode=replace 全量原子替换或 mode=merge 增量合并）
func SyncMembers(c *gin.Context) {
	var members []model.Member
	if err := c.ShouldBindJSON(&members); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的成员数据: " + err.Error()})
		return
	}

	mode := c.DefaultQuery("mode", "merge")
	var syncErr error
	if mode == "replace" {
		syncErr = model.ReplaceMembers(members)
	} else {
		syncErr = model.SyncMembers(members)
	}

	if syncErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入本地成员数据失败: " + syncErr.Error()})
		return
	}

	// 重新生成本地 sing-box 服务端配置并热重载
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		log.Printf("[SyncMembers] 成员同步成功，但 sing-box 配置应用告警: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"message": "成员已存入数据库，但 sing-box 配置重载提示: " + err.Error(),
			"count":   len(members),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "members synchronized successfully",
		"count":   len(members),
	})
}

// BroadcastSyncMembers 主控端主动触发全集群成员凭据广播同步
func BroadcastSyncMembers(c *gin.Context) {
	total, success, failed, errs := node.BroadcastMembersSync()
	var errMsgs []string
	for _, e := range errs {
		errMsgs = append(errMsgs, e.Error())
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "全网凭据广播同步完成",
		"total":   total,
		"success": success,
		"failed":  failed,
		"errors":  errMsgs,
	})
}


