package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/executil"
	"vps-auto-deploy/internal/pkg/netutil"
)

type SystemOverview struct {
	PublicIP   string                    `json:"public_ip"`
	OS         string                    `json:"os"`
	Arch       string                    `json:"arch"`
	NumCPU     int                       `json:"num_cpu"`
	BBR        bool                      `json:"bbr"`
	Components []component.ServiceStatus `json:"components"`
	SecretPath string                    `json:"secret_path"`
}

func GetSystemOverview(c *gin.Context) {
	pubIP, _ := model.GetSetting("public_ip")
	if pubIP == "" {
		pubIP = netutil.GetPublicIP()
	}

	secretPath, _ := model.GetSetting("secret_path")

	// 检查 BBR 状态
	bbrEnabled := false
	out, err := executil.RunCommand(2*time.Second, "sysctl", "net.ipv4.tcp_congestion_control")
	if err == nil && strings.Contains(out, "bbr") {
		bbrEnabled = true
	}

	comps, _ := component.ListAll()

	c.JSON(http.StatusOK, SystemOverview{
		PublicIP:   pubIP,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		NumCPU:     runtime.NumCPU(),
		BBR:        bbrEnabled,
		Components: comps,
		SecretPath: secretPath,
	})
}

// EnableBBR 一键开启 BBR 拥塞控制算法
func EnableBBR(c *gin.Context) {
	bbrConf := "\nnet.core.default_qdisc=fq\nnet.ipv4.tcp_congestion_control=bbr\n"
	f, err := os.OpenFile("/etc/sysctl.conf", os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(bbrConf)
		f.Close()
		_, _ = executil.RunCommand(3*time.Second, "sysctl", "-p")
		c.JSON(http.StatusOK, gin.H{"message": "BBR enabled successfully"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write sysctl: " + err.Error()})
}

// StreamLogs SSE 实时推送 journalctl 实时日志
func StreamLogs(c *gin.Context) {
	unit := c.Query("unit")
	if unit == "" {
		unit = "sing-box"
	}
	if !strings.HasSuffix(unit, ".service") {
		unit += ".service"
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	logChan, err := executil.StreamJournalctlLogs(ctx, unit, 50)
	if err != nil {
		c.SSEvent("error", err.Error())
		return
	}

	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case line, ok := <-logChan:
			if !ok {
				return false
			}
			c.SSEvent("message", line)
			return true
		case <-time.After(30 * time.Second):
			// 保活心跳 ping
			c.SSEvent("ping", fmt.Sprintf("%d", time.Now().Unix()))
			return true
		}
	})
}

