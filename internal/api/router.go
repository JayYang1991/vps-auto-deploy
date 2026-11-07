package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/api/handler"
	"vps-auto-deploy/internal/api/middleware"
	"vps-auto-deploy/internal/model"
)

// SetupRouter 初始化 Gin 路由
func SetupRouter(webFS fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.Use(gin.Recovery())
	r.Use(middleware.CORSMiddleware())

	// 1. 公开免认证订阅接口
	r.GET("/sub/:token", handler.ServeSubscription)

	// 2. 带安全路径前缀的 API 区域 /:secret/api/...
	secGroup := r.Group("/:secret")
	secGroup.Use(middleware.SecretPathMiddleware())
	{
		// 登录接口
		secGroup.POST("/api/auth/login", handler.Login)

		// 需登录的 API 接口
		api := secGroup.Group("/api")
		api.Use(middleware.JWTAuthMiddleware())
		{
			// 设置与密码
			api.POST("/auth/password", handler.UpdatePassword)
			api.GET("/settings", handler.GetSettings)
			api.POST("/settings", handler.UpdateSettings)

			// 系统监控与日志
			api.GET("/system/overview", handler.GetSystemOverview)
			api.POST("/system/bbr", handler.EnableBBR)
			api.GET("/system/logs", handler.StreamLogs)

			// 组件管理
			api.GET("/components", handler.ListComponents)
			api.POST("/components/:name/install", handler.InstallComponent)
			api.GET("/components/:name/install-status", handler.GetInstallStatus)
			api.GET("/components/:name/install-logs", handler.StreamInstallLogs)
			api.POST("/components/:name/action", handler.ActionComponent)

			// 入站节点与 Reality
			api.GET("/inbounds", handler.ListInbounds)
			api.POST("/inbounds", handler.CreateInbound)
			api.PUT("/inbounds/:id", handler.UpdateInbound)
			api.DELETE("/inbounds/:id", handler.DeleteInbound)
			api.POST("/inbounds/reality-keys", handler.GenerateRealityKeys)
			api.GET("/inbounds/preview", handler.PreviewSingboxConfig)
			api.POST("/inbounds/apply", handler.ApplySingboxConfig)

			// 出站规则管理
			api.GET("/outbounds", handler.ListOutbounds)
			api.POST("/outbounds", handler.CreateOutbound)
			api.GET("/outbounds/:id", handler.GetOutbound)
			api.PUT("/outbounds/:id", handler.UpdateOutbound)
			api.POST("/outbounds/:id/toggle", handler.ToggleOutbound)
			api.DELETE("/outbounds/:id", handler.DeleteOutbound)

			// sing-box 完整配置透视与重载
			api.GET("/singbox/config", handler.GetSingboxFullConfig)
			api.POST("/singbox/reload", handler.ReloadSingboxService)

			// 隧道管理
			api.GET("/tunnels", handler.ListTunnels)
			api.POST("/tunnels", handler.CreateTunnel)
			api.DELETE("/tunnels/:id", handler.DeleteTunnel)

			// 成员与订阅
			api.GET("/members", handler.ListMembers)
			api.POST("/members", handler.CreateMember)
			api.POST("/members/:id/reset-token", handler.ResetMemberToken)
			api.DELETE("/members/:id", handler.DeleteMember)
		}
	}

	// 3. 静态前端资源分发与 SPA Fallback
	r.NoRoute(func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		actualSecret, err := model.GetSetting("secret_path")
		if err != nil || actualSecret == "" {
			c.String(http.StatusNotFound, "404 page not found")
			return
		}

		secretPrefix := "/" + actualSecret

		// 访问 /<secret> 或 /<secret>/ 或其子资源
		if reqPath == secretPrefix || strings.HasPrefix(reqPath, secretPrefix+"/") {
			subPath := strings.TrimPrefix(reqPath, secretPrefix)
			subPath = strings.TrimPrefix(subPath, "/")

			if strings.HasPrefix(subPath, "api") {
				c.JSON(http.StatusNotFound, gin.H{"error": "api endpoint not found"})
				return
			}

			if webFS != nil {
				if subPath == "" || subPath == "index.html" {
					if content, err := fs.ReadFile(webFS, "index.html"); err == nil {
						c.Data(http.StatusOK, "text/html; charset=utf-8", content)
						return
					}
				}

				// 检查静态资源
				if content, err := fs.ReadFile(webFS, subPath); err == nil {
					mimeType := mime.TypeByExtension(filepath.Ext(subPath))
					if mimeType == "" {
						mimeType = "application/octet-stream"
					}
					c.Data(http.StatusOK, mimeType, content)
					return
				}

				// SPA fallback 到 index.html
				if content, err := fs.ReadFile(webFS, "index.html"); err == nil {
					c.Data(http.StatusOK, "text/html; charset=utf-8", content)
					return
				}
			}
		}

		// 未匹配或未授权访问直接 404 伪装
		c.String(http.StatusNotFound, "404 page not found")
	})

	return r
}
