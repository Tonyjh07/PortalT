# PortalT 插件系统重构计划

> 状态：**Phase 1、2、3、4 已完成（access 收敛 + Caddy 交互 + native 进程化 + 部署闭环 + 前端增强）**；
> 本文档是重构的设计蓝图，实施结果以
> `docs/plugins.md`、`docs/interfaces.md`、`About.md` 进度表为准。
>
> 决策前提（已确认）：
> - 项目尚未正式发布，**允许完全破坏性重构**（`plugins` 表、管理 API、前端页面全部重写，不做向后兼容迁移）。
> - 官方插件源码经 **git submodule** 引入（`backend/plugins/<id>` 指向独立仓库）；
>   用户插件 = 本地自行投放编译产物，与源码解耦。
> - access 插件可与 Caddy 交互：管理界面编写 Caddy 规则，落盘到 `plugins.d/` 并触发 reload。
> - 暂移除 cron 插件，未来重新实现（本文档不再为旧 cron 设计迁移路径）。

## 1. 现状结论

- 现有三类插件 `iframe` / `proxy` / `native`：
  - `iframe` / `proxy`：纯配置型，靠 `plugins` 表字段驱动（`iframe_url` / `api_url`+`endpoints` 白名单），无后端代码。
  - `native`：编译进二进制的 Go 插件（`backend/internal/plugins/registry.go` 编译期注册），实现
    `Info()` / `Mount()` / `StaticFS()` 三方法；**无生命周期、无热加载**（注释明确 Windows 下 `.so` 不可行）。
  - `esxiadmin` 实为 iframe 嵌入型（`/config` 返回 provider/web_url + 占位页 iframe 嵌入 ESXi Web UI）。
  - `cron` 为后台 goroutine 型（内存调度器 + API + 静态前端）。
- 权限三层叠加：`plugin:view` 组级入口 → 每插件 `IsActive` 启用闸门 → 插件声明 `Permission` 硬校验
  （`nativeGate` / proxy handler 强制执行）。**这套权限模型是核心资产，重构必须完整保留。**
- 技术栈现状：无 gRPC / `.proto` / fsnotify；仅单 module `backend/go.mod`（无 `go.work`）；
  前端 Nuxt 3 按 `plugin.type` 分派渲染（`frontend/pages/plugins/[...slug].vue`）。
- Caddy 部署：`deploy/install.sh` 将 `caddy/Caddyfile` 复制到 `/etc/caddy/Caddyfile`，
  systemd drop-in 注入 `CADDY_PORT` / `ESXI_UPSTREAM` 环境变量；主文件用命名片段
  `(esxi_proxy_routes)` + `import esxi_proxy_routes`，ESXi 反代规则写死在仓库 Caddyfile 内。

## 2. 目标架构

### 2.1 插件类型收敛为两类

| 类型 | 运行形态 | 配置来源 | 页面 | Caddy 交互 |
|------|----------|----------|------|------------|
| `access` | 纯配置型（无进程） | `plugins` 表（DB） | iframe 嵌入 + API 面板（可共存） | **是**：`CaddyRules` 字段 |
| `native` | 独立进程（PortalT spawn 监督） | `manifest.json` + DB 同步 | `/native/<id>/`（插件自带 HTTP 服务） | 否 |

- `access` 合并旧 `iframe` + `proxy`：`iframe_url`（嵌入外部页面）与 `api_url`+`endpoints`（API 白名单）
  **可同时存在**，前端一页两区块；仅提供一种时退化为旧行为。
- `native` 由"编译进二进制"改为"独立进程 + gRPC 控制面 + HTTP 数据面"，支持任意语言。

### 2.2 native 进程通信（gRPC 控制面 + HTTP 数据面）

