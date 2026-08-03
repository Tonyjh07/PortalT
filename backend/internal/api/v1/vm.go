package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
	"portalt/internal/ports"
)

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
	response.OK(c, vms)
}

// Get GET /api/v1/vms/:id
// 返回单个虚拟机详情。
func (h *VMHandler) Get(c *gin.Context) {
	vm, ok := h.findVM(c)
	if !ok {
		return
	}
	response.OK(c, vm)
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
// body 为键值对象；值为 null 的键删除。
func (h *VMHandler) UpdateMetadata(c *gin.Context) {
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求体必须是 JSON 对象")
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
	response.OK(c, vm)
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
	response.OK(c, vm)
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
