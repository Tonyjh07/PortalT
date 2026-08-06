# PortalT 接口文档

> 仅记录当前已实现的接口与契约（截至 Phase 9）。

## 响应格式

所有 REST API 统一采用以下格式（`internal/api/response` 包）：

```json
{ "code": 200, "message": "success", "data": { ... } }
```

错误格式：

```json
{ "code": 4001, "message": "用户名或密码错误" }
```

### 已实现的错误码

| 码 | 含义 | HTTP 状态 |
|----|------|-----------|
| 200 | 成功 | 200 |
| 4001 | 用户名或密码错误 | 401 |
| 4002 | 令牌无效或已过期 | 401 |
| 4003 | 缺少访问令牌 | 401 |
| 4004 | 请求参数错误 | 400 |
| 4005 | 权限不足 | 403 |
| 4006 | 资源不存在 | 404 |
| 4007 | 操作在当前状态不允许 | 409 |
| 4008 | 资源冲突（如用户名已存在） | 409 |
| 5000 | 服务器内部错误 | 500 |

## 已实现 HTTP 接口

### 健康检查

```
GET /healthz
```

- 响应：`200 OK`，正文 `PortalT v0.1`（text/plain）

### 认证

```
POST /api/v1/auth/login
```

- 请求：`{"username":"admin","password":"admin123"}`
- 响应：`{"access_token":"...","refresh_token":"...","expires_in":900,"user":{...}}`
- 错误：4001 凭据无效（401）

```
POST /api/v1/auth/refresh
```

- 请求：`{"refresh_token":"..."}`
- 响应：`{"access_token":"...","expires_in":900}`
- 访问令牌不能用作刷新令牌（类型隔离）

```
GET /api/v1/auth/me
```

- 需 `Authorization: Bearer <access_token>`
- 响应：当前用户信息（无密码字段）+ `permissions`（当前用户的权限集合，排序确定，
  来自角色矩阵 `AttachPermissions` 装载；未装载时回退内置角色表）

### 虚拟机管理（需认证）

权限：所有路由需 `vm:view`；电源操作分别需 `vm:start`/`vm:stop`/`vm:restart`。
**资源级授权**：配置了授权表（`vm_access`，默认启用）时，`vm:manage` 用户放行全部；
其余用户仅可见/可操作 `vm_access` 中授权的 VM（列表过滤，详情/状态/操作未授权按 404
处理防枚举）。

```
GET /api/v1/vms                    → 虚拟机列表（资源级过滤）
GET /api/v1/vms/:id                → 虚拟机详情
GET /api/v1/vms/:id/status         → 实时状态（从平台回刷，轮询用）
POST /api/v1/vms/:id/start         → 启动（仅关机/挂起态，否则 409/4007）
POST /api/v1/vms/:id/stop          → 停止（仅运行态，否则 409/4007）
POST /api/v1/vms/:id/restart       → 重启（仅运行态，否则 409/4007）
```

- 电源操作成功后返回最新 VM（服务层从平台回刷并落库）
- 服务启动时执行一次全量同步（`SyncVMs`），目录为空时列表为空数组

### 动态菜单（需认证）

```
GET /api/v1/menu
```

- 需 `plugin:view`（viewer 角色无此权限 → 403）
- 返回当前用户可见的已启用插件，按 `sort_order` 升序
- 过滤规则：插件已启用 + 无 `permission` 要求或用户具备对应权限；权限集合优先取角色矩阵
  （`AttachPermissions` 中间件），未加载时回退 `Plugin.CanAccess` 的内置表

### 插件管理（需认证 + `plugin:manage`，仅管理员）

```
GET    /api/v1/plugins     → 全部插件（含停用）
POST   /api/v1/plugins     → 注册插件（id 可选，缺省自动生成；name/route 必填）
PUT    /api/v1/plugins/:id → 更新插件（全字段覆盖）
DELETE /api/v1/plugins/:id → 删除插件（native 类型插件由代码托管，不建议删除）
```

