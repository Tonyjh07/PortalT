package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
	"portalt/internal/ports"
)

// metadataSensitiveRe 匹配 metadata 中的敏感键（密码/令牌），
// 列表与详情接口返回时移除，避免低权限角色（vm:view）获取远程桌面凭证。
// rustdesk.key 为自建服务器公钥（非秘密），不在剔除之列；
// rustdesk.password 等已由 password 模式覆盖。
var metadataSensitiveRe = regexp.MustCompile(`(?i)password|passwd|secret|token`)

// sanitizeVM 返回 VM 的脱敏副本：metadata 中键名匹配敏感模式的项被移除。
// 不修改原对象（仓储缓存仍保留完整数据）。
func sanitizeVM(vm *domain.VM) *domain.VM {
	if vm == nil || len(vm.Metadata) == 0 {
		return vm
	}
	cp := *vm
	md := make(map[string]any, len(vm.Metadata))
	for k, v := range vm.Metadata {
		if !metadataSensitiveRe.MatchString(k) {
			md[k] = v
		}
	}
	cp.Metadata = md
	return &cp
}

// validateMetadataPatch 校验 metadata 更新中的受控键（远程桌面参数）。
func validateMetadataPatch(patch map[string]any) error {
	for k, v := range patch {
		if v == nil {
			continue
		}
		switch k {
		case "guac.protocol":
			s, ok := v.(string)
			if !ok || !slices.Contains([]string{"vnc", "rdp", "ssh", "telnet"}, s) {
				return errors.New("guac.protocol 仅支持 vnc/rdp/ssh/telnet")
			}
		case "guac.port":
			n, err := strconv.Atoi(fmtPort(v))
			if err != nil || n < 1 || n > 65535 {
				return errors.New("guac.port 必须为 1-65535 的整数")
			}
		case "guac.hostname":
			if strings.TrimSpace(fmtPort(v)) == "" {
				return errors.New("guac.hostname 不能为空")
			}
		case "rustdesk.id":
			if strings.TrimSpace(fmtPort(v)) == "" {
				return errors.New("rustdesk.id 不能为空")
			}
		}
	}
	return nil
}

// fmtPort 将 JSON 数字（float64）或字符串统一转字符串。
func fmtPort(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// VMHandler 虚拟机管理接口处理器。
type VMHandler struct {
	svc    *services.VMService
	access ports.VMAccessRepository
}

// NewVMHandler 创建虚拟机处理器。
// access 为 nil 时跳过资源级授权（仅依赖权限中间件）。
func NewVMHandler(svc *services.VMService, access ports.VMAccessRepository) *VMHandler {
	return &VMHandler{svc: svc, access: access}
}

// List GET /api/v1/vms
// 返回虚拟机列表（按名称排序，仓储层保证）；
// 非 vm:manage 用户仅返回 vm_access 授权的 VM。
func (h *VMHandler) List(c *gin.Context) {
	vms, err := h.svc.ListVMs(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机失败")
		return
	}
	if ids, all := h.visibleVMIDs(c); !all {
		if ids == nil {
			return
		}
		filtered := make([]*domain.VM, 0, len(vms))
		for _, vm := range vms {
			if _, ok := ids[vm.ID]; ok {
				filtered = append(filtered, vm)
			}
		}
		vms = filtered
	}
	if vms == nil {
		vms = []*domain.VM{}
	}
	sanitized := make([]*domain.VM, len(vms))
	for i, vm := range vms {
		sanitized[i] = sanitizeVM(vm)
	}
	response.OK(c, sanitized)
}

// Get GET /api/v1/vms/:id
// 返回单个虚拟机详情（metadata 敏感键已脱敏）。
func (h *VMHandler) Get(c *gin.Context) {
	if !authorizeVM(c, h.access, c.Param("id")) {
		return
	}
	vm, ok := h.findVM(c)
	if !ok {
		return
	}
	response.OK(c, sanitizeVM(vm))
}

// Start POST /api/v1/vms/:id/start
func (h *VMHandler) Start(c *gin.Context) {
	h.powerOp(c, "启动", h.svc.StartVM)
}

// Stop POST /api/v1/vms/:id/stop
func (h *VMHandler) Stop(c *gin.Context) {
	h.powerOp(c, "停止", h.svc.StopVM)
}

// Restart POST /api/v1/vms/:id/restart
func (h *VMHandler) Restart(c *gin.Context) {
	h.powerOp(c, "重启", h.svc.RestartVM)
}

// Status GET /api/v1/vms/:id/status
// 返回虚拟机实时状态（轮询用，从提供者回刷）。
func (h *VMHandler) Status(c *gin.Context) {
	if !authorizeVM(c, h.access, c.Param("id")) {
		return
	}
	vm, err := h.svc.GetVMStatus(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "虚拟机不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机状态失败")
		return
	}
	response.OK(c, gin.H{
		"id":     vm.ID,
		"name":   vm.Name,
		"status": vm.Status,
	})
}

