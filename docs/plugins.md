# PortalT 插件开发指南

PortalT 的插件机制收敛为**两类**，均为门户侧"菜单项 + 页面"的扩展点：

| 类型 | 运行形态 | 配置来源 | 页面 | Caddy 交互 |
|------|----------|----------|------|------------|
| `access` | 纯配置型（无进程） | `plugins` 表（管理界面/API） | iframe 嵌入 + API 面板（可共存） | **是**：`caddy_rules` 字段落盘 `plugins.d/` 并 reload |
| `native` | 独立进程（PortalT spawn 监督） | `manifest.json` + DB 同步 | `/native/<id>/`（插件自带 HTTP 服务） | 否 |

> 状态：`access` 已实现（合并旧 iframe/proxy）；`native` 进程化（重构 Phase 3）**已实现**：
> PluginHost 扫描 `PLUGINS_DIR` spawn/监督插件进程、gRPC 控制面握手 + HTTP 数据面反代、
> fsnotify 热加载、崩溃自动重启、管理 API 生命周期（启用/停用/重启）。
> 示例插件见 `backend/plugins/examples/hello/`。
> **Phase 4 已实现**：部署脚本创建 `PLUGINS_DIR`、`/etc/caddy/plugins.d`；systemd 注入相关环境变量；
> Caddyfile 主站 `import plugins.d/*.caddy`；管理界面 native 行轮询运行态 + 启用/停用/重启按钮、
> access 行显示 Caddy 规则落盘状态（`caddy_applied`）；更新脚本备份插件目录。

## access 插件：纯配置型

无后端代码，管理界面（`/plugins-admin`，`plugin:manage`）或管理 API 配置即可生效。
一份 access 插件可同时提供三种能力，**任意共存**，仅提供一种时退化为旧行为：

- `iframe_url`：门户 iframe 嵌入页面。允许外部 `http(s)` 地址，或门户内相对路径
  （如 `/esxi/ui/`，由插件自己的 Caddy 规则反代到目标）。保存时校验 scheme 防注入。
- `api_url` + `endpoints`：标准 API 白名单代理（见 `docs/interfaces.md` 代理一节）。
  门户只转发白名单内的端点并注入调用者身份头，不暴露内部接口。
- `caddy_rules`：原始 Caddy `handle` 片段，落盘到 `<PLUGIN_CADDY_DIR>/<id>.caddy` 并触发
  reload 热生效。

### access 与 Caddy 交互

```
前端 plugins-admin → 保存 CaddyRules → 后端校验 → 落盘 <PLUGIN_CADDY_DIR>/<id>.caddy
  → 执行 <CADDY_RELOAD_CMD> → Caddy 热生效
Caddyfile 主文件尾部：import plugins.d/*.caddy
```

- 环境变量：
  - `PLUGIN_CADDY_DIR`：插件 Caddy 规则目录（默认部署机 `/etc/caddy/plugins.d`）；
    **为空 = 本地 dev 无 Caddy，保存只接受不落盘不报错**
  - `CADDY_RELOAD_CMD`：reload 命令（默认 `systemctl reload caddy`）；为空或失败时规则已落盘
    但提示"将随下次 reload 生效"
- 删除插件会移除对应 `<id>.caddy` 文件并 reload；**停用（`is_active=false`）或清空 `caddy_rules`**
  同样移除其文件（停用插件不再占用反代路径）。
  启动时 `SyncAll` 全量对齐：启用且含规则的 access 插件写入，磁盘上多余/孤儿文件清理，随后 reload 一次。
- 管理页头部"**重载 Caddy**"按钮走 `POST /api/v1/plugins/caddy-reload`（`plugin:manage`）：
  以数据库为准**全量对齐**规则文件并 reload（补写未落盘、清理孤儿文件），用于规则保存后
  reload 失败或手工改盘后的一次性主动修复；reload 失败提示"已落盘、将随下次 reload 生效"。
- 落盘前若环境存在 `caddy` 可执行文件且规则不含 `{env.*}` 占位符，后端先 `caddy validate`
  （包装为 `:0` 最小站点）校验片段语法，校验失败不落盘——避免语法错误规则残留导致后续 reload 持续失败。
  含 `{env.*}` 占位符的规则交由 Caddy 加载期处理（其值依赖运行时环境，如 `{env.ESXI_UPSTREAM}`）。
