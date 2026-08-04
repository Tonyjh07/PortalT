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

	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
	"portalt/internal/ports"
)

// metadataSensitiveRe 匹配 metadata 中的敏感键（密码/令牌），
// 列表与详情接口返回时移除，避免低权限角色（vm:view）获取远程桌面凭证。
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
	svc *services.VMService
}

// NewVMHandler 创建虚拟机处理器。
func NewVMHandler(svc *services.VMService) *VMHandler {
	return &VMHandler{svc: svc}
}

// List GET /api/v1/vms
// 返回全部虚拟机（按名称排序，仓储层保证）。
func (h *VMHandler) List(c *gin.Context) {
	vms, err := h.svc.ListVMs(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机失败")
		return
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

// powerOp 电源操作统一处理：校验错误映射 + 成功返回最新状态。
func (h *VMHandler) powerOp(c *gin.Context, verb string, fn func(ctx context.Context, id string) (*domain.VM, error)) {
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