- 请求体：`{"id","name","icon","route","type","iframe_url","api_url","endpoints","permission","sort_order","is_active"}`
- `type`：`iframe`（嵌入页面，默认）/ `proxy`（脚本标准 API 代理）/ `native`（Go 原生插件）
- `proxy` 类型必填 `api_url`（http/https）与 `endpoints`（方法+路径白名单，路径以 `/` 开头）
- `native` 类型不能通过接口创建，由启动时 `SyncNativePlugins` 按代码注册表 upsert（保留管理员对权限/启用状态的设置）

### 脚本插件标准 API 代理（需认证 + `plugin:view`）

```
GET/POST/PUT/DELETE /api/v1/plugin-proxy/:pluginId/*path
```

- 仅转发插件 `endpoints` 白名单内的端点（方法+路径精确匹配，路径忽略前导斜杠），其余 404
- 转发目标：`api_url + path`（保留 query/body/Content-Type），注入 `X-PortalT-User` /
  `X-PortalT-Role` / `X-PortalT-Perms`（用户权限集合的 JSON 数组，排序确定，供插件侧二次鉴权）头
- 响应**不带信封**：状态码、响应头、body 原样透传（目标不可达 → 502）
- 插件未启用 → 403；插件不存在 → 404

### 原生插件（需认证 + `plugin:view`，Phase 9）

- API：`/api/v1/plugins/native/:pluginId/...`（路由由插件自身 `Mount` 挂载；插件在 plugins 表中
  不存在或已停用 → 404）
- **权限声明**：插件 `Info()` 的 `Permission` 为最小访问权限——`nativeGate` 强制校验
  （用户权限集合/角色矩阵不具备 → 403），同时作为启动同步的默认值（plugins 表记录权限为空时
  才回填，管理员配置不覆盖）；声明值须在权限字典内（管理 API 层校验，见插件管理一节）
- 静态前端：`/native/:pluginId/`（公开托管内嵌页，数据访问一律走鉴权 API；前端 iframe 用
  `/native/<id>/` 嵌入）
- 机制：`internal/plugins.Registry` 启动时注册，`Deps` 注入 `Provider`（平台类型）与
  `WebURL`（平台 Web 界面地址，如 ESXi 的 `https://host/ui/`）
- 完整开发规范见 [plugins.md](./plugins.md)
- 示例插件：`esxi-admin`（iframe 嵌入 ESXi Web 管理界面，`internal/plugins/examples/esxiadmin`，
  声明 `plugin:view`）、`cron`（内存定时任务，`internal/plugins/examples/cron`，声明 `plugin:manage`）

### 用户管理（需认证 + `user:manage`，管理员）

```
GET    /api/v1/users     → 全部用户
POST   /api/v1/users     → 创建用户（username/password 必填，同名 409/4008；role 必须是角色表中存在的角色）
PUT    /api/v1/users/:id → 更新用户（role/email；password 可选，留空不修改）
DELETE /api/v1/users/:id → 删除用户（不能删除自己）
GET    /api/v1/users/:id/vm-access  → 用户当前授权的 VM ID 列表（{vm_ids:[...]}）
PUT    /api/v1/users/:id/vm-access  → 全量替换用户授权（{vm_ids:[...]}，空数组清空）
```

- 用户角色不再局限于内置三角色：任意存在于角色表（含自定义角色）的角色值均可分配

### 角色权限（需认证 + `user:manage`，管理员）

```
GET    /api/v1/roles           → 全部角色（内置 admin/user/viewer + 自定义）
GET    /api/v1/roles/permissions → 权限字典（10 项，中文描述）
POST   /api/v1/roles           → 创建自定义角色（id/name 必填；id 仅小写字母/数字/下划线/连字符 1-32 位）
PUT    /api/v1/roles/:id       → 更新角色权限集合（内置角色可改）
DELETE /api/v1/roles/:id       → 删除角色（内置角色不可删）
```

