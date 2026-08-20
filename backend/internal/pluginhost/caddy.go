// Package pluginhost 提供插件运行时宿主能力。
//
// 本期（Phase B）实现 access 插件的 Caddy 规则管理：规则落盘 plugins.d/ 并触发 reload；
// native 插件的进程监督 / 热加载 / 生命周期在后续阶段于此包扩展。
package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"portalt/internal/domain"
)

// ErrReloadFailed reload 命令执行失败（规则已落盘，但 Caddy 未热生效）。
// 调用方可据此区分"落盘失败"与"reload 失败"，向前端返回提示而非硬错误。
var ErrReloadFailed = errors.New("Caddy reload 失败")

// ReloadTimeout reload 命令超时时间。
const ReloadTimeout = 15 * time.Second

// DefaultESXIAdminCaddyRulesV1 esxi-admin 插件无鉴权版的默认规则（历史值），
// 仅用于 seed 迁移：识别旧默认规则并升级为带鉴权闸口的新默认值
// （DefaultESXIAdminCaddyRules），精确匹配才覆盖，保留管理员自定义。
const DefaultESXIAdminCaddyRulesV1 = `handle /esxi/* {
	uri strip_prefix /esxi
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -X-Frame-Options
		header_down -Content-Security-Policy
	}
}
handle /ui/* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -X-Frame-Options
		header_down -Content-Security-Policy
	}
}
handle /screen* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /sdk* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /sts* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /ticket* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /vfeed/* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /converter/* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /eam/* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /pbm/* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /sms/* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /vsan/* {
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}`

// DefaultESXIAdminCaddyRulesV2 esxi-admin 插件缺 /ha-nfc 时的默认规则（历史值，带鉴权闸口）：
// ESXi Host Client 反代（/esxi/* 剥前缀 + 绝对路径资源 + /ticket* WebSocket；
// 缺 /ha-nfc NFC 端点，虚拟机导出/下载不可用），
// 仅用于 seed 迁移：识别旧默认带鉴权规则并升级为含 /ha-nfc 的新默认
// （DefaultESXIAdminCaddyRules），归一化精确匹配才覆盖，保留管理员自定义。
const DefaultESXIAdminCaddyRulesV2 = `handle /esxi/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	uri strip_prefix /esxi
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -X-Frame-Options
		header_down -Content-Security-Policy
	}
}
handle /ui/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -X-Frame-Options
		header_down -Content-Security-Policy
	}
}
handle /screen* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /sdk* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /sts* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /ticket* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /vfeed/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /converter/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /eam/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /pbm/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /sms/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}
handle /vsan/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}`

// DefaultESXIAdminCaddyRules esxi-admin 插件的默认 Caddy 规则（含 /ha-nfc）：
// ESXi Host Client 反代（/esxi/* 剥前缀 + /ui|/sdk 等绝对路径资源 + /ticket* WebSocket），
// 作为该插件 CaddyRules 的默认值，管理员可在插件管理界面修改后重载。
// 每个 handle 先经 forward_auth 回调门户鉴权闸口（/api/v1/auth/gate?perm=esxi-admin:use，
// 校验请求 cookie 中的 access/refresh 令牌与 esxi-admin:use 权限），
// 未登录返回 401、无权限返回 403，放行后才反代到 ESXi。
// /ha-nfc/* 是 ESXi NFC（Network File Copy）端点：Host Client 虚拟机导出/拉取文件时
// 对其发 HEAD 读取各文件 Content-Length（size）后再 GET 下载；该路径缺代理时会被门户
// SPA 兜底命中（text/html），导出即报 "Required property not defined: size"。
// 目标主机由 {env.ESXI_UPSTREAM} 在运行时解析（Caddy 进程环境变量）。
const DefaultESXIAdminCaddyRules = DefaultESXIAdminCaddyRulesV2 + `
handle /ha-nfc/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -Content-Security-Policy
	}
}`

// CaddyManager 管理插件 Caddy 规则：落盘 rulesDir/<id>.caddy 并触发 reload。
// reloadCmd 为空表示不执行 reload（本地 dev 无 Caddy），仅落盘不报错。
type CaddyManager struct {
	rulesDir  string
	reloadCmd string
}

// NewCaddyManager 创建 Caddy 规则管理器。
func NewCaddyManager(rulesDir, reloadCmd string) *CaddyManager {
	return &CaddyManager{rulesDir: rulesDir, reloadCmd: reloadCmd}
}

// Apply 写入（或更新）某插件的规则文件；rules 为空时等同 Remove。
// 落盘前若环境中存在 caddy 可执行文件且规则不含 {env.*} 占位符，则先 caddy validate
// （包装为最小站点校验片段语法），校验失败不落盘——避免语法错误的规则文件残留导致
// 后续任何插件规则的 reload 持续失败。仅落盘，不触发 reload。
func (m *CaddyManager) Apply(id, rules string) error {
	if m == nil {
		return nil
	}
	if rules == "" {
		return m.Remove(id)
	}
	if err := m.validateCaddy(rules); err != nil {
		return err
	}
	return m.writeFile(id, rules)
}

