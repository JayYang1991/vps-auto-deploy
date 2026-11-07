package handler

import (
	"fmt"
	"net/http"
	"strings"

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
		member, err := subgen.GetMemberByToken(token)
		if err != nil {
			c.String(http.StatusBadRequest, "Subscription Error: "+err.Error())
			return
		}

		uris, err := subgen.GenerateURIs(member)
		if err != nil || len(uris) == 0 {
			c.String(http.StatusBadRequest, "当前无可用入站节点供转换")
			return
		}
		// 将节点链接以管道符 '|' 连接直接传给 Subconverter，零内部网络回环依赖
		sourceNodes := strings.Join(uris, "|")

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

		body, contentType, err := ProxySubconverterConvert(clientTarget, sourceNodes, config, extraParams)
		if err != nil {
			c.String(http.StatusBadGateway, fmt.Sprintf("Subconverter 转换失败: %v", err))
			return
		}

		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"sub-%s-%s.txt\"", token[:8], clientTarget))
		c.Header("Profile-Update-Interval", "24")
		c.Data(http.StatusOK, contentType, body)
		return
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
