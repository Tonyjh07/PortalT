package v1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/wwt/guac"

	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// GuacdHandler Guacamole 原生隧道处理器 (Phase 8)。
//
// 架构：浏览器 (guacamole-common-js) --WS--> PortalT --原生指令--> guacd(:4822)。
// 与 GuacHandler 的区别：不再转发到 Guacamole Web 应用的 WebSocket 隧道，
// 而是由 PortalT 直连 guacd 并服务端完成协议握手（select/args/size/connect/ready）。
// 连接参数（VM metadata 中 guac.* 键）在握手时注入，浏览器侧无法自行指定
// 任意主机与端口。
type GuacdHandler struct {
	// guacdURL guacd 的 TCP 地址（如 127.0.0.1:4822）
	guacdURL string
	// getVM 按 ID 获取虚拟机（用于读取 metadata 连接参数）
	getVM func(ctx context.Context, id string) (*domain.VM, error)
	// access 虚拟机资源授权表（nil 时仅依赖权限中间件）
	access ports.VMAccessRepository
}

// NewGuacdHandler 创建 guacd 原生隧道处理器。
func NewGuacdHandler(guacdURL string, getVM func(ctx context.Context, id string) (*domain.VM, error), access ports.VMAccessRepository) *GuacdHandler {
	return &GuacdHandler{guacdURL: guacdURL, getVM: getVM, access: access}
}

// GuacProxy Guacamole 隧道代理处理器接口（兼容 Web 应用代理与 guacd 原生隧道）。
type GuacProxy interface {
	Proxy(c *gin.Context)
}

// GuacHandlerForEnv 根据环境变量选择隧道模式：
//   - GUACD_URL 已配置 → guacd 原生隧道（Phase 8，推荐，浏览器直连 guacd）；
//   - GUAC_URL 已配置 → 转发 Guacamole Web 应用 WebSocket 隧道（旧模式）；
//   - 均未配置 → 返回 nil（路由层返回 503）。
//
// access 为虚拟机资源授权表（nil 时仅做权限中间件校验，无资源级校验）。
func GuacHandlerForEnv(getVM func(ctx context.Context, id string) (*domain.VM, error), access ports.VMAccessRepository) GuacProxy {
	if d := os.Getenv("GUACD_URL"); d != "" {
		return NewGuacdHandler(d, getVM, access)
	}
	if w := GuacURLFromEnv(); w != "" {
		return NewGuacHandler(w)
	}
	return nil
}

// 连接参数默认值。
const (
	guacdDialTimeout = 5 * time.Second
	defaultVNCWidth  = 1280
	defaultVNCHeight = 800
)

// internalOpcodePrefix 客户端内部指令（如稳定性 ping）的字节前缀。
// 内部指令 opcode 为空字符串，线格式为 "0.,..."。
var internalOpcodePrefix = []byte("0.")

