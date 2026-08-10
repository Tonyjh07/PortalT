# PortalT 项目需求与开发流程文档
---

## 📋 项目概述

**项目名称**：PortalT - HomeLab统一门户系统

**核心目标**：构建一个运行在虚拟机中的Web门户，统一管理ESXi上的所有虚拟机，提供远程桌面访问、云盘服务，并具备可扩展的插件系统。

**关键特性**：
- ✅ 不暴露ESXi管理界面，通过门户虚拟机代理
- ✅ 浏览器直接操作客户机（通过Guacamole代理RDP/VNC/SSH）
- ✅ 多用户身份认证与RBAC权限管理
- ✅ 插件化架构，支持动态扩展菜单和功能
- ✅ 可移植设计，支持切换虚拟化平台（ESXi/Proxmox/KVM）

---

## 🏗️ 技术架构

### 技术栈
| 层级 | 技术选型 | 用途 |
|------|---------|------|
| 后端 | Go + Gin + GORM | RESTful API，低开销，高性能 |
| 前端 | Nuxt 3 + Vue 3 + Element Plus | 现代化UI，SSR优化 |
| 数据库 | PostgreSQL 15 | 用户、权限、插件配置存储 |
| Web服务器 | Caddy 2 | 反向代理 + 自动HTTPS |
| 远程桌面 | Apache Guacamole | Web远程桌面网关 |
| 虚拟化驱动 | govmomi (ESXi) | 连接vSphere API |

### 架构分层（盖尔定律设计）
```
外层（可替换）    →  认证方式（本地/LDAP/OAuth）
                  数据库（PostgreSQL/MySQL/SQLite）
                  虚拟化平台（ESXi/Proxmox/KVM）
                  前端主题
                      ↓
中间层（接口）    →  Repository模式抽象
                  VirtualizationProvider接口
                  AuthenticationProvider接口
                      ↓
内层（核心）      →  领域模型（VM/User/Permission）
                  业务逻辑（权限检查/VM操作）
```

---

## 📁 项目目录结构

```
PortalT/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go                # 程序入口，依赖注入
│   ├── internal/
│   │   ├── domain/                     # 核心领域模型（不依赖外部）
│   │   │   ├── vm.go                  # VM实体 + 业务方法
│   │   │   ├── user.go                # User + Role实体
│   │   │   ├── permission.go          # 权限定义
│   │   │   ├── plugin.go              # 插件实体
│   │   │   └── services/              # 业务服务
│   │   │       ├── vm_service.go      # VM同步/操作
│   │   │       ├── auth_service.go    # 认证逻辑
│   │   │       └── plugin_service.go  # 插件管理
│   │   ├── ports/                      # 接口定义（依赖倒置）
│   │   │   ├── repository.go          # 仓储接口
│   │   │   ├── virtualization.go      # 虚拟化提供者接口
│   │   │   └── auth.go               # 认证提供者接口
│   │   ├── adapters/                   # 具体实现（可替换）
│   │   │   ├── postgres/              # PostgreSQL实现
│   │   │   │   ├── vm_repo.go
│   │   │   │   ├── user_repo.go
│   │   │   │   └── plugin_repo.go
│   │   │   ├── esxi/                  # ESXi虚拟化驱动
│   │   │   │   └── provider.go
│   │   │   ├── mock/                  # Mock实现（测试用）
│   │   │   │   ├── virt_provider.go
│   │   │   │   └── memory_repo.go
│   │   │   └── auth/                  # 认证实现
│   │   │       ├── local.go           # 本地密码认证
│   │   │       └── jwt.go             # JWT生成/验证
│   │   ├── api/
│   │   │   ├── middleware/
│   │   │   │   ├── auth.go           # JWT验证中间件
│   │   │   │   ├── rbac.go           # 权限检查中间件
│   │   │   │   └── cors.go
│   │   │   └── v1/                    # API版本v1
│   │   │       ├── auth.go           # /api/v1/auth/* 登录/刷新
│   │   │       ├── vm.go             # /api/v1/vm/* 虚拟机操作
│   │   │       ├── guac.go           # /api/v1/guac/* Guacamole代理
│   │   │       ├── menu.go           # /api/v1/menu 动态菜单
│   │   │       └── plugin.go         # /api/v1/plugin/* 插件管理
│   │   └── config/
│   │       └── config.go              # Viper配置加载
│   ├── migrations/                     # 数据库迁移
│   │   ├── 001_init.up.sql            # 初始化表结构
│   │   └── 001_init.down.sql
│   ├── pkg/                            # 可复用工具包
│   │   ├── crypt/                     # 加密工具
│   │   └── validator/                 # 验证器
│   ├── go.mod
│   ├── go.sum
│   └── Dockerfile
│
├── frontend/
│   ├── pages/
│   │   ├── login.vue                  # 登录页
│   │   ├── dashboard.vue              # 仪表盘主页
│   │   ├── vms/
│   │   │   ├── index.vue              # VM列表
│   │   │   └── [id].vue               # VM详情 + 远程桌面
│   │   └── plugins/
│   │       └── [...slug].vue          # 动态插件路由
│   ├── components/
│   │   ├── layout/
│   │   │   ├── default.vue            # 主布局（侧栏+顶栏）
│   │   │   └── auth.vue               # 认证布局（干净）
│   │   ├── menu/
│   │   │   ├── SideMenu.vue           # 动态侧边栏
│   │   │   └── MenuItem.vue           # 递归菜单项
│   │   └── cards/
│   │       └── VMStatusCard.vue       # VM状态卡片
│   ├── composables/
│   │   ├── useAuth.ts                 # 认证状态管理
│   │   ├── useMenu.ts                 # 菜单加载
│   │   └── useAPI.ts                  # API调用封装
│   ├── stores/                         # Pinia状态（可选）
│   │   ├── auth.ts
│   │   └── menu.ts
│   ├── server/                         # Nuxt服务端API（代理）
│   │   └── api/
│   │       └── proxy/[...].ts         # 转发到Go后端
│   ├── nuxt.config.ts
│   ├── package.json
│   ├── Dockerfile
│   └── .env.example
│
├── caddy/
│   ├── Caddyfile
│   └── Dockerfile
│
├── docker-compose.yml
├── docker-compose.dev.yml
├── Makefile                           # 统一构建命令
├── .github/workflows/
│   ├── ci.yml                         # 持续集成
│   └── cd.yml                         # 持续部署
└── README.md
```

---

## 🔄 开发流程（按Phase执行）

### Phase 0：基础设施搭建
**目标**：搭建开发环境，验证工具链

**验证标准**：
```bash
make version   # 输出 Go 1.21+, Node 20+, Docker 24+
make init      # 创建目录结构，下载依赖
make run       # 启动空服务，输出 "PortalT v0.1"
```

