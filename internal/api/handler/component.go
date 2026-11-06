package handler

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component"
)

type ComponentActionRequest struct {
	Action string `json:"action" binding:"required"` // "start" | "stop" | "restart" | "uninstall"
}

type ComponentInstallRequest struct {
	Version string `json:"version"` // 默认为 "latest"
}

func ListComponents(c *gin.Context) {
	checkLatest := c.Query("check_updates") == "true"
	comps, err := component.ListAll(checkLatest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, comps)
}

func InstallComponent(c *gin.Context) {
	name := c.Param("name")
	var req ComponentInstallRequest
	_ = c.ShouldBindJSON(&req)

	version := req.Version
	if version == "" {
		version = "latest"
	}

	task, err := component.StartInstallTask(name, version)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("已启动 %s 安装/升级任务", name),
		"task":    task,
	})
}

func GetInstallStatus(c *gin.Context) {
	name := c.Param("name")
	task := component.GetInstallTask(name)
	if task == nil {
		c.JSON(http.StatusOK, gin.H{"status": "idle"})
		return
	}
	c.JSON(http.StatusOK, task)
}

// StreamInstallLogs 通过 SSE 实时推送安装/升级进度日志
func StreamInstallLogs(c *gin.Context) {
	name := c.Param("name")
	task := component.GetInstallTask(name)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active install task for " + name})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	ch := task.Subscribe()
	defer task.Unsubscribe(ch)

	notify := c.Request.Context().Done()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-notify:
			return false
		case logLine, ok := <-ch:
			if !ok {
				return false
			}
			c.SSEvent("log", logLine)
			return true
		case <-time.After(30 * time.Second):
			c.SSEvent("ping", "keepalive")
			return true
		}
	})
}

func ActionComponent(c *gin.Context) {
	name := c.Param("name")
	var req ComponentActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action is required"})
		return
	}

	drv, err := component.Get(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var actionErr error
	switch req.Action {
	case "start":
		actionErr = drv.Start()
	case "stop":
		actionErr = drv.Stop()
	case "restart":
		actionErr = drv.Restart()
	case "uninstall":
		actionErr = drv.Uninstall()
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action: " + req.Action})
		return
	}

	if actionErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": actionErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": req.Action + " executed successfully"})
}
