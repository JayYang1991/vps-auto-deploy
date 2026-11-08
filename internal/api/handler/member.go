package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
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

func ListMembers(c *gin.Context) {
	rows, err := model.DB.Query("SELECT id, name, uuid, sub_token, enabled, created_at, updated_at FROM members ORDER BY id ASC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var list []model.Member
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.ID, &m.Name, &m.UUID, &m.SubToken, &m.Enabled, &m.CreatedAt, &m.UpdatedAt); err == nil {
			list = append(list, m)
		}
	}
	c.JSON(http.StatusOK, list)
}

func CreateMember(c *gin.Context) {
	var req MemberCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
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
		req.Name, newUUID, subToken, req.Enabled, now, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// 重新生成 sing-box 配置
	if err := configgen.ApplyAndReload(singbox.ConfigPath); err != nil {
		_, _ = model.DB.Exec("DELETE FROM members WHERE id = ?", id)
		_ = configgen.ApplyAndReload(singbox.ConfigPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "成员创建后配置生效失败，已自动回滚: " + err.Error()})
		return
	}

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
	err = model.DB.QueryRow("SELECT id, name, uuid, enabled FROM members WHERE id = ?", id).
		Scan(&existing.ID, &existing.Name, &existing.UUID, &existing.Enabled)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	name := existing.Name
	if strings.TrimSpace(req.Name) != "" {
		name = strings.TrimSpace(req.Name)
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
	if err := configgen.ApplyAndReload(singbox.ConfigPath); err != nil {
		_, _ = model.DB.Exec(
			"UPDATE members SET name = ?, uuid = ?, enabled = ?, updated_at = ? WHERE id = ?",
			existing.Name, existing.UUID, existing.Enabled, existing.UpdatedAt, id,
		)
		_ = configgen.ApplyAndReload(singbox.ConfigPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "成员更新后配置生效失败，已自动回滚: " + err.Error()})
		return
	}

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
	err = model.DB.QueryRow("SELECT id, name, uuid, sub_token, enabled, created_at, updated_at FROM members WHERE id = ?", id).
		Scan(&oldM.ID, &oldM.Name, &oldM.UUID, &oldM.SubToken, &oldM.Enabled, &oldM.CreatedAt, &oldM.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	_, err = model.DB.Exec("DELETE FROM members WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.ConfigPath); err != nil {
		_, _ = model.DB.Exec(
			"INSERT INTO members (id, name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			oldM.ID, oldM.Name, oldM.UUID, oldM.SubToken, oldM.Enabled, oldM.CreatedAt, oldM.UpdatedAt,
		)
		_ = configgen.ApplyAndReload(singbox.ConfigPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "成员删除后配置生效失败，已自动恢复: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member deleted"})
}

