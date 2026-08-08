# PortalT 官方插件

本目录存放**官方原生插件**源码（native 类型），每个插件为独立 git 仓库的
submodule，目录名 = 插件 ID，构建产物投放运行时插件目录 `PLUGINS_DIR`
（部署机默认 `<app>/plugins`，见 `docs/plugins.md`）。

```
backend/plugins/
├── <id>/            # 插件源码（git submodule，指向独立仓库）
│   ├── cmd/...      # 插件可执行文件入口（监听 gRPC 控制面 + HTTP 数据面）
│   ├── manifest.json
│   └── static/      # 插件静态前端（可选，挂载 /native/<id>/）
├── template/        # 插件仓库模板（独立 Go 模块，fork/clone 后自定义）
├── examples/        # 本地示例（非 submodule，直接投放 PLUGINS_DIR 验证链路）
└── README.md
```

## 插件仓库模板

`template/` 是一个完整的、可独立构建的 native 插件仓库模板，包含：

- `cmd/plugin/main.go`：gRPC 控制面（Handshake/Health/Shutdown/Notify）+ HTTP 数据面（/healthz + /api + 首页）
- `manifest.json`：插件元信息（编辑 ID/名称/路由/权限等）
- `static/index.html`：可选静态前端占位
- `Makefile`：构建与投放命令
- 独立的 `go.mod`（通过 `replace` 引用 `portalt` 模块的 proto 包）

用法：fork/clone 后编辑 `manifest.json` 与 `cmd/plugin/main.go` 实现业务逻辑，构建后投放 `PLUGINS_DIR/<id>/`。

## 登记新官方插件

1. 官方插件仓库就绪后执行：
   ```bash
   git submodule add <url> backend/plugins/<id>
   ```
   `.gitmodules` 自动登记条目。
2. 无网络 / 仓库未就绪时，先用本地目录占位（含 `manifest.json` 模板），
   待就绪后替换为 submodule。

## 与用户插件的区别

- **官方插件**：源码以 submodule 引入本仓库，`deploy/install.sh` 构建产物
  投放 `PLUGINS_DIR`。
- **用户插件**：本地自行维护源码与产物，直接投放预编译产物到 `PLUGINS_DIR`
  即可（任意语言），不经过本目录。

## 本地示例（examples/）

`examples/hello/` 是一个最小的 native 插件（Go 实现），用于本地 dev 冒烟与
集成测试。构建并投放：

```bash
cd backend
go build -o <PLUGINS_DIR>/hello/plugin ./plugins/examples/hello/cmd/hello
# manifest.json 随目录投放（见 examples/hello/manifest.json）
```

其运行形态即官方插件模板：gRPC 控制面（Handshake/Health/Shutdown/Notify）+
HTTP 数据面（/healthz + /native 前端 + /api 示例）。