**AI生成任务**：
1. 生成 `Makefile`，包含命令：`init`, `run`, `test`, `build`, `docker-build`
2. 生成 `docker-compose.yml`，包含 PostgreSQL + Caddy 服务
3. 生成 `backend/cmd/server/main.go`，仅输出 "PortalT starting..."

---

### Phase 1：领域模型定义
**目标**：定义核心业务实体，不依赖任何外部框架

**验证标准**：
```bash
make test-domain   # 单元测试覆盖率 > 90%
```

**AI生成任务**：
1. `internal/domain/vm.go` - VM结构体，包含字段：ID, Name, Status, CPU, MemoryMB, IPAddress, Host。实现方法：`CanStart()`, `CanStop()`, `CanRestart()`
2. `internal/domain/user.go` - User结构体，包含字段：ID, Username, Password, Email, Role。实现方法：`HasPermission(perm string) bool`
3. `internal/domain/permission.go` - 预定义权限常量（如 `PERM_VM_START`, `PERM_VM_STOP`, `PERM_VIEW_ALL`）
4. `internal/domain/plugin.go` - Plugin结构体，包含字段：ID, Name, Icon, Route, IframeURL, Permission, SortOrder, IsActive
5. 对应的单元测试文件，验证所有业务方法

**关键约束**：
- ❌ 禁止导入任何外部框架（Gin/GORM等）
- ✅ 仅使用标准库

---

### Phase 2：仓储接口与内存实现
**目标**：定义数据访问接口，用内存存储验证业务逻辑

**验证标准**：
```bash
make test-repo   # 所有仓储测试PASS
```

**AI生成任务**：
1. `internal/ports/repository.go` - 定义接口：
   ```go
   type VMRepository interface {
       Save(vm *domain.VM) error
       FindByID(id string) (*domain.VM, error)
       FindAll() ([]*domain.VM, error)
       Delete(id string) error
   }
   type UserRepository interface { /* ... */ }
   ```
2. `internal/adapters/memory/vm_repo.go` - 使用 `sync.RWMutex` + `map` 实现
3. `internal/adapters/memory/user_repo.go` - 同上
4. 单元测试验证CRUD操作
5. `internal/domain/services/vm_service.go` - 实现 `SyncVMs()` 方法，从虚拟化提供者拉取数据并保存

---

### Phase 3：PostgreSQL适配器
**目标**：用PostgreSQL替换内存存储，验证可替换性

**验证标准**：
```bash
make test-integration   # 连接实际数据库，所有测试PASS
```

**AI生成任务**：
1. `migrations/001_init.up.sql` - 创建表：users, vms, plugins, permissions
   - users: id, username, password_hash, email, role, created_at
   - vms: id, name, status, cpu, memory_mb, ip_address, host, metadata (jsonb)
   - plugins: id, name, icon, route, iframe_url, permission, sort_order, is_active
2. `internal/adapters/postgres/vm_repo.go` - 使用GORM实现 `ports.VMRepository`
   - 添加gorm标签到domain.VM
3. `internal/adapters/postgres/user_repo.go` - 同上
4. `internal/adapters/postgres/db.go` - GORM连接初始化
5. 集成测试（使用testcontainers或实际PostgreSQL）

**替换验证**：在 `main.go` 中切换 `memory.NewVMRepo()` → `postgres.NewVMRepo(db)` 只需一行

---

### Phase 4：虚拟化适配器 - ESXi
**目标**：连接真实ESXi，获取虚拟机列表

**验证标准**：
```bash
make test-esxi   # 配置.env后，成功列出虚拟机
```

**验证结果（2026-08-01）**：`make test-esxi` 通过 —— govmomi v0.55.1 + vcsim 模拟 vCenter（`simulator.VPX()`），7 个用例全绿（ListVMs 字段映射/主机名解析/电源操作/NotFound/GetHostInfo/连接失败/会话复用），覆盖率 86.5%；`make test-virt`（mock+工厂）88.9%。`CGO_ENABLED=0` 构建验证通过。

**真实环境验证（2026-08-06）**：ESXi 7.0.3（build-21930508，i5-13400TEF，4 核/6142MB）上集成测试全绿（`go test -tags integration ./internal/adapters/esxi/...`）—— 连接/ListVMs 字段完整映射（含 `Metadata["moid"]`）/GetHostInfo/电源操作全链路（关机-开机-重启）/会话复用/不存在 VM 报错/状态枚举映射；后端以 `VIRT_PROVIDER=esxi` 跑通 API 端到端（登录 → VM 列表 → stop/start/restart）。真实测试需环境变量 `TEST_ESXI_URL/USERNAME/PASSWORD/TEST_ESXI_VM`，见 `docs/conventions.md`。

**完成说明**：
- `internal/adapters/esxi/provider.go`：govmomi 客户端，惰性连接（首次调用建立会话），ListVMs 用 ContainerView 批量拉取（summary/guest/config），宿主机名二次属性收集解析；幂等 `Close()`；`findVM` 支持 UUID（SearchIndex.FindByUuid）与 MOID（vm- 前缀）两种标识
- 电源操作（StartVM/StopVM/RestartVM）带 Task.Wait，瞬时故障指数退避重试（200ms 起，3 次默认，`MaxRetries`/`Timeout` 可配）
- 状态映射：PowerState → `domain.VMStatus`；ID 取 `Summary.Config.Uuid`（MOID 存入 `Metadata["moid"]` 供后续控制台使用）
- `GetHostInfo`：CPU 型号/核数（MHz 换算核数）、内存 MB、平台版本、连接状态
- `internal/adapters/mock/provider.go`：内存模拟器（3 台示例 VM + 宿主），`SetVMs`/`SetHostInfo` 测试控制
- `internal/adapters/virt_factory.go`：`NewVirtualizationProvider("esxi"|"mock", config)`；配置键 url/username/password/insecure
- 可移植性：`VIRT_PROVIDER=mock` 即可零环境开发（接线 Phase 5/6 落地）

**AI生成任务**：
1. `internal/ports/virtualization.go` - 定义接口：
   ```go
   type VirtualizationProvider interface {
       ListVMs() ([]*domain.VM, error)
       StartVM(id string) error
       StopVM(id string) error
       RestartVM(id string) error
       GetHostInfo() (*HostInfo, error)
   }
   ```
2. `internal/adapters/esxi/provider.go` - 使用 `github.com/vmware/govmomi`
   - 连接：`govmomi.NewClient(ctx, host, user, pass, true)`
   - 获取虚拟机列表：通过 `FindAll` 遍历 `VirtualMachine` 对象
   - 状态映射：`vm.Summary.Runtime.PowerState` → `domain.VMStatus`
