// PortalT 后端服务入口 (Phase 0)
//
// 程序入口，负责依赖注入与启动 HTTP 服务。
// Phase 0 仅验证工具链与基础服务可运行，后续阶段将在此注入
// 仓储、虚拟化提供者、认证等依赖。
package main

import (
	"fmt"
	"log"
	"net/http"
)

const (
	// AppVersion 当前版本号
	AppVersion = "v0.1"
	// listenAddr HTTP 监听地址
	listenAddr = ":8080"
)

func main() {
	log.Println("PortalT", AppVersion, "starting...")

	// 健康检查接口，验证服务可用
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "PortalT %s", AppVersion)
	})

	log.Printf("PortalT listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