// UpdateMetadata PUT /api/v1/vms/:id/metadata
// 合并更新虚拟机 metadata（如远程桌面参数 guac.*），需 vm:manage 权限。
// body 为键值对象；值为 null 的键删除。返回的 VM 已脱敏（密码只写不回）。
func (h *VMHandler) UpdateMetadata(c *gin.Context) {
	if !authorizeVM(c, h.access, c.Param("id")) {
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求体必须是 JSON 对象")
		return
	}
	if err := validateMetadataPatch(patch); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, err.Error())
		return
	}
	vm, err := h.svc.UpdateMetadata(c.Request.Context(), c.Param("id"), patch)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "虚拟机不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "更新虚拟机配置失败")
		return
	}
	response.OK(c, sanitizeVM(vm))
}

// powerOp 电源操作统一处理：资源授权校验 + 错误映射 + 成功返回最新状态。
func (h *VMHandler) powerOp(c *gin.Context, verb string, fn func(ctx context.Context, id string) (*domain.VM, error)) {
	if !authorizeVM(c, h.access, c.Param("id")) {
		return
	}
	vm, err := fn(c.Request.Context(), c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrNotFound):
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "虚拟机不存在")
		case errors.Is(err, ports.ErrInvalidOperation):
			response.Error(c, http.StatusConflict, response.CodeInvalidOperation, "当前状态不允许"+verb, err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, response.CodeServerError, verb+"虚拟机失败")
		}
		return
	}
	response.OK(c, sanitizeVM(vm))
}

func (h *VMHandler) findVM(c *gin.Context) (*domain.VM, bool) {
	vm, err := h.svc.GetVM(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "虚拟机不存在")
			return nil, false
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机失败")
		return nil, false
	}
	return vm, true
}

// authorizeVM 资源级授权：用户具备 vm:manage 时放行全部，
// 否则要求 vm_access 表中存在该用户对目标 VM 的授权。
// access 为 nil（未配置授权表）时直接放行；未授权按 404 处理（防枚举）。
// 返回 false 时响应已写入。VM 详情/电源/远程桌面等资源端点共用。
func authorizeVM(c *gin.Context, access ports.VMAccessRepository, vmID string) bool {
	if access == nil {
		return true
	}
	user := middleware.CurrentUser(c)
	if user == nil {
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidToken, "令牌无效或已过期")
		return false
	}
	if hasFullVM(c) {
		return true
	}
	ok, err := access.IsAuthorized(user.ID, vmID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机授权失败")
		return false
	}
	if !ok {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "虚拟机不存在")
		return false
	}
	return true
}

// hasFullVM 判断当前用户是否持有 vm:manage（资源级放行全部）。
func hasFullVM(c *gin.Context) bool {
	_, ok := middleware.CurrentPerms(c)[domain.PERM_VM_MANAGE]
	return ok
}

// visibleVMIDs 返回当前用户可见的 VM ID 集合（vm:manage 时为 nil 表示全部可见）。
// all=false 且 ids==nil 时表示内部错误，响应已写入。
func (h *VMHandler) visibleVMIDs(c *gin.Context) (map[string]struct{}, bool) {
	if h.access == nil || hasFullVM(c) {
		return nil, true
	}
	user := middleware.CurrentUser(c)
	if user == nil {
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidToken, "令牌无效或已过期")
		return nil, false
	}
	ids, err := h.access.VisibleVMIDs(user.ID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机授权失败")
		return nil, false
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, false
}
