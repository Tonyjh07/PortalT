// Package workstation 实现 VMware Workstation Pro 虚拟化平台提供者。
//
// 通过 Workstation 16+ 内置的 REST API（vmrest，默认 127.0.0.1:8697）
// 管理本机虚拟机，方便开发调试。启用方式：
//
//	cd "C:\Program Files (x86)\VMware\VMware Workstation"
//	vmrest.exe -C                  # 首次设置凭证（保存到 %USERPROFILE%\vmrest.cfg）
//	vmrest                         # 启动 HTTP 服务（HTTPS 需 -c 证书 -k 私钥）
//
// 使用标准库实现，无第三方依赖；API 响应字段在不同版本间存在差异，
// 解析时对常见字段名做容错（见 strValue/intValue 的键名候选列表）。
package workstation

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"portalt/internal/domain"
)

// Config Workstation REST API 连接配置。
type Config struct {
	// URL API 地址，默认 http://127.0.0.1:8697
	URL string
	// Username vmrest 凭证用户名（vmrest.exe -C 设置）
	Username string
	// Password vmrest 凭证密码
	Password string
	// Insecure 跳过 TLS 证书校验（HTTPS 自签证书环境）
	Insecure bool
	// Timeout 单次请求超时，默认 10s
	Timeout time.Duration
}

// withDefaults 补齐默认配置。
func (c Config) withDefaults() Config {
	if c.URL == "" {
		c.URL = "http://127.0.0.1:8697"
	}
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	return c
}

// Provider VMware Workstation 虚拟化平台提供者（无状态，每次调用独立请求）。
type Provider struct {
	cfg    Config
	client *http.Client
	base   string
}

// NewProvider 创建 Workstation 提供者。
func NewProvider(cfg Config) *Provider {
	cfg = cfg.withDefaults()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Insecure {
		transport.TLSClientConfig = getInsecureTLSConfig()
	}
	base := strings.TrimSuffix(cfg.URL, "/")
	base = strings.TrimSuffix(base, "/api")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	return &Provider{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout, Transport: transport},
		base:   base,
	}
}

// ListVMs 返回 Workstation 中注册的全部虚拟机。
// vmrest 列表接口仅返回 id 与 vmx 路径，CPU/内存/状态等需逐个查询详情；
// 各虚拟机详情并行查询（含 /power、/ip 子接口），单台失败则整体失败。
func (p *Provider) ListVMs() ([]*domain.VM, error) {
	entries, err := p.listEntries()
	if err != nil {
		return nil, err
	}
	vms := make([]*domain.VM, len(entries))
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for i, e := range entries {
		wg.Add(1)
		go func(i int, e listEntry) {
			defer wg.Done()
			vm, err := p.vmDetail(e)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			vms[i] = vm
		}(i, e)
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return nil, err
	default:
	}
	return vms, nil
}

