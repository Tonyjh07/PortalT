package frc

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

// commonSection 为 frp INI 格式的全局设置段名（服务端连接参数所在）。
const commonSection = "common"

// parseINI 解析 frpc.ini 文本。
// [common] 段映射为 ServerConfig（server_addr/server_port/token 外的键进 Extra），
// 其余段按文件顺序映射为 Proxy（名称 = 段名）。
func parseINI(content []byte) (*Config, error) {
	cfg, err := ini.Load(content)
	if err != nil {
		return nil, fmt.Errorf("解析 INI 失败: %w", err)
	}
	c := &Config{
		Format: FormatINI,
	}
	for _, sec := range cfg.Sections() {
		if sec.Name() == ini.DEFAULT_SECTION {
			continue // ini.v1 的 DEFAULT 段（无名称），忽略
		}
		if sec.Name() == commonSection {
			c.Server = serverFromINI(sec)
			c.order = append(c.order, commonSection)
			continue
		}
		p := proxyFromINI(sec)
		c.Proxies = append(c.Proxies, p)
		c.order = append(c.order, p.Name)
	}
	return c, nil
}

// serverFromINI 从 [common] 段读取服务端设置。
func serverFromINI(sec *ini.Section) ServerConfig {
	s := ServerConfig{Extra: map[string]any{}}
	for _, key := range sec.Keys() {
		name := key.Name()
		val := key.Value()
		switch name {
		case "server_addr":
			s.ServerAddr = val
		case "server_port":
			if n, err := strconv.Atoi(val); err == nil {
				s.ServerPort = n
			}
		case "token":
			s.Token = val
		default:
			s.Extra[name] = val
		}
	}
	if len(s.Extra) == 0 {
		s.Extra = nil
	}
	return s
}

// proxyFromINI 从代理段读取代理配置。
// 段名 = 代理名；type/local_ip/local_port/remote_port/custom_domains 为建模键，
// 其余键（use_encryption/plugin 等）进 Extra 保留。
func proxyFromINI(sec *ini.Section) Proxy {
	p := Proxy{Name: sec.Name(), Extra: map[string]any{}}
	for _, key := range sec.Keys() {
		name := key.Name()
		val := key.Value()
		switch name {
		case "type":
			p.Type = val
		case "local_ip":
			p.LocalIP = val
		case "local_port":
			if n, err := strconv.Atoi(val); err == nil {
				p.LocalPort = n
			}
		case "remote_port":
			if n, err := strconv.Atoi(val); err == nil {
				p.RemotePort = n
			}
		case "custom_domains":
			p.CustomDomains = splitCSV(val)
		default:
			p.Extra[name] = val
		}
	}
	if len(p.Extra) == 0 {
		p.Extra = nil
	}
	return p
}

// renderINI 序列化 Config 为 frpc.ini 文本。
// 服务端设置写 [common] 段（建模键按固定顺序，Extra 按原键序追加），
// 代理按 c.order（缺省为插入序）依次成段。
func renderINI(c *Config) ([]byte, error) {
	cfg := ini.Empty()
	cfg.Section(commonSection) // 先创建 common 段，保证在最前
	writeServerINI(cfg.Section(commonSection), c.Server)
	for _, p := range c.Proxies {
		sec, err := cfg.NewSection(p.Name)
		if err != nil {
			return nil, fmt.Errorf("创建代理段 %q 失败: %w", p.Name, err)
		}
		writeProxyINI(sec, p)
	}
	var sb strings.Builder
	if _, err := cfg.WriteTo(&sb); err != nil {
		return nil, fmt.Errorf("序列化 INI 失败: %w", err)
	}
	return []byte(sb.String()), nil
}

// writeServerINI 写 [common] 段。
func writeServerINI(sec *ini.Section, s ServerConfig) {
	if s.ServerAddr != "" {
		_, _ = sec.NewKey("server_addr", s.ServerAddr)
	}
	if s.ServerPort != 0 {
		_, _ = sec.NewKey("server_port", strconv.Itoa(s.ServerPort))
	}
	if s.Token != "" {
		_, _ = sec.NewKey("token", s.Token)
	}
	writeExtra(sec, s.Extra)
}

// writeProxyINI 写代理段。
func writeProxyINI(sec *ini.Section, p Proxy) {
	if p.Type != "" {
		_, _ = sec.NewKey("type", p.Type)
	}
	if p.LocalIP != "" {
		_, _ = sec.NewKey("local_ip", p.LocalIP)
	}
	if p.LocalPort != 0 {
		_, _ = sec.NewKey("local_port", strconv.Itoa(p.LocalPort))
	}
	if p.RemotePort != 0 {
		_, _ = sec.NewKey("remote_port", strconv.Itoa(p.RemotePort))
	}
	if len(p.CustomDomains) > 0 {
		_, _ = sec.NewKey("custom_domains", joinCSV(p.CustomDomains))
	}
	writeExtra(sec, p.Extra)
}

// writeExtra 追加未知键（键序不保证稳定，但内容保留）。
func writeExtra(sec *ini.Section, extra map[string]any) {
	if extra == nil {
		return
	}
	for k, v := range extra {
		if _, err := sec.NewKey(k, fmt.Sprint(v)); err != nil {
			continue
		}
	}
}

// splitCSV 拆分逗号分隔的字符串列表（frp custom_domains 等）。
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// joinCSV 把字符串列表拼为逗号分隔（frp custom_domains 等）。
func joinCSV(items []string) string {
	return strings.Join(items, ",")
}