3. `internal/adapters/virt_factory.go` - 工厂模式：
   ```go
   func NewVirtualizationProvider(virtType string, config map[string]string) (ports.VirtualizationProvider, error)
   ```
   - 支持 "esxi", "mock" 类型
4. 错误处理与重试机制

**可移植验证**：修改环境变量 `VIRT_PROVIDER=mock` 切换回Mock

---

### Phase 5：认证与JWT
**目标**：实现多认证方式切换（本地密码 + JWT）

**验证标准**：
```bash
make test-auth   # API登录返回token
curl -X POST http://localhost:8080/api/v1/auth/login -d '{"username":"admin","password":"admin123"}'
# 预期返回 {"access_token":"...", "refresh_token":"..."}
```

**AI生成任务**：
1. `internal/ports/auth.go` - 定义接口：
   ```go
   type AuthenticationProvider interface {
       Authenticate(username, password string) (*domain.User, error)
   }
   ```
2. `internal/adapters/auth/local.go` - 本地密码认证
   - 密码哈希：`golang.org/x/crypto/bcrypt`
   - 查找用户：调用 `UserRepository.FindByUsername`
3. `internal/adapters/auth/jwt.go` - JWT生成/验证
   - 使用 `github.com/golang-jwt/jwt/v5`
   - 生成access_token（15分钟）+ refresh_token（7天）
   - 从环境变量读取 `JWT_SECRET`
4. `internal/api/middleware/auth.go` - 验证中间件
   - 从 `Authorization: Bearer <token>` 解析
   - 解析后的用户存入 `gin.Context`
5. `internal/api/v1/auth.go` - 登录接口
   - POST `/api/v1/auth/login` → 返回token对
   - POST `/api/v1/auth/refresh` → 刷新token
6. 注册管理员初始账号（通过环境变量 `ADMIN_USERNAME`/`ADMIN_PASSWORD` 在启动时创建）

**验证结果（2026-08-01）**：`make test-auth` 通过（认证适配器 91.9% + API 层全绿）；curl 端到端实测：`/healthz` 200、登录返回双令牌、`/auth/me` 返回 admin 用户、refresh 换新 access token、错误密码 401+4001、无令牌 401+4003。

**完成说明**：
- `ports/auth.go`：`AuthenticationProvider`（Authenticate）+ `TokenManager`（签发/解析/TTL），错误哨兵 `ErrInvalidCredentials`/`ErrInvalidToken`
- `adapters/auth/local.go`：bcrypt 哈希；用户不存在时用固定哈希做恒定时间比较，防用户名枚举；`EnsureAdminUser` 幂等引导（默认 admin/admin123）
- `adapters/auth/jwt.go`：HS256，access 15 分钟 + refresh 7 天（`JWT_ACCESS_TTL`/`JWT_REFRESH_TTL` 秒级可配，`JWT_SECRET` 读取，缺省开发密钥）；令牌类型声明隔离（access 不能当 refresh 用）
- `api/`：Gin 路由（响应统一 `{code,message,data}`，抽 `api/response` 叶子包防循环依赖）；`middleware.AuthRequired`（Bearer 解析）+ `v1.AuthHandler`（login/refresh/me）
- `cmd/server/main.go`：完成依赖接线 —— OpenDBFromEnv → gormstore 仓储 → 认证/JWT → 管理员引导 → Gin 路由；`ADMIN_USERNAME`/`ADMIN_PASSWORD` 环境变量

**经验**：`internal/api/v1` 测试引用 `internal/api` 会构成测试循环依赖，端到端测试置于 `api` 包（`auth_api_test.go`）解决；负 TTL 会被 JWT 管理器归为默认值，过期令牌测试需手工构造。

---

### Phase 6：核心API实现
**目标**：实现VM管理、菜单、Guacamole代理API

**验证标准**：
```bash
make test-api   # 所有API测试PASS
```

**验证结果（2026-08-01）**：`make test-api` 通过（v1 处理器 54.9%、middleware、api 端到端全绿）；全量回归（test-unit/test-integration/test-esxi/test-sqlite/test-race）通过；curl 端到端实测：启动同步 3 台 mock VM → 列表/详情/状态轮询 → stop/start 电源操作（重复 stop 409）→ 插件创建/停用/删除 → 菜单按权限过滤 → 未认证 401。

**完成说明**：
- `ports`：新增 `PluginRepository` 接口与 `ErrInvalidOperation` 哨兵；响应码新增 4005（权限不足/403）、4006（不存在/404）、4007（状态不允许/409）
- `domain/services`：`StartVM/StopVM/RestartVM/GetVM/GetVMStatus` —— 电源操作统一编排（加载 → 状态规则校验 → 提供者调用 → 回刷入库）；`GetVMStatus` 提供者不可达时回退仓储缓存（轮询安全）
- `adapters`：插件仓储四件套（gormstore 共享层 + memory + postgres/sqlite 绑定包装），`FindActive` 按 sort_order 确定性排序；`auth.NewID` 导出供插件创建使用
- `api`：`rbac.go`（RequirePermission，基于角色权限表）；`v1/vm.go`（列表/详情/状态/电源操作）；`v1/menu.go`（按 CanAccess 过滤）；`v1/plugin.go`（管理员 CRUD）；`v1/guac.go`（gorilla/websocket 双向代理，注入 X-PortalT-* 身份头，GUAC_URL 未配置返回 503）
- `cmd/server`：启动时执行一次 `SyncVMs` 全量同步，目录立即可用；`VIRT_ESXI_*` 环境变量接入工厂
- 测试：VM 服务层（状态规则/回退/错误）、插件仓储（三方言）、RBAC 中间件、处理器单测（含 WebSocket 回环与头部断言）、API 端到端（角色权限矩阵：admin 全通、user 可启停但不可管理插件、viewer 仅查看）

**经验**：PowerShell `Set-Content` 会破坏 UTF-8 中文注释，批量改写必须用工具直写；测试桩里的电源操作必须真实变更状态，否则"回刷"断言恒失败；viewer 角色无 `plugin:view`，菜单 403 是正确行为而非过滤 bug（演示过滤需用 user 角色）。

**AI生成任务**：
1. `internal/api/v1/vm.go` - VM管理API：
   - GET `/api/v1/vms` → 返回所有虚拟机（仅用户有权限的）
   - GET `/api/v1/vms/:id` → 返回单个VM详情
   - POST `/api/v1/vms/:id/start` → 启动VM
   - POST `/api/v1/vms/:id/stop` → 停止VM
   - POST `/api/v1/vms/:id/restart` → 重启VM
   - GET `/api/v1/vms/:id/status` → 实时状态更新（轮询）
