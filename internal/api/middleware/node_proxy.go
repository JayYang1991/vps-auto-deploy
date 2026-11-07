package middleware

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
)

// NodeProxyMiddleware 智能反向代理中间件：
// 当检测到 X-Target-Node-ID 或 node_id 目标指向远端被纳管节点时，
// 自动重写请求路径与凭据，透明转发 API 与 SSE 日志流
func NodeProxyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		targetIDStr := c.GetHeader("X-Target-Node-ID")
		if targetIDStr == "" {
			targetIDStr = c.Query("node_id")
		}

		// 若未指定节点，或指定为本机主控 (ID <= 1)，继续由当前服务处理
		if targetIDStr == "" || targetIDStr == "1" || targetIDStr == "0" {
			c.Next()
			return
		}

		// 集群管理自身接口必须在主控节点执行，不予代理
		reqPath := c.Request.URL.Path
		secret := c.Param("secret")
		basePrefix := "/" + secret + "/api"
		if strings.HasPrefix(reqPath, basePrefix+"/nodes") {
			c.Next()
			return
		}

		targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
		if err != nil || targetID <= 1 {
			c.Next()
			return
		}

		targetNode, err := model.GetNode(targetID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "指定的集群节点不存在"})
			return
		}

		targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d", targetNode.Host, targetNode.PanelPort))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "解析目标节点地址失败"})
			return
		}

		// 计算目标机器上的真实路径
		// 截取掉主控端的 /:secret 前缀，拼上远端节点的 secret_path
		restOfPath := strings.TrimPrefix(reqPath, "/"+secret)
		remotePath := "/" + strings.Trim(targetNode.SecretPath, "/") + restOfPath

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.FlushInterval = 50 * time.Millisecond // 确保 SSE 实时日志流不被缓冲卡顿

		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = fmt.Sprintf("%s:%d", targetNode.Host, targetNode.PanelPort)
			req.URL.Path = remotePath
			req.URL.RawPath = ""

			// 注入远端节点的 JWT Bearer Token
			if targetNode.APIToken != "" {
				req.Header.Set("Authorization", "Bearer "+targetNode.APIToken)
			}
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			_ = model.UpdateNodeStatus(targetNode.ID, "offline", 0)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"无法连接至远端节点 %s (%s): %v"}`, targetNode.Name, targetNode.Host, err)))
		}

		proxy.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
