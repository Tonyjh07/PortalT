// Package pluginhost 提供插件运行时宿主能力。
//
// 本包实现 native 插件（独立进程）的完整生命周期管理：
//   - 扫描 PLUGINS_DIR 按 manifest.json 校验并 upsert 插件表记录
//   - spawn / kill / restart 插件进程（gRPC 控制面 + HTTP 数据面）
//   - 健康探测与崩溃自动重启（退避 + 重启次数上限）
//   - fsnotify 热加载（新增 / 替换 / 删除）
//   - 生命周期钩子（Notify: enabled / disabled / config_changed / restarting）
//
// 进程通信协议见 backend/proto/plugin/v1/plugin.proto。端口分配语义：
// PortalT 分配 gRPC 与 HTTP 两个回环端口，经环境变量
// PORTALT_PLUGIN_GRPC_PORT / PORTALT_PLUGIN_HTTP_PORT 下发，
// 插件绑定后经 Handshake 确认（HandshakeRequest.HttpPort 即 PortalT 分配的端口）。
package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"portalt/internal/domain"
	"portalt/internal/pluginpkg"
	"portalt/internal/ports"

	pluginv1 "portalt/proto/plugin/v1"
)

// 插件运行态（写入 plugins.status）。
const (
	// StatusRunning 进程运行中且握手成功
	StatusRunning = "running"
	// StatusStopped 已停止（停用 / 未启用）
	StatusStopped = "stopped"
	// StatusError 健康探测失败或崩溃
	StatusError = "error"
	// StatusMissing 插件目录被删除
	StatusMissing = "missing"
)

// 进程监督参数。
const (
	// spawnRetries 等待插件 gRPC 就绪的最大重试次数
	spawnRetries = 40
	// spawnRetryInterval 重试间隔
	spawnRetryInterval = 250 * time.Millisecond
	// rpcTimeout 控制面 RPC（握手/健康/停机/通知）单次超时
	rpcTimeout = 10 * time.Second
	// maxRestarts 连续崩溃重启上限，超过后进入 error 不再自动重启
	maxRestarts = 5
	// backoffBase 崩溃重启退避基准（指数增长，封顶 30s）
	backoffBase = 2 * time.Second
	// backoffMax 退避封顶
	backoffMax = 30 * time.Second
	// probeTimeout 插件 HTTP 数据面探测超时
	probeTimeout = 5 * time.Second
)

// 已定义错误哨兵。
var (
	// ErrNotInstalled 插件目录不存在或缺少可执行文件
	ErrNotInstalled = errors.New("插件未安装（目录或可执行文件缺失）")
	// ErrNotNative 目标记录不是 native 类型
	ErrNotNative = errors.New("插件不是 native 类型")
)

// Manager native 插件进程管理器。
type Manager struct {
	dir     string // PLUGINS_DIR
	repo    ports.PluginRepository
	version string // PortalT 版本号，握手时下发给插件

	mu    sync.Mutex
	procs map[string]*Proc

	logf func(format string, args ...any)
}

// NewManager 创建管理器。dir 为空时创建禁用态管理器（全部操作幂等成功，
// 供本地 dev 无 PLUGINS_DIR 时使用）。
func NewManager(dir string, repo ports.PluginRepository, version string) *Manager {
	return &Manager{
		dir:     strings.TrimSpace(dir),
		repo:    repo,
		version: version,
		procs:   make(map[string]*Proc),
		logf:    log.Printf,
	}
}

// SetLogger 覆盖默认日志输出（测试用）。
func (m *Manager) SetLogger(f func(format string, args ...any)) {
	if f != nil {
		m.logf = f
	}
}

// Disabled 判断管理器是否处于禁用态（未配置 PLUGINS_DIR）。
func (m *Manager) Disabled() bool {
	return m.dir == ""
}

// Enabled 判断插件是否已安装（目录 + 可执行文件就绪）。
func (m *Manager) Enabled() bool { return !m.Disabled() }