2. `internal/api/v1/menu.go` - 动态菜单API：
   - GET `/api/v1/menu` → 返回当前用户有权限的菜单树
   - 查询 `plugins` 表，按 `sort_order` 排序
   - 过滤 `permission` 字段（用户无权限的菜单不返回）
3. `internal/api/v1/guac.go` - Guacamole代理：
   - GET `/api/v1/guac/ws/:vmId` → WebSocket升级代理
   - 使用 `gorilla/websocket` 转发到Guacamole服务器
   - 注入JWT用户信息到连接请求头
4. `internal/api/v1/plugin.go` - 插件管理API（管理员）：
   - POST `/api/v1/plugins` → 注册新插件
   - PUT `/api/v1/plugins/:id` → 更新插件配置
   - DELETE `/api/v1/plugins/:id` → 删除插件
5. `internal/api/middleware/rbac.go` - 权限中间件
   ```go
   func RequirePermission(perm string) gin.HandlerFunc
   ```

---

### Phase 7：前端实现
**目标**：完整的用户界面，包含登录、仪表盘、VM操作、远程桌面

**验证标准**：
```bash
npm run dev     # 浏览器访问 http://localhost:3000
# 流程：登录 → 仪表盘 → 虚拟机列表 → 点击VM → 远程桌面界面
```

**AI生成任务**：

**7.1 布局与路由**：
- `nuxt.config.ts` - 配置SSR、环境变量、Element Plus
- `layouts/default.vue` - 主布局（侧栏+顶栏），侧栏调用 `useMenu()` 加载
- `layouts/auth.vue` - 认证布局（干净，无菜单）

**7.2 认证**：
- `pages/login.vue` - 登录页（表单+提交），调用 `/api/v1/auth/login`
- `composables/useAuth.ts` - 管理登录状态、cookie存储、登出
  ```typescript
  const token = useCookie('access_token')
  const user = useState('user', () => null)
  ```

**7.3 仪表盘**：
- `pages/dashboard.vue` - 显示：
  - 统计卡片（VM总数、运行数、CPU使用率）
  - 最近操作日志（可选）
  - 快速启动菜单（常用插件）

**7.4 虚拟机管理**：
- `pages/vms/index.vue` - 表格/卡片列表：
  - 显示：名称、状态（颜色标签）、CPU/内存、IP
  - 操作按钮：启动/停止/重启（根据状态禁用）
  - 点击行进入详情
- `pages/vms/[id].vue` - 详情页：
  - VM信息（资源使用、网络）
  - 操作按钮
  - **远程桌面iframe**：嵌入Guacamole（URL: `/api/v1/guac/ws/${id}`）

**7.5 动态菜单**：
- `components/menu/SideMenu.vue` - 侧边栏：
  - 调用 `useMenu()` 加载 `/api/v1/menu`
  - 支持二级菜单（递归渲染 `MenuItem.vue`）
  - 高亮当前路由
- `composables/useMenu.ts`：
  ```typescript
  const { data: menu } = await useFetch('/api/v1/menu', { headers: { Authorization: `Bearer ${token}` } })
  ```

**7.6 插件页**：
- `pages/plugins/[...slug].vue` - 动态路由：
  ```vue
  <template>
    <iframe :src="plugin.iframe_url" class="w-full h-full" />
  </template>
  ```

**7.7 主题定制**：
- 支持暗色/亮色切换
- 自定义CSS变量（品牌色、字体）

**验证结果（2026-08-01）**：`npm run build` 通过（Nuxt 3 + Element Plus，SSR 关闭的 SPA）；`npm run dev` 启动后经前端 `/api` 代理（nitro devProxy，含 ws）全链路实测：登录（admin）→ 菜单（空库 0 项）→ VM 列表（3 台 mock）→ stop/status/重复 stop 409/start → guac ws 未配置返回 503。

**完成说明**：
- 脚手架：`nuxt.config.ts`（SSR 关闭、`@element-plus/nuxt` 暗色主题、nitro devProxy `/api` → 后端 8080 含 WebSocket、`apiBase` 运行时配置）；dev 阶段前端 3000 端口直连后端
- 认证：`composables/useAuth.ts`（access/refresh cookie + 用户状态）；`useApi.ts` 统一 API 客户端（`Authorization` 头注入 + 401 自动刷新重试一次 + 失败登出）；`middleware/auth` 路由守卫；`pages/login.vue`（表单校验 + 登录失败提示）；`layouts/auth.vue`（无菜单登录布局）
- 布局：`layouts/default.vue`（可折叠侧栏 + 顶栏）；`SideMenu.vue` + 递归 `MenuItem.vue`（按 route 层级构建菜单树，支持二级）；`AppHeader.vue`（用户信息/角色标签/暗亮主题切换/登出）
- 页面：`dashboard.vue`（VM 统计卡：总数/运行/核数/内存 + 最近 VM 表 + 快速启动插件入口）；`vms/index.vue`（表格：状态彩色标签、按状态禁用电源按钮、行点击进详情）；`vms/[id].vue`（详情 + 10s 状态轮询 + 电源操作 + 远程桌面 Phase 8 占位面板）；`plugins/[...slug].vue`（动态路由 iframe 嵌入插件页面）
- 主题：`useTheme.ts`（亮/暗切换，`html.dark` + Element Plus dark vars，localStorage 持久化 + 跟随系统偏好）
- 其他：`IconRenderer.vue`（`mdi:*` 图标名 → Element Plus 图标组件映射）；`utils/permissions.ts`（前端角色权限表，与后端 RBAC 对齐）；Dockerfile（node:22-alpine 多阶段构建）；Makefile 新增 `run-frontend`/`build-frontend`，`init-frontend` 执行 npm install

**经验**：Nuxt nitro `devProxy` 会**剥离前缀**再转发（`/api/...` → 目标根），target 需写成 `http://localhost:8080/api`；`@element-plus/icons-vue` 组件自动导入不可靠，统一走 `IconRenderer` 映射；dev 代理含 ws 支持（Phase 8 Guacamole 通道复用同一入口）。

---

### Phase 8：Guacamole集成
**目标**：实现浏览器远程桌面

**验证标准**：
```bash
docker-compose up guacamole
# 浏览器访问 VM详情页，点击"远程桌面"，显示客户机桌面
```

**AI生成任务**：

1. **Docker配置**：添加 `guacamole/guacd` 服务到 `docker-compose.yml`
2. **客户端配置**：在 `pages/vms/[id].vue` 中嵌入Guacamole
   ```vue
   <iframe src="/api/v1/guac/ws/{{ vmId }}" />
   ```
3. **Guacamole连接配置**：
   - 客户机必须开启RDP（Windows）或VNC/SSH（Linux）
   - 在数据库或配置文件中存储连接参数（IP、端口、协议、凭证）

