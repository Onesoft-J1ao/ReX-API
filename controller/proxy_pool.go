package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/proxy_pool_setting"

	"github.com/gin-gonic/gin"
)

// proxyPoolRequestTimeout bounds one admin-triggered pool call. It is longer
// than the per-request pool timeout because one call may fan out per channel.
const proxyPoolRequestTimeout = 20 * time.Second

// GetProxyPoolStatus /api/proxy_pool/status — report whether the pool is
// reachable and how many proxies it holds, so an operator can tell a config
// mistake from an empty pool.
func GetProxyPoolStatus(c *gin.Context) {
	cfg := proxy_pool_setting.GetSetting()
	data := gin.H{
		"enabled":              cfg.Enabled,
		"auto_assign_channels": cfg.AutoAssignChannels,
		"pool_url":             proxy_pool_setting.GetPoolURL(),
		"proxy_type":           cfg.ProxyType,
		"count":                0,
		"reachable":            false,
		"error":                "",
	}
	if !cfg.Enabled {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), proxyPoolRequestTimeout)
	defer cancel()
	client, err := service.NewProxyPoolClient()
	if err == nil {
		var count int
		count, err = client.Count(ctx)
		if err == nil {
			data["count"] = count
			data["reachable"] = true
		}
	}
	if err != nil {
		data["error"] = err.Error()
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

// ListProxyPoolProxies /api/proxy_pool/all — return the pool contents.
func ListProxyPoolProxies(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), proxyPoolRequestTimeout)
	defer cancel()
	client, err := service.NewProxyPoolClient()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	proxies, err := client.GetAll(ctx)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"count": len(proxies), "proxies": proxies},
	})
}

type proxyPoolAssignRequest struct {
	ChannelIds []int `json:"channel_ids"`
}

// AssignProxyPoolProxies /api/proxy_pool/assign — draw one proxy per channel and
// persist it as that channel's outbound proxy, reusing the existing channel
// setting field so the normal relay path picks it up with no extra wiring.
func AssignProxyPoolProxies(c *gin.Context) {
	req := proxyPoolAssignRequest{}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || len(req.ChannelIds) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(req.ChannelIds) > proxy_pool_setting.GetMaxAssignBatch() {
		common.ApiErrorMsg(c, "too many channels in one request")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), proxyPoolRequestTimeout)
	defer cancel()
	client, err := service.NewProxyPoolClient()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	assigned := make([]gin.H, 0, len(req.ChannelIds))
	failed := make([]gin.H, 0)
	for _, channelId := range req.ChannelIds {
		channel, err := model.GetChannelById(channelId, true)
		if err != nil {
			failed = append(failed, gin.H{"channel_id": channelId, "error": err.Error()})
			continue
		}
		proxy, err := client.GetOne(ctx)
		if err != nil {
			failed = append(failed, gin.H{"channel_id": channelId, "error": err.Error()})
			continue
		}
		channelSetting := channel.GetSetting()
		channelSetting.Proxy = proxy
		channel.SetSetting(channelSetting)
		if err := channel.Update(); err != nil {
			failed = append(failed, gin.H{"channel_id": channelId, "error": err.Error()})
			continue
		}
		assigned = append(assigned, gin.H{
			"channel_id":   channelId,
			"channel_name": channel.Name,
			"proxy":        proxy,
		})
	}
	if len(assigned) > 0 {
		model.InitChannelCache()
	}
	recordManageAudit(c, "proxy_pool.assign", map[string]any{
		"assigned": len(assigned),
		"failed":   len(failed),
		"total":    len(req.ChannelIds),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"assigned": assigned, "failed": failed},
	})
}

type proxyPoolReportRequest struct {
	Proxy string `json:"proxy"`
}

// ReportBadProxyPoolProxy /api/proxy_pool/report_bad — remove a proxy that failed
// in production from the pool and drop it from the auto-assignment cache.
func ReportBadProxyPoolProxy(c *gin.Context) {
	req := proxyPoolReportRequest{}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Proxy == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), proxyPoolRequestTimeout)
	defer cancel()
	client, err := service.NewProxyPoolClient()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = client.Delete(ctx, req.Proxy)
	// The cache is dropped regardless: if the proxy is dead we must stop handing
	// it out even when the pool refused to forget it.
	service.ForgetPoolProxy()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "proxy_pool.report_bad", map[string]any{"proxy": req.Proxy})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
