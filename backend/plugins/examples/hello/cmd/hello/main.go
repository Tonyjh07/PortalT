// Package main 是 PortalT native 插件「hello」的可执行文件入口。
//
// 运行形态（作为独立进程被 PortalT 插件宿主 spawn）：
//   - gRPC 控制面：监听 127.0.0.1:<PORTALT_PLUGIN_GRPC_PORT>，实现
//     plugin.v1.PluginControl（Handshake / Health / Shutdown / Notify）。
//   - HTTP 数据面：监听 127.0.0.1:<PORTALT_PLUGIN_HTTP_PORT>，
//     由 PortalT 反向代理暴露为 /native/hello/（静态前端）与
//     /api/v1/plugins/native/hello/api/*（示例 API）。
//
// 本插件是 native 插件开发的模板与集成测试目标：任意语言实现同构协议即可。
package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"

	pluginv1 "portalt/proto/plugin/v1"
)

func main() {
	grpcPort := os.Getenv("PORTALT_PLUGIN_GRPC_PORT")
	httpPort := os.Getenv("PORTALT_PLUGIN_HTTP_PORT")
	if grpcPort == "" || httpPort == "" {
		log.Fatalf("缺少 PORTALT_PLUGIN_GRPC_PORT / PORTALT_PLUGIN_HTTP_PORT 环境变量")
	}

	// 静态前端（简单演示页）
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"plugin":  "hello",
			"message": "Hello from native plugin",
			"user":    r.Header.Get("X-PortalT-User"),
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="zh"><head><meta charset="utf-8"><title>Hello 插件</title></head>
<body><h1>Hello 示例插件</h1>
<p>插件运行态由 PortalT 进程宿主管理；本页面由插件自身 HTTP 服务提供。</p>
<p><a href="/api/hello">调用示例 API /api/hello</a></p>
</body></html>`))
	})

	// 启动 HTTP 数据面
	httpLn, err := net.Listen("tcp", "127.0.0.1:"+httpPort)
	if err != nil {
		log.Fatalf("监听 HTTP 数据面失败: %v", err)
	}
	go func() {
		if err := http.Serve(httpLn, mux); err != nil {
			log.Printf("HTTP 数据面退出: %v", err)
		}
	}()

	// 启动 gRPC 控制面
	grpcLn, err := net.Listen("tcp", "127.0.0.1:"+grpcPort)
	if err != nil {
		log.Fatalf("监听 gRPC 控制面失败: %v", err)
	}
	srv := grpc.NewServer()
	pluginv1.RegisterPluginControlServer(srv, &control{})
	log.Printf("hello 插件就绪 (grpc=%s http=%s)", grpcPort, httpPort)
	if err := srv.Serve(grpcLn); err != nil {
		log.Fatalf("gRPC 控制面退出: %v", err)
	}
}

// control 实现 plugin.v1.PluginControl。
type control struct {
	pluginv1.UnimplementedPluginControlServer
}

// Handshake 返回启用状态：PortalT 分配端口并下发 manifest，插件确认。
func (c *control) Handshake(_ context.Context, req *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	id := req.GetManifest().GetId()
	log.Printf("握手: id=%s http_port=%d", id, req.GetHttpPort())
	return &pluginv1.HandshakeResponse{
		Enabled:       true,
		PortalVersion: req.GetManifest().GetName(), // 占位，真实插件可做兼容性判断
	}, nil
}

// Health 健康探测：插件始终健康。
func (c *control) Health(context.Context, *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	return &pluginv1.HealthResponse{Healthy: true}, nil
}

// Shutdown 优雅停机：触发进程退出。
func (c *control) Shutdown(_ context.Context, req *pluginv1.ShutdownRequest) (*pluginv1.ShutdownResponse, error) {
	log.Printf("收到停机指令: %s", req.GetReason())
	// 由主流程在收到信号后退出（此处简单返回，主进程由宿主 kill 兜底）
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()
	return &pluginv1.ShutdownResponse{}, nil
}

// Notify 事件通知：enabled / disabled / config_changed / restarting。
func (c *control) Notify(_ context.Context, req *pluginv1.NotifyRequest) (*pluginv1.NotifyResponse, error) {
	log.Printf("收到事件通知: %v", req.GetEvent())
	return &pluginv1.NotifyResponse{}, nil
}