**验证结果（2026-08-01）**：浏览器 E2E（puppeteer）全链路通过 —— 登录 → VM 详情页 → 远程桌面 canvas 挂载 → 界面显示"已连接" → VNC 桌面内容渲染（1024×768 非纯黑像素）→ 鼠标输入发送 → 截图 1400×950 → 无 console 错误；Node WS 冒烟（直连 8080 与经 dev 代理 3000 两条路径）均收到完整渲染指令流（mouse/size/img/blob/end/cursor/set/sync）且连接保持活跃。

**完成说明**：
- **后端**：`internal/api/v1/guac_tunnel.go` —— GuacdHandler 直连 guacd(:4822) 并服务端完成协议握手（select/args/size/connect/ready），连接参数全部取自 VM metadata `guac.*` 键（hostname/port 缺省回退 VM 固有属性与协议默认值），浏览器无法覆盖目标与凭证；客户端内部指令（稳定性 ping）回显不转发；握手失败以 WS Close 1001 关闭；`GuacHandlerForEnv` 按 `GUACD_URL`（新）/`GUAC_URL`（旧代理）/未配置（503）三态接线；认证中间件支持 `?token=` 查询参数（WS 无法带自定义头），并在首个 `?`/`&` 截断以容忍 guacamole-common-js 追加的 `?<data>` 后缀；WS 升级器回显子协议 `guacamole`；mock 提供者注入演示参数（vnc + host.docker.internal:5900 + portalt-demo）
- **测试**：`guac_tunnel_test.go` mockGuacd TCP 服务器 + 7 个隧道测试（握手/双向转发/ping 回显/客户端断开/502/404/内部错误关闭）+ splitInstructions/Config 单测；`auth_test.go` 新增 query token 与追加数据截断用例；`go test ./... -count=1` 全绿
- **docker-compose.yml**：新增 `guacd`（guacamole/guacd:1.5.5，:4822，extra_hosts host.docker.internal:host-gateway）与 `vnc-demo`（dorowu/ubuntu-desktop-lxde-vnc，VNC_PASSWORD=portalt-demo，:5900）两个演示服务
- **前端**：`components/vm/RemoteDesktop.vue` —— guacamole-common-js WebSocketTunnel + Client；连接状态用 `Guacamole.Client.State`（WAITING/CONNECTED 视为已连接；Tunnel.State 是另一套枚举，混用会导致状态永不更新）；sendSize 自适应 + display.scale 等比缩放；全局键盘 + 画布鼠标；错误重连按钮；全屏由详情页 el-card 管理；`types/guacamole-common-js.d.ts` 手写最小类型面；`pages/vms/[id].vue` 扩展信息展示（过滤 password/secret/token 键）+ 连接状态标签 + 全屏
- **dev 代理**：Nuxt nitro `devProxy` 的 WS 升级转发不可靠（升级穿透到 dev worker 被当作普通请求），新增 `frontend/modules/wsProxy.ts`（仅 dev 生效）：复用 `ws: true` 规则，在 nuxi 父进程覆盖 `nuxt.server.upgrade`，用 `httpxy`（devDependencies）直连后端 origin 并保留原始路径（避免 target 路径双重拼接）

**经验**：Nuxt 自动导入组件会加目录前缀（`components/vm/RemoteDesktop.vue` → 全局名 `VmRemoteDesktop`，页面按不带前缀名字引用会渲染成未注册的空自定义元素）；guacamole-common-js 的 `WebSocketTunnel` 会把 connect 参数追加为 `?<data>`（无参时为 `?undefined`），服务端解析 token 需截断；`client.onstatechange` 收的是 `Client.State`（CONNECTED=3，首帧 sync 到达）而非 `Tunnel.State`；浏览器要求升级响应回显 `Sec-WebSocket-Protocol: guacamole`，否则握手失败。

---

### Phase 9：插件系统
**目标**：实现动态可扩展菜单

**验证标准**：
```bash
# 数据库插入插件记录
INSERT INTO plugins (name, icon, route, iframe_url, permission, sort_order)
VALUES ('Home Assistant', 'mdi:home', '/ha', 'https://ha.local', 'view_ha', 1);

# 重启服务，菜单自动出现
```

**AI生成任务**：
1. 插件CRUD API（Phase 6已包含）
2. 前端菜单动态加载（Phase 7已包含）
3. **插件权限验证**：在API中间件中检查用户是否有 `plugin.permission`
4. **高级插件支持**（可选）：支持Go插件`.so`动态加载

---

### Phase 10：CI/CD与部署
**目标**：自动化构建、测试、部署

**验证标准**：
```bash
git push main
# GitHub Actions自动触发 → 构建镜像 → 推送到GHCR → SSH部署到服务器
```

**AI生成任务**：
1. `.github/workflows/ci.yml`：
   - Lint代码（golangci-lint, eslint）
   - 运行单元测试 + 集成测试
   - 构建Docker镜像（不推送）
2. `.github/workflows/cd.yml`：
   - 触发条件：`push tags v*` 或 `push main`
   - 构建并推送镜像到 `ghcr.io/${{ github.repository }}`
   - SSH执行远程部署脚本
3. `deploy/install.sh` / `deploy/update.sh` - 服务器部署脚本（一键安装/更新，见 `docs/how-to-use.md` §3）：
   ```bash
   bash deploy/install.sh
   bash deploy/update.sh
   ```
4. `Makefile` 增加命令：
   - `make build` → 构建所有服务
   - `make docker-build` → 构建镜像
   - `make docker-push` → 推送镜像

---

## 🧪 测试策略