// Proxy GET /api/v1/guac/ws/:vmId（guacd 模式）
// 浏览器打开 WS 后由本处理器代发握手，随后双向透传指令流。
func (h *GuacdHandler) Proxy(c *gin.Context) {
	vmID := c.Param("vmId")
	// 资源级授权（vm:console 由路由中间件校验；此处校验 vm_access 表），
	// 未授权按不存在处理（防枚举），与 VM 详情/电源端点共用同一语义。
	if !authorizeVM(c, h.access, vmID) {
		return
	}
	vm, err := h.getVM(c.Request.Context(), vmID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "虚拟机不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询虚拟机失败")
		return
	}

	// 先拨号 guacd：失败时返回普通 HTTP 错误，避免升级后立刻断连
	guacdConn, err := net.DialTimeout("tcp", h.guacdURL, guacdDialTimeout)
	if err != nil {
		response.Error(c, http.StatusBadGateway, response.CodeServerError, "连接 guacd 失败")
		return
	}
	defer guacdConn.Close()

	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // gin 已写入升级失败响应
	}
	defer clientConn.Close()

	// 服务端完成与 guacd 的协议握手（连接参数全部来自 VM metadata）
	stream := guac.NewStream(guacdConn, guac.SocketTimeout)
	cfg := guacConfigFromVM(vm)
	if err := stream.Handshake(&cfg); err != nil {
		// 握手失败（guacd 拒绝/目标不可达等）：以内部错误码关闭 WS
		_ = clientConn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "guacd 握手失败"),
			time.Now().Add(2*time.Second))
		return
	}

	done := make(chan struct{}, 2)
	abort := func() {
		guacdConn.Close()
		clientConn.Close()
	}

	// 两个转发 goroutine 都会写 clientConn，gorilla/websocket 不支持并发写
	// （ping 回显与 guacd 数据转发同时发生时 panic），必须串行化。
	var writeMu sync.Mutex
	write := func(msg []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return clientConn.WriteMessage(websocket.TextMessage, msg)
	}

	// 客户端 → guacd：内部指令（稳定性 ping）回显给客户端，其余原样转发
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			_, data, err := clientConn.ReadMessage()
			if err != nil {
				abort()
				return
			}
			if bytes.HasPrefix(data, internalOpcodePrefix) {
				_ = write(data)
				continue
			}
			for _, raw := range splitInstructions(data) {
				if _, err := stream.Write(raw); err != nil {
					abort()
					return
				}
			}
		}
	}()

	// guacd → 客户端：原样转发
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			raw, err := stream.ReadSome()
			if err != nil {
				abort()
				return
			}
			if err := write(raw); err != nil {
				abort()
				return
			}
		}
	}()

	// keepalive：guacd 1.5.x 对用户输入有 15 秒超时（GUACAMOLE-2233），
	// Chrome/Edge 降频 keepalive 后会触发超时导致 "User is not responding"。
	// 后端定期发送 nop 指令重置该计时器。
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		nop := []byte("3.nop;")
		for {
			select {
			case <-ticker.C:
				if _, err := stream.Write(nop); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	<-done
}

// guacConfigFromVM 依据 VM metadata（guac.* 键）构造 guacd 握手配置。
// 优先级：metadata 显式值 > 虚拟机固有属性（IP）> 协议默认值。
// 浏览器侧不参与握手，因此 hostname/port/password 等敏感参数无法被覆盖。
func guacConfigFromVM(vm *domain.VM) guac.Config {
	protocol := metadataString(vm, "guac.protocol")
	if protocol == "" {
		protocol = "vnc"
	}
	host := metadataString(vm, "guac.hostname")
	if host == "" && vm != nil && vm.IPAddress != "" {
		host = vm.IPAddress
	}
	port := metadataString(vm, "guac.port")
	if port == "" {
		switch protocol {
		case "vnc":
			port = "5900"
		case "rdp":
			port = "3389"
		case "ssh", "telnet":
			port = "22"
		}
	}
	width := metadataInt(vm, "guac.width", defaultVNCWidth)
	height := metadataInt(vm, "guac.height", defaultVNCHeight)

	return guac.Config{
		Protocol: protocol,
		Parameters: map[string]string{
			"hostname":               host,
			"port":                   port,
			"username":               metadataString(vm, "guac.username"),
			"password":               metadataString(vm, "guac.password"),
			"domain":                 metadataString(vm, "guac.domain"),
			"security":               metadataString(vm, "guac.security"),
			"ignore-cert":            metadataString(vm, "guac.ignore-cert"),
			"read-only":              metadataString(vm, "guac.read-only"),
			"autoretry":              metadataString(vm, "guac.autoretry"),
			"color-depth":            metadataString(vm, "guac.color-depth"),
			"disable-bitmap-caching": metadataString(vm, "guac.disable-bitmap-caching"),
			"enable-wallpaper":       metadataString(vm, "guac.enable-wallpaper"),
			"enable-theming":         metadataString(vm, "guac.enable-theming"),
			"enable-font-smoothing":  metadataString(vm, "guac.enable-font-smoothing"),
			"create-recording-path":  metadataString(vm, "guac.create-recording-path"),
		},
		OptimalScreenWidth:  width,
		OptimalScreenHeight: height,
		OptimalResolution:   96,
		AudioMimetypes:      []string{},
		VideoMimetypes:      []string{},
		ImageMimetypes:      []string{},
	}
}

// metadataString 读取 VM metadata 中字符串值，不存在或空返回 ""。
func metadataString(vm *domain.VM, key string) string {
	if vm == nil || vm.Metadata == nil {
		return ""
	}
	v, ok := vm.Metadata[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// metadataInt 读取 VM metadata 中整数值，缺失或非法时返回默认值。
func metadataInt(vm *domain.VM, key string, fallback int) int {
	if vm == nil || vm.Metadata == nil {
		return fallback
	}
	v, ok := vm.Metadata[key]
	if !ok || v == nil {
		return fallback
	}
	if n, err := strconv.Atoi(fmt.Sprint(v)); err == nil && n > 0 {
		return n
	}
	return fallback
}

// splitInstructions 按 Guacamole 指令语法（<len>.<内容>[,|;]）拆分一段缓冲，
// 兼容客户端把多条指令合并到一次发送的情况。解析失败时原样返回整段。
func splitInstructions(buf []byte) [][]byte {
	out := make([][]byte, 0, 2)
	pos := 0
	last := 0
	for pos < len(buf) {
		start := pos
		for pos < len(buf) && buf[pos] >= '0' && buf[pos] <= '9' {
			pos++
		}
		if pos >= len(buf) || buf[pos] != '.' || pos == start {
			break // 非规范指令数据
		}
		length := 0
		for _, ch := range buf[start:pos] {
			length = length*10 + int(ch-'0')
		}
		pos++
		if pos+length > len(buf) {
			break // 元素被截断
		}
		pos += length
		term := buf[pos]
		pos++
		if term == ';' {
			out = append(out, buf[last:pos])
			last = pos
		} else if term != ',' {
			break
		}
	}
	if len(out) == 0 {
		// 无法按指令解析：整段原样返回，由上游决定如何处理
		return [][]byte{buf}
	}
	if last < len(buf) {
		out = append(out, buf[last:])
	}
	return out
}
