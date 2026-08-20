package v1

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wwt/guac"

	"portalt/internal/adapters/memory"
	"portalt/internal/api/middleware"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// mockGuacd 模拟 guacd 服务器（TCP）：完成握手（select/args/size/connect/ready），
// 记录 select、size 与 connect 参数，之后进入指令回显模式。
type mockGuacd struct {
	t          *testing.T
	listener   net.Listener
	selectGot  chan []string
	sizeGot    chan []string
	connectGot chan []string
}

func newMockGuacd(t *testing.T) *mockGuacd {
	t.Helper()
	m := &mockGuacd{
		t:          t,
		selectGot:  make(chan []string, 1),
		sizeGot:    make(chan []string, 1),
		connectGot: make(chan []string, 1),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	m.listener = ln
	go m.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return m
}

func (m *mockGuacd) addr() string { return m.listener.Addr().String() }

func (m *mockGuacd) serve() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		go m.handle(conn)
	}
}

func (m *mockGuacd) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	stream := guac.NewStream(conn, 5*time.Second)

	sel, err := guac.ReadOne(stream)
	if err != nil {
		return
	}
	m.selectGot <- sel.Args

	if _, err := stream.Write(guac.NewInstruction("args", "hostname", "port", "password",
		"color-depth", "enable-wallpaper", "enable-theming", "enable-font-smoothing").Byte()); err != nil {
		return
	}
	// size（隧道代发：宽、高、分辨率）
	size, err := guac.ReadOne(stream)
	if err != nil {
		return
	}
	m.sizeGot <- size.Args
	// audio / video / image（隧道代发，空列表）
	for i := 0; i < 3; i++ {
		if _, err := guac.ReadOne(stream); err != nil {
			return
		}
	}
	// connect（隧道代发，参数已被 metadata 注入）
	connect, err := guac.ReadOne(stream)
	if err != nil {
		return
	}
	m.connectGot <- connect.Args
	if _, err := stream.Write(guac.NewInstruction("ready", "mock-conn-1").Byte()); err != nil {
		return
	}

	// 回显模式：验证 ready 之后的指令流双向转发
	for {
		raw, err := stream.ReadSome()
		if err != nil {
			return
		}
		if _, err := stream.Write(raw); err != nil {
			return
		}
	}
}

// setupGuacd 组装 guacd 原生隧道路由（认证桩注入用户 + VM 桩）。
func setupGuacd(t *testing.T, user *domain.User, guacdAddr string, vm *domain.VM, vmErr error) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.AuthRequired(&stubTokenManager{user: user}))
	h := NewGuacdHandler(guacdAddr, func(_ context.Context, id string) (*domain.VM, error) {
		if vmErr != nil {
			return nil, vmErr
		}
		if vm == nil || vm.ID != id {
			return nil, ports.ErrNotFound
		}
		return vm, nil
	}, nil)
	r.GET("/guac/ws/:vmId", h.Proxy)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// setupGuacdAuth 构造带资源授权校验的隧道环境（guacd 地址不可达，授权通过也不会到达拨号阶段）。