```
PortalT ──spawn──▶ 插件进程
   │                环境变量下发：PORTALT_PLUGIN_ID / PORTALT_PLUGIN_GRPC_PORT
   │
   ├─ gRPC 控制面（127.0.0.1:<PortalT 分配的空闲端口>）
   │    Handshake(Info)      → 校验 manifest 一致性，同步 DB
   │    Health()             → 周期健康探测（间隔取自 manifest）
   │    Shutdown()           → 优雅停（禁用 / 升级 / 删除时）
   │    Notify(event)        → 钩子回调（enabled / disabled / config_changed）
   └─ HTTP 数据面（插件自起本地端口，经 Handshake 上报，PortalT 校验回环后反代）
        /api/v1/plugins/native/<id>/...   与   /native/<id>/...
```

- 端口分配：PortalT 启动时分配空闲 gRPC 端口经 env 下发，避免 manifest 固定端口冲突。
- 数据面：插件进程需同时监听 HTTP + gRPC 两个本地端口（简单、任意语言友好；不做 grpc-gateway 合并）。
- 安全：仅执行 `PLUGINS_DIR` 内文件（防路径穿越）；反代前校验插件上报 HTTP 地址为回环地址。

### 2.3 运行时插件目录与热加载

```
<PLUGINS_DIR>（可配置，默认部署机 <app>/plugins）
plugins/
├── <id>/                 # 目录名 = 插件 ID
│   ├── plugin            # 可执行文件（任意语言编译产物）
│   ├── manifest.json     # 元数据 / 权限 / 钩子配置（native 必备）
│   └── static/           # 插件静态前端（可选，挂 /native/<id>/）
└── ...（fsnotify 监视整个根目录）
```

- 官方插件：submodule 源码 → 构建脚本产出到 `PLUGINS_DIR`。
- 用户插件：直接投放预编译产物（任意语言），无需源码入库。
- 热加载（`fsnotify`）：
  - 新增 `<id>/` 目录 → 校验 manifest + 可执行文件 → 注册（**默认 `is_active=false`**，管理员界面手动启用）。
  - manifest / 二进制被替换 → 停止旧进程并重启（升级）。
  - 目录被删除 → 停止进程，DB 记录标记 `missing`（不自动删除记录，保留管理员配置）。

### 2.4 生命周期状态机与钩子

```
discovered → installed → disabled → enabled → running
                                        ↘ error（健康探测失败 / 崩溃）
                                        ↘ missing（目录被删除）
```

- 钩子：启用 → `Notify(enabled)` + spawn；停用 → `Shutdown()` + kill；
  升级（替换二进制）→ 先停再启；配置变更 → `Notify(config_changed)`。
- 进程监督：崩溃自动重启（带退避与重启次数上限，防止疯狂重启）；健康探测失败标记 error。

## 3. 数据模型（破坏性重构）

`plugins` 表新结构（`backend/migrations/` 新增迁移，重建表）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | TEXT PK | 插件 ID（native 时 = 目录名） |
| `name` | TEXT | 显示名称 |
| `icon` | TEXT | 图标标识 |
| `route` | TEXT UNIQUE | 前端路由路径 |
| `type` | TEXT | `access` / `native` |
| `iframe_url` | TEXT | access：嵌入页面地址（可选） |
| `api_url` | TEXT | access：API 服务地址（可选） |
| `endpoints` | JSON | access：API 端点白名单 |
| `caddy_rules` | TEXT | access：Caddy 规则片段（可选） |
| `permission` | TEXT | 访问所需权限，空 = 无需额外权限 |
| `sort_order` | INT | 排序权重 |
| `is_active` | BOOL | 启用开关 |
| `status` | TEXT | native 运行态：`running/stopped/error/missing`（host 回写） |
| `manifest_json` | JSON | native：manifest 缓存 |
| `created_at` / `updated_at` | TIMESTAMPTZ | 时间戳 |

- `access`：管理 API 全 CRUD；`caddy_rules` 变更触发落盘 + reload。
- `native`：记录由 PluginHost 按 manifest 自动 upsert；管理 API 仅允许修改权限 / 启用状态，
  不可创建 / 删除（同现有"原生同步保留管理员配置"语义）。

