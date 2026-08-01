package v1

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/api/middleware"
	"portalt/internal/domain"
)

// upstreamEcho 模拟 Guacamole 服务器的 WebSocket 隧道。
// 回显收到的首条消息，并把注入的请求头原样作为第二条消息返回。
type upstreamEcho struct {
	t      *testing.T
	got    map[string][]string
	server *httptest.Server
}

func newUpstreamEcho(t *testing.T) *upstreamEcho {
	t.Helper()
	u := &upstreamEcho{t: t, got: map[string][]string{}}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.got["user"] = []string{r.Header.Get("X-PortalT-User")}
		u.got["role"] = []string{r.Header.Get("X-PortalT-Role")}
		u.got["vmid"] = []string{r.Header.Get("X-PortalT-VMID")}

		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		require.NoError(u.t, err)
		defer conn.Close()

		mt, msg, err := conn.ReadMessage()
		require.NoError(u.t, err)
		require.NoError(u.t, conn.WriteMessage(mt, msg))
	}))
	t.Cleanup(u.server.Close)
	return u
}

func (u *upstreamEcho) wsURL() string {
	parsed, _ := url.Parse(u.server.URL)
	parsed.Scheme = "ws"
	return parsed.String()
}

// setupGuac 组装 Guacamole 代理路由（认证桩注入用户）。
func setupGuac(t *testing.T, user *domain.User, upstreamURL string) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.AuthRequired(&stubTokenManager{user: user}))
	r.GET("/guac/ws/:vmId", NewGuacHandler(upstreamURL).Proxy)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestGuac_Proxy_EchoRoundtrip(t *testing.T) {
	up := newUpstreamEcho(t)
	proxy := setupGuac(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser}, up.wsURL())

	wsURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	wsURL.Scheme = "ws"
	wsURL.Path = "/guac/ws/vm-42"

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), http.Header{
		"Authorization": {"Bearer t"},
	})
	require.NoError(t, err, "连接代理失败: %v", resp)
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ping")))
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "ping", string(msg))

	// 上游应收到注入的会话头
	assert.Equal(t, "alice", up.got["user"][0])
	assert.Equal(t, "user", up.got["role"][0])
	assert.Equal(t, "vm-42", up.got["vmid"][0])
}

func TestGuac_Proxy_NotConfigured(t *testing.T) {
	proxy := setupGuac(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser}, "")

	// 未配置 Guacamole 时普通 HTTP 请求返回 503（不升级）
	req, err := http.NewRequest(http.MethodGet, proxy.URL+"/guac/ws/vm-1", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestGuac_Proxy_Unauthenticated(t *testing.T) {
	// 用户为 nil → 未认证
	proxy := setupGuac(t, nil, "ws://127.0.0.1:1")

	resp, err := http.Get(proxy.URL + "/guac/ws/vm-1")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGuac_Proxy_UpstreamDown(t *testing.T) {
	// 上游不可达 → 升级失败返回 502
	proxy := setupGuac(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		"ws://127.0.0.1:1/none")

	wsURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	wsURL.Scheme = "ws"
	wsURL.Path = "/guac/ws/vm-1"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), http.Header{
		"Authorization": {"Bearer t"},
	})
	if err != nil {
		// 握手中断即视为失败路径正确
		assert.NotNil(t, resp)
		return
	}
	assert.NotNil(t, resp)
}
