// Package api 提供 frpc-admin 插件的 HTTP 数据面端点。
//
// 路由约定（经 PortalT 反代暴露）：
//   - 鉴权 API：/api/v1/plugins/native/frpc-admin/*   → 插件内 /api/*
//   - 静态前端：/native/frpc-admin/*                 → 插件内 /（仅非 /api 路径）
//
// 端点列表见 app.go 的 registerRoutes。所有响应 JSON 均为 { ... } 业务体
// （插件反代透传状态码与 body，不加门户信封）。
package api

import (
	"net/http"
	"os"
	"path"

	"portalt-plugins/frpc-admin/internal/configstore"
)

// App 插件 HTTP 数据面处理器集合。
type App struct {
	store    configStore
	staticFS http.Handler
}

// configStore 抽象，便于测试注入。
type configStore interface {
	Save(c configstore.Connection) error
	Get(vmID string) (configstore.Connection, bool)
	List() []configstore.Connection
	Delete(vmID string) error
}

// NewApp 创建应用：store 为连接配置存储；staticDir 为静态前端目录（可选）。
func NewApp(store configStore, staticDir string) *App {
	a := &App{store: store}
	if staticDir != "" {
		if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
			a.staticFS = http.FileServer(http.Dir(staticDir))
		}
	}
	return a
}

// RegisterRoutes 注册全部路由（Go 1.22+ 方法+路径模式）。
func (a *App) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /api/info", a.handleInfo)

	// 连接配置 CRUD
	mux.HandleFunc("GET /api/connections", a.handleListConnections)
	mux.HandleFunc("PUT /api/connections/{vmId}", a.handleSaveConnection)
	mux.HandleFunc("DELETE /api/connections/{vmId}", a.handleDeleteConnection)

	// frpc 操作
	mux.HandleFunc("GET /api/vms/{vmId}/probe", a.handleProbe)
	mux.HandleFunc("GET /api/vms/{vmId}/config", a.handleGetConfig)
	mux.HandleFunc("PUT /api/vms/{vmId}/config", a.handlePutConfig)

	// 静态前端（仅非 /api 路径；插件进程只监听回环，且宿主静态反代拒绝 /api）
	if a.staticFS != nil {
		mux.HandleFunc("/", a.handleStatic)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "note": "未内置静态前端"})
				return
			}
			http.NotFound(w, r)
		})
	}
}

// handleStatic 提供插件静态前端；/ → index.html。
func (a *App) handleStatic(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Path
	if reqPath == "/" {
		reqPath = "/index.html"
	}
	// 防路径穿越：清理后仍以 / 开头才放行（用 path 包做 URL 路径语义，避免 Windows 分隔符差异）
	if path.Clean(reqPath) != reqPath {
		http.NotFound(w, r)
		return
	}
	r.URL.Path = reqPath
	a.staticFS.ServeHTTP(w, r)
}

func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"plugin":  "frpc-admin",
		"version": "1.0.0",
		"user":    r.Header.Get("X-PortalT-User"),
		"role":    r.Header.Get("X-PortalT-Role"),
		"perms":   r.Header.Get("X-PortalT-Perms"),
	})
}

// vmID 从路径取 vmId 参数。
func vmID(r *http.Request) string {
	return r.PathValue("vmId")
}