// Proc 单个插件进程的监督状态。
type Proc struct {
	id       string
	dir      string // 插件目录绝对路径
	bin      string // 可执行文件绝对路径
	manifest *pluginpkg.Manifest

	grpcPort int
	httpPort int

	mu         sync.Mutex
	cmd        *exec.Cmd
	conn       *grpc.ClientConn
	client     pluginv1.PluginControlClient
	status     string
	stopping   bool
	restarting bool
	spawning   bool // 正在拉起进程（防并发 spawn 双进程）
	restarts   int
}

// ID 返回插件 ID。
func (p *Proc) ID() string { return p.id }

// binary 返回可执行文件路径（锁内读取，防与热加载更新竞态）。
func (p *Proc) binary() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.bin
}

// manifestInfo 返回插件 manifest（锁内读取，防与热加载更新竞态）。
func (p *Proc) manifestInfo() *pluginpkg.Manifest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.manifest
}

// clientRef 返回当前 gRPC 客户端（可能为 nil；调用方自行判空）。
func (p *Proc) clientRef() pluginv1.PluginControlClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

// setPorts 记录宿主分配的端口（锁内写入）。
func (p *Proc) setPorts(grpcPort, httpPort int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.grpcPort = grpcPort
	p.httpPort = httpPort
}

// setClient 记录 gRPC 连接与客户端（锁内写入）。
func (p *Proc) setClient(conn *grpc.ClientConn, client pluginv1.PluginControlClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn = conn
	p.client = client
}

// setBinManifest 更新可执行文件与 manifest（热加载升级时锁内写入）。
func (p *Proc) setBinManifest(bin string, manifest *pluginpkg.Manifest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bin = bin
	p.manifest = manifest
}

// Status 返回当前运行态。
func (p *Proc) Status() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// HTTPPort 返回插件 HTTP 数据面端口；未运行返回 0。
func (p *Proc) HTTPPort() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status != StatusRunning || p.httpPort == 0 {
		return 0
	}
	return p.httpPort
}

func (p *Proc) setStatus(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
}

func (p *Proc) setStopping(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopping = v
}

// setCmd 记录进程句柄并切换运行态。
func (p *Proc) setCmd(cmd *exec.Cmd) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cmd = cmd
	if cmd != nil {
		p.status = StatusRunning
	} else {
		p.status = StatusStopped
	}
}

// freePort 分配一个空闲回环端口（临时监听取号后关闭，返回端口号）。
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

// updateDBStatus 把运行态回写 plugins.status（native 记录）。
func (m *Manager) updateDBStatus(id, status string) {
	p, err := m.repo.FindByID(id)
	if err != nil || p == nil {
		return
	}
	if domain.NormalizePluginType(p.Type) != domain.PluginTypeNative {
		return
	}
	if p.Status == status {
		return
	}
	p.Status = status
	if err := m.repo.Save(p); err != nil {
		m.logf("插件 %s 状态回写失败: %v", id, err)
	}
}

// Start 初始化并启动监督：首次全量加载 + 启动 fsnotify 热加载。
// ctx 取消时停止全部插件进程并退出。禁用态管理器直接返回。
func (m *Manager) Start(ctx context.Context) error {
	if m.Disabled() {
		return nil
	}
	if err := m.Load(ctx); err != nil {
		return err
	}
	go m.watch(ctx)
	return nil
}

// Shutdown 优雅停止全部插件进程（逐个调用 Shutdown RPC 后 kill）。
func (m *Manager) Shutdown(ctx context.Context) {
	if m.Disabled() {
		return
	}
	m.mu.Lock()
	ids := make([]string, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.stopProc(ctx, id, "shutdown")
	}
}

