// Package esxiadmin 是 PortalT 原生插件机制的示例插件。
//
// 功能：在门户内 iframe 嵌入 ESXi 内置 Web 管理界面
// （https://<esxi-host>/ui/），未连接/未配置时展示现代化占位页与配置指引。
//
// 约定：插件 ID 与 route 一致（"/" + ID）；菜单/权限记录在启动时
// 由 services.SyncNativePlugins 自动 upsert 到 plugins 表。
package esxiadmin

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/domain"
	"portalt/internal/plugins"
)

//go:embed static/*
var staticFiles embed.FS

// Plugin esxi-admin 原生插件。
type Plugin struct{}

// New 创建插件实例。
func New() *Plugin { return &Plugin{} }

// Info 插件元信息（菜单/权限同步用）。
func (p *Plugin) Info() domain.Plugin {
	return domain.Plugin{
		ID:         "esxi-admin",
		Name:       "ESXi 管理",
		Icon:       "mdi:server-network",
		Route:      "/esxi-admin",
		SortOrder:  90,
		Permission: domain.PERM_PLUGIN_VIEW,
		IsActive:   true,
	}
}

// Mount 挂载插件 API 路由（已位于 /api/v1/plugins/native/esxi-admin/ 下）。
func (p *Plugin) Mount(rt *gin.RouterGroup, deps plugins.Deps) {
	h := &handler{provider: deps.Provider, webURL: deps.WebURL}
	rt.GET("/config", h.config)
}

// StaticFS 返回内嵌静态前端（托管于 /native/esxi-admin/）。
func (p *Plugin) StaticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil
	}
	return sub
}

type handler struct {
	provider string
	webURL   string
}

// config 返回平台连接状态与 Web 管理界面地址：
// connected=true 表示当前为 ESXi 平台；web_url 为空表示未配置可嵌入的管理界面。
func (h *handler) config(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"provider":  h.provider,
			"connected": h.provider == "esxi",
			"web_url":   h.webURL,
		},
	})
}
