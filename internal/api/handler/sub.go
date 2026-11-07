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
	engine := c.Query("engine")
	config := c.Query("config")
	ua := c.GetHeader("User-Agent")

	// 若请求使用 Subconverter 引擎或指定了自定义规则配置
	if engine == "subconverter" || config != "" {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		host := c.Request.Host
		sourceURL := fmt.Sprintf("%s://%s/sub/%s?target=base64", scheme, host, token)

		extraParams := make(map[string]string)
		if emoji := c.Query("emoji"); emoji != "" {
			extraParams["emoji"] = emoji
		}
		if udp := c.Query("udp"); udp != "" {
			extraParams["udp"] = udp
		}
		if scv := c.Query("scv"); scv != "" {
			extraParams["scv"] = scv
		}

		clientTarget := target
		if clientTarget == "" {
			clientTarget = "clash"
		}

		body, contentType, err := ProxySubconverterConvert(clientTarget, sourceURL, config, extraParams)
		if err == nil {
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"sub-%s-%s.txt\"", token[:8], clientTarget))
			c.Header("Profile-Update-Interval", "24")
			c.Data(http.StatusOK, contentType, body)
			return
		}
	}

	res, err := subgen.GenerateSubscription(token, target, ua)
	if err != nil {
		c.String(http.StatusBadRequest, "Subscription Error: "+err.Error())
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", res.Filename))
	c.Header("Profile-Update-Interval", "24")
	c.Data(http.StatusOK, res.ContentType, []byte(res.Content))
}

