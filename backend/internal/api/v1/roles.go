package v1

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// RoleHandler 角色（权限矩阵）管理接口处理器（user:manage，管理员）。
type RoleHandler struct {
	roles ports.RoleRepository
	perms ports.PermissionRepository
	loader *middleware.RoleLoader
}

// NewRoleHandler 创建角色管理处理器。
func NewRoleHandler(roles ports.RoleRepository, perms ports.PermissionRepository, loader *middleware.RoleLoader) *RoleHandler {
	return &RoleHandler{roles: roles, perms: perms, loader: loader}
}

// Loader 返回权限加载器（供路由装配 AttachPermissions 中间件）。
func (h *RoleHandler) Loader() *middleware.RoleLoader { return h.loader }

// roleUpdateRequest 更新角色请求体。
type roleUpdateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// roleCreateRequest 创建角色请求体（id 作为角色的唯一标识与用户 Role 值）。
type roleCreateRequest struct {
	ID          string   `json:"id" binding:"required"`
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// roleIDPattern 角色 ID 允许的字符集（小写字母/数字/下划线/连字符）。
var roleIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// List GET /api/v1/roles
// 返回全部角色（含权限矩阵）。
func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.roles.FindAll()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询角色失败")
		return
	}
	response.OK(c, roles)
}

// Permissions GET /api/v1/roles/permissions
// 返回系统全部可用权限字典（供权限矩阵界面渲染选项）。
func (h *RoleHandler) Permissions(c *gin.Context) {
	response.OK(c, domain.AllPermissions())
}

// Create POST /api/v1/roles
// 创建自定义角色；权限必须全部来自权限字典，ID 不可与内置角色/已有角色冲突。
func (h *RoleHandler) Create(c *gin.Context) {
	var req roleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	if !roleIDPattern.MatchString(req.ID) {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "角色 ID 仅允许小写字母/数字/下划线/连字符（1-32 位）")
		return
	}
	if domain.IsBuiltinRole(req.ID) {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "内置角色不允许重复创建")
		return
	}
	if _, err := h.roles.FindByID(req.ID); err == nil {
		response.Error(c, http.StatusConflict, response.CodeConflict, "角色 ID 已存在")
		return
	} else if !errors.Is(err, ports.ErrNotFound) {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询角色失败")
		return
	}
	perms := dedupe(req.Permissions)
	if err := h.validatePerms(perms); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	role := &domain.RoleDefinition{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Permissions: perms,
	}
	if err := h.roles.Save(role); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "创建角色失败")
		return
	}
	response.OK(c, role)
}

// Update PUT /api/v1/roles/:id
// 更新角色名称/描述/权限矩阵；内置角色同样允许调整权限（权限矩阵入库是运行时单一事实来源）。
func (h *RoleHandler) Update(c *gin.Context) {
	id := c.Param("id")
	role, err := h.roles.FindByID(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "角色不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询角色失败")
		return
	}

	var req roleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	perms := dedupe(req.Permissions)
	if err := h.validatePerms(perms); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	role.Name = req.Name
	role.Description = req.Description
	role.Permissions = perms
	if err := h.roles.Save(role); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "更新角色失败")
		return
	}
	h.loader.Invalidate(domain.Role(role.ID))
	response.OK(c, role)
}

// Delete DELETE /api/v1/roles/:id
// 删除自定义角色；内置角色（admin/user/viewer）不允许删除。
func (h *RoleHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if domain.IsBuiltinRole(id) {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "内置角色不允许删除")
		return
	}
	if err := h.roles.Delete(id); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "角色不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "删除角色失败")
		return
	}
	h.loader.Invalidate(domain.Role(id))
	response.OK(c, nil)
}

// dedupe 权限列表去重并保持顺序。
func dedupe(perms []string) []string {
	seen := make(map[string]struct{}, len(perms))
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// validatePerms 校验权限列表全部来自权限字典；perms 为 nil 时跳过（权限字典未启用）。
func (h *RoleHandler) validatePerms(perms []string) error {
	if h.perms == nil {
		return nil
	}
	for _, p := range perms {
		ok, err := h.perms.Exists(p)
		if err != nil {
			return errors.New("查询权限字典失败")
		}
		if !ok {
			return errors.New("未知权限: " + p + "（仅允许权限字典中的权限，见 GET /api/v1/roles/permissions）")
		}
	}
	return nil
}