// Load 全量扫描 PLUGINS_DIR：校验 manifest + 可执行文件，upsert 插件表记录；
// 已启用且安装的插件自动 spawn。启用状态由管理员在界面控制（新插件默认禁用）。
func (m *Manager) Load(ctx context.Context) error {
	if m.Disabled() {
		return nil
	}
	installed := map[string]bool{}
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			m.logf("插件目录不存在，跳过: %s", m.dir)
			return nil
		}
		return fmt.Errorf("读取插件目录 %s: %w", m.dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		bin, manifest, err := m.inspect(m.dir, id)
		if err != nil {
			m.logf("插件 %s 检查失败: %v", id, err)
			continue
		}
		installed[id] = true
		if err := m.upsert(manifest, bin); err != nil {
			m.logf("插件 %s 记录同步失败: %v", id, err)
			continue
		}
		if err := m.ensureProc(ctx, id); err != nil {
			m.logf("插件 %s 进程初始化失败: %v", id, err)
		}
	}
	// 数据库中存在但目录被删除的 native 插件 → 标记 missing
	all, err := m.repo.FindAll()
	if err != nil {
		return err
	}
	for _, p := range all {
		if domain.NormalizePluginType(p.Type) != domain.PluginTypeNative {
			continue
		}
		if installed[p.ID] {
			continue
		}
		m.markMissing(ctx, p.ID)
	}
	return nil
}

// inspect 校验插件目录：manifest.json 合法且 ID 与目录名一致，可执行文件存在。
// 返回可执行文件路径与解析后的 manifest。二进制约定名：plugin（Windows: plugin.exe）。
func (m *Manager) inspect(pluginsDir, id string) (string, *pluginpkg.Manifest, error) {
	dir := filepath.Join(pluginsDir, id)
	manifest, err := pluginpkg.Load(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return "", nil, err
	}
	if manifest.ID != id {
		return "", nil, fmt.Errorf("manifest.id %q 与目录名 %q 不一致", manifest.ID, id)
	}
	bin := filepath.Join(dir, "plugin")
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		bin = filepath.Join(dir, "plugin.exe")
		if _, err2 := os.Stat(bin); os.IsNotExist(err2) {
			return "", nil, fmt.Errorf("%w: 未找到 plugin/plugin.exe", ErrNotInstalled)
		}
	}
	return bin, manifest, nil
}

// upsert 按 manifest 同步插件表记录（native 类型）。
// 新记录默认 is_active=false（管理员手动启用）；已存在记录保留 is_active 与 permission
// （管理员配置优先），其余 manifest 字段更新。
func (m *Manager) upsert(manifest *pluginpkg.Manifest, bin string) error {
	manifestJSON, err := manifestJSON(manifest)
	if err != nil {
		return err
	}
	existing, err := m.repo.FindByID(manifest.ID)
	if errors.Is(err, ports.ErrNotFound) {
		return m.repo.Save(&domain.Plugin{
			ID:           manifest.ID,
			Name:         manifest.Name,
			Icon:         manifest.Icon,
			Route:        manifest.Route,
			Type:         domain.PluginTypeNative,
			Permission:   manifest.Permission,
			SortOrder:    manifest.SortOrder,
			IsActive:     false,
			Status:       StatusStopped,
			ManifestJSON: manifestJSON,
		})
	}
	if err != nil {
		return err
	}
	if domain.NormalizePluginType(existing.Type) != domain.PluginTypeNative {
		return fmt.Errorf("%w: ID %q 已存在且类型为 %s", ErrNotNative, manifest.ID, existing.Type)
	}
	existing.Name = manifest.Name
	existing.Icon = manifest.Icon
	existing.Route = manifest.Route
	existing.SortOrder = manifest.SortOrder
	existing.ManifestJSON = manifestJSON
	return m.repo.Save(existing)
}