// validateCaddy 尝试用 caddy 命令校验规则片段（包装为 :0 最小站点）。
// 跳过场景：未配置规则目录（本地 dev 无 Caddy）、caddy 可执行文件不在 PATH、
// 片段含 {env.*} 占位符（其值依赖运行时环境，校验会产生误报，交由 Caddy 加载期处理）。
// 校验失败返回错误（调用方不落盘）。
func (m *CaddyManager) validateCaddy(rules string) error {
	if m == nil || m.rulesDir == "" {
		return nil
	}
	if _, err := exec.LookPath("caddy"); err != nil {
		return nil
	}
	if strings.Contains(rules, "{env.") {
		return nil
	}
	tmp, err := os.CreateTemp("", "portalt-caddy-*.caddy")
	if err != nil {
		return nil
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(":0 {\n" + rules + "\n}"); err != nil {
		_ = tmp.Close()
		return nil
	}
	if err := tmp.Close(); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), ReloadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "caddy", "validate", "--config", tmp.Name(), "--adapter", "caddyfile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Caddy 规则校验失败: %w（%s）", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Remove 删除某插件的规则文件；文件不存在时静默返回。仅落盘，不触发 reload。
func (m *CaddyManager) Remove(id string) error {
	if m == nil {
		return nil
	}
	path := m.filePath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

// SyncAll 以插件列表为准全量对齐规则文件（启动引导与手动重载均走此路径）：
// 启用且含 CaddyRules 的 access 插件写入规则文件，其余（含磁盘上无对应插件的
// 孤儿文件）移除，最后仅 reload 一次。幂等。
func (m *CaddyManager) SyncAll(plugins []*domain.Plugin) error {
	if m == nil {
		return nil
	}
	if m.rulesDir == "" {
		return nil
	}
	// 期望存在的文件集合（<id>.caddy）
	want := make(map[string]bool)
	changed := false
	failed := make([]string, 0)
	for _, p := range plugins {
		if p == nil {
			continue
		}
		active := domain.NormalizePluginType(p.Type) == domain.PluginTypeAccess &&
			p.IsActive && p.CaddyRules != ""
		if !active {
			// 不期望其规则文件（清理循环会移除磁盘上的旧文件）
			continue
		}
		// 落盘前先校验（与单插件 Apply 路径一致），坏规则不落盘——
		// 否则启动时原样写入，reload 失败会让整个 Caddy 拒绝热载，
		// 所有插件规则一起失效。坏规则只跳过自身，不影响其他插件对齐。
		// 校验/落盘失败者仍加入 want：保留其原规则文件（与 Apply 一致），
		// 防止清理循环把此前正常落盘的旧文件删掉。
		want[p.ID+".caddy"] = true
		if err := m.validateCaddy(p.CaddyRules); err != nil {
			failed = append(failed, p.ID)
			continue
		}
		if err := m.writeFile(p.ID, p.CaddyRules); err != nil {
			failed = append(failed, p.ID)
			continue
		}
		changed = true
	}
	// 清理磁盘上多余文件（含孤儿文件）
	entries, err := os.ReadDir(m.rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取规则目录 %s: %w", m.rulesDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".caddy") {
			continue
		}
		if want[e.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(m.rulesDir, e.Name())); err != nil {
			return fmt.Errorf("清理插件规则 %s: %w", e.Name(), err)
		}
		changed = true
	}
	if len(failed) > 0 {
		// 有失败插件：磁盘仍是安全状态（失败者保留旧文件或本就没有）。
		// 若有成功写入/清理则仍 reload 让已生效部分热载，最后再报告失败列表。
		if changed {
			if err := m.Reload(); err != nil {
				return err
			}
		}
		return fmt.Errorf("以下插件 Caddy 规则校验/落盘失败，已跳过（保留原规则文件）: %s", strings.Join(failed, ", "))
	}
	if !changed {
		return nil
	}
	return m.Reload()
}

// Enabled 是否配置了 Caddy 规则目录（rulesDir 非空）。未配置时
// 落盘/重载均为空操作（本地 dev 无 Caddy）。
func (m *CaddyManager) Enabled() bool {
	return m != nil && m.rulesDir != ""
}

// ReloadEnabled 是否配置了 Caddy reload 命令（reloadCmd 非空）。
// 规则目录已配置但 reload 命令未配置时，规则可落盘但不会热生效，
// 调用方应明确提示而非静默声称已重载。
func (m *CaddyManager) ReloadEnabled() bool {
	return m != nil && m.reloadCmd != ""
}

// HasRuleFile 判断某插件的规则文件当前是否已落盘。未配置规则目录或文件不存在返回 false。
func (m *CaddyManager) HasRuleFile(id string) bool {
	if m == nil || m.rulesDir == "" {
		return false
	}
	_, err := os.Stat(m.filePath(id))
	return err == nil
}

// Reload 执行 reload 命令；命令未配置时静默返回（本地 dev 无 Caddy）。
func (m *CaddyManager) Reload() error {
	if m == nil || m.reloadCmd == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), ReloadTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", m.reloadCmd)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w（命令 %q）: %s", ErrReloadFailed, m.reloadCmd, strings.TrimSpace(string(out)))
	}
	return nil
}

// IsReloadFailed 判断错误是否为 reload 失败（区别于落盘失败）。
func IsReloadFailed(err error) bool {
	return errors.Is(err, ErrReloadFailed)
}

func (m *CaddyManager) writeFile(id, rules string) error {
	if m.rulesDir == "" {
		// 未配置规则目录（本地 dev 无 Caddy）→ 不落盘不报错
		return nil
	}
	if err := os.MkdirAll(m.rulesDir, 0o755); err != nil {
		return fmt.Errorf("创建规则目录 %s: %w", m.rulesDir, err)
	}
	path := m.filePath(id)
	if err := os.WriteFile(path, []byte(rules), 0o644); err != nil {
		return fmt.Errorf("写入插件规则 %s: %w", path, err)
	}
	return nil
}

func (m *CaddyManager) filePath(id string) string {
	return filepath.Join(m.rulesDir, id+".caddy")
}
