// Package frc 提供 frpc（frp 客户端）配置的结构化解析、编辑与序列化。
//
// 同时支持两种格式：
//   - INI（frp < 0.52，frpc.ini）：[common] 段为服务端设置，其余段为代理。
//   - TOML（frp >= 0.52，frpc.toml）：顶层键为服务端设置，[[proxies]] 为代理数组。
//
// 设计目标：忠实 round-trip —— 未知键（未在本包建模的键）一律保留，
// 避免"改一个代理清空其他配置"。解析结果含有序代理列表，序列化按格式还原。
package frc

import (
	"errors"
	"fmt"
	"strings"
)

// Format 配置格式标识。
type Format string

const (
	// FormatINI 旧版 INI 格式。
	FormatINI Format = "ini"
	// FormatTOML 新版 TOML 格式。
	FormatTOML Format = "toml"
	// FormatAuto 自动检测（仅解析入口使用，序列化前会解析为具体格式）。
	FormatAuto Format = "auto"
)

// ValidateFormat 校验格式标识合法。
func ValidateFormat(f string) bool {
	return f == string(FormatINI) || f == string(FormatTOML)
}

// ServerConfig 服务端设置（frps 连接参数 + 未知键）。
type ServerConfig struct {
	ServerAddr string         `json:"server_addr"`
	ServerPort int            `json:"server_port"`
	Token      string         `json:"token"`
	Extra      map[string]any `json:"extra,omitempty"`
}

// Proxy 一条代理（隧道）配置。
type Proxy struct {
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	LocalIP       string         `json:"local_ip"`
	LocalPort     int            `json:"local_port"`
	RemotePort    int            `json:"remote_port"`
	CustomDomains []string       `json:"custom_domains,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}

// Config 完整 frpc 配置的结构化表示。
type Config struct {
	Format  Format         `json:"format"`
	Server  ServerConfig   `json:"server"`
	Proxies []Proxy        `json:"proxies"`
	order   []string       // INI: 段顺序（首项为 common 时服务端在前）
}

// Parse 解析 frpc 配置原文。format 为 auto 时自动检测（见 Detect）。
// 解析失败返回错误（含行号等定位信息，供语法检查提示）。
func Parse(content []byte, format string) (*Config, error) {
	f, err := resolveFormat(content, format)
	if err != nil {
		return nil, err
	}
	switch f {
	case FormatINI:
		return parseINI(content)
	case FormatTOML:
		return parseTOML(content)
	default:
		return nil, fmt.Errorf("不支持的格式: %q", format)
	}
}

// Detect 自动检测配置格式。
// 规则（按优先级）：
//  1. 含 [common] 段 → INI；
//  2. 顶层含 serverAddr / auth. / [[proxies]] 等 TOML 特征 → TOML；
//  3. 其它默认按 INI 解析（frp 旧版默认行为），失败再尝试 TOML。
func Detect(content []byte) Format {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return FormatINI
	}
	for _, line := range strings.Split(trimmed, "\n") {
		l := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if strings.HasPrefix(l, "[") {
			// 形如 [common] 或 [proxy-name]
			if l == "[common]" || l == "[common] " {
				return FormatINI
			}
			// [[proxies]] 是 TOML 数组表；[auth] 是 TOML 嵌套表
			if strings.HasPrefix(l, "[[") || l == "[auth]" {
				return FormatTOML
			}
			continue
		}
		if strings.HasPrefix(l, "serverAddr") || strings.HasPrefix(l, "serverPort") ||
			strings.HasPrefix(l, "auth.") || strings.HasPrefix(l, "loginFailExit") {
			return FormatTOML
		}
	}
	return FormatINI
}

// resolveFormat 解析用户指定格式：auto 时自动检测，非法格式报错。
func resolveFormat(content []byte, format string) (Format, error) {
	if format == "" || format == string(FormatAuto) {
		return Detect(content), nil
	}
	f := Format(format)
	switch f {
	case FormatINI, FormatTOML:
		return f, nil
	default:
		return "", fmt.Errorf("不支持的格式: %q（应为 ini / toml / auto）", format)
	}
}

// Render 按 Config.Format 序列化为配置原文。
func (c *Config) Render() ([]byte, error) {
	switch c.Format {
	case FormatINI:
		return renderINI(c)
	case FormatTOML:
		return renderTOML(c)
	default:
		return nil, fmt.Errorf("未确定配置格式，无法序列化")
	}
}

// SyntaxCheck 校验配置内容语法：能成功解析即视为语法正确。
// format 为 auto/空时自动检测。返回解析错误（含定位信息）或 nil。
func SyntaxCheck(content []byte, format string) error {
	if len(strings.TrimSpace(string(content))) == 0 {
		return errors.New("配置内容为空")
	}
	_, err := Parse(content, format)
	return err
}
