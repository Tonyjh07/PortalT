package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// MenuHandler 动态菜单接口处理器。
type MenuHandler struct {
	plugins ports.PluginRepository
}

// NewMenuHandler 创建菜单处理器。
func NewMenuHandler(plugins ports.PluginRepository) *MenuHandler {
	return &MenuHandler{plugins: plugins}
}

// Menu GET /api/v1/menu
// 返回当前用户有权限的动态菜单列表（已启用，按 sort_order 升序）。
// 前端据此渲染侧边栏；权限集合优先取角色矩阵（AttachPermissions），
// 未加载时回退 Plugin.CanAccess 的内置表。
func (h *MenuHandler) Menu(c *gin.Context) {
	user := middleware.CurrentUser(c)
	perms := middleware.CurrentPerms(c)
	active, err := h.plugins.FindActive()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询菜单失败")
		return
	}

	menus := make([]*domain.Plugin, 0, len(active))
	for _, p := range active {
		if perms != nil {
			if p.Permission == "" || domain.HasPermissionWith(sliceOfKeys(perms), p.Permission) {
				menus = append(menus, p)
			}
			continue
		}
		if p.CanAccess(user) {
			menus = append(menus, p)
		}
	}
	response.OK(c, menus)
}

// sliceOfKeys 提取 map 键为切片（权限集合 → 权限列表）。
func sliceOfKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
