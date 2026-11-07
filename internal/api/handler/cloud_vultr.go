package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/cloud/vultr"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/node"
)

// GetVultrConfig 获取 Vultr 配置状态与账户概览
func GetVultrConfig(c *gin.Context) {
	apiKey, err := model.GetVultrAPIKey()
	if err != nil || strings.TrimSpace(apiKey) == "" {
		c.JSON(http.StatusOK, gin.H{
			"configured": false,
			"masked_key": "",
			"account":    nil,
		})
		return
	}

	// 掩码脱敏
	masked := maskAPIKey(apiKey)

	client := vultr.NewClient(apiKey)
	acc, err := client.GetAccount()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"configured": true,
			"masked_key": masked,
			"account":    nil,
			"error":      "Token 校验失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"configured": true,
		"masked_key": masked,
		"account":    acc,
	})
}

// SaveVultrConfig 保存或更新 Vultr API Token（保存前强制校验合法性）
func SaveVultrConfig(c *gin.Context) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数无效: " + err.Error()})
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Vultr API Token 不能为空"})
		return
	}

	// 远程验证合法性
	client := vultr.NewClient(apiKey)
	acc, err := client.GetAccount()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Vultr API Token 校验失败，请检查 Token 是否正确或权限是否完整: " + err.Error()})
		return
	}

	if err := model.SetVultrAPIKey(apiKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "加密保存 Token 失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Vultr API Token 验证通过并已加密保存",
		"masked_key": maskAPIKey(apiKey),
		"account":    acc,
	})
}

// GetVultrCatalog 获取格式化后的区域、机型套餐与兼容操作系统镜像目录树
func GetVultrCatalog(c *gin.Context) {
	apiKey, err := model.GetVultrAPIKey()
	if err != nil || strings.TrimSpace(apiKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "尚未配置 Vultr API Token，请先在面板中绑定 Token"})
		return
	}

	forceRefresh := c.Query("refresh") == "true" || c.Query("refresh") == "1"
	client := vultr.NewClient(apiKey)

	regions, err := client.ListRegions(forceRefresh)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Vultr 区域列表失败: " + err.Error()})
		return
	}

	plans, err := client.ListPlans(forceRefresh)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Vultr 规格套餐失败: " + err.Error()})
		return
	}

	osList, err := client.ListOSList(forceRefresh)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Vultr 操作系统镜像失败: " + err.Error()})
		return
	}

	// 构造前端友好目录结构
	type EnrichedRegion struct {
		vultr.Region
		FlagEmoji string `json:"flag_emoji"`
		CityZh    string `json:"city_zh"`
	}

	var enrichedRegions []EnrichedRegion
	for _, r := range regions {
		flag, cityZh := getRegionMeta(r.ID, r.Country, r.City)
		enrichedRegions = append(enrichedRegions, EnrichedRegion{
			Region:    r,
			FlagEmoji: flag,
			CityZh:    cityZh,
		})
	}

	type EnrichedOS struct {
		vultr.OS
		Recommended bool   `json:"recommended"`
		TagZh       string `json:"tag_zh"`
	}
	var enrichedOS []EnrichedOS
	for _, o := range osList {
		rec := false
		tag := ""
		lowerName := strings.ToLower(o.Name)
		if strings.Contains(lowerName, "debian 12") {
			rec = true
			tag = "⭐ 首选推荐 (自带 BBR / 低内存占用)"
		} else if strings.Contains(lowerName, "ubuntu 24.04") {
			tag = "🔥 热门稳定 LTS"
		} else if strings.Contains(lowerName, "rocky linux 9") || strings.Contains(lowerName, "almalinux 9") {
			tag = "🛡️ 企业级兼容"
		}
		enrichedOS = append(enrichedOS, EnrichedOS{
			OS:          o,
			Recommended: rec,
			TagZh:       tag,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"regions": enrichedRegions,
		"plans":   plans,
		"os_list": enrichedOS,
	})
}

// ProvisionVultrNode 启动 Vultr 弹性创建与自动化纳管异步任务
func ProvisionVultrNode(c *gin.Context) {
	var req node.ProvisionVultrRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	if strings.TrimSpace(req.Region) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择数据中心区域 (Region)"})
		return
	}
	if strings.TrimSpace(req.Plan) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择实例规格套餐 (Plan)"})
		return
	}
	if req.OSID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择合法的操作系统镜像 (OS)"})
		return
	}

	taskName := req.Name
	if strings.TrimSpace(taskName) == "" {
		taskName = fmt.Sprintf("Vultr-%s-%s", strings.ToUpper(req.Region), req.Plan)
	}

	task := node.CreateDeployTask(taskName, "vultr-"+req.Region)
	go node.RunProvisionVultr(task, req)

	c.JSON(http.StatusOK, gin.H{
		"task_id": task.ID,
		"message": "Vultr 弹性创建与纳管任务已启动",
	})
}

// 辅助工具函数：生成 API 掩码
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "••••••••"
	}
	return "VULTR••••••••" + key[len(key)-4:]
}

// 辅助工具函数：国旗与地名中文化
func getRegionMeta(id, country, city string) (string, string) {
	switch strings.ToLower(id) {
	case "nrt":
		return "🇯🇵", "日本东京 (Tokyo)"
	case "kix":
		return "🇯🇵", "日本大阪 (Osaka)"
	case "sgp":
		return "🇸🇬", "新加坡 (Singapore)"
	case "icn":
		return "🇰🇷", "韩国首尔 (Seoul)"
	case "lax":
		return "🇺🇸", "美国洛杉矶 (Los Angeles)"
	case "sjc":
		return "🇺🇸", "美国硅谷 (Silicon Valley)"
	case "sea":
		return "🇺🇸", "美国西雅图 (Seattle)"
	case "ewr":
		return "🇺🇸", "美国新泽西 (New Jersey)"
	case "ord":
		return "🇺🇸", "美国芝加哥 (Chicago)"
	case "dfw":
		return "🇺🇸", "美国达拉斯 (Dallas)"
	case "atl":
		return "🇺🇸", "美国亚特兰大 (Atlanta)"
	case "mia":
		return "🇺🇸", "美国迈阿密 (Miami)"
	case "yto":
		return "🇨🇦", "加拿大多伦多 (Toronto)"
	case "fra":
		return "🇩🇪", "德国法兰克福 (Frankfurt)"
	case "lhr":
		return "🇬🇧", "英国伦敦 (London)"
	case "ams":
		return "🇳🇱", "荷兰阿姆斯特丹 (Amsterdam)"
	case "par":
		return "🇫🇷", "法国巴黎 (Paris)"
	case "mad":
		return "🇪🇸", "西班牙马德里 (Madrid)"
	case "syd":
		return "🇦🇺", "澳大利亚悉尼 (Sydney)"
	case "mel":
		return "🇦🇺", "澳大利亚墨尔本 (Melbourne)"
	default:
		flag := "🌐"
		switch strings.ToUpper(country) {
		case "US":
			flag = "🇺🇸"
		case "JP":
			flag = "🇯🇵"
		case "SG":
			flag = "🇸🇬"
		case "KR":
			flag = "🇰🇷"
		case "DE":
			flag = "🇩🇪"
		case "GB", "UK":
			flag = "🇬🇧"
		case "CA":
			flag = "🇨🇦"
		case "AU":
			flag = "🇦🇺"
		}
		return flag, fmt.Sprintf("%s (%s)", city, country)
	}
}