// ensureProc 确保插件进程对象存在；已启用且安装的插件自动 spawn。
func (m *Manager) ensureProc(ctx context.Context, id string) error {
	p, err := m.repo.FindByID(id)
	if err != nil {
		return err
	}
	if domain.NormalizePluginType(p.Type) != domain.PluginTypeNative {
		return nil
	}
	bin, manifest, err := m.inspect(m.dir, id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	proc, ok := m.procs[id]
	if !ok {
		proc = &Proc{
			id:       id,
			dir:      filepath.Join(m.dir, id),
			bin:      bin,
			manifest: manifest,
			status:   StatusStopped,
		}
		m.procs[id] = proc
	} else {
		proc.setBinManifest(bin, manifest)
	}
	m.mu.Unlock()
	if !p.IsActive {
		m.updateDBStatus(id, StatusStopped)
		return nil
	}
	if proc.Status() == StatusRunning {
		return nil
	}
	return m.spawn(ctx, proc)
}

// spawn 启动插件进程：分配端口 → 拉起进程 → 等待 gRPC 就绪 → 握手 → 探测 HTTP 数据面。
// spawning 锁防并发 spawn 双进程；stopping 时拒绝拉起。
func (m *Manager) spawn(ctx context.Context, proc *Proc) error {
	proc.mu.Lock()
	if proc.status == StatusRunning || proc.stopping || proc.spawning {
		proc.mu.Unlock()
		return nil
	}
	proc.spawning = true
	proc.mu.Unlock()
	defer func() {
		proc.mu.Lock()
		proc.spawning = false
		proc.mu.Unlock()
	}()

	if _, err := os.Stat(proc.binary()); err != nil {
		m.updateDBStatus(proc.id, StatusMissing)
		return ErrNotInstalled
	}
	grpcPort, err := freePort()
	if err != nil {
		return fmt.Errorf("分配 gRPC 端口失败: %w", err)
	}
	httpPort, err := freePort()
	if err != nil {
		return fmt.Errorf("分配 HTTP 端口失败: %w", err)
	}
	proc.setPorts(grpcPort, httpPort)

	bin := proc.binary()
	cmd := exec.Command(bin)
	cmd.Dir = proc.dir
	cmd.Env = append(os.Environ(),
		"PORTALT_PLUGIN_ID="+proc.id,
		fmt.Sprintf("PORTALT_PLUGIN_GRPC_PORT=%d", grpcPort),
		fmt.Sprintf("PORTALT_PLUGIN_HTTP_PORT=%d", httpPort),
	)
	// 插件 stdout/stderr 接入 PortalT 日志（含前缀便于区分）
	cmd.Stdout = logWriter{m.logf, fmt.Sprintf("[plugin:%s]", proc.id)}
	cmd.Stderr = logWriter{m.logf, fmt.Sprintf("[plugin:%s]", proc.id)}

	if err := cmd.Start(); err != nil {
		proc.setStatus(StatusError)
		m.updateDBStatus(proc.id, StatusError)
		return fmt.Errorf("启动插件进程失败: %w", err)
	}
	proc.setCmd(cmd)

	// 等待 gRPC 控制面就绪
	conn, err := m.dialWithRetry(ctx, grpcPort)
	if err != nil {
		m.logf("插件 %s gRPC 就绪超时: %v", proc.id, err)
		proc.setStatus(StatusError)
		m.updateDBStatus(proc.id, StatusError)
		proc.kill()
		return err
	}
	proc.setClient(conn, pluginv1.NewPluginControlClient(conn))

	// 握手：下发 manifest 与 HTTP 端口，换取启用状态
	hsCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	req := &pluginv1.HandshakeRequest{
		Manifest: proc.manifestInfo().ToProto(),
		HttpPort: int32(httpPort),
	}
	resp, err := proc.clientRef().Handshake(hsCtx, req)
	if err != nil {
		m.logf("插件 %s 握手失败: %v", proc.id, err)
		proc.setStatus(StatusError)
		m.updateDBStatus(proc.id, StatusError)
		proc.kill()
		return err
	}
	if !resp.Enabled {
		// 握手返回 disabled：记录已停用，停止进程（预期路径：启用态与 DB 一致）
		m.logf("插件 %s 握手返回未启用: %s", proc.id, resp.Reason)
		proc.kill()
		m.updateDBStatus(proc.id, StatusStopped)
		return nil
	}

	// 探测 HTTP 数据面回环可用性（防插件上报不可达端口）
	if err := probeHTTP(ctx, httpPort); err != nil {
		m.logf("插件 %s HTTP 数据面探测失败: %v", proc.id, err)
		proc.setStatus(StatusError)
		m.updateDBStatus(proc.id, StatusError)
		proc.kill()
		return err
	}

	proc.setStatus(StatusRunning)
	proc.restarts = 0
	m.updateDBStatus(proc.id, StatusRunning)
	m.logf("插件 %s 已启动 (gRPC:%d HTTP:%d)", proc.id, grpcPort, httpPort)

	// 进程退出监控 + 健康探测
	go m.watchExit(proc)
	go m.healthLoop(proc)
	return nil
}

// dialWithRetry 等待插件 gRPC 服务就绪并建立连接。
// 用非阻塞 NewClient + 轮询 connectivity 状态，规避 grpc.NewClient 的懒连接
// 语义（WithBlock 已弃用且不生效），确保返回时连接已 Ready。
func (m *Manager) dialWithRetry(ctx context.Context, port int) (*grpc.ClientConn, error) {
	target := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	for i := 0; i < spawnRetries; i++ {
		state := conn.GetState()
		if state == connectivity.Ready {
			return conn, nil
		}
		conn.Connect()
		waitCtx, cancel := context.WithTimeout(ctx, spawnRetryInterval)
		conn.WaitForStateChange(waitCtx, state)
		cancel()
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, ctx.Err()
		default:
		}
	}
	_ = conn.Close()
	return nil, fmt.Errorf("gRPC 服务 %s 未在 %s 内就绪", target, time.Duration(spawnRetries)*spawnRetryInterval)
}