func setupGuacdAuth(t *testing.T, user *domain.User, access ports.VMAccessRepository) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.AuthRequired(&stubTokenManager{user: user}))
	h := NewGuacdHandler("127.0.0.1:1", func(_ context.Context, id string) (*domain.VM, error) {
		return nil, ports.ErrNotFound
	}, access)
	r.GET("/guac/ws/:vmId", h.Proxy)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestGuacd_Authorization_Unauthorized_NotFound(t *testing.T) {
	access := memory.NewVMAccessRepository()
	require.NoError(t, access.SetForUser("limited", []string{"vm-2"}))
	user := &domain.User{ID: "limited", Username: "limited", Role: domain.RoleUser}
	server := setupGuacdAuth(t, user, access)

	// 未授权 VM：普通 HTTP 404（不升级 WS）
	req, err := http.NewRequest(http.MethodGet, server.URL+"/guac/ws/vm-1", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// 已授权 VM：授权通过后走到 getVM（这里返回 ErrNotFound → 404），与未授权表现一致
	req, err = http.NewRequest(http.MethodGet, server.URL+"/guac/ws/vm-2", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer t")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// dialTunnel 以 WebSocket 客户端身份连接隧道，返回连接。
func dialTunnel(t *testing.T, server *httptest.Server, vmID string) *websocket.Conn {
	t.Helper()
	return dialTunnelQuery(t, server, vmID, "")
}

// dialTunnelQuery 带会话级 query 参数（如 mode）连接隧道。
func dialTunnelQuery(t *testing.T, server *httptest.Server, vmID, query string) *websocket.Conn {
	t.Helper()
	wsURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	wsURL.Scheme = "ws"
	wsURL.Path = "/guac/ws/" + vmID
	wsURL.RawQuery = query

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL.String(), http.Header{
		"Authorization": {"Bearer t"},
	})
	require.NoError(t, err, "连接隧道失败: %v", resp)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// testVM 构造带远程桌面连接参数的虚拟机。
func testVM() *domain.VM {
	return &domain.VM{
		ID:        "vm-1",
		Name:      "web-vm",
		Status:    domain.VMStatusPoweredOn,
		IPAddress: "10.0.0.9",
		Metadata: map[string]any{
			"guac.protocol": "vnc",
			"guac.hostname": "10.1.1.1",
			"guac.port":     "5999",
			"guac.password": "s3cret",
		},
	}
}

// readRaw 读取下一条 WS 消息原文（断言辅助）。
func readRaw(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)
	return raw
}

func TestGuacd_Tunnel_ServerSideHandshake_InjectsParams(t *testing.T) {
	// 浏览器（guacamole-common-js）不参与握手：select/args/size/connect 全部
	// 由隧道代发，连接参数来自 VM metadata，客户端无法注入恶意目标。
	mock := newMockGuacd(t)
	proxy := setupGuacd(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		mock.addr(), testVM(), nil)
	conn := dialTunnel(t, proxy, "vm-1")

	// 隧道代发 select，协议由 metadata 指定
	assert.Equal(t, []string{"vnc"}, <-mock.selectGot)
	// connect 参数被 metadata 覆盖（恶意值无从谈起——浏览器根本没机会发）
	got := <-mock.connectGot
	assert.Equal(t, []string{"10.1.1.1", "5999", "s3cret", "", "", "", ""}, got)

	// 会话建立后输入指令双向回显（等价验证 ready 已被服务端握手消费）
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("3.key,1.1,3.120;")))
	assert.Equal(t, []byte("3.key,1.1,3.120;"), readRaw(t, conn))
}

func TestGuacd_Tunnel_HostnameFallsBackToIP(t *testing.T) {
	vm := testVM()
	delete(vm.Metadata, "guac.hostname") // 未配置 hostname → 回退 VM.IPAddress
	mock := newMockGuacd(t)
	proxy := setupGuacd(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		mock.addr(), vm, nil)
	conn := dialTunnel(t, proxy, "vm-1")

	<-mock.selectGot
	got := <-mock.connectGot
	assert.Equal(t, []string{"10.0.0.9", "5999", "s3cret", "", "", "", ""}, got)

	// 回显往返验证隧道已进入转发状态
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("3.key,1.1,3.120;")))
	readRaw(t, conn)
}

func TestGuacd_Tunnel_DefaultPortWithoutMetadata(t *testing.T) {
	vm := testVM()
	vm.Metadata = nil // 无连接参数 → hostname 回退 IP、port 取协议默认 5900
	mock := newMockGuacd(t)
	proxy := setupGuacd(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		mock.addr(), vm, nil)
	conn := dialTunnel(t, proxy, "vm-1")

	<-mock.selectGot
	got := <-mock.connectGot
	assert.Equal(t, []string{"10.0.0.9", "5900", "", "", "", "", ""}, got)

	// 回显往返验证隧道已进入转发状态
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("3.key,1.1,3.120;")))
	readRaw(t, conn)
}

