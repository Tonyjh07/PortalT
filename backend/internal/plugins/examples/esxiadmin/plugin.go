// Package esxiadmin 是 PortalT 原生插件机制的示例插件。
//
// 演示能力：自定义 API（宿主信息 + VM 电源快捷操作）+ 内嵌静态前端
// （/native/esxi-admin/ 下自包含单页，iframe 嵌入门户）。
//
// 约定：插件 ID 与 route 一致（"/" + ID）；菜单/权限记录在启动时
// 由 services.SyncNativePlugins 自动 upsert 到 plugins 表。
package esxiadmin

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
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
		ID:        "esxi-admin",
		Name:      "ESXi 管理",
		Icon:      "mdi:server-network",
		Route:     "/esxi-admin",
		SortOrder: 90,
		IsActive:  true,
	}
}

// Mount 挂载插件 API 路由（已位于 /api/v1/plugins/native/esxi-admin/ 下）。
func (p *Plugin) Mount(rt *gin.RouterGroup, deps plugins.Deps) {
	if deps.VMs == nil {
		return
	}
	h := &handler{vm: deps.VMs}
	rt.GET("/host", h.host)
	rt.GET("/vms", h.listVMs)
	rt.POST("/vms/:id/start", middleware.RequirePermission(domain.PERM_VM_START), h.start)
	rt.POST("/vms/:id/stop", middleware.RequirePermission(domain.PERM_VM_STOP), h.stop)
	rt.POST("/vms/:id/restart", middleware.RequirePermission(domain.PERM_VM_RESTART), h.restart)
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
	vm plugins.VMServiceFacade
}

// host 返回宿主机信息。
func (h *handler) host(c *gin.Context) {
	info, err := h.vm.GetHostInfo(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusBadGateway, response.CodeServerError, "获取宿主信息失败: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": info})
}

// listVMs 返回精简 VM 列表（不暴露 metadata）。
func (h *handler) listVMs(c *gin.Context) {
	vms, err := h.vm.ListVMs(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusBadGateway, response.CodeServerError, "获取 VM 列表失败: "+err.Error())
		return
	}
	out := make([]gin.H, 0, len(vms))
	for _, vm := range vms {
		out = append(out, gin.H{
			"id":      vm.ID,
			"name":    vm.Name,
			"status":  vm.Status,
			"ip":      vm.IPAddress,
			"cpu":     vm.CPU,
			"memory":  vm.MemoryMB,
			"host":    vm.Host,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *handler) start(c *gin.Context)   { h.power(c, h.vm.StartVM) }
func (h *handler) stop(c *gin.Context)    { h.power(c, h.vm.StopVM) }
func (h *handler) restart(c *gin.Context) { h.power(c, h.vm.RestartVM) }

type powerOp func(ctx context.Context, id string) (*domain.VM, error)

func (h *handler) power(c *gin.Context, op powerOp) {
	id := strings.TrimSpace(c.Param("id"))
	vm, err := op(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidOperation, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": vm.ID, "name": vm.Name, "status": vm.Status}})
}
