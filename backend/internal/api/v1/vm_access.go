package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/response"
	"portalt/internal/ports"
)

// VMAccessHandler 虚拟机资源授权分配接口（user:manage，管理员）。
// 授权粒度为「用户 → 可见/可操作的 VM 集合」；vm:manage 权限用户天然放行全部。
type VMAccessHandler struct {
	access ports.VMAccessRepository
}

// NewVMAccessHandler 创建资源授权处理器。
func NewVMAccessHandler(access ports.VMAccessRepository) *VMAccessHandler {
	return &VMAccessHandler{access: access}
}

// vmAccessRequest 授权分配请求体（全量替换语义）。
type vmAccessRequest struct {
	VMIDs []string `json:"vm_ids"`
}

// Get GET /api/v1/users/:id/vm-access
// 返回用户当前授权的 VM ID 列表。
func (h *VMAccessHandler) Get(c *gin.Context) {
	ids, err := h.access.VisibleVMIDs(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机授权失败")
		return
	}
	if ids == nil {
		ids = []string{}
	}
	response.OK(c, gin.H{"vm_ids": ids})
}

// Set PUT /api/v1/users/:id/vm-access
// 全量替换用户授权列表（传入空数组即清空全部授权）。
func (h *VMAccessHandler) Set(c *gin.Context) {
	var req vmAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}
	if err := h.access.SetForUser(c.Param("id"), req.VMIDs); err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "更新虚拟机授权失败")
		return
	}
	response.OK(c, nil)
}
