package v1

import (
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
)

// GuacHandler Guacamole WebSocket 代理处理器。
//
// 职责：将客户端的 WebSocket 连接升级后，转发到后端 Guacamole
// 服务器（GUAC_URL），并把当前登录用户信息注入上游连接请求头，
// 使 Guacamole 无需独立认证即可获知会话身份。
type GuacHandler struct {
	// guacWSURL Guacamole WebSocket 隧道地址（如 ws://guacamole:8080/guacamole/websocket-tunnel）
	guacWSURL string
}

// NewGuacHandler 创建 Guacamole 代理处理器。
// guacWSURL 为空时禁用代理（返回 503），保证无 Guacamole 环境下服务可用。
func NewGuacHandler(guacWSURL string) *GuacHandler {
	return &GuacHandler{guacWSURL: guacWSURL}
}

// upgrader WebSocket 升级器（关闭原始来源校验，由认证中间件把关）。
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Proxy GET /api/v1/guac/ws/:vmId
// 双向转发客户端与 Guacamole 服务器的 WebSocket 数据。
// 注入请求头：X-PortalT-User（用户名）、X-PortalT-Role、X-PortalT-VMID。
func (h *GuacHandler) Proxy(c *gin.Context) {
	if h.guacWSURL == "" {
		response.Error(c, http.StatusServiceUnavailable, response.CodeServerError, "Guacamole 未配置")
		return
	}
	vmID := c.Param("vmId")
	user := middleware.CurrentUser(c)
	if user == nil {
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidToken, "令牌无效或已过期")
		return
	}

	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // gin 已写入升级失败响应
	}
	defer clientConn.Close()

	target, err := url.Parse(h.guacWSURL)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "Guacamole 配置无效")
		return
	}

	upstream, _, err := websocket.DefaultDialer.Dial(target.String(), http.Header{
		"X-PortalT-User": {user.Username},
		"X-PortalT-Role": {string(user.Role)},
		"X-PortalT-VMID": {vmID},
	})
	if err != nil {
		response.Error(c, http.StatusBadGateway, response.CodeServerError, "连接 Guacamole 失败")
		return
	}
	defer upstream.Close()

	// 双向拷贝，任一侧关闭即断开
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := upstream.ReadMessage()
			if err != nil {
				return
			}
			if err := clientConn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mt, msg, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			if err := upstream.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}()
	<-done
}

// GuacURLFromEnv 从环境变量读取 Guacamole WebSocket 地址。
// 支持 GUAC_URL（HTTP/HTTPS/WS 前缀自动转换）。
func GuacURLFromEnv() string {
	raw := os.Getenv("GUAC_URL")
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return ""
	}
	return u.String()
}