| 测试类型 | 覆盖范围 | 执行命令 | 环境 |
|---------|---------|---------|------|
| 单元测试 | domain, adapters/memory | `make test-unit` | 本地 |
| 集成测试 | adapters/postgres, esxi | `make test-integration` | 本地+testcontainers |
| API测试 | api/v1/* | `make test-api` | 本地+PostgreSQL |
| E2E测试 | 完整用户流程 | `make test-e2e` | Docker Compose |

**测试覆盖率要求**：
- domain层：> 90%
- adapters层：> 80%
- api层：> 70%

---

## 🔧 配置管理

### 环境变量（`.env`）
```env
# 数据库
DB_HOST=postgres
DB_PORT=5432
DB_USER=portalt
DB_PASSWORD=securepassword
DB_NAME=portalt

# JWT
JWT_SECRET=your-super-secret-key-change-me
JWT_ACCESS_TTL=900          # 15分钟
JWT_REFRESH_TTL=604800      # 7天

# 虚拟化平台
VIRT_PROVIDER=esxi          # esxi | proxmox | mock
ESXI_HOST=192.168.1.100
ESXI_USER=root
ESXI_PASS=password
ESXI_INSECURE=true          # 忽略证书验证

# Guacamole
GUACD_URL=guacd:4822              # Phase 8 远程桌面：guacd 原生隧道（推荐）
GUAC_URL=http://guacamole:8080    # 旧模式：转发 Guacamole Web 应用隧道（GUACD_URL 优先）

# 管理员初始账号
ADMIN_USERNAME=admin
ADMIN_PASSWORD=admin123

# Caddy
DOMAIN=portal.yourlab.com
```

### 配置加载（`internal/config/config.go`）
- 使用 `github.com/spf13/viper`
- 支持环境变量（自动前缀 `PORTALT_`）
- 支持 `config.yaml` 覆盖

---

## 📊 API设计规范

### 响应格式
```json
{
  "code": 200,
  "message": "success",
  "data": { ... }
}
```

### 错误格式
```json
{
  "code": 4001,
  "message": "invalid credentials",
  "details": "username or password is incorrect"
}
```

### 错误码范围
- 1000-1999: 认证错误
- 2000-2999: 权限错误
- 3000-3999: VM操作错误
- 4000-4999: 数据库错误
- 5000-5999: 虚拟化平台错误

---

## 🚀 性能要求

- API响应时间：< 200ms (P95)
- 登录页加载：< 1s (SSR)
- 远程桌面延迟：< 100ms (WebSocket)
- 内存占用：< 500MB (Go后端)
- 支持并发用户：> 50

---

## 🔒 安全要求

1. **JWT**：HS256签名，access_token有效期15分钟
2. **密码**：bcrypt哈希 (cost=12)
3. **HTTPS**：Caddy自动Let's Encrypt
4. **CORS**：仅允许配置的域名
5. **SQL注入**：GORM参数化查询
6. **XSS**：Nuxt自动转义输出
7. **CSRF**：SameSite Cookie + Token验证
8. **审计日志**：记录所有敏感操作（登录、VM操作）

---

## 🎨 UI/UX要求

- **风格**：现代简约，类云厂商控制台
- **配色**：深色/浅色双主题
- **响应式**：支持桌面端（1920×1080）和平板（768×1024）
- **交互**：
  - VM操作二次确认（防止误操作）
  - 操作反馈（Loading动画、Toast消息）
  - 实时状态轮询（每5秒更新VM状态）

---

## 📝 给AI的附加指令

1. **代码规范**：
   - Go：遵循 `golangci-lint` 默认规则
   - Vue：使用 `<script setup>` 语法
   - 所有函数添加注释（中文可接受）

2. **错误处理**：
   - Go：返回 `error`，不panic
   - 前端：try-catch包裹异步操作，显示友好错误消息

3. **日志**：
   - 使用 `slog` 结构化日志（Go）
   - 级别：Info（生产）、Debug（开发）

4. **注释**：
   - 每个导出的类型/函数都有注释
   - 复杂逻辑添加说明注释

5. **测试**：
   - 每个公开函数至少一个测试用例
   - 使用 `testify` 断言库
   - Mock外部依赖

---

## ✅ 验收标准

- [ ] Phase 0-10 全部完成
- [x] ~~Phase 0-1~~（完成，见"开发进度"表）
- [ ] 所有测试通过（覆盖率达标）
- [ ] 可部署到生产环境（`deploy/install.sh` 一键部署）
- [ ] 文档完整（README + API文档）
- [ ] 插件系统可正常工作（添加/删除菜单）
- [x] ~~插件系统~~（Phase 9 完成：菜单/权限/脚本与原生插件）
- [ ] 支持至少2种认证方式切换（本地 + 待扩展）
- [ ] 支持至少2种虚拟化平台切换（ESXi + Mock）

---

## 📊 开发进度（Development Progress）

| 阶段 | 状态 | 完成日期 | 验证结果 |
|------|------|---------|---------|
| Phase 0: 基础设施 | ✅ 完成 | 2026-08-01 | `make version/init/run/build` 全部通过；Go 1.26.5 + Make 3.81 已安装（winget） |
| Phase 1: 领域模型 | ✅ 完成 | 2026-08-01 | `make test-domain` 通过，覆盖率 100% |
| Phase 2: 仓储接口与内存实现 | ✅ 完成 | 2026-08-01 | `make test-repo` 通过（100%）；`make test-race` 通过（含并发检测） |
| Phase 3: PostgreSQL适配器 | ✅ 完成 | 2026-08-01 | `make test-integration` 通过（docker compose PostgreSQL 15 + GORM，含 jsonb metadata 与并发 upsert） |
| Phase 3.5: SQLite适配器 | ✅ 完成 | 2026-08-01 | 用户调试需求追加：纯Go驱动（glebarez/sqlite，无CGO），gormstore 共享仓储包，`make test-sqlite` 通过；`DB_DRIVER=sqlite` 切换 |
| Phase 4: ESXi适配器 | ✅ 完成 | 2026-08-01 | govmomi + vcsim 模拟 vCenter，`make test-esxi` 通过；mock 提供者 + 工厂（VIRT_PROVIDER 切换） |
| Phase 5: 认证与JWT | ✅ 完成 | 2026-08-01 | bcrypt 本地认证 + JWT（access 15m/refresh 7d）+ Gin 登录/刷新/me + 管理员引导，`make test-auth` + curl 实测通过 |
| Phase 6: 核心API | ✅ 完成 | 2026-08-01 | VM 管理/状态轮询 + 动态菜单 + 插件管理 + Guacamole WS 代理 + RBAC 中间件；`make test-api` + curl 实测通过 |
| Phase 7: 前端 | ✅ 完成 | 2026-08-01 | Nuxt 3 + Element Plus：登录/仪表盘/VM 管理/动态菜单/插件页/暗色主题；`npm run build` + dev 代理链实测通过 |
| Phase 8: Guacamole集成 | ✅ 完成 | 2026-08-01 | guacd 原生隧道（服务端握手 + VM metadata guac.* 参数注入）+ RemoteDesktop.vue（guacamole-common-js）+ docker-compose 演示容器（guacd + vnc-demo）；浏览器 E2E 全链路通过（登录 → 已连接 → VNC 桌面渲染 → 鼠标输入） |
| Phase 8 增强 | ✅ 完成 | 2026-08-03 | ①RDP 黑屏根因修复（guacamole-common-js 画布 `z-index:-1`，容器 `.rd-canvas` 补 `position:relative; z-index:0` 创建 stacking context）；②服务端 NOP keepalive 每 10s 规避 guacd 1.5.x 用户输入 15s 超时（GUACAMOLE-2233）；③远程桌面配置面板（PUT /vms/:id/metadata + metadata 同步合并策略）；④详情页布局重构（左：基本信息+扩展信息，右：远程桌面）+ 全屏占满视口；⑤workstation 适配器支持 /power、/ip 子接口；⑥RustDesk 一键连接（metadata `rustdesk.*` + 详情页 `rustdesk://` 唤起本机客户端） |
| Phase 8 增强（低带宽优化 + 质量模式） | ✅ 完成 | 2026-08-06 | ①加载提速：构建产物预压缩（nitro compressPublicAssets gzip+brotli，1.07MB→约 300KB 传输）+ Caddy `encode gzip` 兜底（本机 caddy 未编译 br encoder）+ guacamole-common-js 两级懒加载（Lazy 组件 + 动态 import，VM 详情页首屏 chunk 96KB→23KB，桌面库 72KB 延迟到打开桌面才下载）②远程桌面质量/流畅度模式：`?mode=auto|quality|fluency` 会话级参数（fluency 强制 1024×640 + 低色深 + 关 RDP 特效 + 静音；quality 开 RDP 音频；auto 按网络类型初选 + 指令/帧率监测连续 6 秒掉帧自动重连降档可撤销）+ 详情页切换控件 + 选择持久化；③修复 `display.scale` 无效赋值 bug（库中为方法）+ 类型声明补全（connect(data) 参数通道，规避 `?undefined` 污染）；后端模式矩阵单测 + `go test` 全绿 + `npm run build` 通过 |
| Phase 9: 插件系统 | ✅ 完成 | 2026-08-02 | 三部分全部交付：①权限管理（roles 表+迁移 002、RoleLoader/AttachPermissions 中间件、用户/角色 CRUD API+前端、RBAC 改走角色矩阵）②插件系统（proxy 脚本插件白名单转发+身份头注入、native Go 插件 registry+Deps 注入+内嵌静态页、迁移 003、迁移器版本追踪）③示例插件 esxi-admin（宿主信息+VM 快捷电源）；`go test ./... -count=1` 全绿 + `npm run build` 通过 + 运行时全链路验证（native API/静态页、proxy 转发 200 且 X-PortalT-User 正确注入） |
| Phase 9 增强（ESXi 嵌入） | ✅ 完成 | 2026-08-06 | esxi-admin 升级为 iframe 嵌入 ESXi Host Client（`/esxi/ui/` 相对路径反代）：Caddy 反代全部 ESXi 绝对路径资源（/ui、/sdk、/sts、/ticket WS 等）+ 剥除 X-Frame-Options/CSP 后，隧道 https 下 UI 加载/登录/VM 控制台全通；本机入口用 `https://127.0.0.1:8443`（自签 RSA 证书——Caddy `tls internal` 的 ECC 证书在 Windows schannel 握手失败），curl 与浏览器双重验证；详见 `docs/external-access.md` §四·六 |
| Phase 9 增强（权限系统完善） | ✅ 完成 | 2026-08-06 | ①权限字典入库（permissions 表启用，启动幂等 seed）+ `vm:console` 权限点（从 vm:view 拆出，guac 路由改走它）②角色管理 API 扩展（POST /roles 创建自定义角色，权限必须来自字典）+ 用户可分配任意角色表角色（不再锁死三角色）③资源级 VM 授权（vm_access 表+迁移 004、`GET/PUT /users/:id/vm-access` 分配接口、VM 列表过滤/详情与电源操作未授权按 404 防枚举、guac 资源校验）④插件权限声明（`Info().Permission` 作默认值不覆盖管理员配置、nativeGate API 层强制校验、proxy 注入 `X-PortalT-Perms` 头、管理 API 校验声明在字典内；esxi-admin 声明专属权限 `esxi-admin:use`（默认仅 admin 持有，普通用户不再可见 ESXi 菜单，迁移 005 覆盖存量库旧值）、cron 示例声明）⑤前端真实权限（`/auth/me` 返回 permissions 集合、useAuth.hasPerm、vm:console 远程桌面入口、用户页资源授权分配、角色页新建角色、动态角色下拉）；`go test ./... -count=1` 全绿 + `npm run build` 通过 |
| 插件重构 Phase 1-2（access 收敛 + Caddy 交互） | ✅ 完成 | 2026-08-08 | ①协议与骨架：`proto/plugin/v1` gRPC 控制面（Handshake/Health/Shutdown/Notify）+ `pluginpkg` manifest 契约 + `backend/plugins/` submodule 约定与模板②access 收敛：type 收敛 `access|native`（旧 iframe/proxy 合并，`iframe_url`+`api_url`+`endpoints`+`caddy_rules` 任意共存）、迁移 006 破坏性重建 plugins 表、domain/仓储/管理 API/代理全链路适配、新增 `/api/v1/platform`、`pluginhost/caddy.go`（落盘 plugins.d/<id>.caddy + reload，无 Caddy 安全降级）、esxi-admin 迁移为 access 种子（含 ESXi 反代规则默认值，内置 Caddyfile 未瘦身）、移除 cron 示例③前端：插件页 access 一页双区块（iframe+API 面板）+ esxi-admin 三态占位、插件管理页类型收敛 + Caddy 规则编辑器 + native 只读状态④文档与进度同步；`go test ./... -count=1` 全绿 + `go build/vet` 干净 + `npm run build` 通过 + 运行时冒烟（登录→platform→esxi-admin 菜单含默认规则） |
| 插件重构 Phase 3（native 进程化 + 热加载 + 生命周期） | ✅ 完成 | 2026-08-08 | ①gin 动态路由 spike（方案 A 前缀占位路由 + manager 内部分发，`pluginhost/route_spike_test.go`）②`pluginhost/manager.go` 进程监督：端口分配经 env 下发（PORTALT_PLUGIN_GRPC/HTTP_PORT）、gRPC 握手（connectivity 轮询避开 NewClient 懒连接）、健康探测、崩溃退避重启（上限 5 次）、运行态回写 `status` ③`watcher.go` fsnotify 热加载（新增默认禁用/替换重启/删除标记 missing，300ms 去抖）④`Load/upsert` 扫描 `PLUGINS_DIR`（manifest.id 必须=目录名，新插件默认 is_active=false，保留管理员权限/启用配置）⑤反代 `api/v1/native_proxy.go`（`/api/v1/plugins/native/:pluginId/*path` + `/native/:pluginId/*path`，三层权限 + 身份头注入，防 SSRF）⑥管理 API 生命周期：native 不可手动创建/删除、`PUT` 仅改 permission/is_active（启停经宿主）、`POST /:id/restart` ⑦示例插件 `backend/plugins/examples/hello/`（gRPC 控制面 + HTTP 数据面）⑧删除旧 `internal/plugins` 编译期注册包；`go test ./... -count=1` 全绿 + `go build/vet` 干净 + `go test -tags integration ./internal/pluginhost/` 真实 spawn 全生命周期通过 |
| 插件重构 Phase 4（装配 + 部署 + 前端增强 + 文档） | ✅ 完成 | 2026-08-08 | ①`deploy/install.sh` 创建 `PLUGINS_DIR`、`systemd` 补 `PLUGINS_DIR/PLUGIN_CADDY_DIR/CADDY_RELOAD_CMD`、`Caddyfile` 主站添加 `import plugins.d/*.caddy`（access 插件规则落盘入口）；官方 submodule 插件构建循环（当前无官方插件，循环为空）②`deploy/update.sh` 插件目录备份/回滚（更新前备份，回滚时恢复）+ 官方插件重建（循环同上）③前端管理增强：native 行轮询运行态（15s interval，仅含 native 时触发）、启用/停用/重启按钮；access 行显示 `caddy_applied` 落盘状态（后端 List 响应计算字段）④接口文档同步：Plugin JSON 增加 `caddy_applied` 字段说明；`go test ./... -count=1` 全绿 + `go build/vet` 干净 + `npm run build` 通过 |
| ESXi 反代接入门户鉴权（Caddy forward_auth） | ✅ 完成 | 2026-08-09 | ①后端新增鉴权闸口 `GET /api/v1/auth/gate?perm=<权限>`：令牌按 Authorization/`?token=`/`access_token` cookie 提取，失效回退 `refresh_token`（双令牌续期），RoleLoader 角色矩阵校验（`esxi-admin:use`），未登录 401 / 无权限 403（中文 HTML 提示页）；`NewAuthHandler` 注入 RoleLoader、公开组注册 ②Caddy `forward_auth` 接入：`DefaultESXIAdminCaddyRules` 每个 handle 加闸口回调（旧无鉴权默认 `DefaultESXIAdminCaddyRulesV1` + seed 自动升级迁移，仅精确匹配旧默认才覆盖、保留管理员自定义）；内置 Caddyfile 精简掉全部 ESXi handle，ESXi 规则仅由插件提供（停用插件即移除规则、访问收回）③前端插件页每 5 分钟静默续期 access cookie（`pages/plugins/[...slug].vue`）④`gate_test.go` 12 用例（令牌来源/权限/矩阵/续期回退/缺参）+ 路由级 3 用例（含 cookie 路径）+ caddy_test 断言 + Caddy 组合配置 docker validate 通过 + `go test ./... -count=1` 全绿 + `npm run build` 通过 |
| Caddy 手动重载（插件管理页） | ✅ 完成 | 2026-08-09 | ①后端 `POST /api/v1/plugins/caddy-reload`（`plugin:manage`）：以 DB 为准全量对齐 access 插件 Caddy 规则并 reload（补写未落盘、清理孤儿规则文件），供规则保存后 reload 失败/手工改盘后一次性修复；`ports.CaddyApplier` 新增 `SyncAll`（原 `WriteAll` 改名复用，启动引导同路径）②前端插件管理页头部新增"重载 Caddy"按钮（成功/告警提示，`caddy_applied` 状态随之刷新）③`plugin_test.go` 新增 3 用例 + `go test ./... -count=1` 全绿 + `npm run build` 通过 |
| Phase 10: CI/CD与部署 | 🔄 部分完成（部署脚本） | 2026-08-07 | `deploy/install.sh` + `deploy/update.sh` 一键安装/更新（仅依赖 bash + 包管理器，以生产为标准：systemd + Caddy 8808 + Docker 容器 guacd/postgres + /opt/portalt 布局）；`update.sh` 增强：无新提交不更新（`--force` 强制）、`--rollback [n]` 回滚到前 n 个版本（最多 2，来源为更新时自动备份的二进制/前端产物/插件目录）、分段跳过参数（`--skip-pull/backend/frontend/plugins/restart/health`）、Caddyfile 差异自动同步（校验失败回滚）、编译走 goproxy.cn 镜像、脚本自更新；生产服务器实测通过（安装/更新/健康检查/失败回滚）；CI/CD workflow（GitHub Actions）尚未实施 |
| frpc-admin 插件（后端核心） | ✅ 完成 | 2026-08-10 | 仓库内 native 插件 `backend/plugins/frpc-admin/`（独立 go.mod，非 submodule）：SSH 连接配置持久化（脱敏）、frp 版本/配置路径探测、frpc 配置 INI/TOML 双格式结构化 + 原文双模式编辑、保存=备份+语法检查+应用+重启+失败自动回滚；新增权限字典项 `frpc-admin:manage`（默认仅 admin 持有，manifest 声明）；`go build/vet` 干净 + 插件与后端 `go test ./... -count=1` 全绿；待做：前端 SPA、Makefile/README、投放与冒烟 |

### 环境与配置备注
- **Go 模块名**：`portalt`（仓库无远端，未使用 github 路径）
- **GOPROXY**：已持久化配置为 `https://goproxy.cn,direct`（默认 proxy.golang.org 在本网络无法访问，见 2026-08-01 下载失败记录）
- **GNU Make**：Windows 下需 msys sh.exe 支持；npm 命令在 MSYS 环境使用 `npm.cmd`（Makefile 已自动处理）
- **MinGW（-race 竞态检测）**：已装 WinLibs（winget `BrechtSanders.WinLibs.MCF.UCRT`）；msys64 cygwin gcc 位于机器 PATH 优先于用户 PATH，故 `make test-race` 自动探测 WinLibs gcc 路径并显式指定 CC
- **Docker 镜像源**：原 163/tencentyun 镜像失效，已改 daocloud + 1ms.run（`~/.docker/daemon.json`）；修改后必须完全退出 Docker Desktop（`com.docker.backend.exe` 只在启动时读一次配置并缓存）
- **SQLite 调试模式**：`DB_DRIVER=sqlite DB_DSN=./portalt.db` 即可零依赖运行（无需 Docker/PostgreSQL），纯 Go 驱动（glebarez/sqlite）保证 CGO_ENABLED=0 可构建
- **控制台乱码**：GBK 控制台显示 UTF-8 中文输出会乱码，文件内容本身正确，建议终端使用 UTF-8 代码页

### 文档化规范
- 每个 Phase 完成时更新本进度表、`验收标准` 勾选框及对应说明
- 记录开发过程中发现的工具链问题与解决方案（如 GOPROXY）

---

## 📚 参考文档

- [Go Gin框架文档](https://gin-gonic.com/docs/)
- [Nuxt 3文档](https://nuxt.com/docs)
- [GORM文档](https://gorm.io/docs/)
- [govmomi文档](https://github.com/vmware/govmomi)
- [Apache Guacamole文档](https://guacamole.apache.org/doc/)

---

**文档版本**：v1.4  
**最后更新**：2026-08-09  
**维护者**：Tonyjh07

---