## 4. 官方插件 submodule 组织

- 结构：`backend/plugins/<id>/` 为 submodule（`.gitmodules`），指向官方插件独立仓库。
- 当前阶段：**本地目录占位 + 配置 `.gitmodules`，仓库 URL 后补**。
  即先创建 `backend/plugins/esxi-admin/` 等目录（含占位 README / manifest 模板），
  并在 `.gitmodules` 中登记条目，URL 暂时留空或标注 TODO，待官方仓库就绪后执行
  `git submodule add <url> backend/plugins/<id>` 填充。
- 用户自定义插件：不放 submodule，源码与产物均在本地自行维护，运行时投放 `PLUGINS_DIR`。

## 5. access 与 Caddy 交互

```
前端 plugins-admin → 保存 CaddyRules → 后端校验 / 落盘 <plugin_caddy_dir>/<id>.caddy
  → 执行 reload 命令（可配置）→ Caddy 热生效
Caddyfile 主文件尾部：import plugins.d/*.caddy
```

- 新增环境变量：
  - `PLUGINS_DIR`：插件运行时目录（默认部署机 `<app>/plugins`）
  - `PLUGIN_CADDY_DIR`：插件 Caddy 规则目录（默认 `/etc/caddy/plugins.d`）
  - `CADDY_RELOAD_CMD`：reload 命令（默认 `systemctl reload caddy`；为空或不可用则仅落盘并告警，
    本地 dev 无 Caddy 不报错）
- 内置 Caddyfile 瘦身：仅保留监听端口、`/api`、`/native`、`/healthz` 反代后端、fallback 到前端、gzip。
- `esxiadmin` 迁移为 access 插件：其 ESXi 反代规则（`/esxi/*`、`/ui/*` 等含 `{env.ESXI_UPSTREAM}` 的规则）
  作为该插件 `CaddyRules` 的默认值，从内置 Caddyfile 迁出，交由插件栏管理。
- Caddy 规则作用域：允许用户写任意 `handle` 片段（管理员权限 `plugin:manage` 才可写），
  跨插件路径冲突由 `handle` 声明顺序决定，文档注明作用域与安全提示。

## 6. 权限模型（保留并复用）

- 三层不变：组级 `plugin:view` → 每插件 `IsActive` 闸门（native 额外校验进程健康）→
  声明 `Permission` 硬校验（`nativeGate` / proxy handler 强制执行）。
- `X-PortalT-User / X-PortalT-Role / X-PortalT-Perms` 身份透传逻辑保留，proxy 白名单匹配保留
  （`FindEndpoint` + 方法 / 路径精确匹配）。

## 7. 开发阶段设计

> 每阶段完成必须 `go test ./... -count=1` 全绿 + 提交前 subagent 审查 + 文档同步，
> 遵循仓库协作约定（中文注释 / Conventional Commits / testify）。

### Phase 1：协议与仓库骨架（无破坏）
- 目标：打好协议与目录基础，不触碰现有功能。
- 产出：
  1. `backend/proto/plugin/v1/plugin.proto`：定义 `Handshake / Health / Shutdown / Notify` 与消息结构。
  2. protoc 生成 Go 代码（生成产物提交入库，CI 不装 protoc）；`backend/go.mod` 新增 `google.golang.org/grpc`。
  3. `backend/internal/pluginpkg/manifest.go`：`manifest.json` 解析 / 校验
     （ID / Name / Icon / Route / SortOrder / Permission / HealthInterval）。
  4. 官方插件 submodule 占位：`backend/plugins/<id>/` 目录 + `.gitmodules`（URL 后补）+ manifest 模板 + README。
  5. `docs/plugins.md` 重写骨架（access / native 两类 + manifest 契约 + Caddy 规则约定）。
- 验收：`go build ./...` 通过；submodule 结构就绪；不破坏现有插件运行。
- 状态：**已完成**（proto/pluginpkg/submodule 约定/manifest 模板已落地，`go build ./...` 通过）。