- **作用域与安全**：`caddy_rules` 为原始 Caddy 片段，仅 `plugin:manage`（管理员）可写；
  跨插件路径冲突由 `handle` 声明顺序决定。只应写本插件的 `handle` 块，不要包含站点监听、
  全局指令等（会破坏 Caddyfile 语法）。部署机建议规则落盘后做 `caddy validate` 校验。
- 现状说明：`esxi-admin` 插件的 `caddy_rules` 默认值（`DefaultESXIAdminCaddyRules`）是 ESXi
  反代规则的**唯一来源**（含 `/esxi/*`、`/ui/*`、`/screen*`、`/sdk*`、`/ticket*` 等 handle，
  目标主机由 `{env.ESXI_UPSTREAM}` 运行时解析）；**内置 `caddy/Caddyfile` 已瘦身、不再包含
  ESXi handle**——停用/删除该插件即移除规则文件，`/esxi/*` 不再反代（访问自然收回）。
- **鉴权闸口**：默认规则每个 handle 先经 `forward_auth 127.0.0.1:8080 { uri
  /api/v1/auth/gate?perm=esxi-admin:use }` 回调门户鉴权（校验请求 cookie 中的
  access/refresh 令牌 + `esxi-admin:use` 权限），未登录 401、无权限 403，放行才反代；
  旧的无鉴权默认值（`DefaultESXIAdminCaddyRulesV1`）会被启动 seed 自动升级为新默认值
  （精确匹配才覆盖，保留管理员自定义）。
- 网关对身份透传不依赖 Caddy `copy_headers`——闸口直接读同源 cookie，无需注入额外头。

### 示例：esxi-admin

`esxi-admin`（`/esxi-admin`，权限 `esxi-admin:use`，仅 admin 默认持有）由旧 native 插件迁移为
**access 种子数据**：启动幂等 seed，`iframe_url = /esxi/ui/`（门户内相对路径），
`caddy_rules` 内置 ESXi Host Client 反代默认值（`/esxi/*`、`/ui/*`、`/screen*` 等含
`{env.ESXI_UPSTREAM}`，**每个 handle 带 `forward_auth` 鉴权闸口，见上文**）。
前端插件页按 `/api/v1/platform` 三态渲染：
未接入 ESXi（provider ≠ esxi）→ 占位提示；已接入 → iframe 嵌入；加载失败 → 提示检查上游配置。
插件页挂载期间每 5 分钟静默续期 access cookie（access_token 仅 15 分钟，iframe 内
`/esxi/*` 子请求经闸口校验），长会话不中断。

### access 插件权限

三层模型（全部保留复用）：

1. 组级：菜单/代理入口需通用 `plugin:view`
2. 每插件闸门：`is_active` 必须启用
3. 声明权限：`permission` 非空时经 `Plugin.CanAccess` 强制校验（用户角色矩阵不具备 → 403）；
   声明值必须存在于权限字典（管理 API 校验），可留空表示无需额外权限

`X-PortalT-User / X-PortalT-Role / X-PortalT-Perms` 身份透传与代理白名单匹配
（`FindEndpoint`，方法+路径精确匹配）保持不变。

## native 插件（进程化，Phase 3 已实现）

### 运行时目录与热加载

```
<PLUGINS_DIR>（部署机默认 <app>/plugins；为空 = 禁用 native 能力）
plugins/
├── <id>/                 # 目录名 = 插件 ID
│   ├── plugin            # 可执行文件（Windows 为 plugin.exe；任意语言编译产物）
│   ├── manifest.json     # 元数据 / 权限 / 钩子配置
│   └── static/           # 插件静态前端（可选，挂 /native/<id>/）
└── ...（fsnotify 监视整个根目录，300ms 去抖合并）
```

- 新增目录 → 校验 manifest + 可执行文件 → upsert 记录（**默认 `is_active=false`**，管理员手动启用）
- manifest / 二进制替换 → 停止旧进程并重启（升级，先停再启）
- 目录删除 → 停止进程，DB 记录标记 `missing`（不自动删除，保留管理员配置）
- `manifest.id` 必须等于目录名，否则该插件被跳过不入库

### 进程通信与端口分配

```
PortalT ──spawn──▶ 插件进程（env：PORTALT_PLUGIN_ID / PORTALT_PLUGIN_GRPC_PORT / PORTALT_PLUGIN_HTTP_PORT）
   ├─ gRPC 控制面（127.0.0.1:<PortalT 分配空闲端口>）
   │    Handshake  Health  Shutdown  Notify(event)
   └─ HTTP 数据面（127.0.0.1:<PortalT 分配空闲端口>，插件绑定后经 Handshake 确认）
```

