package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
)

var (
	proxyTransportMutex sync.RWMutex
	proxyTransports     = make(map[string]*http.Transport)

	// NodeTokenRefresher 供外部注册在远端节点 Token 过期返回 401 时自动重登刷新 Token
	NodeTokenRefresher func(targetNode *model.Node, rawPass string) (string, error)
)

func getPinnedTransport(nodeID int64, fingerprint string) *http.Transport {
	key := fmt.Sprintf("%d:%s", nodeID, fingerprint)
	proxyTransportMutex.RLock()
	t, ok := proxyTransports[key]
	proxyTransportMutex.RUnlock()
	if ok {
		return t
	}

	proxyTransportMutex.Lock()
	defer proxyTransportMutex.Unlock()
	if t, ok := proxyTransports[key]; ok {
		return t
	}

	t = crypto.CreatePinnedTransport(fingerprint)
	proxyTransports[key] = t
	return t
}

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

		// 集群管理自身接口、主控系统设置与统一凭据管理必须在主控节点执行，不予代理
		reqPath := c.Request.URL.Path
		secret := c.Param("secret")
		basePrefix := "/" + secret + "/api"
		if strings.HasPrefix(reqPath, basePrefix+"/nodes") ||
			strings.HasPrefix(reqPath, basePrefix+"/settings") ||
			strings.HasPrefix(reqPath, basePrefix+"/auth") ||
			strings.HasPrefix(reqPath, basePrefix+"/members") ||
			strings.HasPrefix(reqPath, basePrefix+"/cluster") ||
			strings.HasPrefix(reqPath, basePrefix+"/system/upgrade") {
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

		scheme := "http"
		if targetNode.UseTLS {
			scheme = "https"
		}
		targetURL, err := url.Parse(fmt.Sprintf("%s://%s:%d", scheme, targetNode.Host, targetNode.PanelPort))
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

		// 防中间人攻击 (Phase 1)：若启用 TLS，注入带有硬件证书指纹锁定的长连接 Transport
		if targetNode.UseTLS {
			proxy.Transport = getPinnedTransport(targetNode.ID, targetNode.CertFingerprint)
		}

		// 代理响应后置处理：节点在线自愈与系统概览 IP 兜底补齐
		proxy.ModifyResponse = func(resp *http.Response) error {
			// 若远端正常响应 (非 5xx 宕机)，自动自愈纠正节点在线状态
			if resp.StatusCode < 500 && targetNode.Status != "online" {
				_ = model.UpdateNodeStatus(targetNode.ID, "online", 50)
			}

			// 若远端返回 401 Token 失效且保存有凭据，异步触发平滑自愈刷新
			if resp.StatusCode == http.StatusUnauthorized && targetNode.PasswordEncrypted != "" && NodeTokenRefresher != nil {
				go func() {
					rawPass, decErr := model.DecryptSensitiveData(targetNode.PasswordEncrypted)
					if decErr == nil && rawPass != "" {
						_, _ = NodeTokenRefresher(targetNode, rawPass)
					}
				}()
			}

			// 当请求系统概览时，若远端检测到的公网 IP 为空或 127.0.0.1，自动补齐为已知节点的直连 Host
			if strings.HasSuffix(reqPath, "/system/overview") {
				if resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
					bodyBytes, err := io.ReadAll(resp.Body)
					if err != nil {
						return err
					}
					_ = resp.Body.Close()

					var data map[string]interface{}
					if err := json.Unmarshal(bodyBytes, &data); err == nil {
						pubIP, _ := data["public_ip"].(string)
						if (pubIP == "" || pubIP == "127.0.0.1") && targetNode.Host != "" && targetNode.Host != "127.0.0.1" && targetNode.Host != "localhost" {
							data["public_ip"] = targetNode.Host
							newBody, _ := json.Marshal(data)
							resp.Body = io.NopCloser(bytes.NewReader(newBody))
							resp.ContentLength = int64(len(newBody))
							resp.Header.Set("Content-Length", strconv.Itoa(len(newBody)))
							return nil
						}
					}
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}
			return nil
		}

		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = fmt.Sprintf("%s:%d", targetNode.Host, targetNode.PanelPort)
			req.URL.Path = remotePath
			req.URL.RawPath = ""

			// 关键：删除跨节点目标 ID，防止远端节点因本地不存在该节点 ID 而发生二次代理递归报错
			req.Header.Del("X-Target-Node-ID")
			q := req.URL.Query()
			q.Del("node_id")
			req.URL.RawQuery = q.Encode()

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

