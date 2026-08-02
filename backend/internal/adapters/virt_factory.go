package adapters

import (
	"fmt"
	"strings"

	"portalt/internal/adapters/esxi"
	"portalt/internal/adapters/mock"
	"portalt/internal/adapters/workstation"
	"portalt/internal/ports"
)

// NewVirtualizationProvider 按类型创建虚拟化平台提供者。
//
// virtType: "esxi" | "workstation" | "mock"（默认 mock，开发调试无需真实环境）。
// config 配置键：
//
//	url      平台地址（esxi 必填，如 https://esxi.lan/sdk；workstation 必填，默认 http://127.0.0.1:8697）
//	username 登录用户名
//	password 登录密码
//	insecure "true"/"1"/"yes" 时跳过TLS校验
func NewVirtualizationProvider(virtType string, config map[string]string) (ports.VirtualizationProvider, error) {
	switch strings.ToLower(virtType) {
	case "", "mock":
		return mock.NewProvider(config), nil
	case "esxi":
		cfg := esxi.Config{
			URL:      config["url"],
			Username: config["username"],
			Password: config["password"],
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("esxi: 缺少必填配置键 url")
		}
		if v, ok := config["insecure"]; ok {
			cfg.Insecure = v == "true" || v == "1" || v == "yes"
		}
		return esxi.NewProvider(cfg), nil
	case "workstation":
		cfg := workstation.Config{
			URL:      config["url"],
			Username: config["username"],
			Password: config["password"],
		}
		if v, ok := config["insecure"]; ok {
			cfg.Insecure = v == "true" || v == "1" || v == "yes"
		}
		return workstation.NewProvider(cfg), nil
	default:
		return nil, fmt.Errorf("不支持的虚拟化平台类型 %q（可选 esxi/workstation/mock）", virtType)
	}
}