- 创建/更新时权限必须全部来自权限字典（未知权限 → 400），自动去重
- 启动时 `EnsureDefaultRoles` 幂等写入内置三角色种子、`EnsureDefaultPermissions` 幂等写入权限字典
  （均已入库，权限矩阵与字典以数据库为准）；`RoleLoader` 缓存角色→权限矩阵，
  权限变更后 `Invalidate` 使缓存失效
- `RequirePermission` 校验顺序：`auth.perms`（角色矩阵）→ 回退 `user.HasPermission`（内置表）

### 远程桌面隧道（需认证，Phase 8）

```
GET /api/v1/guac/ws/:vmId
```

- 需 `vm:console`（Phase 10 从 vm:view 拆出）+ 资源级授权（未授权 VM 按 404 处理）；
  认证支持 `Authorization: Bearer <token>` 与 `?token=<token>` 两种方式
  （浏览器 WebSocket 无法携带自定义请求头，因此支持查询参数；`token` 值会在首个 `?`/`&` 处截断，
  以容忍 guacamole-common-js 追加的 `?<connect data>` 后缀）
- 隧道模式由环境变量选择（`GuacHandlerForEnv`）：
  - `GUACD_URL` 已配置 → **guacd 原生隧道（推荐）**：服务端直连 guacd(:4822) 并完成
    select/args/size/connect/ready 握手，连接参数全部来自 VM metadata `guac.*` 键（见下表），
    浏览器侧不可覆盖目标与凭证
  - `GUAC_URL` 已配置 → 旧模式：转发 Guacamole Web 应用 WebSocket 隧道（注入
    `X-PortalT-User`/`X-PortalT-Role`/`X-PortalT-VMID` 头）
  - 均未配置 → 503
- 握手失败以 WS Close 1001（内部错误）关闭；guacd 不可达返回 502；VM 不存在返回 404
- 子协议：升级时回显 `guacamole`（guacamole-common-js 固定携带该子协议）
- 客户端内部指令（稳定性 ping，opcode 为空）由服务端回显，不转发 guacd

#### VM metadata `guac.*` 连接参数契约

| 键 | 必填 | 说明 |
|----|------|------|
| `guac.protocol` | 否（默认 vnc） | vnc / rdp / ssh / telnet |
| `guac.hostname` | 否 | 目标主机；缺省回退 VM `ip_address` |
| `guac.port` | 否 | 目标端口；缺省 vnc 5900 / rdp 3389 / ssh、telnet 22 |
| `guac.username` / `guac.password` | 视目标而定 | 登录凭证 |
| `guac.width` / `guac.height` | 否 | 初始分辨率（默认 1280×800） |
| `guac.security` / `guac.domain` / `guac.read-only` / `guac.autoretry` / `guac.color-depth` | 否 | 透传协议参数 |
| `rustdesk.id` | 否 | RustDesk 设备 ID（详情页显示一键连接入口） |
| `rustdesk.password` | 否 | RustDesk 连接密码（只写不回） |
| `rustdesk.server` / `rustdesk.key` | 否 | 自建 hbbs 地址 `host:port` / 服务器公钥（留空用官方服务器） |

> 敏感键脱敏：列表/详情/更新接口返回的 VM **不包含**键名匹配
> `password|passwd|secret|token` 的 metadata（凭证只写不回，仍存于库中供隧道使用）；
> 更新接口校验受控键：`guac.protocol` 仅限 vnc/rdp/ssh/telnet，`guac.port` 为
> 1–65535 整数，`guac.hostname` 非空，`rustdesk.id` 非空。

使用与故障排查见 [remote-desktop.md](./remote-desktop.md)。

### RBAC 中间件（internal/api/middleware/rbac.go）

