// Package plugins 提供 Go 原生插件机制。
//
// 原生插件编译进 PortalT 二进制（Windows 下 .so 动态插件不可行），
// 启动时注册到 Registry，可提供：
//   - 自定义 API 路由（Mount，挂载在鉴权后的 /api/v1/plugins/native/<id>/...）
//   - 内嵌静态前端（StaticFS，托管在 /native/<id>/，供 iframe 嵌入）
//
// 插件的菜单记录（plugins 表）由 services.SyncNativePlugins 在启动时
// 按 Info() 自动 upsert，权限/启用状态仍由管理员在界面控制。
package plugins

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"sync"

	"github.com/gin-gonic/gin"

	"portalt/internal/domain"
)

// Deps 原生插件可用的门户依赖（按需取用，勿持有跨请求状态）。
type Deps struct {
	// VMs 虚拟机服务（列表/电源操作/宿主机信息），esxi-admin 等插件使用
	VMs VMServiceFacade
	// Provider 当前虚拟化平台类型（esxi / workstation / mock），
	// 供插件展示平台状态与配置指引
	Provider string
	// WebURL 目标平台 Web 管理界面地址（如 ESXi 的 https://host/ui/），
	// 供 iframe 类插件嵌入；为空表示未配置
	WebURL string
}

// VMServiceFacade 暴露给原生插件的虚拟机能力子集。
type VMServiceFacade interface {
	GetVMStatus(ctx context.Context, id string) (*domain.VM, error)
	ListVMs(ctx context.Context) ([]*domain.VM, error)
	StartVM(ctx context.Context, id string) (*domain.VM, error)
	StopVM(ctx context.Context, id string) (*domain.VM, error)
	RestartVM(ctx context.Context, id string) (*domain.VM, error)
	GetHostInfo(ctx context.Context) (*domain.HostInfo, error)
}

// Plugin 原生插件接口。
type Plugin interface {
	// Info 插件元信息（ID/名称/图标/路由等；route 约定为 "/"+ID）
	Info() domain.Plugin
	// Mount 注册插件 API 路由（rt 已挂载在 protected 分组下，调用方负责鉴权）
	Mount(rt *gin.RouterGroup, deps Deps)
	// StaticFS 返回插件内嵌静态前端（fs.Sub 风格），无前端时返回 nil
	StaticFS() fs.FS
}

// Registry 原生插件注册表（进程内单例，启动时注册完毕）。
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

// NewRegistry 创建注册表。
func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]Plugin)}
}

// Register 注册插件；ID 冲突时返回错误。
func (r *Registry) Register(p Plugin) error {
	if p == nil {
		return fmt.Errorf("plugins: 不能注册 nil 插件")
	}
	info := p.Info()
	if info.ID == "" {
		return fmt.Errorf("plugins: 插件 ID 不能为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[info.ID]; exists {
		return fmt.Errorf("plugins: 插件 %q 已注册", info.ID)
	}
	r.plugins[info.ID] = p
	return nil
}

// Get 按 ID 获取插件。
func (r *Registry) Get(id string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// All 返回全部插件，按 ID 排序（确定性输出）。
func (r *Registry) All() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Info().ID < out[j].Info().ID
	})
	return out
}

// MountAPI 把每个原生插件的 API 路由挂载到 rt 下（rt 为受保护分组，
// 路径前缀 /plugins/native/<id>/）。每个插件挂载前先过启用闸门：
// 插件在 plugins 表中不存在或已停用 → 404/403。
func (r *Registry) MountAPI(rt *gin.RouterGroup, deps Deps, repo PluginRepo) {
	if rt == nil {
		return
	}
	for _, p := range r.All() {
		id := p.Info().ID
		g := rt.Group("/" + id, nativeGate(repo, id))
		p.Mount(g, deps)
	}
}

// MountStatic 把内嵌静态前端托管到 /native/<id>/（公开：静态资源
// 不含敏感数据，数据一律走鉴权 API）。
func (r *Registry) MountStatic(router *gin.Engine) {
	for _, p := range r.All() {
		info := p.Info()
		if info.ID == "" {
			continue
		}
		if fsys := p.StaticFS(); fsys != nil {
			router.StaticFS("/native/"+info.ID+"/", http.FS(fsys))
		}
	}
}

// PluginRepo 插件仓储子集（启用状态检查用）。
type PluginRepo interface {
	FindByID(id string) (*domain.Plugin, error)
}

// nativeGate 原生插件 API 的启用闸门中间件。
func nativeGate(repo PluginRepo, id string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := repo.FindByID(id)
		if err != nil || !p.IsEnabled() {
			c.JSON(http.StatusNotFound, gin.H{"code": 4004, "message": "插件不存在或未启用"})
			c.Abort()
			return
		}
		c.Next()
	}
}
