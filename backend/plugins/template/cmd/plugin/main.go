// PortalT 原生插件入口模版。
//
// 插件作为独立进程被 PortalT 插件宿主（pluginhost.Manager）spawn 监督：
//   - gRPC 控制面：监听 127.0.0.1:<PORTALT_PLUGIN_GRPC_PORT>，
//     实现 plugin.v1.PluginControl（Handshake / Health / Shutdown / Notify）。
//   - HTTP 数据面：监听 127.0.0.1:<PORTALT_PLUGIN_HTTP_PORT>，
//     由 PortalT 反向代理暴露为 /native/<id>/* 与 /api/v1/plugins/native/<id>/api/*。
//
// 任意语言只要实现同构协议即可，本模版以 Go 示范。
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

// 插件 ID（与 manifest.json 一致，由宿主 Handshake 校验）。
const pluginID = "my-plugin"

func main() {
	grpcPort := os.Getenv("PORTALT_PLUGIN_GRPC_PORT")
	httpPort := os.Getenv("PORTALT_PLUGIN_HTTP_PORT")
	if grpcPort == "" || httpPort == "" {
		log.Fatalf("缺少 PORTALT_PLUGIN_GRPC_PORT / PORTALT_PLUGIN_HTTP_PORT 环境变量")
	}

	// ---- HTTP 数据面 ----
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"plugin":  pluginID,
			"version": "1.0.0",
			"user":    r.Header.Get("X-PortalT-User"),
			"role":    r.Header.Get("X-PortalT-Role"),
			"perms":   r.Header.Get("X-PortalT-Perms"),
		})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="zh">
<head><meta charset="utf-8"><title>` + pluginID + ` 插件</title></head>
<body><h1>` + pluginID + ` 插件</h1>
<p>插件运行态由 PortalT 进程宿主管理；本页面由插件自身 HTTP 服务提供。</p>
<p><a href="/api/info">调用 API /api/info</a></p>
</body></html>`))
	})

	httpLn, err := net.Listen("tcp", "127.0.0.1:"+httpPort)
	if err != nil {
		log.Fatalf("监听 HTTP 数据面失败: %v", err)
	}
	go func() {
		if err := http.Serve(httpLn, mux); err != nil {
			log.Printf("HTTP 数据面退出: %v", err)
		}
	}()

	// ---- gRPC 控制面 ----
	grpcLn, err := net.Listen("tcp", "127.0.0.1:"+grpcPort)
	if err != nil {
		log.Fatalf("监听 gRPC 控制面失败: %v", err)
	}
	srv := grpc.NewServer()
	pluginv1.RegisterPluginControlServer(srv, &control{})
	log.Printf("%s 插件就绪 (grpc=%s http=%s)", pluginID, grpcPort, httpPort)
	if err := srv.Serve(grpcLn); err != nil {
		log.Fatalf("gRPC 控制面退出: %v", err)
	}
}

// control 实现 plugin.v1.PluginControl。
type control struct {
	pluginv1.UnimplementedPluginControlServer
}

func (c *control) Handshake(_ context.Context, req *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	id := req.GetManifest().GetId()
	log.Printf("握手: id=%s http_port=%d", id, req.GetHttpPort())
	return &pluginv1.HandshakeResponse{
		Enabled: true,
	}, nil
}

func (c *control) Health(context.Context, *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	return &pluginv1.HealthResponse{Healthy: true}, nil
}

func (c *control) Shutdown(_ context.Context, req *pluginv1.ShutdownRequest) (*pluginv1.ShutdownResponse, error) {
	log.Printf("停机: reason=%s", req.GetReason())
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()
	return &pluginv1.ShutdownResponse{}, nil
}

func (c *control) Notify(_ context.Context, req *pluginv1.NotifyRequest) (*pluginv1.NotifyResponse, error) {
	log.Printf("事件: %v data=%v", req.GetEvent(), req.GetData())
	return &pluginv1.NotifyResponse{}, nil
}