// 会话级质量模式（WS query mode）端到端：fluency 强制小分辨率与渲染参数，
// quality/非法值回退默认并尊重 metadata。
func TestGuacd_Tunnel_ModeQuery(t *testing.T) {
	rdpVM := &domain.VM{
		ID:        "vm-1",
		Name:      "win-vm",
		Status:    domain.VMStatusPoweredOn,
		IPAddress: "10.0.0.9",
		Metadata: map[string]any{
			"guac.protocol": "rdp",
			"guac.width":    "1600",
			"guac.height":   "900",
		},
	}
	user := &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser}

	t.Run("fluency 覆盖分辨率并降渲染参数", func(t *testing.T) {
		mock := newMockGuacd(t)
		proxy := setupGuacd(t, user, mock.addr(), rdpVM, nil)
		conn := dialTunnelQuery(t, proxy, "vm-1", "mode=fluency")

		assert.Equal(t, []string{"rdp"}, <-mock.selectGot)
		assert.Equal(t, []string{"1024", "640", "96"}, <-mock.sizeGot)
		got := <-mock.connectGot
		assert.Equal(t, []string{"10.0.0.9", "3389", "", "16", "false", "false", "false"}, got)

		// 会话建立后指令回显（验证握手已完成）
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("3.key,1.1,3.120;")))
		readRaw(t, conn)
	})

	t.Run("quality 尊重 metadata 分辨率", func(t *testing.T) {
		mock := newMockGuacd(t)
		proxy := setupGuacd(t, user, mock.addr(), rdpVM, nil)
		conn := dialTunnelQuery(t, proxy, "vm-1", "mode=quality")

		<-mock.selectGot
		assert.Equal(t, []string{"1600", "900", "96"}, <-mock.sizeGot)
		got := <-mock.connectGot
		assert.Equal(t, []string{"10.0.0.9", "3389", "", "", "", "", ""}, got)

		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("3.key,1.1,3.120;")))
		readRaw(t, conn)
	})

	t.Run("非法 mode 回退默认", func(t *testing.T) {
		mock := newMockGuacd(t)
		proxy := setupGuacd(t, user, mock.addr(), rdpVM, nil)
		conn := dialTunnelQuery(t, proxy, "vm-1", "mode=hack")

		<-mock.selectGot
		assert.Equal(t, []string{"1600", "900", "96"}, <-mock.sizeGot)

		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("3.key,1.1,3.120;")))
		readRaw(t, conn)
	})
}

func TestGuacd_Tunnel_BatchedClientMessage(t *testing.T) {
	// 客户端把多条输入指令合并为一条 WS 消息，隧道需拆分转发且不破坏内容
	mock := newMockGuacd(t)
	proxy := setupGuacd(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		mock.addr(), testVM(), nil)
	conn := dialTunnel(t, proxy, "vm-1")

	<-mock.selectGot
	<-mock.connectGot

	batched := "4.size,3.800,3.600,1.1;" + "3.key,1.1,3.120;"
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(batched)))
	assert.Equal(t, []byte("4.size,3.800,3.600,1.1;"), readRaw(t, conn))
	assert.Equal(t, []byte("3.key,1.1,3.120;"), readRaw(t, conn))
}

func TestGuacd_Tunnel_ClientPingEchoed(t *testing.T) {
	// 客户端稳定性 ping（内部 opcode ""）不得转发给 guacd，需原样回显
	mock := newMockGuacd(t)
	proxy := setupGuacd(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		mock.addr(), testVM(), nil)
	conn := dialTunnel(t, proxy, "vm-1")

	<-mock.selectGot
	<-mock.connectGot

	ping := "0.,4.ping,13.1750000000000;"
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(ping)))
	assert.Equal(t, []byte(ping), readRaw(t, conn))
}