- `RequirePermission(perm)`：需在 `AuthRequired` 之后使用
- 校验顺序：`auth.perms`（`AttachPermissions` 加载的角色矩阵）→ 回退 `user.HasPermission`（内置表）
- 用户无对应权限或未认证 → 403/4005（权限常量见 `internal/domain/permission.go`）
- `RequireAnyPermission(perms...)`：满足任一权限即通过

### 角色加载中间件（internal/api/middleware/role_loader.go）

- `RoleLoader`：缓存角色→权限集合（启动预载默认角色，`PermissionsFor` 懒加载未命中角色）
- `AttachPermissions(loader)`：将当前用户权限集合写入 gin.Context（`auth.perms`），`CurrentPerms(c)` 读取
- `Invalidate(role)`：角色更新后调用，下次请求重新加载

### 认证中间件（internal/api/middleware/auth.go）

- `AuthRequired(tokenManager)`：解析 `Bearer` 头或 `?token=` 查询参数（WebSocket 场景），成功后将用户存入 gin.Context
- `CurrentUser(c)`：读取当前用户；未认证返回 nil
- 失败统一返回 4002/4003

## 认证接口（internal/ports/auth.go）

```go
type AuthenticationProvider interface {
    Authenticate(username, password string) (*domain.User, error)
}

type TokenManager interface {
    GenerateAccessToken(user *domain.User) (string, error)
    GenerateRefreshToken(user *domain.User) (string, error)
    ParseAccessToken(token string) (*domain.User, error)
    ParseRefreshToken(token string) (*domain.User, error)
    AccessTTL() time.Duration
    RefreshTTL() time.Duration
}
```

- 错误哨兵：`ports.ErrInvalidCredentials`、`ports.ErrInvalidToken`
- 已实现：`internal/adapters/auth/local`（bcrypt，恒定时间比较防用户名枚举）
- 已实现：`internal/adapters/auth/jwt`（HS256，access 15 分钟 / refresh 7 天，类型声明隔离）
- 启动引导：`EnsureAdminUser(repo, ADMIN_USERNAME, ADMIN_PASSWORD)`（默认 admin/admin123，幂等）

## 仓储接口（internal/ports/repository.go）

```go
type VMRepository interface {
    Save(vm *domain.VM) error      // upsert语义
    FindByID(id string) (*domain.VM, error)
    FindAll() ([]*domain.VM, error)
    Delete(id string) error
}

type UserRepository interface {
    Save(user *domain.User) error
    FindByID(id string) (*domain.User, error)
    FindByUsername(username string) (*domain.User, error)
    FindAll() ([]*domain.User, error)
    Delete(id string) error
}
```

- 记录不存在返回 `ports.ErrNotFound`；参数无效返回 `ports.ErrInvalidArgument`
- 已实现：`internal/adapters/memory`（sync.RWMutex + map，100% 覆盖率，通过 -race）
- 已实现：`internal/adapters/gormstore`（方言无关 GORM 实现，postgres/sqlite 共用）
- 已实现：`internal/adapters/postgres`（薄包装 + PostgreSQL 方言迁移，原子 upsert `clause.OnConflict`，jsonb metadata 归一化）
- 已实现：`internal/adapters/sqlite`（薄包装 + SQLite 方言迁移，纯 Go 驱动无 CGO）

```go
// 权限字典（Phase 10）
type PermissionRepository interface {
    FindAll() ([]*domain.PermissionInfo, error)
    Exists(id string) (bool, error)                       // 权限是否在字典中
    EnsureDefault(perms []domain.PermissionInfo) error    // 幂等 seed，已存在不覆盖
}

// 虚拟机资源授权（Phase 10）
type VMAccessRepository interface {
    SetForUser(userID string, vmIDs []string) error   // 全量替换
    VisibleVMIDs(userID string) ([]string, error)
    IsAuthorized(userID, vmID string) (bool, error)
    DeleteForUser(userID string) error
}
```

