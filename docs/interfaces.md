# PortalT 接口文档

> 仅记录当前已实现的接口与契约（截至 Phase 8）。

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
- 响应：当前用户信息（无密码字段）

### 虚拟机管理（需认证）

权限：所有路由需 `vm:view`；电源操作分别需 `vm:start`/`vm:stop`/`vm:restart`。

```
GET /api/v1/vms                    → 全部虚拟机（按名称排序）
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
- 过滤规则（`domain.Plugin.CanAccess`）：插件已启用 + 无 `permission` 要求或用户具备对应权限

### 插件管理（需认证 + `plugin:manage`，仅管理员）

```
GET    /api/v1/plugins     → 全部插件（含停用）
POST   /api/v1/plugins     → 注册插件（name/route 必填，自动生成ID）
PUT    /api/v1/plugins/:id → 更新插件（全字段覆盖）
DELETE /api/v1/plugins/:id → 删除插件
```

- 请求体：`{"name","icon","route","iframe_url","permission","sort_order","is_active"}`

### 远程桌面隧道（需认证，Phase 8）

```
GET /api/v1/guac/ws/:vmId
```

- 需 `vm:view`；认证支持 `Authorization: Bearer <token>` 与 `?token=<token>` 两种方式
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

使用与故障排查见 [remote-desktop.md](./remote-desktop.md)。

### RBAC 中间件（internal/api/middleware/rbac.go）

- `RequirePermission(perm)`：需在 `AuthRequired` 之后使用
- 用户无对应权限或未认证 → 403/4005（权限常量见 `internal/domain/permission.go`）

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
- 端点：`GET /api/vms` → id+path 列表，详情逐台查询；`PUT /api/vms/{id}/power` body 为裸字符串 `on/off/reset`，Content-Type `application/vnd.vmware.vmw.rest-v1+json`；`GET /api/vms/{id}/ipaddress`；`GET /api/host`（404 时回退最小信息）
- 容错：状态映射对 `on`/`poweredOn` 等大小写变体归一；CPU/内存支持新旧字段名及子对象（`cpu.processors`、`memory.memory_MiB`）；name 缺省回退 vmx 文件名
- 远程桌面：`guac.hostname` 自动写入虚拟机 IP（若详情可取得），方便 guacd 隧道开箱调试

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
| iframe_url | string | 嵌入地址 |
| permission | string | 访问所需权限，空=无需权限 |
| sort_order | int | 排序权重（小在前） |
| is_active | bool | 是否启用 |

业务方法：`CanAccess(user *User) bool`（启用 + 权限双检查）、`IsEnabled()`。

## 权限常量（internal/domain/permission.go）

| 常量 | 值 | 说明 |
|------|-----|------|
| PERM_VIEW_ALL | view_all | 查看全部资源 |
| PERM_VM_VIEW | vm:view | 查看虚拟机 |
| PERM_VM_START | vm:start | 启动虚拟机 |
| PERM_VM_STOP | vm:stop | 停止虚拟机 |
| PERM_VM_RESTART | vm:restart | 重启虚拟机 |
| PERM_VM_MANAGE | vm:manage | 管理虚拟机 |
| PERM_PLUGIN_VIEW | plugin:view | 查看插件 |
| PERM_PLUGIN_MANAGE | plugin:manage | 管理插件 |
| PERM_USER_MANAGE | user:manage | 管理用户 |

### 角色权限矩阵（已实现）

| 权限 | admin | user | viewer |
|------|:-----:|:----:|:------:|
| view_all / vm:view | ✅ | ✅ | ✅ |
| vm:start / vm:stop / vm:restart | ✅ | ✅ | ❌ |
| plugin:view | ✅ | ✅ | ❌ |
| vm:manage / plugin:manage / user:manage | ✅ | ❌ | ❌ |

## 数据库表结构（backend/migrations/001_init.up.sql）

| 表 | 关键字段 | 说明 |
|----|---------|------|
| users | id(PK), username(UNIQUE), password_hash, email, role, created_at | 用户账号 |
| vms | id(PK), name, status, cpu, memory_mb, ip_address, host, metadata(JSONB), created_at, updated_at | 虚拟机目录 |
| plugins | id(PK), name, icon, route(UNIQUE), iframe_url, permission, sort_order, is_active, created_at, updated_at | 插件菜单 |
| permissions | id(PK), name(UNIQUE), description, created_at | 权限字典（预留） |

- 迁移脚本：`001_init.up.sql` / `001_init.down.sql`，由 `postgres.Migrate(db, dir)` 按文件名顺序执行
- SQLite 方言迁移：`migrations/sqlite/001_init.{up,down}.sql`（metadata 用 TEXT 存 JSON，is_active 用 0/1）
- `make test-integration` 自动应用迁移后测试；`TEST_DATABASE_URL` 可覆盖连接

## 数据库工厂（internal/adapters/db.go）

```go
OpenDB(driver, dsn string) (*gorm.DB, error)  // driver: "postgres" | "sqlite"
OpenDBFromEnv(ctx) (*gorm.DB, error)
```

- `OpenDBFromEnv` 读取 `DB_DRIVER`（默认 sqlite）、`DB_DSN`、`DB_MIGRATIONS_DIR`（默认 backend/migrations）并自动应用对应方言迁移
- 生产 `DB_DRIVER=postgres`；调试/轻量部署 `DB_DRIVER=sqlite DB_DSN=./portalt.db`
