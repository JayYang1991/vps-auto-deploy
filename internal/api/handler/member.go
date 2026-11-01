package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
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
	_ = configgen.ApplyAndReload(singbox.ConfigPath)

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

func DeleteMember(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	_, err := model.DB.Exec("DELETE FROM members WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = configgen.ApplyAndReload(singbox.ConfigPath)

	c.JSON(http.StatusOK, gin.H{"message": "member deleted"})
}

