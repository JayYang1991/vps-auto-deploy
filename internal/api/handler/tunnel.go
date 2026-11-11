package handler

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
		var enabledInt int
		var createdAtRaw, updatedAtRaw any
		if err := rows.Scan(&t.ID, &t.Name, &t.Mode, &t.Token, &t.Domain, &t.TargetPort, &t.EdgeIPVersion, &t.Protocol, &t.Status, &enabledInt, &createdAtRaw, &updatedAtRaw); err == nil {
			t.Enabled = enabledInt == 1
			t.CreatedAt = model.ParseTime(createdAtRaw)
			t.UpdatedAt = model.ParseTime(updatedAtRaw)
			list = append(list, t)
		}
	}
	if list == nil {
		list = []model.Tunnel{}
	}
	c.JSON(http.StatusOK, list)
}

func extractToken(raw string) string {
	raw = regexp.MustCompile(`[\r\n\t]+`).ReplaceAllString(raw, " ")
	re := regexp.MustCompile(`(?:--token|install)\s+([A-Za-z0-9_\-\.]+)`)
	if matches := re.FindStringSubmatch(raw); len(matches) > 1 {
		return matches[1]
	}
	return regexp.MustCompile(`\s+`).ReplaceAllString(raw, "")
}

func CreateTunnel(c *gin.Context) {
	var req TunnelCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TargetPort <= 0 || req.TargetPort > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "目标端口必须介于 1 ~ 65535 之间"})
		return
	}

	cleanMode := strings.ToLower(strings.TrimSpace(req.Mode))
	if cleanMode != "token" && cleanMode != "quick" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "隧道模式仅支持 quick (临时试用) 或 token (持久命名)"})
		return
	}

	token := extractToken(req.Token)
	if cleanMode == "token" {
		if token == "" || !regexp.MustCompile(`^[A-Za-z0-9_\-\.]{10,512}$`).MatchString(token) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Token 格式无效，请填写 Cloudflare 控制台给出的合法 Tunnel Token"})
			return
		}
	}

	edgeIP := strings.ToLower(strings.TrimSpace(req.EdgeIPVersion))
	if edgeIP == "" {
		edgeIP = "auto"
	}
	proto := strings.ToLower(strings.TrimSpace(req.Protocol))
	if proto == "" {
		proto = "auto"
	}
	if proto != "auto" && proto != "quic" && proto != "http2" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "传输协议仅支持 auto, quic 或 http2"})
		return
	}

	now := time.Now()
	res, err := model.DB.Exec(
		"INSERT INTO tunnels (name, mode, token, domain, target_port, edge_ip_version, protocol, status, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'stopped', 1, ?, ?)",
		req.Name, cleanMode, token, req.Domain, req.TargetPort, edgeIP, proto, now, now,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()

	// 启动并配置隧道（若 cloudflared 已安装）
	drv := cloudflared.NewDriver()
	ver, _ := drv.GetInstalledVersion()
	if ver != "" {
		_ = drv.ConfigureService(cleanMode, token, req.TargetPort, proto)
		_ = drv.Start()

		// 若为 quick 模式，异步从日志中提取 trycloudflare 域名
		if cleanMode == "quick" {
			go captureQuickTunnelDomain(id)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      id,
		"message": "tunnel saved successfully",
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