### Phase 2：access 插件（收敛 + Caddy 交互）
- 目标：把 iframe / proxy 收敛为 access，并打通 Caddy 规则链路。
- 产出：
  1. `domain/plugin.go`：类型收敛为 `access | native`；`IframeURL / ApiURL / Endpoints` 共存；新增 `CaddyRules`。
  2. `backend/migrations/`：破坏性迁移重建 `plugins` 表（新字段 + 新列）。
  3. `gormstore.PluginRepository` 同步新 model；`memory` / `postgres` 仓库适配。
  4. 管理 API `api/v1/plugin.go`：access 全 CRUD，校验至少提供 iframe_url 或 api_url+endpoints 之一；
     支持 `CaddyRules` 读写。
  5. `api/v1/plugin_proxy.go`：接受 access 类型（原 proxy 逻辑保留：白名单 + 身份透传 + isURLSafe 防 SSRF）。
  6. 新增 `pluginhost/caddy.go`：规则校验（`caddy validate` 若可用）、落盘 `plugins.d/<id>.caddy`、
     触发 reload；删除 / 停用插件时移除对应文件。
  7. 前端：
     - `pages/plugins/[...slug].vue`：`type === 'access'` 一页双区块（iframe + API 面板）。
     - `pages/plugins-admin.vue`：access 表单增加 Caddy 规则编辑器 + 保存 / 重载状态提示。
  8. `esxiadmin` 迁移：native → access，ESXi 反代规则迁入其 `CaddyRules` 默认值；
     `cmd/server/main.go` 移除 esxiadmin 编译期注册；内置 Caddyfile 瘦身。
- 验收：access 插件 CRUD / 代理 / Caddy 落盘全链路可用；旧 iframe / proxy 配置语义经新 API 表达。
- 状态：**已完成**（迁移 006 重建表、domain/仓储/API 收敛、`pluginhost/caddy.go` 落盘+reload、
  前端一页双区块与插件管理页、esxi-admin 迁移 access 种子、移除 cron 示例；`go test ./...` 全绿、
  `go build/vet` 干净、`npm run build` 通过）。注：「内置 Caddyfile 瘦身」未随本阶段实施——
  内置 `esxi_proxy_routes` 保留，esxi-admin 的 `caddy_rules` 默认值为其副本（双份共存幂等），
  瘦身迁入插件栏列为后续迁移项。

### Phase 3：native 进程化 + 热加载 + 生命周期（核心）
- 目标：native 变为独立进程，支持热加载与生命周期管理。
- 前置：**gin 动态路由 spike 已完成**（`pluginhost/route_spike_test.go`，结论记录于文件头）：
  - 方案 A（前缀占位路由 + manager 内部分发）可行：一条固定通配路由
    `/api/v1/plugins/native/:pluginId/*path` 承载任意插件，`:pluginId` 与 `*path` 可共存，
    多插件不冲突，运行时无需重注册 gin 路由，天然适配热加载。**采用方案 A。**
  - 方案 B（启动前全量扫描重建路由）否决：gin 对同前缀 `handle` 重复注册 panic
    （`http: multiple registrations`），动态增删路由需重建 engine，复杂且易错。
- 产出：
  1. `backend/internal/pluginhost/`：
     - `manager.go`：进程监督（spawn / kill / restart，状态机，退避 / 崩溃限制）。
     - `loader.go`：扫描 `PLUGINS_DIR` → 校验 manifest + 可执行文件 → upsert DB；新插件默认禁用。
     - `watcher.go`：fsnotify 监听目录变动（新增 / 替换 / 删除）。
     - `grpc_client.go`：连接 / 健康探测 / 钩子调用（含超时与重连）。
     - `http_proxy.go`：校验插件上报 HTTP 地址为回环后反代 API 与静态。
  2. `registry.go` 重构：编译期注册 → manager 动态注册 / 注销；`MountAPI / MountStatic` 改为运行时动态。
  3. 生命周期钩子：启用 / 停用 / 升级 / 配置变更的 Notify 链路。
  4. `cmd/server/main.go`：初始化 `pluginhost.Manager`，替换 `builtinPlugins()`；
     `SyncNativePlugins` 改为 manager 动态驱动；移除 `NativeDeps`。
  5. 测试：`pluginhost` 单元测试（fake 插件进程）+ 集成测试（真实 spawn）。
