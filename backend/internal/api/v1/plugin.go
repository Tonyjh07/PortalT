package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/adapters/auth"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// PluginHandler 插件管理接口处理器（管理员）。
type PluginHandler struct {
	plugins ports.PluginRepository
}

// NewPluginHandler 创建插件处理器。
func NewPluginHandler(plugins ports.PluginRepository) *PluginHandler {
	return &PluginHandler{plugins: plugins}
}

// pluginRequest 插件创建/更新请求体。
type pluginRequest struct {
	Name       string `json:"name" binding:"required"`
	Icon       string `json:"icon"`
	Route      string `json:"route" binding:"required"`
	IframeURL  string `json:"iframe_url"`
	Permission string `json:"permission"`
	SortOrder  int    `json:"sort_order"`
	IsActive   bool   `json:"is_active"`
}

// toDomain 将请求体转换为领域实体（复用 adapters/auth 的 ID 生成器）。
func (r *pluginRequest) toDomain(id string) *domain.Plugin {
	return &domain.Plugin{
		ID:         id,
		Name:       r.Name,
		Icon:       r.Icon,
		Route:      r.Route,
		IframeURL:  r.IframeURL,
		Permission: r.Permission,
		SortOrder:  r.SortOrder,
		IsActive:   r.IsActive,
	}
}

// Create POST /api/v1/plugins
// 注册新插件（返回完整插件）。
func (h *PluginHandler) Create(c *gin.Context) {
	var req pluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	plugin := req.toDomain(auth.NewID())
	if err := h.plugins.Save(plugin); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "注册插件失败")
		return
	}
	response.OK(c, plugin)
}

// Update PUT /api/v1/plugins/:id
// 更新插件全部业务字段。
func (h *PluginHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.plugins.FindByID(id); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "插件不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}

	var req pluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	plugin := req.toDomain(id)
	if err := h.plugins.Save(plugin); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "更新插件失败")
		return
	}
	response.OK(c, plugin)
}

// Delete DELETE /api/v1/plugins/:id
// 删除插件。
func (h *PluginHandler) Delete(c *gin.Context) {
	if err := h.plugins.Delete(c.Param("id")); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "插件不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "删除插件失败")
		return
	}
	response.OK(c, nil)
}

// List GET /api/v1/plugins
// 返回全部插件（含停用），供管理界面展示。
func (h *PluginHandler) List(c *gin.Context) {
	plugins, err := h.plugins.FindAll()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}
	response.OK(c, plugins)
}
