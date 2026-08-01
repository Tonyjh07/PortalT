# PortalT 接口文档

> 仅记录当前已实现的接口与契约（截至 Phase 1）。

## 响应格式（规划约定）

后续所有 REST API 统一采用以下格式（尚未实现，Phase 6 落地）：

```json
{ "code": 200, "message": "success", "data": { ... } }
```

错误格式：

```json
{ "code": 4001, "message": "invalid credentials", "details": "..." }
```

### 错误码范围（规划）

| 范围 | 含义 |
|------|------|
| 1000-1999 | 认证错误 |
| 2000-2999 | 权限错误 |
| 3000-3999 | VM 操作错误 |
| 4000-4999 | 数据库错误 |
| 5000-5999 | 虚拟化平台错误 |

## 已实现 HTTP 接口

### 健康检查

```
GET /healthz
```

- 说明：后端存活探针，Phase 0 提供
- 响应：`200 OK`，正文 `PortalT v0.1`（text/plain）
- 监听地址：`:8080`（backend/cmd/server/main.go）

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

## 业务服务（internal/domain/services/vm_service.go）

| 方法 | 说明 |
|------|------|
| `SyncVMs(ctx)` | 从提供者拉取全部VM保存入库，删除提供者中已不存在的陈旧记录；提供者报错时不做任何变更 |
| `ListVMs(ctx)` | 返回仓储中的全部VM |

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
