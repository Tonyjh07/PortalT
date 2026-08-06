package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/adapters/auth"
	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// UserHandler 用户管理接口处理器（user:manage，管理员）。
type UserHandler struct {
	users ports.UserRepository
	roles ports.RoleRepository
	vms   ports.VMAccessRepository
}

// NewUserHandler 创建用户管理处理器。
// vms 可为 nil（未装配资源授权时跳过清理）。
func NewUserHandler(users ports.UserRepository, roles ports.RoleRepository, vms ports.VMAccessRepository) *UserHandler {
	return &UserHandler{users: users, roles: roles, vms: vms}
}

// userCreateRequest 创建用户请求体。
type userCreateRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// userUpdateRequest 更新用户请求体（password 非空才重置）。
type userUpdateRequest struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

// List GET /api/v1/users
// 返回全部用户（密码哈希不参与 JSON 序列化）。
func (h *UserHandler) List(c *gin.Context) {
	users, err := h.users.FindAll()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询用户失败")
		return
	}
	response.OK(c, users)
}

// Create POST /api/v1/users
// 创建用户（用户名唯一，密码 bcrypt 存储）。
func (h *UserHandler) Create(c *gin.Context) {
	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	role, err := h.normalizeRole(req.Role)
	if err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	if _, err := h.users.FindByUsername(req.Username); err == nil {
		response.Error(c, http.StatusConflict, response.CodeConflict, "用户名已存在")
		return
	} else if !errors.Is(err, ports.ErrNotFound) {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询用户失败")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "密码处理失败")
		return
	}
	user := &domain.User{
		ID:       auth.NewID(),
		Username: req.Username,
		Password: hash,
		Email:    req.Email,
		Role:     role,
	}
	if err := h.users.Save(user); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "创建用户失败")
		return
	}
	response.OK(c, user)
}

// Update PUT /api/v1/users/:id
// 更新用户邮箱/角色，password 非空时重置密码。
func (h *UserHandler) Update(c *gin.Context) {
	id := c.Param("id")
	user, err := h.users.FindByID(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "用户不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询用户失败")
		return
	}

	var req userUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	if req.Role != "" {
		role, err := h.normalizeRole(req.Role)
		if err != nil {
			response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
			return
		}
		user.Role = role
	}
	if req.Email != "" || req.Email == user.Email {
		user.Email = req.Email
	}
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, response.CodeServerError, "密码处理失败")
			return
		}
		user.Password = hash
	}
	if err := h.users.Save(user); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "更新用户失败")
		return
	}
	response.OK(c, user)
}

// Delete DELETE /api/v1/users/:id
// 删除用户；不允许删除当前登录账号自身（防止误删导致失去管理入口）。
func (h *UserHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	current := middleware.CurrentUser(c)
	if current != nil && current.ID == id {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "不能删除当前登录账号")
		return
	}
	if err := h.users.Delete(id); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "用户不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "删除用户失败")
		return
	}
	if h.vms != nil {
		if err := h.vms.DeleteForUser(id); err != nil {
			response.Error(c, http.StatusInternalServerError, response.CodeServerError, "删除用户资源授权失败")
			return
		}
	}
	response.OK(c, nil)
}

// normalizeRole 校验角色值：空 → 默认 user；否则必须存在于角色表
// （支持自定义角色，角色表不存在时返回错误）。
func (h *UserHandler) normalizeRole(role string) (domain.Role, error) {
	if role == "" {
		return domain.RoleUser, nil
	}
	if _, err := h.roles.FindByID(role); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return "", errors.New("无效的角色值: " + role + "（角色不存在，见 GET /api/v1/roles）")
		}
		return "", errors.New("查询角色失败")
	}
	return domain.Role(role), nil
}
