package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
)

// GetRelayCandidates 获取可用于中转的目标节点与入站清单（自动剔除当前操作节点）
func GetRelayCandidates(c *gin.Context) {
	callerNodeIDStr := c.GetHeader("X-Target-Node-ID")
	if callerNodeIDStr == "" {
		callerNodeIDStr = c.Query("node_id")
	}

	callerNodeID := int64(1)
	if callerNodeIDStr != "" {
		if id, err := strconv.ParseInt(callerNodeIDStr, 10, 64); err == nil && id > 0 {
			callerNodeID = id
		}
	}

	candidates, err := configgen.GetRelayCandidates(callerNodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取中转候选节点失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, candidates)
}

type BuildRelayRequest struct {
	TargetNodeID int64  `json:"target_node_id" binding:"required"`
	InboundID    int64  `json:"inbound_id" binding:"required"`
	AddrType     string `json:"addr_type"` // "ip" | "tunnel"
}

// BuildRelayOutbound 一键将目标节点的指定 Inbound 解析转换为当前节点的 Outbound 配置
func BuildRelayOutbound(c *gin.Context) {
	var req BuildRelayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	targetNode, err := model.GetNode(req.TargetNodeID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标中转节点不存在"})
		return
	}

	var matchedInbound *model.Inbound
	var tunnelDomain string

	if targetNode.IsMaster {
		// 主控本地入站检索
		var ib model.Inbound
		err := model.DB.QueryRow(
			"SELECT id, tag, protocol, port, spec_json FROM inbounds WHERE id = ?",
			req.InboundID,
		).Scan(&ib.ID, &ib.Tag, &ib.Protocol, &ib.Port, &ib.SpecJSON)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "未在主控节点找到指定的入站协议"})
			return
		}
		matchedInbound = &ib

		// 若为 vless-ws 且关联了 tunnel，查询对应的隧道域名
		if ib.Protocol == "vless-ws" {
			var spec model.VLESSWSSpec
			_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
			if spec.TunnelID > 0 {
				_ = model.DB.QueryRow("SELECT domain FROM tunnels WHERE id = ?", spec.TunnelID).Scan(&tunnelDomain)
			}
			if tunnelDomain == "" && spec.GroupDomain != "" {
				tunnelDomain = spec.GroupDomain
			}
		}
	} else {
		// 远程节点通过安全信道抓取 inventory
		remoteInbounds, remoteTunnels, fetchErr := node.FetchRemoteNodeInventory(targetNode)
		if fetchErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("无法连接目标节点 %s: %v", targetNode.Name, fetchErr)})
			return
		}

		for _, ib := range remoteInbounds {
			if ib.ID == req.InboundID {
				matchedInbound = &ib
				if ib.Protocol == "vless-ws" {
					var spec model.VLESSWSSpec
					_ = json.Unmarshal([]byte(ib.SpecJSON), &spec)
					if spec.TunnelID > 0 && remoteTunnels != nil {
						tunnelDomain = remoteTunnels[spec.TunnelID]
					}
					if tunnelDomain == "" && spec.GroupDomain != "" {
						tunnelDomain = spec.GroupDomain
					}
				}
				break
			}
		}
	}

	if matchedInbound == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标节点上未找到指定的入站协议配置"})
		return
	}

	addrType := req.AddrType
	if addrType == "" {
		addrType = "ip"
	}

	outbound, err := configgen.BuildRelayOutbound(targetNode, matchedInbound, tunnelDomain, addrType, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成中转配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, outbound)
}