- 验收：新增 / 替换 / 删除插件二进制时热生效；进程崩溃自动重启；权限三层校验仍生效。
- 状态：**已完成**（`pluginhost/manager.go` 进程监督+健康探测+退避重启、`watcher.go` fsnotify
  热加载、`Load/upsert` 扫描与 DB 同步、反代 `api/v1/native_proxy.go`（方案 A 前缀占位路由）、
  管理 API 生命周期（native 仅改权限/启用、重启端点）、示例插件 `plugins/examples/hello`、
  单元测试 + `-tags integration` 真实 spawn 全生命周期；`go test ./... -count=1` 全绿 +
  `go build/vet` 干净。注：原计划拆分的 `loader.go / grpc_client.go / http_proxy.go` 已并入
  `manager.go` 与 `api/v1/native_proxy.go`（gin 上下文依赖使反代放 api 层），旧 `internal/plugins`
  编译期注册包已整体删除）

### Phase 4：装配 + 部署 + 文档
- 目标：部署闭环 + 前端管理增强 + 全量文档。
- 产出：
  1. `deploy/install.sh`：创建 `PLUGINS_DIR`、构建官方 submodule 插件产物、
     systemd 补 `PLUGINS_DIR / PLUGIN_CADDY_DIR / CADDY_RELOAD_CMD`、
     Caddyfile 主文件加 `import plugins.d/*.caddy`。
  2. `deploy/update.sh`：插件目录备份 / 回滚。
  3. 前端 `plugins-admin.vue`：native 行显示运行状态（轮询 health）、启用 / 禁用 / 重启按钮；
     access 行显示 Caddy 规则状态（后端计算 `caddy_applied` 字段）。
  4. 文档全量同步：`docs/plugins.md`、`docs/interfaces.md`、`.env.example`（新变量）、
     `About.md` 进度表、`docs/README.md` 索引。
- 验收：生产部署一键生效；管理界面可管理两类插件；文档与实现一致。
- 状态：**已完成**（install.sh 创建 PLUGINS_DIR 与 systemd env、构建官方插件循环、Caddyfile 加 import plugins.d/*.caddy、/etc/caddy/plugins.d 目录；update.sh 插件备份/回滚/重建；前端 Caddy 规则落盘状态 + native 轮询启停重启；docs 与 About.md 同步；`go test ./... -count=1` 全绿 + `go build/vet` 干净 + `npm run build` 通过）。

## 8. 风险与关键点

1. **gin 动态路由**：最大技术风险，须在 Phase 3 先行 spike。
2. **gRPC 端口分配**：PortalT 分配空闲端口经 env 下发，避免固定端口冲突。
3. **Caddy 规则安全**：规则为原始 Caddy 片段，仅 `plugin:manage` 可写；路径冲突由声明顺序决定，文档注明。
4. **submodule 网络依赖**：官方插件仓库需可访问；无网络时 install 跳过构建并提示。
5. **spawn 进程安全**：仅执行 `PLUGINS_DIR` 内文件；插件进程降权 / 资源限制可选。
6. **Windows 下旧 `.so` 方案不可行**（现状注释）——进程化方案天然解决跨平台与任意语言。

## 9. 明确不在本期范围

- cron 插件：暂移除，未来重新实现（不在本计划内设计其迁移路径）。
- gRPC-gateway 合并端口：不做，采用 gRPC + HTTP 双端口。
- 插件市场 / 远程下载：不做，官方插件走 submodule，用户插件本地投放。
