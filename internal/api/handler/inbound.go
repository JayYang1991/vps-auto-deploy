package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
)

type InboundCreateRequest struct {
	Tag      string          `json:"tag" binding:"required"`
	Protocol string          `json:"protocol" binding:"required"` // "vless-reality" | "vless-ws"
	Listen   string          `json:"listen"`
	Port     int             `json:"port" binding:"required"`
	Spec     json.RawMessage `json:"spec" binding:"required"`
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
		if err := rows.Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Listen, &ib.Port, &ib.SpecJSON, &ib.Enabled, &ib.CreatedAt, &ib.UpdatedAt); err == nil {
			list = append(list, ib)
		}
	}
	c.JSON(http.StatusOK, list)
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

func CreateInbound(c *gin.Context) {
	var req InboundCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	listen := req.Listen
	if listen == "" {
		if req.Protocol == "vless-ws" {
			listen = "127.0.0.1"
		} else {
			listen = "::"
		}
	}

	now := time.Now()
	res, err := model.DB.Exec(
		"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		req.Tag, req.Protocol, listen, req.Port, string(req.Spec), req.Enabled, now, now,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to insert inbound: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// 自动触发一次配置生成与热重载
	_ = configgen.ApplyAndReload(singbox.ConfigPath)

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

	listen := req.Listen
	if listen == "" {
		if req.Protocol == "vless-ws" {
			listen = "127.0.0.1"
		} else {
			listen = "::"
		}
	}

	now := time.Now()
	_, err = model.DB.Exec(
		"UPDATE inbounds SET tag = ?, protocol = ?, listen = ?, port = ?, spec_json = ?, enabled = ?, updated_at = ? WHERE id = ?",
		req.Tag, req.Protocol, listen, req.Port, string(req.Spec), req.Enabled, now, id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = configgen.ApplyAndReload(singbox.ConfigPath)

	c.JSON(http.StatusOK, gin.H{"message": "inbound updated"})
}

func DeleteInbound(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	_, err := model.DB.Exec("DELETE FROM inbounds WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_ = configgen.ApplyAndReload(singbox.ConfigPath)

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
	if err := configgen.ApplyAndReload(singbox.ConfigPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sing-box configuration verified and reloaded successfully"})
}

