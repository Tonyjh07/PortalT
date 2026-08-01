package domain

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
	// IframeURL 插件嵌入的页面地址（如 "https://ha.local"）
	IframeURL string `json:"iframe_url"`
	// Permission 访问该插件所需的权限，空字符串表示无需额外权限
	Permission string `json:"permission"`
	// SortOrder 排序权重，值越小越靠前
	SortOrder int `json:"sort_order"`
	// IsActive 是否启用
	IsActive bool `json:"is_active"`
}

// CanAccess 判断指定用户是否可以访问该插件。
// 规则：插件必须已启用；若配置了权限要求，则用户必须具备对应权限。
func (p *Plugin) CanAccess(user *User) bool {
	if p == nil || user == nil {
		return false
	}
	if !p.IsActive {
		return false
	}
	if p.Permission == "" {
		return true
	}
	return user.HasPermission(p.Permission)
}

// IsEnabled 判断插件是否处于启用状态。
func (p *Plugin) IsEnabled() bool {
	return p != nil && p.IsActive
}
