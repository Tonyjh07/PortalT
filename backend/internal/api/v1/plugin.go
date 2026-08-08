package v1

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"portalt/internal/adapters/auth"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// pluginIDPattern 插件 ID 白名单：字母数字开头，仅含字母数字与 . _ -，
// 防止恶意 ID（如 ../）被拼进 Caddy 规则文件路径造成路径穿越。
var pluginIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// PluginHandler 插件管理接口处理器（管理员）。
type PluginHandler struct {
	plugins ports.PluginRepository
	// perms 权限字典（可选；nil 时跳过声明权限校验）
	perms ports.PermissionRepository
	// caddy Caddy 规则应用器（access 插件落盘与 reload；nil 时跳过）
	caddy ports.CaddyApplier
}

// NewPluginHandler 创建插件处理器。
func NewPluginHandler(plugins ports.PluginRepository, perms ports.PermissionRepository, caddy ports.CaddyApplier) *PluginHandler {
	return &PluginHandler{plugins: plugins, perms: perms, caddy: caddy}
}

// validatePerm 校验插件声明的访问权限存在于权限字典。
func (h *PluginHandler) validatePerm(perm string) error {
	if perm == "" || h.perms == nil {
		return nil
	}
	ok, err := h.perms.Exists(perm)
	if err != nil {
		return errors.New("查询权限字典失败")
	}
	if !ok {
		return errors.New("未知权限: " + perm + "（仅允许权限字典中的权限，见 GET /api/v1/roles/permissions）")
	}
	return nil
}

// pluginRequest 插件创建/更新请求体。
type pluginRequest struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name" binding:"required"`
	Icon       string                  `json:"icon"`
	Route      string                  `json:"route" binding:"required"`
	Type       domain.PluginType       `json:"type"`
	IframeURL  string                  `json:"iframe_url"`
	ApiURL     string                  `json:"api_url"`
	Endpoints  []domain.PluginEndpoint `json:"endpoints"`
	CaddyRules string                  `json:"caddy_rules"`
	Permission string                  `json:"permission"`
	SortOrder  int                     `json:"sort_order"`
	IsActive   bool                    `json:"is_active"`
}

// validate 校验插件类型与必要字段。
func (r *pluginRequest) validate() error {
	if r.ID != "" && !pluginIDPattern.MatchString(r.ID) {
		return errors.New("插件 ID 仅允许字母数字及 . _ -，且须以字母或数字开头")
	}
	if !domain.IsValidPluginType(r.Type) {
		return errors.New("无效的插件类型（可选 access/native）")
	}
	switch domain.NormalizePluginType(r.Type) {
	case domain.PluginTypeAccess:
		// access 至少提供一种能力：iframe 嵌入地址，或 api_url + 端点白名单
		if r.IframeURL == "" && (r.ApiURL == "" || len(r.Endpoints) == 0) {
			return errors.New("access 类型插件必须配置 iframe_url，或 api_url + 至少一个端点")
		}
		if r.IframeURL != "" && !isIframeURLSafe(r.IframeURL) {
			return errors.New("iframe_url 必须是 http/https 地址或门户内相对路径（以 / 开头）")
		}
		if r.ApiURL != "" && !isURLSafe(r.ApiURL) {
			return errors.New("api_url 必须是 http/https 地址")
		}
		// 端点路径白名单：只要声明了端点就校验格式（与是否配置 api_url 无关）
		for _, e := range r.Endpoints {
			if e.Path == "" {
				return errors.New("端点路径不能为空")
			}
			if !strings.HasPrefix(e.Path, "/") {
				return errors.New("端点路径必须以 / 开头（如 /api/info）")
			}
		}
	}
	return nil
}

// isIframeURLSafe 校验 iframe 嵌入地址：允许 http/https 或门户内相对路径
// （如 "/esxi/ui/" 由 Caddy 规则反代），拒绝 "//" 协议相对地址与其他 scheme 防注入。
func isIframeURLSafe(raw string) bool {
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return true
	}
	return isURLSafe(raw)
}

// toDomain 将请求体转换为领域实体（复用 adapters/auth 的 ID 生成器）。
func (r *pluginRequest) toDomain(id string) *domain.Plugin {
	return &domain.Plugin{
		ID:         id,
		Name:       r.Name,
		Icon:       r.Icon,
		Route:      r.Route,
		Type:       domain.NormalizePluginType(r.Type),
		IframeURL:  r.IframeURL,
		ApiURL:     r.ApiURL,
		Endpoints:  r.Endpoints,
		CaddyRules: r.CaddyRules,
		Permission: r.Permission,
		SortOrder:  r.SortOrder,
		IsActive:   r.IsActive,
	}
}

// syncCaddy 将 access 插件的 Caddy 规则落盘并触发 reload。
// 停用（is_active=false）或清空规则时移除规则文件，避免停用插件仍占用反代路径。
// 返回 (reload 警告, 错误)：错误 = 落盘/移除失败（500）；警告 = reload 失败（规则已落盘/移除）。
func (h *PluginHandler) syncCaddy(p *domain.Plugin) (string, error) {
	if h.caddy == nil || domain.NormalizePluginType(p.Type) != domain.PluginTypeAccess {
		return "", nil
	}
	if !p.IsActive || p.CaddyRules == "" {
		if err := h.caddy.Remove(p.ID); err != nil {
			return "", err
		}
		if err := h.caddy.Reload(); err != nil {
			return "插件已保存，但 Caddy reload 失败（规则已移除，将随下次 reload 生效）", nil
		}
		return "", nil
	}
	if err := h.caddy.Apply(p.ID, p.CaddyRules); err != nil {
		return "", err
	}
	if err := h.caddy.Reload(); err != nil {
		return "插件已保存，但 Caddy reload 失败（规则已落盘，将随下次 reload 生效）", nil
	}
	return "", nil
}

// Create POST /api/v1/plugins
// 注册新插件（ID 未提供时自动生成；返回完整插件）。
func (h *PluginHandler) Create(c *gin.Context) {
	var req pluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	if err := h.validatePerm(req.Permission); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	id := req.ID
	if id == "" {
		id = auth.NewID()
	}
	plugin := req.toDomain(id)
	if err := h.plugins.Save(plugin); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "注册插件失败")
		return
	}
	if warn, err := h.syncCaddy(plugin); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "插件已注册，但 Caddy 规则落盘失败: "+err.Error())
		return
	} else if warn != "" {
		response.OKWithMessage(c, warn, plugin)
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
	if err := req.validate(); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	if err := h.validatePerm(req.Permission); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	plugin := req.toDomain(id)
	if err := h.plugins.Save(plugin); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "更新插件失败")
		return
	}
	if warn, err := h.syncCaddy(plugin); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "插件已更新，但 Caddy 规则落盘失败: "+err.Error())
		return
	} else if warn != "" {
		response.OKWithMessage(c, warn, plugin)
		return
	}
	response.OK(c, plugin)
}

// Delete DELETE /api/v1/plugins/:id
// 删除插件，并移除其 Caddy 规则文件。
func (h *PluginHandler) Delete(c *gin.Context) {
	if err := h.plugins.Delete(c.Param("id")); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "插件不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "删除插件失败")
		return
	}
	if h.caddy != nil {
		if err := h.caddy.Remove(c.Param("id")); err != nil {
			response.OKWithMessage(c, "插件已删除，但 Caddy 规则文件移除失败: "+err.Error(), nil)
			return
		}
		if err := h.caddy.Reload(); err != nil {
			response.OKWithMessage(c, "插件已删除，但 Caddy reload 失败（规则文件已移除，将随下次 reload 生效）", nil)
			return
		}
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