## 虚拟化提供者接口（internal/ports/virtualization.go）

```go
type VirtualizationProvider interface {
    ListVMs() ([]*domain.VM, error)
    StartVM(id string) error
    StopVM(id string) error
    RestartVM(id string) error
    GetHostInfo() (*domain.HostInfo, error)
}
```

- 实现计划：esxi（Phase 4）/ mock / proxmox
- 领域服务经此接口编排，实现平台可移植
- 已实现：`internal/adapters/esxi`（govmomi，惰性连接 + 指数退避重试；ID=VM UUID，MOID 存 metadata；`make test-esxi` 用 vcsim 验证）
- 已实现：`internal/adapters/workstation`（VMware Workstation vmrest REST API，纯标准库无新依赖；`VIRT_PROVIDER=workstation` 本机调试，见下）
- 已实现：`internal/adapters/mock`（内存态模拟器，内置示例数据；`VIRT_PROVIDER=mock` 开发调试）

### VMware Workstation 适配器（internal/adapters/workstation）

Workstation 17+ 内置 REST API 服务 `vmrest`（默认 `http://127.0.0.1:8697`，Basic 认证）。启用：

```
cd "C:\Program Files (x86)\VMware\VMware Workstation"
vmrest.exe -C        # 设置凭证（保存到 %USERPROFILE%\vmrest.cfg），需管理员
vmrest               # 启动服务（HTTPS 需 -c 证书 -k 私钥）
```

- 配置：`VIRT_PROVIDER=workstation` + `VIRT_WS_URL/USERNAME/PASSWORD/INSECURE`（url 缺省本机 8697）
- 端点：`GET /api/vms` → id+path 列表，详情逐台查询；`PUT /api/vms/{id}/power` body 为裸字符串 `on/off/reset`，Content-Type `application/vnd.vmware.vmw.rest-v1+json`；`GET /api/vms/{id}/ip`（返回 `{"ip": "..."}`，关机/无 Tools 时 404 容错为空）；`GET /api/host`（404 时回退最小信息）
- 容错：状态映射对 `on`/`poweredOn` 等大小写变体归一；CPU/内存支持新旧字段名及子对象（`cpu.processors`、`memory.memory_MiB`）；name 缺省回退 vmx 文件名；电源状态取 `/power` 子接口、IP 取 `/ip` 子接口（详情接口仅含 id/cpu/memory）
- 远程桌面：IP 写入 `vm.ip_address` 字段，guacd 隧道缺省回退到该 IP 作为目标（无需写 metadata；手动配置 `guac.hostname` 优先级更高）

### 提供者工厂（internal/adapters/virt_factory.go）

```go
NewVirtualizationProvider(virtType string, config map[string]string) (ports.VirtualizationProvider, error)
```

| virtType | 配置键 | 说明 |
|----------|--------|------|
| `mock`（默认） | 无 | 内存模拟，含 3 台示例 VM |
| `esxi` | `url`（必填）、`username`、`password`、`insecure` | 连接延迟到首次调用 |
| `workstation` | `url`、`username`、`password`、`insecure` | url 缺省 `http://127.0.0.1:8697`，无真实环境时 401/拒连为预期错误 |

切换方式：`VIRT_PROVIDER=mock`、`esxi` 或 `workstation`；配置走通用 `VIRT_URL/USERNAME/PASSWORD/INSECURE`（缺省回退 `VIRT_ESXI_*`、`VIRT_WS_*`）。

## 业务服务（internal/domain/services/vm_service.go）