// probeHTTP 探测插件 HTTP 数据面（任意非连接错误响应即视为可达）。
func probeHTTP(ctx context.Context, port int) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	client := &http.Client{Timeout: probeTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/healthz", port), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// watchExit 监控进程退出：非预期退出（非停止流程）标记 error 并按退避重启。
// 与 healthLoop 共用 handleCrash，保证同一时刻只有一个退避重启流程在跑。
func (m *Manager) watchExit(proc *Proc) {
	// 锁内取 cmd 副本，避免与 kill()（置 nil）竞态导致 nil 解引用
	proc.mu.Lock()
	cmd := proc.cmd
	proc.mu.Unlock()
	if cmd == nil {
		return
	}
	err := cmd.Wait()
	m.handleCrash(proc, fmt.Sprintf("进程退出 %v", err))
}

// healthLoop 周期健康探测：失败标记 error 并按退避重启。
// 间隔取 manifest.health_interval_seconds（缺省默认值）。
func (m *Manager) healthLoop(proc *Proc) {
	interval := time.Duration(proc.manifestInfo().HealthInterval()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if proc.Status() != StatusRunning {
			return
		}
		// 锁内取 client 副本，避免与 kill() 竞态
		proc.mu.Lock()
		client := proc.client
		proc.mu.Unlock()
		if client == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		resp, err := client.Health(ctx, &pluginv1.HealthRequest{})
		cancel()
		if err != nil || !resp.Healthy {
			m.logf("插件 %s 健康探测失败: %v %s", proc.id, err, resp.GetDetail())
			m.handleCrash(proc, fmt.Sprintf("健康探测失败 %v", err))
			return
		}
	}
}

// handleCrash 崩溃统一处理：标记 error → 停止进程 → 退避重启。
// restarting 锁保证崩溃路径（watchExit/healthLoop 并发触发）只执行一次重启。
func (m *Manager) handleCrash(proc *Proc, cause string) {
	proc.mu.Lock()
	if proc.stopping || proc.restarting {
		proc.mu.Unlock()
		return
	}
	proc.restarting = true
	proc.mu.Unlock()

	proc.setStatus(StatusError)
	m.updateDBStatus(proc.id, StatusError)
	m.logf("插件 %s %s", proc.id, cause)
	proc.kill()
	m.restartWithBackoff(proc)

	proc.mu.Lock()
	proc.restarting = false
	proc.mu.Unlock()
}

// restartWithBackoff 崩溃/探测失败后的退避重启；连续失败超限进入 error 不再自动重启。
// 仅在插件仍启用时 spawn；期间若被 stopProc 标记 stopping 则放弃。
func (m *Manager) restartWithBackoff(proc *Proc) {
	proc.mu.Lock()
	proc.restarts++
	attempt := proc.restarts
	proc.mu.Unlock()
	if attempt > maxRestarts {
		m.logf("插件 %s 连续失败 %d 次，停止自动重启（请人工处理）", proc.id, attempt)
		return
	}
	delay := backoffBase << (attempt - 1)
	if delay > backoffMax {
		delay = backoffMax
	}
	m.logf("插件 %s 将在 %s 后自动重启（第 %d/%d 次）", proc.id, delay, attempt, maxRestarts)
	time.Sleep(delay)

	proc.mu.Lock()
	stopping := proc.stopping
	proc.mu.Unlock()
	if stopping {
		return
	}
	p, err := m.repo.FindByID(proc.id)
	if err != nil || p == nil || !p.IsActive {
		proc.setStatus(StatusStopped)
		m.updateDBStatus(proc.id, StatusStopped)
		return
	}
	if err := m.spawn(ctxBackground(), proc); err != nil {
		m.logf("插件 %s 重启失败: %v", proc.id, err)
	}
}

// ctxBackground 返回可被服务关闭打断的后台上下文（当前用 Background，
// 未来接入主 ctx 后改为传递）。
func ctxBackground() context.Context { return context.Background() }

// stopProc 停止插件进程：先调用 Shutdown RPC（优雅停机），随后 kill 兜底。
// 无论当前是否在运行都置 stopping=true，阻止退避重启线程复活该进程
// （否则停机/停用瞬间的待重启退避会再拉起进程，形成孤儿进程）。
func (m *Manager) stopProc(ctx context.Context, id, reason string) {
	m.mu.Lock()
	proc, ok := m.procs[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	proc.setStopping(true)
	if proc.Status() != StatusRunning {
		proc.setStatus(StatusStopped)
		m.updateDBStatus(id, StatusStopped)
		return
	}
	if client := proc.clientRef(); client != nil {
		rpcCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		_, _ = client.Shutdown(rpcCtx, &pluginv1.ShutdownRequest{Reason: reason})
		cancel()
	}
	proc.kill()
	proc.setStatus(StatusStopped)
	m.updateDBStatus(id, StatusStopped)
	m.logf("插件 %s 已停止 (%s)", id, reason)
}

// kill 强制终止进程并关闭 gRPC 连接。
func (p *Proc) kill() {
	p.mu.Lock()
	cmd := p.cmd
	conn := p.conn
	p.cmd = nil
	p.conn = nil
	p.client = nil
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

// markMissing 目录被删除 → 停止进程，DB 记录标记 missing（保留管理员配置）。
func (m *Manager) markMissing(ctx context.Context, id string) {
	m.stopProc(ctx, id, "missing")
	m.mu.Lock()
	delete(m.procs, id)
	m.mu.Unlock()
	m.updateDBStatus(id, StatusMissing)
}

// Enable 启用插件：更新 is_active=true，spawn 进程，发送 enabled 通知。
func (m *Manager) Enable(ctx context.Context, id string) error {
	if m.Disabled() {
		return fmt.Errorf("%w（未配置 PLUGINS_DIR）", ErrNotInstalled)
	}
	p, err := m.repo.FindByID(id)
	if err != nil {
		return err
	}
	if domain.NormalizePluginType(p.Type) != domain.PluginTypeNative {
		return ErrNotNative
	}
	proc, err := m.procFor(id)
	if err != nil {
		return err
	}
	// 记录本次是否由我们翻转 is_active，spawn 失败时回滚（防 API 重试被跳过）
	flipped := false
	if !p.IsActive {
		p.IsActive = true
		flipped = true
		if err := m.repo.Save(p); err != nil {
			return err
		}
	}
	// 清除上次 stopProc 遗留的 stopping，允许重新拉起
	proc.setStopping(false)
	if err := m.spawn(ctx, proc); err != nil {
		if flipped {
			// 回滚启用态：进程未起来却标了启用，管理界面无法再触发生命周期
			p.IsActive = false
			_ = m.repo.Save(p)
			m.updateDBStatus(id, StatusError)
		}
		return err
	}
	m.notify(ctx, proc, pluginv1.NotifyEvent_ENABLED)
	return nil
}

// Disable 停用插件：更新 is_active=false，停止进程，发送 disabled 通知。
func (m *Manager) Disable(ctx context.Context, id string) error {
	if m.Disabled() {
		return nil
	}
	p, err := m.repo.FindByID(id)
	if err != nil {
		return err
	}
	if domain.NormalizePluginType(p.Type) != domain.PluginTypeNative {
		return ErrNotNative
	}
	if p.IsActive {
		p.IsActive = false
		if err := m.repo.Save(p); err != nil {
			return err
		}
	}
	m.mu.Lock()
	proc, ok := m.procs[id]
	m.mu.Unlock()
	if ok && proc.Status() == StatusRunning {
		m.notify(ctx, proc, pluginv1.NotifyEvent_DISABLED)
		m.stopProc(ctx, id, "disable")
	}
	m.updateDBStatus(id, StatusStopped)
	return nil
}

// Restart 重启插件：发送 restarting 通知 → 停止 → 重新 spawn。
// 仅对启用且安装的插件生效；目录缺失返回 ErrNotInstalled。
func (m *Manager) Restart(ctx context.Context, id string) error {
	if m.Disabled() {
		return fmt.Errorf("%w（未配置 PLUGINS_DIR）", ErrNotInstalled)
	}
	p, err := m.repo.FindByID(id)
	if err != nil {
		return err
	}
	if domain.NormalizePluginType(p.Type) != domain.PluginTypeNative {
		return ErrNotNative
	}
	proc, err := m.procFor(id)
	if err != nil {
		return err
	}
	m.notify(ctx, proc, pluginv1.NotifyEvent_RESTARTING)
	m.stopProc(ctx, id, "restart")
	if !p.IsActive {
		m.updateDBStatus(id, StatusStopped)
		return nil
	}
	// 清除 stopProc 遗留的 stopping，允许重新拉起
	proc.setStopping(false)
	return m.spawn(ctx, proc)
}

// notify 调用插件 Notify RPC（失败仅告警，不影响主流程）。
func (m *Manager) notify(ctx context.Context, proc *Proc, event pluginv1.NotifyEvent) {
	if proc.Status() != StatusRunning {
		return
	}
	client := proc.clientRef()
	if client == nil {
		return
	}
	rpcCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	if _, err := client.Notify(rpcCtx, &pluginv1.NotifyRequest{Event: event}); err != nil {
		m.logf("插件 %s 通知 %s 失败: %v", proc.id, event, err)
	}
}

// Status 返回插件运行态；目录缺失返回 missing，未运行返回 stopped。
func (m *Manager) Status(id string) string {
	if m.Disabled() {
		return ""
	}
	p, err := m.repo.FindByID(id)
	if err != nil {
		return ""
	}
	if domain.NormalizePluginType(p.Type) != domain.PluginTypeNative {
		return ""
	}
	m.mu.Lock()
	proc, ok := m.procs[id]
	m.mu.Unlock()
	if !ok {
		return p.Status
	}
	return proc.Status()
}

// HTTPAddress 返回插件 HTTP 数据面地址（回环）；未运行返回空串。
// 代理层据此反代，避免直连任意主机（防 SSRF）。
func (m *Manager) HTTPAddress(id string) string {
	if m.Disabled() {
		return ""
	}
	m.mu.Lock()
	proc, ok := m.procs[id]
	m.mu.Unlock()
	if !ok {
		return ""
	}
	port := proc.HTTPPort()
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func (m *Manager) procFor(id string) (*Proc, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if proc, ok := m.procs[id]; ok {
		return proc, nil
	}
	return nil, ErrNotInstalled
}

// logWriter 把子进程输出逐行转发到宿主日志（带插件前缀）。
type logWriter struct {
	logf func(format string, args ...any)
	pref string
}

func (w logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(p)), "\n") {
		if line != "" {
			w.logf("%s %s", w.pref, line)
		}
	}
	return len(p), nil
}

// 编译期断言：Manager 实现 ports.NativeHost。
var _ ports.NativeHost = (*Manager)(nil)
