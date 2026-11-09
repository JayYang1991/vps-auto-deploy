package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/configgen"
	"vps-auto-deploy/internal/model"
)

type RouteRequest struct {
	Name          string `json:"name" binding:"required"`
	Action        string `json:"action"`        // route | reject
	OutboundTag   string `json:"outbound_tag"`  // direct | block | <outbound_tag>
	InboundTags   string `json:"inbound_tags"`
	Domains       string `json:"domains"`
	DomainSuffix  string `json:"domain_suffix"`
	DomainKeyword string `json:"domain_keyword"`
	IPCIDR        string `json:"ip_cidr"`
	IPIsPrivate   bool   `json:"ip_is_private"`
	Port          string `json:"port"`
	PortRange     string `json:"port_range"`
	Protocol      string `json:"protocol"`
	RuleSet       string `json:"rule_set"`
	RuleSetURLs   string `json:"rule_set_urls"`
	Invert        bool   `json:"invert"`
	Priority      int    `json:"priority"`
	Enabled       bool   `json:"enabled"`
}

type ReorderRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

// ListRoutes 查询所有路由规则
func ListRoutes(c *gin.Context) {
	rules, err := model.ListRouteRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询路由规则失败: " + err.Error()})
		return
	}
	if rules == nil {
		rules = []model.RouteRule{}
	}
	c.JSON(http.StatusOK, rules)
}

// GetRoute 获取指定路由规则详情
func GetRoute(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的路由规则 ID"})
		return
	}

	r, err := model.GetRouteRule(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "路由规则不存在"})
		return
	}
	c.JSON(http.StatusOK, r)
}

// CreateRoute 新建路由规则
func CreateRoute(c *gin.Context) {
	var req RouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数校验失败: " + err.Error()})
		return
	}

	outboundTag := strings.TrimSpace(req.OutboundTag)
	if outboundTag == "" {
		outboundTag = "direct"
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		if outboundTag == "block" {
			action = "reject"
		} else {
			action = "route"
		}
	}

	priority := req.Priority
	if priority <= 0 {
		priority = 10
	}

	rule := &model.RouteRule{
		Name:          strings.TrimSpace(req.Name),
		Action:        action,
		OutboundTag:   outboundTag,
		InboundTags:   strings.TrimSpace(req.InboundTags),
		Domains:       strings.TrimSpace(req.Domains),
		DomainSuffix:  strings.TrimSpace(req.DomainSuffix),
		DomainKeyword: strings.TrimSpace(req.DomainKeyword),
		IPCIDR:        strings.TrimSpace(req.IPCIDR),
		IPIsPrivate:   req.IPIsPrivate,
		Port:          strings.TrimSpace(req.Port),
		PortRange:     strings.TrimSpace(req.PortRange),
		Protocol:      strings.TrimSpace(req.Protocol),
		RuleSet:       strings.TrimSpace(req.RuleSet),
		RuleSetURLs:   strings.TrimSpace(req.RuleSetURLs),
		Invert:        req.Invert,
		Priority:      priority,
		Enabled:       req.Enabled,
	}

	id, err := model.CreateRouteRule(rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "创建路由规则失败: " + err.Error()})
		return
	}
	rule.ID = id

	// 触发重新生成并重载内核
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		_ = model.DeleteRouteRule(id)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("路由规则语法校验或应用失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// UpdateRoute 更新路由规则
func UpdateRoute(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的路由规则 ID"})
		return
	}

	oldRule, err := model.GetRouteRule(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "路由规则不存在"})
		return
	}

	var req RouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数校验失败: " + err.Error()})
		return
	}

	outboundTag := strings.TrimSpace(req.OutboundTag)
	if outboundTag == "" {
		outboundTag = "direct"
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		if outboundTag == "block" {
			action = "reject"
		} else {
			action = "route"
		}
	}

	priority := req.Priority
	if priority <= 0 {
		priority = oldRule.Priority
	}

	newRule := &model.RouteRule{
		ID:            id,
		Name:          strings.TrimSpace(req.Name),
		Action:        action,
		OutboundTag:   outboundTag,
		InboundTags:   strings.TrimSpace(req.InboundTags),
		Domains:       strings.TrimSpace(req.Domains),
		DomainSuffix:  strings.TrimSpace(req.DomainSuffix),
		DomainKeyword: strings.TrimSpace(req.DomainKeyword),
		IPCIDR:        strings.TrimSpace(req.IPCIDR),
		IPIsPrivate:   req.IPIsPrivate,
		Port:          strings.TrimSpace(req.Port),
		PortRange:     strings.TrimSpace(req.PortRange),
		Protocol:      strings.TrimSpace(req.Protocol),
		RuleSet:       strings.TrimSpace(req.RuleSet),
		RuleSetURLs:   strings.TrimSpace(req.RuleSetURLs),
		Invert:        req.Invert,
		Priority:      priority,
		Enabled:       req.Enabled,
	}

	if err := model.UpdateRouteRule(newRule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "更新路由规则失败: " + err.Error()})
		return
	}

	// 重新生成配置并重载
	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		_ = model.UpdateRouteRule(oldRule)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("修改后的路由规则语法校验失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, newRule)
}

// DeleteRoute 删除路由规则
func DeleteRoute(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的路由规则 ID"})
		return
	}

	oldRule, err := model.GetRouteRule(id)
	if err != nil {
		// 幂等性保障：若该路由规则已被删除或已不存在，确保触发一次配置对齐并直接返回成功
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusOK, gin.H{"message": "路由规则已被删除"})
		return
	}

	if err := model.DeleteRouteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除路由规则失败: " + err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		// 恢复原规则
		_, _ = model.CreateRouteRule(oldRule)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("删除后配置重载失败，已自动回滚: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// ToggleRoute 切换路由规则启用/停用状态
func ToggleRoute(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的路由规则 ID"})
		return
	}

	active, err := model.ToggleRouteRule(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "切换状态失败: " + err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		_, _ = model.ToggleRouteRule(id)
		_ = configgen.ApplyAndReload(singbox.GetConfigPath())
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("规则状态应用失败，已回滚: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "enabled": active})
}

// ReorderRoutes 批量更新规则执行优先级顺序
func ReorderRoutes(c *gin.Context) {
	var req ReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数校验失败: " + err.Error()})
		return
	}

	if err := model.ReorderRouteRules(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "排序更新失败: " + err.Error()})
		return
	}

	if err := configgen.ApplyAndReload(singbox.GetConfigPath()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("重新排序后配置重载失败: %v", err)})
		return
	}

	rules, _ := model.ListRouteRules()
	c.JSON(http.StatusOK, rules)
}