| 方法 | 说明 |
|------|------|
| `SyncVMs(ctx)` | 从提供者拉取全部VM保存入库，删除提供者中已不存在的陈旧记录；提供者报错时不做任何变更 |
| `ListVMs(ctx)` | 返回仓储中的全部VM |
| `GetVM(ctx, id)` | 按ID查询VM详情（不存在 → ErrNotFound） |
| `StartVM/StopVM/RestartVM(ctx, id)` | 电源操作：加载 → 校验状态规则（CanStart/CanStop/CanRestart）→ 调用提供者 → 回刷状态入库；违反状态规则 → `ErrInvalidOperation` |
| `GetVMStatus(ctx, id)` | 实时状态：优先从提供者拉取并回写仓储，提供者不可达时回退仓储缓存 |

## PluginRepository 接口（internal/ports/repository.go）

```go
type PluginRepository interface {
    Save(p *domain.Plugin) error              // upsert语义
    FindByID(id string) (*domain.Plugin, error)
    FindActive() ([]*domain.Plugin, error)    // 已启用，按 sort_order 升序
    FindAll() ([]*domain.Plugin, error)       // 全部（含停用）
    Delete(id string) error
}
```

- 已实现：`internal/adapters/gormstore`（共享层）+ `memory`；postgres/sqlite 为类型绑定包装
- `plugins` 表结构见迁移脚本 `migrations/001_init.up.sql`

## HostInfo JSON 契约（internal/domain/host.go）

| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 宿主机名称 |
| version | string | 平台版本 |
| cpu_model | string | CPU型号 |
| total_cpu / used_cpu | int | CPU 总核数/已用 |
| total_memory_mb / used_memory_mb | int | 内存（MB） |
| status | string | connected / disconnected |

辅助方法：`CPUUsagePercent()`、`MemoryUsagePercent()`（除零安全）。

## 领域模型 JSON 契约

以下结构体通过 `encoding/json` 序列化，字段名即前端 API 契约（Phase 6 起生效）。

### VM（internal/domain/vm.go）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 唯一标识 |
| name | string | 名称 |
| status | string | 电源状态：poweredOn / poweredOff / suspended / unknown |
| cpu | int | CPU 核数 |
| memory_mb | int | 内存（MB） |
| ip_address | string | IP 地址 |
| host | string | 宿主机 |
| metadata | object/null | 扩展元数据（jsonb），如远程桌面连接参数 |

业务方法：`CanStart()`（关机/挂起可启动）、`CanStop()`、`CanRestart()`（运行中可执行）、`IsRunning()`。

### User（internal/domain/user.go）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 唯一标识 |
| username | string | 用户名 |
| password | - | 密码哈希，JSON 序列化**隐藏** |
| email | string | 邮箱 |
| role | string | admin / user / viewer |

业务方法：`HasPermission(perm string) bool`、`IsAdmin()`。

### Plugin（internal/domain/plugin.go）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 唯一标识 |
| name | string | 显示名称 |
| icon | string | 图标标识（如 mdi:home） |
| route | string | 前端路由 |
| type | string | iframe / proxy / native（空=iframe） |
| iframe_url | string | 嵌入地址（iframe 类型） |
| api_url | string | 插件 API 服务地址（proxy 类型） |
| endpoints | array | 端点白名单 `[{method, path, name, description}]`（proxy 类型） |
| permission | string | 访问所需权限，空=无需权限 |
| sort_order | int | 排序权重（小在前） |
| is_active | bool | 是否启用 |

业务方法：`CanAccess(user *User)`（启用 + 权限双检查）、`IsEnabled()`、`FindEndpoint(method, path)`（白名单匹配，路径忽略前导斜杠）。

### RoleDefinition（internal/domain/role.go）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 角色 ID（admin / user / viewer / 自定义） |
| name | string | 显示名称 |
| description | string | 描述 |
| permissions | array | 权限 ID 集合 |

- 内置角色：admin（全部权限）、user（VM 电源 + plugin:view）、viewer（仅查看）
- `domain.Role` 为用户实体上的角色类型（string）；权限实体为 `RoleDefinition`，两者不冲突

## 权限常量（internal/domain/permission.go）