// listEntry 列表接口的最小条目（id + vmx 路径）。
type listEntry struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// listEntries 拉取全部虚拟机 id 列表。
func (p *Provider) listEntries() ([]listEntry, error) {
	var out []listEntry
	if err := p.doJSON("GET", "/api/vms", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// vmDetail 查询单台虚拟机详情并映射为领域实体。
// 真实 vmrest 的详情接口仅返回 id/cpu/memory（不包含电源状态与 IP），
// 电源状态需额外查询 /api/vms/<id>/power 子接口，IP 需查询 /api/vms/<id>/ip；
// 老版本可能内嵌在详情中，两者均兼容。
func (p *Provider) vmDetail(e listEntry) (*domain.VM, error) {
	var raw map[string]any
	if err := p.doJSON("GET", "/api/vms/"+e.ID, nil, &raw); err != nil {
		return nil, err
	}

	name := strValue(raw, "name", "display_name")
	if name == "" {
		name = baseName(e.Path)
	}

	status := mapPowerState(strValue(raw, "power_state", "state"))
	if status == domain.VMStatusUnknown {
		status = p.queryPowerState(e.ID)
	}

	ip := strValue(raw, "ip_address", "ip")
	if ip == "" {
		ip = p.queryIP(e.ID) // 详情缺失时查子接口（关机/无 Tools 时 404，容错为空）
	}

	vm := &domain.VM{
		ID:        e.ID,
		Name:      name,
		Status:    status,
		CPU:       intValue(raw, "num_cpu", "cpu", "processors"),
		MemoryMB:  intValue(raw, "memory_size_MiB", "memory_size", "memory_MiB", "size_MiB", "memory"),
		Host:      "Workstation",
		IPAddress: ip,
		Metadata:  map[string]any{},
	}
	return vm, nil
}

// queryIP 查询虚拟机 IP（子接口 /ip，返回 {"ip": "..."}）。
// 接口不可用（VM 关机/未装 Tools）时返回空串，不阻断列表。
func (p *Provider) queryIP(id string) string {
	var raw map[string]any
	if err := p.doJSON("GET", "/api/vms/"+id+"/ip", nil, &raw); err != nil {
		return ""
	}
	return strValue(raw, "ip", "ip_address")
}

// queryPowerState 查询电源子接口获取状态；接口不可用时返回 unknown（不阻断列表）。
func (p *Provider) queryPowerState(id string) domain.VMStatus {
	var pwr map[string]any
	if err := p.doJSON("GET", "/api/vms/"+id+"/power", nil, &pwr); err != nil {
		return domain.VMStatusUnknown
	}
	return mapPowerState(strValue(pwr, "power_state", "state"))
}

// StartVM 启动指定虚拟机。
func (p *Provider) StartVM(id string) error {
	return p.powerOp(id, "on")
}

// StopVM 强制关机指定虚拟机（等价 Workstation "Power Off"）。
func (p *Provider) StopVM(id string) error {
	return p.powerOp(id, "off")
}

// RestartVM 重启指定虚拟机（等价 Workstation "Reset"）。
func (p *Provider) RestartVM(id string) error {
	return p.powerOp(id, "reset")
}

// powerOp 执行电源操作。body 为 on/off/reset/suspend 之一。
func (p *Provider) powerOp(id, op string) error {
	req, err := p.newRequest("PUT", "/api/vms/"+id+"/power", strings.NewReader(op))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/vnd.vmware.vmw.rest-v1+json")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("workstation: 电源操作 %s 请求失败: %w", op, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("workstation: 电源操作 %s 失败 (HTTP %d): %s", op, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// GetHostInfo 获取宿主机信息。优先调用 /api/host，接口不可用时回退最小信息。
func (p *Provider) GetHostInfo() (*domain.HostInfo, error) {
	info := &domain.HostInfo{
		Name:   hostnameOr("Workstation"),
		Status: "connected",
	}
	var raw map[string]any
	if err := p.doJSON("GET", "/api/host", nil, &raw); err == nil {
		if v := strValue(raw, "host_name", "hostname", "name"); v != "" {
			info.Name = v
		}
		if v := strValue(raw, "version"); v != "" {
			info.Version = v
		}
		info.CPUModel = strValue(raw, "cpu_model", "cpu_desc")
		info.TotalCPU = intValue(raw, "cpu_total", "total_cpu", "cpu")
		info.UsedCPU = intValue(raw, "cpu_used", "used_cpu")
		info.TotalMemoryMB = intValue(raw, "memory_total", "total_memory", "memory_size_MiB")
		info.UsedMemoryMB = intValue(raw, "memory_used", "used_memory")
	}
	return info, nil
}

// doJSON 发起带 Basic 认证的请求并解析 JSON 响应。
// 目标非 2xx 时返回带状态码的错误；未提供凭证时提示配置。
func (p *Provider) doJSON(method, path string, body io.Reader, out any) error {
	req, err := p.newRequest(method, path, body)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("workstation: 请求 %s 失败: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("workstation: 认证失败 (HTTP 401)，请检查 VIRT_WS_USERNAME/PASSWORD 或重新运行 vmrest.exe -C")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("workstation: 请求 %s 失败 (HTTP %d)", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("workstation: 解析 %s 响应失败: %w", path, err)
	}
	return nil
}

// newRequest 构造带 Basic 认证的请求。
func (p *Provider) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, p.base+path, body)
	if err != nil {
		return nil, err
	}
	if p.cfg.Username != "" {
		req.SetBasicAuth(p.cfg.Username, p.cfg.Password)
	}
	req.Header.Set("Accept", "application/vnd.vmware.vmw.rest-v1+json")
	return req, nil
}

// mapPowerState 归一化 vmrest 电源状态（各版本取值有 "on" 与 "poweredOn" 之分）。
func mapPowerState(s string) domain.VMStatus {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "on"):
		return domain.VMStatusPoweredOn
	case strings.Contains(s, "off"):
		return domain.VMStatusPoweredOff
	case strings.Contains(s, "suspend"):
		return domain.VMStatusSuspended
	default:
		return domain.VMStatusUnknown
	}
}

// strValue 从 JSON 对象按候选键名依次取字符串值。
func strValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case nil:
			continue
		}
		// 部分版本把标量包在子对象里（如 "cpu": {"processors": 2}），
		// 对子对象再做一次取值
		if sub, ok := m[k].(map[string]any); ok {
			for _, sk := range keys {
				if s, ok := sub[sk].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// intValue 从 JSON 对象按候选键名依次取整数值（支持数字与数字字符串，含子对象）。
func intValue(m map[string]any, keys ...string) int {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n)
		case string:
			if n != "" {
				if i, err := strconv.Atoi(n); err == nil {
					return i
				}
			}
		case map[string]any:
			for _, sk := range keys {
				if s, ok := n[sk]; ok {
					switch m2 := s.(type) {
					case float64:
						return int(m2)
					case string:
						if i, err := strconv.Atoi(m2); err == nil && i > 0 {
							return i
						}
					}
				}
			}
		}
	}
	return 0
}

// baseName 从 vmx 路径提取虚拟机名（去掉扩展名）。
func baseName(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	p = strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(p, ".vmx")
}

// getInsecureTLSConfig 返回跳过证书校验的 TLS 配置（自签证书调试环境）。
func getInsecureTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 自签证书环境为显式用户配置
}

// hostnameOr 返回本机主机名，失败时返回兜底值。
func hostnameOr(fallback string) string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return fallback
}

// ensureDialOK 校验网络可达（供测试/启动探测使用）。
func (p *Provider) ensureDialOK() error {
	u, err := url.Parse(p.base)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", u.Host, p.cfg.Timeout)
	if err != nil {
		return fmt.Errorf("workstation: 无法连接 %s: %w", u.Host, err)
	}
	_ = conn.Close()
	return nil
}
