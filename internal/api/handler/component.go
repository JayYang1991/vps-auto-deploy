package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component"
)

type ComponentActionRequest struct {
	Action string `json:"action" binding:"required"` // "start" | "stop" | "restart"
}

func ListComponents(c *gin.Context) {
	comps, err := component.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, comps)
}

func InstallComponent(c *gin.Context) {
	name := c.Param("name")
	drv, err := component.Get(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	go func() {
		_ = drv.InstallOrUpgrade("latest")
	}()

	c.JSON(http.StatusOK, gin.H{"message": "installation task initiated for " + name})
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
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action"})
		return
	}

	if actionErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": actionErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": req.Action + " executed successfully"})
}