| 常量 | 值 | 说明 |
|------|-----|------|
| PERM_VIEW_ALL | view_all | 查看全部资源 |
| PERM_VM_VIEW | vm:view | 查看虚拟机 |
| PERM_VM_START | vm:start | 启动虚拟机 |
| PERM_VM_STOP | vm:stop | 停止虚拟机 |
| PERM_VM_RESTART | vm:restart | 重启虚拟机 |
| PERM_VM_MANAGE | vm:manage | 管理虚拟机（含资源级放行全部） |
| PERM_VM_CONSOLE | vm:console | 远程桌面/控制台（vm:view 拆出） |
| PERM_PLUGIN_VIEW | plugin:view | 查看插件 |
| PERM_PLUGIN_MANAGE | plugin:manage | 管理插件 |
| PERM_USER_MANAGE | user:manage | 管理用户 |

### 角色权限矩阵

内置三角色为种子数据（启动幂等写入），`admin` 全权限；`user` 具备 VM 电源操作、
远程桌面与插件查看；`viewer` 仅查看。**矩阵可在角色管理界面动态调整**
（`PUT /roles/:id`），不再局限于下表：

| 权限 | admin | user | viewer |
|------|:-----:|:----:|:------:|
| view_all / vm:view | ✅ | ✅ | ✅ |
| vm:start / vm:stop / vm:restart | ✅ | ✅ | ❌ |
| vm:console | ✅ | ✅ | ❌ |
| plugin:view | ✅ | ✅ | ❌ |
| vm:manage / plugin:manage / user:manage | ✅ | ❌ | ❌ |

自定义角色：任意权限子集；插件声明的权限在 API 层强制校验（nativeGate）。

## 数据库表结构（backend/migrations/）

| 表 | 关键字段 | 说明 |
|----|---------|------|
| users | id(PK), username(UNIQUE), password_hash, email, role, created_at | 用户账号 |
| vms | id(PK), name, status, cpu, memory_mb, ip_address, host, metadata(JSONB), created_at, updated_at | 虚拟机目录 |
| plugins | id(PK), name, icon, route(UNIQUE), type, iframe_url, api_url, endpoints, permission, sort_order, is_active, created_at, updated_at | 插件菜单（Phase 9 扩展 type/api_url/endpoints） |
| roles | id(PK), name, description, permissions(JSON 数组), created_at, updated_at | 角色权限矩阵（Phase 9，迁移 002） |
| permissions | id(PK), name(UNIQUE), description, created_at | 权限字典（Phase 10 起生效，启动幂等 seed） |
| vm_access | id(PK), user_id, vm_id, created_at（user_id+vm_id 唯一） | 虚拟机资源授权（Phase 10，迁移 004） |
| schema_migrations | version(PK), applied_at | 迁移版本追踪（已应用迁移自动跳过，幂等启动） |

- 迁移脚本：`001_init` / `002_roles` / `003_plugin_types` / `004_vm_access`（{up,down}.sql），按文件名顺序执行
- SQLite 方言迁移：`migrations/sqlite/`（`003` 的 ALTER ADD COLUMN 在 SQLite 无 IF NOT EXISTS，
  旧库重放报 "duplicate column name" 由迁移器视为已应用，兼容无版本表时期的存量库）
- `make test-integration` 自动应用迁移后测试；`TEST_DATABASE_URL` 可覆盖连接

## 数据库工厂（internal/adapters/db.go）

```go
OpenDB(driver, dsn string) (*gorm.DB, error)  // driver: "postgres" | "sqlite"
OpenDBFromEnv(ctx) (*gorm.DB, error)
```

- `OpenDBFromEnv` 读取 `DB_DRIVER`（默认 sqlite）、`DB_DSN`、`DB_MIGRATIONS_DIR`（默认 backend/migrations）并自动应用对应方言迁移
- 生产 `DB_DRIVER=postgres`；调试/轻量部署 `DB_DRIVER=sqlite DB_DSN=./portalt.db`
