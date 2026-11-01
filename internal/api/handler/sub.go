package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/subgen"
)

func ServeSubscription(c *gin.Context) {
	token := c.Param("token")
	target := c.Query("target")
	ua := c.GetHeader("User-Agent")

	res, err := subgen.GenerateSubscription(token, target, ua)
	if err != nil {
		c.String(http.StatusBadRequest, "Subscription Error: "+err.Error())
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", res.Filename))
	c.Header("Profile-Update-Interval", "24")
	c.Data(http.StatusOK, res.ContentType, []byte(res.Content))
}

