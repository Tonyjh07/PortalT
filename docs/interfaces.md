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
