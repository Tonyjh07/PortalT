package frc

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// tomlTopKeys TOML 格式顶层建模键（服务端设置）。
// auth.token 经 auth 表内嵌（auth.token = ...），其余未知顶层键进 Server.Extra。
const (
	tomlKeyServerAddr = "serverAddr"
	tomlKeyServerPort = "serverPort"
	tomlKeyProxies    = "proxies"
	tomlAuthTable     = "auth"
)

// parseTOML 解析 frpc.toml 文本。
// 顶层 serverAddr/serverPort/auth.token 映射为 ServerConfig，其余顶层键进 Extra；
// [[proxies]] 数组按出现顺序映射为 Proxy 列表。
func parseTOML(content []byte) (*Config, error) {
	// 先做一次纯语法校验（BurntSushi 对 map[string]any 的宽松解码可能漏报类型错误）。
	// 用 MetaData 获取已解析键，便于区分"键不存在"与"解析失败"。
	var raw map[string]any
	md, err := toml.Decode(string(content), &raw)
	if err != nil {
		return nil, fmt.Errorf("解析 TOML 失败: %w", err)
	}
	_ = md

	c := &Config{
		Format: FormatINI, // 占位，下方修正
	}
	c.Format = FormatTOML
	c.Server = serverFromTOML(raw)
	c.Proxies = proxiesFromTOML(raw)
	return c, nil
}

// serverFromTOML 从顶层 map 提取服务端设置。
func serverFromTOML(raw map[string]any) ServerConfig {
	s := ServerConfig{Extra: map[string]any{}}
	if v, ok := raw[tomlKeyServerAddr].(string); ok {
		s.ServerAddr = v
	}
	if n, ok := tomlInt(raw[tomlKeyServerPort]); ok {
		s.ServerPort = n
	}
	if auth, ok := raw[tomlAuthTable].(map[string]any); ok {
		if v, ok := auth["token"].(string); ok {
			s.Token = v
		}
	}
	for k, v := range raw {
		switch k {
		case tomlKeyServerAddr, tomlKeyServerPort, tomlKeyProxies:
			continue
		default:
			s.Extra[k] = v
		}
	}
	// auth 表其余键（method 等）保留
	if auth, ok := raw[tomlAuthTable].(map[string]any); ok {
		rest := map[string]any{}
		for k, v := range auth {
			if k != "token" {
				rest[k] = v
			}
		}
		if len(rest) > 0 {
			s.Extra[tomlAuthTable] = rest
		}
	}
	if len(s.Extra) == 0 {
		s.Extra = nil
	}
	return s
}

// proxiesFromTOML 从顶层 [[proxies]] 提取代理列表（保持数组顺序）。
func proxiesFromTOML(raw map[string]any) []Proxy {
	arr, ok := raw[tomlKeyProxies].([]map[string]any)
	if !ok {
		// 也可能是 []any（解码细节差异），归一化
		if anyArr, ok := raw[tomlKeyProxies].([]any); ok {
			arr = make([]map[string]any, 0, len(anyArr))
			for _, item := range anyArr {
				if m, ok := item.(map[string]any); ok {
					arr = append(arr, m)
				}
			}
		}
	}
	out := make([]Proxy, 0, len(arr))
	for _, m := range arr {
		p := Proxy{Extra: map[string]any{}}
		if v, ok := m["name"].(string); ok {
			p.Name = v
		}
		if v, ok := m["type"].(string); ok {
			p.Type = v
		}
		if v, ok := m["localIP"].(string); ok {
			p.LocalIP = v
		}
		if n, ok := tomlInt(m["localPort"]); ok {
			p.LocalPort = n
		}
		if n, ok := tomlInt(m["remotePort"]); ok {
			p.RemotePort = n
		}
		switch cd := m["customDomains"].(type) {
		case []any:
			for _, v := range cd {
				if s, ok := v.(string); ok {
					p.CustomDomains = append(p.CustomDomains, s)
				}
			}
		case []string:
			p.CustomDomains = cd
		}
		for k, v := range m {
			switch k {
			case "name", "type", "localIP", "localPort", "remotePort", "customDomains":
				continue
			default:
				p.Extra[k] = v
			}
		}
		if len(p.Extra) == 0 {
			p.Extra = nil
		}
		out = append(out, p)
	}
	return out
}

// renderTOML 序列化 Config 为 frpc.toml 文本。
// 重建顶层 map：服务端设置键优先，Extra 键保留，最后写 [[proxies]]。
func renderTOML(c *Config) ([]byte, error) {
	top := map[string]any{}
	// 先铺 Extra（含 auth 表其余键），再覆盖建模键，保证 auth.token 与 auth.method 共存
	for k, v := range c.Server.Extra {
		if k == tomlAuthTable {
			auth, ok := v.(map[string]any)
			if !ok {
				continue
			}
			merged := map[string]any{}
			for ak, av := range auth {
				merged[ak] = av
			}
			top[tomlAuthTable] = merged
			continue
		}
		top[k] = v
	}
	if c.Server.ServerAddr != "" {
		top[tomlKeyServerAddr] = c.Server.ServerAddr
	}
	if c.Server.ServerPort != 0 {
		top[tomlKeyServerPort] = c.Server.ServerPort
	}
	if c.Server.Token != "" {
		auth, _ := top[tomlAuthTable].(map[string]any)
		if auth == nil {
			auth = map[string]any{}
		}
		auth["token"] = c.Server.Token
		top[tomlAuthTable] = auth
	}
	if len(c.Proxies) > 0 {
		arr := make([]map[string]any, 0, len(c.Proxies))
		for _, p := range c.Proxies {
			m := map[string]any{}
			for k, v := range p.Extra {
				m[k] = v
			}
			if p.Name != "" {
				m["name"] = p.Name
			}
			if p.Type != "" {
				m["type"] = p.Type
			}
			if p.LocalIP != "" {
				m["localIP"] = p.LocalIP
			}
			if p.LocalPort != 0 {
				m["localPort"] = p.LocalPort
			}
			if p.RemotePort != 0 {
				m["remotePort"] = p.RemotePort
			}
			if len(p.CustomDomains) > 0 {
				m["customDomains"] = p.CustomDomains
			}
			arr = append(arr, m)
		}
		top[tomlKeyProxies] = arr
	}

	var sb strings.Builder
	enc := toml.NewEncoder(&sb)
	if err := enc.Encode(top); err != nil {
		return nil, fmt.Errorf("序列化 TOML 失败: %w", err)
	}
	return []byte(sb.String()), nil
}

// tomlInt 从 TOML 解码值中提取整数（解码为 int64）。
func tomlInt(v any) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}
