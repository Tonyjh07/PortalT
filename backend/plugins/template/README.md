# PortalT 原生插件模板

本模板是 **PortalT native 插件**的独立仓库起点。模板实现完整的最小插件（gRPC 控制面 + HTTP 数据面），开发者 fork/clone 后修改 `manifest.json` 与业务逻辑即可。

## 目录结构

```
template/
├── manifest.json            # 插件元信息（编辑：id/name/icon/route/permission 等）
├── cmd/
│   └── plugin/
│       └── main.go          # 入口：gRPC 控制面 + HTTP 数据面（实现业务逻辑处）
├── static/                  # 静态前端资源（可选，挂载到 /native/<id>/）
│   └── index.html
├── go.mod                   # Go 模块，replace 指向 PortalT 根目录（见下方说明）
├── Makefile                 # 构建与投放命令
└── README.md
```

## 用法

### 方式一：作为 PortalT 官方 submodule（推荐）

1. 在本仓库根目录执行：
   ```bash
   cd PortalT
   git submodule add <你的仓库URL> backend/plugins/<你的插件ID>
   ```
2. 编辑 `backend/plugins/<你的插件ID>/manifest.json` 设置插件 ID、名称等
3. 在 `cmd/<你的插件ID>/main.go`（或调整目录）实现业务逻辑
4. 部署机 `deploy/install.sh` / `deploy/update.sh` 自动构建并投放

### 方式二：作为用户插件（独立开发）

1. fork/clone 本模板到本地
2. 编辑 `manifest.json`：`id` = 插件 ID，须等于最终投放的目录名
3. 在 `cmd/plugin/main.go` 实现业务逻辑
4. 构建并投放：
   ```bash
   make build     # 编译 plugin 可执行文件
   make install   # 复制到 <PLUGINS_DIR>/<id>/
   ```
5. 或手动：`go build -o plugin ./cmd/plugin`，将 `plugin` + `manifest.json` 放入 `PLUGINS_DIR/<id>/`
6. PortalT 管理界面启用插件（默认新增插件为禁用态）

## go.mod 与 replace

本模板的 `go.mod` 通过 `replace` 指令引用 PortalT 源码中的 `portalt/proto/plugin/v1` 包：

```
replace portalt => ../../../backend
```

- **作为 submodule**（`backend/plugins/<id>/` 下）：`../../../backend` 指向 PortalT backend 模块，无需修改。
- **独立开发**：调整 `replace` 路径指向本地 PortalT 的 `backend/` 目录（如 `replace portalt => /path/to/PortalT/backend`），或拷贝 `proto/plugin/v1/` 下的 `.pb.go` 文件到本模块内。

## 通信协议

| 层面 | 协议 | 端口来源 | 说明 |
|------|------|---------|------|
| 控制面 | gRPC | `PORTALT_PLUGIN_GRPC_PORT` 环境变量 | Handshake → Health（周期）→ Shutdown / Notify |
| 数据面 | HTTP | `PORTALT_PLUGIN_HTTP_PORT` 环境变量，Handshake 确认 | `/healthz`（宿主探测）+ 自定义端点 |

- 插件仅监听 `127.0.0.1` 回环地址（PortalT 校验，防 SSRF）。
- 任意语言可实现同构协议（proto 文件见 PortalT 仓库 `proto/plugin/v1/plugin.proto`）。

## manifest.json 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 插件 ID，须等于 PLUGINS_DIR 下的目录名 |
| name | string | 显示名称（插件管理界面展示） |
| icon | string | 图标标识（如 mdi:puzzle，Element Plus 图标） |
| route | string | 前端路由（形如 `/my-plugin`） |
| sort_order | int | 排序权重（小在前） |
| permission | string | 最小访问权限（空 = 无需额外权限，须在 PortalT 权限字典内） |
| health_interval_seconds | int | 宿主健康探测间隔（默认 30，示例 5） |

详见 PortalT 仓库 `docs/plugins.md`。

## 许可

随宿主项目许可证。