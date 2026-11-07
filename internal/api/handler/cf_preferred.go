package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/cfpref"
	"vps-auto-deploy/internal/model"
)

// GetCFPreferredStatus 获取优选 IP 状态与配置
func GetCFPreferredStatus(c *gin.Context) {
	status, err := cfpref.GetStatusSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

// SaveCFPreferredSettings 保存优选 IP 配置与调度计划
func SaveCFPreferredSettings(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败: " + err.Error()})
		return
	}

	if err := cfpref.SaveSettings(body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "优选 IP 抓取与调度配置已成功保存！"})
}

// TriggerCFPreferredFetch 手动立即拉取最新优选 IP
func TriggerCFPreferredFetch(c *gin.Context) {
	res, err := cfpref.TriggerManualFetch()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "获取优选 IP 失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "优选 IP 同步完成",
		"data":    res,
	})
}

// ListCFPreferredIPs 获取本地已持久化的优选 IP 列表
func ListCFPreferredIPs(c *gin.Context) {
	keyword := c.Query("keyword")
	list, err := model.ListCFPreferredIPs(keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// ClearCFPreferredIPs 清空本地优选 IP 列表
func ClearCFPreferredIPs(c *gin.Context) {
	if err := model.ClearCFPreferredIPs(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "本地优选 IP 列表已清空"})
}

// DeleteCFPreferredIP 删除单个优选 IP
func DeleteCFPreferredIP(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	if err := model.DeleteCFPreferredIP(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已成功删除"})
}