- **端口由 PortalT 分配并经环境变量下发**（避免 manifest 固定端口冲突），插件绑定后经
  `HandshakeRequest.HttpPort` 确认；宿主随后探测 `/healthz` 验证 HTTP 数据面回环可达。
- 协议见 `backend/proto/plugin/v1/plugin.proto`；`Shutdown` 为优雅停机 RPC，宿主兜底 kill。
- 生命周期状态机：`installed → disabled → enabled → running`
  ↘ `error`（健康探测失败/崩溃，退避重启上限 5 次）↘ `missing`（目录被删除）
- 钩子：启用 `Notify(enabled)`+spawn；停用 `Notify(disabled)`+`Shutdown()`+kill；
  升级先停再启；重启 `Notify(restarting)`
- 崩溃自动重启：健康探测失败或进程意外退出 → 指数退避（2s 起，封顶 30s）重启，连续失败
  超过 5 次进入 `error` 不再自动重启（人工处理）
- 运行态回写 `plugins.status`：`running / stopped / error / missing`
- 安全：仅执行 `PLUGINS_DIR` 内文件（防路径穿越）；反代目标恒为宿主返回的回环地址
  （`HTTPAddress`），不信任插件上报的主机

### 管理 API（native 生命周期）

- native 记录**不可手动创建/删除**（宿主按 manifest 自动 upsert；删除插件目录即标记 missing）
- `PUT /api/v1/plugins/:id`：native 仅允许修改 `permission` 与 `is_active`（manifest 字段不可改）；
  `is_active` 变更经宿主触发 spawn/停进程
- `POST /api/v1/plugins/:id/restart`：重启 native 进程（`plugin:manage`）

### 反代入口

- API：`/api/v1/plugins/native/:pluginId/*path`（`plugin:view` + 插件启用闸门 + 声明权限 +
  注入 `X-PortalT-User/Role/Perms`），反代到插件 HTTP 数据面
- 静态：`/native/<id>/*`（公开，供 iframe 嵌入；仅要求插件运行中）。为防插件 API 端点
  无鉴权暴露到公网，静态路径对 `/api/*` 一律拒绝——插件数据端点必须走上面的鉴权 API。

### manifest.json 契约（`internal/pluginpkg` 已实现解析/校验）

模板见 `backend/plugins/manifest.example.json`；示例见 `backend/plugins/examples/hello/manifest.json`：

```json
{
  "id": "my-plugin",
  "name": "我的插件",
  "icon": "mdi:puzzle",
  "route": "/my-plugin",
  "sort_order": 100,
  "permission": "",
  "health_interval_seconds": 30
}
```

- `id` 必须等于目录名；`route` 形如 `/id`；`permission` 声明最小访问权限（须在权限字典内，
  空 = 无需额外权限）；`health_interval_seconds` 为宿主健康探测间隔（秒，缺省默认值）
- `backend/plugins/examples/hello/cmd/hello/main.go` 为最小 native 插件模板（Go）：gRPC 控制面
  （Handshake/Health/Shutdown/Notify）+ HTTP 数据面（/healthz + /api/hello + 首页）；
  集成测试真实 spawn 该插件验证全生命周期，`go test -tags integration ./internal/pluginhost/...`

## 插件仓库模板

`backend/plugins/template/` 是一个完整的、可独立构建的 native 插件仓库模板，供开发者在 fork/clone 后自定义。详见 `backend/plugins/README.md`。

### 官方插件 submodule 组织

- `backend/plugins/<id>/` 为独立 git 仓库 submodule（目录结构见 `backend/plugins/README.md`）
- 当前无官方 native 插件（`esxi-admin` 为 access 配置型，无需 submodule）；`.gitmodules` 已登记约定
- 用户插件：本地自行维护源码与产物，直接投放预编译产物到 `PLUGINS_DIR`（任意语言），不经本仓库

## 常见坑

- `caddy_rules` 只写本插件的 `handle` 块；写全局指令会导致 Caddy reload 失败
- access 代理只暴露白名单端点——未声明的方法/路径一律 403
- `iframe_url` 用门户内相对路径时必须配套同插件的 Caddy 规则，否则浏览器 404/空白
- 插件声明权限须在权限字典内，否则管理界面保存报错
