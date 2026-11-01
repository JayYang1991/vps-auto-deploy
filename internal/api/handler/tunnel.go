package handler

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/cloudflared"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/executil"
)

type TunnelCreateRequest struct {
	Name          string `json:"name" binding:"required"`
	Mode          string `json:"mode" binding:"required"` // "quick" | "token"
	Token         string `json:"token"`
	Domain        string `json:"domain"`
	TargetPort    int    `json:"target_port" binding:"required"`
	EdgeIPVersion string `json:"edge_ip_version"`
	Protocol      string `json:"protocol"`
}

func ListTunnels(c *gin.Context) {
	rows, err := model.DB.Query("SELECT id, name, mode, token, domain, target_port, edge_ip_version, protocol, status, enabled, created_at, updated_at FROM tunnels ORDER BY id DESC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var list []model.Tunnel
	for rows.Next() {
		var t model.Tunnel
		if err := rows.Scan(&t.ID, &t.Name, &t.Mode, &t.Token, &t.Domain, &t.TargetPort, &t.EdgeIPVersion, &t.Protocol, &t.Status, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err == nil {
			list = append(list, t)
		}
	}
	c.JSON(http.StatusOK, list)
}

func CreateTunnel(c *gin.Context) {
	var req TunnelCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	edgeIP := req.EdgeIPVersion
	if edgeIP == "" {
		edgeIP = "auto"
	}
	proto := req.Protocol
	if proto == "" {
		proto = "auto"
	}

	now := time.Now()
	res, err := model.DB.Exec(
		"INSERT INTO tunnels (name, mode, token, domain, target_port, edge_ip_version, protocol, status, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'stopped', 1, ?, ?)",
		req.Name, req.Mode, req.Token, req.Domain, req.TargetPort, edgeIP, proto, now, now,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// 启动并配置隧道
	drv := cloudflared.NewDriver()
	_ = drv.ConfigureService(req.Mode, req.Token, req.TargetPort, proto)
	_ = drv.Start()

	// 若为 quick 模式，异步从日志中提取 trycloudflare 域名
	if req.Mode == "quick" {
		go captureQuickTunnelDomain(id)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      id,
		"message": "tunnel created and service started",
	})
}

func DeleteTunnel(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	drv := cloudflared.NewDriver()
	_ = drv.Stop()

	_, err := model.DB.Exec("DELETE FROM tunnels WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tunnel deleted"})
}

func captureQuickTunnelDomain(tunnelID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	logChan, err := executil.StreamJournalctlLogs(ctx, cloudflared.UnitName, 20)
	if err != nil {
		return
	}

	re := regexp.MustCompile(`https://([a-zA-Z0-9-]+\.trycloudflare\.com)`)

	for line := range logChan {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			domain := matches[1]
			_, _ = model.DB.Exec("UPDATE tunnels SET domain = ?, status = 'running' WHERE id = ?", domain, tunnelID)
			return
		}
	}
}

