package domain

import "strings"

// PluginType 插件类型。
type PluginType string

const (
	// PluginTypeAccess 配置型插件：iframe 嵌入外部页面 + API 白名单代理 + Caddy 规则，
	// 三者可任意共存（仅提供一种时退化为旧 iframe / proxy 行为）
	PluginTypeAccess PluginType = "access"
	// PluginTypeNative 原生插件：独立进程 + gRPC 控制面 + HTTP 数据面（进程化改造后落地）
	PluginTypeNative PluginType = "native"
)

// PluginEndpoint 脚本插件的标准 API 端点声明。
// PortalT 只转发白名单内的端点，避免把内部接口暴露给第三方服务。
type PluginEndpoint struct {
	// Method HTTP 方法（GET/POST/PUT/DELETE）
	Method string `json:"method"`
	// Path 端点路径（相对插件 ApiURL，如 "/api/info"）
	Path string `json:"path"`
	// Name 端点名称
	Name string `json:"name"`
	// Description 端点说明
	Description string `json:"description"`
}

// Plugin 插件领域实体，描述门户中的一个可动态扩展的菜单/功能模块。
type Plugin struct {
	// ID 唯一标识
	ID string `json:"id"`
	// Name 显示名称
	Name string `json:"name"`
	// Icon 图标标识（如 "mdi:home"）
	Icon string `json:"icon"`
	// Route 前端路由路径（如 "/ha"）
	Route string `json:"route"`
	// Type 插件类型：access | native（默认 access）
	Type PluginType `json:"type"`
	// IframeURL access 类型插件的嵌入页面地址（如 "https://ha.local" 或门户内 "/esxi/ui/"）
	IframeURL string `json:"iframe_url"`
	// ApiURL access 类型插件的 API 服务地址（如 "http://127.0.0.1:8701"）
	ApiURL string `json:"api_url"`
	// Endpoints access 类型插件声明的标准 API 端点白名单
	Endpoints []PluginEndpoint `json:"endpoints"`
	// CaddyRules access 类型插件的 Caddy 规则片段（原始 Caddyfile handle 块，
	// 由插件宿主落盘到 plugins.d/ 并触发 reload；仅管理员可写）
	CaddyRules string `json:"caddy_rules"`
	// Permission 访问该插件所需的权限，空字符串表示无需额外权限
	Permission string `json:"permission"`
	// SortOrder 排序权重，值越小越靠前
	SortOrder int `json:"sort_order"`
	// IsActive 是否启用
	IsActive bool `json:"is_active"`
	// Status native 插件运行态：running/stopped/error/missing（宿主回写；access 恒为空）
	Status string `json:"status"`
	// ManifestJSON native 插件 manifest 缓存（宿主按 manifest 自动同步）
	ManifestJSON string `json:"manifest_json"`
}

// IsValidPluginType 判断插件类型是否合法（空视为默认 access）。
func IsValidPluginType(t PluginType) bool {
	switch t {
	case "", PluginTypeAccess, PluginTypeNative:
		return true
	default:
		return false
	}
}

// NormalizePluginType 归一化插件类型（空 → access）。
func NormalizePluginType(t PluginType) PluginType {
	if t == "" {
		return PluginTypeAccess
	}
	return t
}

// FindEndpoint 按方法与路径匹配端点白名单（路径两侧忽略前导斜杠）。
func (p *Plugin) FindEndpoint(method, path string) (*PluginEndpoint, bool) {
	path = strings.TrimPrefix(path, "/")
	for i := range p.Endpoints {
		e := &p.Endpoints[i]
		if strings.EqualFold(e.Method, method) && strings.TrimPrefix(e.Path, "/") == path {
			return e, true
		}
	}
	return nil, false
}

// CanAccess 判断指定用户是否可以访问该插件。
// 规则：插件必须已启用；若配置了权限要求，则用户必须具备对应权限。
// 运行时权限集合优先（角色矩阵），未提供时回退内置表。
func (p *Plugin) CanAccess(user *User, perms ...map[string]struct{}) bool {
	if p == nil || user == nil {
		return false
	}
	if !p.IsActive {
		return false
	}
	if p.Permission == "" {
		return true
	}
	if len(perms) > 0 && perms[0] != nil {
		_, ok := perms[0][p.Permission]
		return ok
	}
	return user.HasPermission(p.Permission)
}

// IsEnabled 判断插件是否处于启用状态。
func (p *Plugin) IsEnabled() bool {
	return p != nil && p.IsActive
}
