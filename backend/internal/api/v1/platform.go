package v1

import (
	"github.com/gin-gonic/gin"

	"portalt/internal/api/response"
)

// PlatformHandler 平台信息接口（供前端插件页判断虚拟化平台接入状态）。
type PlatformHandler struct {
	provider string
	webURL   string
}

// NewPlatformHandler 创建平台信息处理器。
// provider 为当前虚拟化平台类型（esxi/workstation/mock），webURL 为其 Web 管理界面地址。
func NewPlatformHandler(provider, webURL string) *PlatformHandler {
	return &PlatformHandler{provider: provider, webURL: webURL}
}

// Info GET /api/v1/platform
// 返回当前平台连接状态，供 access 插件（如 esxi-admin）的占位页判断可嵌入性。
func (h *PlatformHandler) Info(c *gin.Context) {
	response.OK(c, gin.H{
		"provider":  h.provider,
		"web_url":   h.webURL,
		"connected": h.provider == "esxi",
	})
}