func TestGuacd_Tunnel_VMNotFound(t *testing.T) {
	mock := newMockGuacd(t)
	proxy := setupGuacd(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		mock.addr(), nil, nil)

	req, err := http.NewRequest(http.MethodGet, proxy.URL+"/guac/ws/no-such", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGuacd_Tunnel_GuacdDown(t *testing.T) {
	proxy := setupGuacd(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleUser},
		"127.0.0.1:1", testVM(), nil)

	req, err := http.NewRequest(http.MethodGet, proxy.URL+"/guac/ws/vm-1", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}

func TestSplitInstructions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"单条", "7.connect,1.a,1.b;", []string{"7.connect,1.a,1.b;"}},
		{"多条合并", "4.size,1.1,1.2,1.3;" + "7.connect,1.a,1.b;", []string{"4.size,1.1,1.2,1.3;", "7.connect,1.a,1.b;"}},
		{"空", "", []string{""}},
		{"非规范", "not-an-instruction", []string{"not-an-instruction"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitInstructions([]byte(tc.in))
			want := make([][]byte, len(tc.want))
			for i, s := range tc.want {
				want[i] = []byte(s)
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestGuacConfigFromVM(t *testing.T) {
	cfg := guacConfigFromVM(testVM(), modeAuto)
	assert.Equal(t, "vnc", cfg.Protocol)
	assert.Equal(t, "10.1.1.1", cfg.Parameters["hostname"])
	assert.Equal(t, "5999", cfg.Parameters["port"])
	assert.Equal(t, "s3cret", cfg.Parameters["password"])
	assert.Equal(t, 1280, cfg.OptimalScreenWidth)
	assert.Equal(t, 800, cfg.OptimalScreenHeight)
	assert.Empty(t, cfg.AudioMimetypes)

	// 无 metadata：hostname 回退 IP，port 回退协议默认
	plain := guacConfigFromVM(&domain.VM{IPAddress: "192.168.2.5"}, modeAuto)
	assert.Equal(t, "vnc", plain.Protocol)
	assert.Equal(t, "192.168.2.5", plain.Parameters["hostname"])
	assert.Equal(t, "5900", plain.Parameters["port"])

	// 尺寸与 RDP 默认端口
	rdp := guacConfigFromVM(&domain.VM{Metadata: map[string]any{
		"guac.protocol": "rdp",
		"guac.width":    "1600",
		"guac.height":   900,
	}}, modeAuto)
	assert.Equal(t, "rdp", rdp.Protocol)
	assert.Equal(t, "3389", rdp.Parameters["port"])
	assert.Equal(t, 1600, rdp.OptimalScreenWidth)
	assert.Equal(t, 900, rdp.OptimalScreenHeight)
}

// 质量/流畅度模式的档位矩阵：fluency 强制覆盖分辨率与渲染参数并静音，
// quality 开启 RDP 音频且尊重 metadata 显式值，auto 等同默认参数。
func TestGuacConfigFromVM_Modes(t *testing.T) {
	rdpVM := &domain.VM{Metadata: map[string]any{
		"guac.protocol":              "rdp",
		"guac.width":                 "1600",
		"guac.height":                "900",
		"guac.color-depth":           "32",
		"guac.enable-wallpaper":      "true",
		"guac.enable-font-smoothing": "true",
	}}

	t.Run("auto 尊重 metadata 且静音", func(t *testing.T) {
		cfg := guacConfigFromVM(rdpVM, modeAuto)
		assert.Equal(t, 1600, cfg.OptimalScreenWidth)
		assert.Equal(t, 900, cfg.OptimalScreenHeight)
		assert.Equal(t, "32", cfg.Parameters["color-depth"])
		assert.Equal(t, "true", cfg.Parameters["enable-wallpaper"])
		assert.Empty(t, cfg.AudioMimetypes)
	})

	t.Run("quality 保持 metadata 并开启 RDP 音频", func(t *testing.T) {
		cfg := guacConfigFromVM(rdpVM, modeQuality)
		assert.Equal(t, 1600, cfg.OptimalScreenWidth)
		assert.Equal(t, "32", cfg.Parameters["color-depth"])
		assert.Equal(t, "true", cfg.Parameters["enable-wallpaper"])
		assert.Equal(t, []string{"audio/L16;rate=44100,channels=2"}, cfg.AudioMimetypes)
	})

	t.Run("fluency RDP 强制小分辨率低色深关特效", func(t *testing.T) {
		cfg := guacConfigFromVM(rdpVM, modeFluency)
		assert.Equal(t, fluencyWidth, cfg.OptimalScreenWidth)
		assert.Equal(t, fluencyHeight, cfg.OptimalScreenHeight)
		assert.Equal(t, "16", cfg.Parameters["color-depth"])
		assert.Equal(t, "false", cfg.Parameters["enable-wallpaper"])
		assert.Equal(t, "false", cfg.Parameters["enable-theming"])
		assert.Equal(t, "false", cfg.Parameters["enable-font-smoothing"])
		assert.Empty(t, cfg.AudioMimetypes)
	})

	t.Run("fluency VNC 降 8bit 色深", func(t *testing.T) {
		vncVM := &domain.VM{Metadata: map[string]any{"guac.protocol": "vnc"}}
		cfg := guacConfigFromVM(vncVM, modeFluency)
		assert.Equal(t, "8", cfg.Parameters["color-depth"])
		assert.Equal(t, fluencyWidth, cfg.OptimalScreenWidth)
	})

	t.Run("quality VNC 无音频", func(t *testing.T) {
		cfg := guacConfigFromVM(&domain.VM{Metadata: map[string]any{"guac.protocol": "vnc"}}, modeQuality)
		assert.Empty(t, cfg.AudioMimetypes)
	})
}

func TestParseMode(t *testing.T) {
	assert.Equal(t, modeQuality, parseMode("quality"))
	assert.Equal(t, modeFluency, parseMode("fluency"))
	assert.Equal(t, modeAuto, parseMode("auto"))
	assert.Equal(t, modeAuto, parseMode(""))
	assert.Equal(t, modeAuto, parseMode("ultra"))
}
