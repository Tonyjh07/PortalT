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
	"portalt/internal/pluginhost"
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
	// host native 插件进程宿主（native 生命周期：启用/停用/重启；nil 时无 native 能力）
	host ports.NativeHost
}

// NewPluginHandler 创建插件处理器。
func NewPluginHandler(plugins ports.PluginRepository, perms ports.PermissionRepository, caddy ports.CaddyApplier, host ports.NativeHost) *PluginHandler {
	return &PluginHandler{plugins: plugins, perms: perms, caddy: caddy, host: host}
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
	if domain.NormalizePluginType(plugin.Type) == domain.PluginTypeNative {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidOperation,
			"native 插件由宿主按 manifest 自动注册，不可手动创建")
		return
	}
	if err := h.plugins.Save(plugin); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "注册插件失败")
		return
	}
	if warn, err := h.syncCaddy(plugin); err != nil {
		// 插件已落库（成功），仅 Caddy 规则落盘失败：降级为提示而非 500，
		// 避免 DB 与响应状态不一致（重试保存即可重新落盘）。
		response.OKWithMessage(c, "插件已保存，但 Caddy 规则落盘失败: "+err.Error(), plugin)
		return
	} else if warn != "" {
		response.OKWithMessage(c, warn, plugin)
		return
	}
	response.OK(c, plugin)
}

// Update PUT /api/v1/plugins/:id
// 更新插件业务字段。native 插件仅允许修改权限与启用状态
// （manifest 驱动字段不可改）；启用/停用经宿主 spawn/停进程。
func (h *PluginHandler) Update(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.plugins.FindByID(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "插件不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}

	// native 插件：仅权限与启用状态可写，其余字段以 manifest 为准。
	// 请求体最小化，避免被 access 的必要字段校验约束。
	if domain.NormalizePluginType(existing.Type) == domain.PluginTypeNative {
		h.updateNative(c, existing)
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
		response.OKWithMessage(c, "插件已保存，但 Caddy 规则落盘失败: "+err.Error(), plugin)
		return
	} else if warn != "" {
		response.OKWithMessage(c, warn, plugin)
		return
	}
	response.OK(c, plugin)
}

// nativeUpdateRequest native 插件更新请求体（仅权限与启用状态）。
// Permission 用指针：未提供时不改动现有权限（避免只改启用状态时误清空声明权限）。
type nativeUpdateRequest struct {
	Permission *string `json:"permission"`
	IsActive   *bool   `json:"is_active"`
}

// updateNative 处理 native 插件更新：仅应用权限与启用状态，
// 启用/停用经宿主触发生命周期（spawn / 停进程）。
func (h *PluginHandler) updateNative(c *gin.Context, existing *domain.Plugin) {
	id := c.Param("id")
	var req nativeUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	if req.Permission != nil {
		if err := h.validatePerm(*req.Permission); err != nil {
			response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
			return
		}
		existing.Permission = *req.Permission
	}

	// 启用状态变更才触发生命周期（避免反复 spawn）
	if req.IsActive != nil && existing.IsActive != *req.IsActive {
		ctx := c.Request.Context()
		if h.host == nil {
			response.Error(c, http.StatusInternalServerError, response.CodeServerError,
				"原生插件宿主未启用（未配置 PLUGINS_DIR）")
			return
		}
		var lcErr error
		if *req.IsActive {
			lcErr = h.host.Enable(ctx, id)
		} else {
			lcErr = h.host.Disable(ctx, id)
		}
		if lcErr != nil {
			response.Error(c, http.StatusInternalServerError, response.CodeServerError,
				"插件生命周期操作失败: "+lcErr.Error())
			return
		}
		// 宿主已落库 is_active；本地对象同步以便返回（防宿主实现不落库时的偏差）
		existing.IsActive = *req.IsActive
	}
	if err := h.plugins.Save(existing); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "更新插件失败")
		return
	}
	response.OK(c, existing)
}

// Delete DELETE /api/v1/plugins/:id
// 删除插件，并移除其 Caddy 规则文件。native 插件记录由宿主管理，不可删除。
func (h *PluginHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.plugins.FindByID(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "插件不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}
	if domain.NormalizePluginType(existing.Type) == domain.PluginTypeNative {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidOperation,
			"native 插件由宿主管理，不可删除（删除插件目录即自动标记 missing）")
		return
	}
	if err := h.plugins.Delete(id); err != nil {
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

// ReloadCaddy POST /api/v1/plugins/caddy-reload
// 以数据库为准全量对齐 access 插件的 Caddy 规则并触发 reload（补写未落盘规则、
// 清理孤儿文件）。用于规则保存后 reload 失败、或手工改盘后的一次性主动修复。
// 返回 200；规则未完全生效（部分校验失败/reload 失败）时以 __message 携带告警。
func (h *PluginHandler) ReloadCaddy(c *gin.Context) {
	if h.caddy == nil {
		response.Error(c, http.StatusServiceUnavailable, response.CodeServerError, "Caddy 规则管理未启用")
		return
	}
	plugins, err := h.plugins.FindAll()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}
	if err := h.caddy.SyncAll(plugins); err != nil {
		// reload 失败（规则已落盘，仅未热生效）与对齐未完成（部分插件校验/落盘
		// 失败或磁盘操作异常）分开提示，与 syncCaddy"落盘 vs reload"口径一致。
		if pluginhost.IsReloadFailed(err) {
			response.OKWithMessage(c, "Caddy 规则已对齐，但 reload 失败（规则已落盘，将随下次 reload 生效）: "+err.Error(), nil)
			return
		}
		response.OKWithMessage(c, "Caddy 规则对齐未完全完成: "+err.Error(), nil)
		return
	}
	response.OK(c, gin.H{"reloaded": true})
}

// pluginListItem 插件列表项：在领域实体基础上附加计算字段（供管理界面展示）。
type pluginListItem struct {
	*domain.Plugin
	// CaddyApplied access 插件：规则文件当前是否已落盘（其余类型恒为 false）
	CaddyApplied bool `json:"caddy_applied"`
}

// List GET /api/v1/plugins
// 返回全部插件（含停用），供管理界面展示。
func (h *PluginHandler) List(c *gin.Context) {
	plugins, err := h.plugins.FindAll()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}
	items := make([]pluginListItem, 0, len(plugins))
	for _, p := range plugins {
		applied := false
		if h.caddy != nil && domain.NormalizePluginType(p.Type) == domain.PluginTypeAccess {
			applied = h.caddy.HasRuleFile(p.ID)
		}
		items = append(items, pluginListItem{Plugin: p, CaddyApplied: applied})
	}
	response.OK(c, items)
}

// Restart POST /api/v1/plugins/:id/restart
// 重启 native 插件进程（仅对启用且已安装的插件生效）。
func (h *PluginHandler) Restart(c *gin.Context) {
	id := c.Param("id")
	existing, err := h.plugins.FindByID(id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "插件不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}
	if domain.NormalizePluginType(existing.Type) != domain.PluginTypeNative {
		response.Error(c, http.StatusBadRequest, response.CodeInvalidOperation, "仅 native 插件可重启")
		return
	}
	if h.host == nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "原生插件宿主未启用（未配置 PLUGINS_DIR）")
		return
	}
	if err := h.host.Restart(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "插件重启失败: "+err.Error())
		return
	}
	response.OK(c, nil)
}
