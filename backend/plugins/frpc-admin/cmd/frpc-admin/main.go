// Package main 是 PortalT native 插件「frpc-admin」的可执行文件入口。
//
// 运行形态（作为独立进程被 PortalT 插件宿主 spawn 监督）：
//   - gRPC 控制面：监听 127.0.0.1:<PORTALT_PLUGIN_GRPC_PORT>，实现
//     plugin.v1.PluginControl（Handshake / Health / Shutdown / Notify）。
//   - HTTP 数据面：监听 127.0.0.1:<PORTALT_PLUGIN_HTTP_PORT>，
//     由 PortalT 反向代理暴露为 /native/frpc-admin/（静态前端）与
//     /api/v1/plugins/native/frpc-admin/*（鉴权 API）。
//
// 业务能力：通过 SSH 进入目标 VM 管理 frpc 配置（结构化 + 原文双模式），
// 保存时备份原文件、语法检查、应用并重启，失败自动回滚。
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"

	pluginv1 "portalt/proto/plugin/v1"

	"portalt-plugins/frpc-admin/internal/api"
	"portalt-plugins/frpc-admin/internal/configstore"
)

// 插件 ID（与 manifest.json 一致，由宿主 Handshake 校验）。
const pluginID = "frpc-admin"

func main() {
	grpcPort := os.Getenv("PORTALT_PLUGIN_GRPC_PORT")
	httpPort := os.Getenv("PORTALT_PLUGIN_HTTP_PORT")
	if grpcPort == "" || httpPort == "" {
		log.Fatalf("缺少 PORTALT_PLUGIN_GRPC_PORT / PORTALT_PLUGIN_HTTP_PORT 环境变量")
	}

	// 数据存储目录：可执行文件所在目录（PLUGINS_DIR/frpc-admin/），写入 data/。
	dataDir, err := pluginDataDir()
	if err != nil {
		log.Fatalf("确定插件数据目录失败: %v", err)
	}
	store, err := configstore.New(dataDir)
	if err != nil {
		log.Fatalf("初始化连接配置存储失败: %v", err)
	}
	// 静态前端目录：可执行文件同目录 static/
	exe, _ := os.Executable()
	staticDir := filepath.Join(filepath.Dir(exe), "static")
	app := api.NewApp(store, staticDir)

	// ---- HTTP 数据面 ----
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)

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
	log.Printf("%s 插件就绪 (grpc=%s http=%s data=%s)", pluginID, grpcPort, httpPort, dataDir)
	if err := srv.Serve(grpcLn); err != nil {
		log.Fatalf("gRPC 控制面退出: %v", err)
	}
}

// pluginDataDir 返回插件数据目录（可执行文件目录/data），不存在则创建。
func pluginDataDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(exe), "data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// control 实现 plugin.v1.PluginControl。
type control struct {
	pluginv1.UnimplementedPluginControlServer
}

// Handshake 返回启用状态：PortalT 分配端口并下发 manifest，插件确认。
func (c *control) Handshake(_ context.Context, req *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	log.Printf("握手: id=%s http_port=%d", req.GetManifest().GetId(), req.GetHttpPort())
	return &pluginv1.HandshakeResponse{
		Enabled: true,
	}, nil
}

// Health 健康探测：插件始终健康。
func (c *control) Health(context.Context, *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	return &pluginv1.HealthResponse{Healthy: true}, nil
}

// Shutdown 优雅停机：触发进程退出。
func (c *control) Shutdown(_ context.Context, req *pluginv1.ShutdownRequest) (*pluginv1.ShutdownResponse, error) {
	log.Printf("收到停机指令: %s", req.GetReason())
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
