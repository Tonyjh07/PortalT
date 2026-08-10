// Package api 提供 frpc-admin 插件的 HTTP 数据面端点。
//
// 路由约定（经 PortalT 反代暴露）：
//   - 鉴权 API：/api/v1/plugins/native/frpc-admin/*   → 插件内 /api/*
//   - 静态前端：/native/frpc-admin/*                 → 插件内 /（仅非 /api 路径）
//
// 端点列表见 app.go 的 registerRoutes。所有响应 JSON 均为 { ... } 业务体
// （插件反代透传状态码与 body，不加门户信封）。
// 单连接模型：插件管理一台目标主机，连接配置存于 /api/connection，
// 探测与配置读写（/api/probe、/api/config）直接针对已保存连接，不依赖 PortalT VM。
package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"portalt-plugins/frpc-admin/internal/configstore"
)

// App 插件 HTTP 数据面处理器集合。
type App struct {
	store     configStore
	staticDir string
}

// configStore 抽象，便于测试注入。
type configStore interface {
	Save(c configstore.Connection) error
	Get() (configstore.Connection, bool)
	Delete() error
}

// NewApp 创建应用：store 为连接配置存储；staticDir 为静态前端目录（可选）。
func NewApp(store configStore, staticDir string) *App {
	a := &App{store: store}
	if staticDir != "" {
		if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
			a.staticDir = staticDir
		}
	}
	return a
}

// RegisterRoutes 注册全部路由（Go 1.22+ 方法+路径模式）。
// 单连接模型：所有操作针对已保存的唯一连接配置（/api/connection），
// 探测与配置读写不再携带 vmId（与 PortalT VM 概念解耦）。
func (a *App) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /api/info", a.handleInfo)

	// 连接配置
	mux.HandleFunc("GET /api/connection", a.handleGetConnection)
	mux.HandleFunc("PUT /api/connection", a.handleSaveConnection)
	mux.HandleFunc("DELETE /api/connection", a.handleDeleteConnection)

	// frpc 操作（针对已保存连接）
	mux.HandleFunc("POST /api/probe", a.handleProbe)
	mux.HandleFunc("GET /api/config", a.handleGetConfig)
	mux.HandleFunc("PUT /api/config", a.handlePutConfig)

	// 静态前端（仅非 /api 路径；插件进程只监听回环，且宿主静态反代拒绝 /api）
	if a.staticDir != "" {
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
// 不用 http.FileServer：Go 1.26 起 FileServer 对 /index.html 会 301 到 ./，
// 在插件经宿主反代（/native/<id>/ 路径语义）挂载时形成重定向循环。
// 改为显式打开文件返回，仅服务 static/ 内的真实文件。
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
	f, err := os.Open(filepath.Join(a.staticDir, filepath.FromSlash(strings.TrimPrefix(reqPath, "/"))))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(reqPath))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// contentTypeFor 按扩展名推断 Content-Type（避免 ServeContent 依赖文件名推断时
// 对 .js/.css 的 mime 表差异；纯 SPA 仅 html/js/css/svg）。
func contentTypeFor(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".woff", ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
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
