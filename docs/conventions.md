# PortalT 开发规范

> 仅记录当前已落地执行的规范（截至 Phase 1）。

## 工具链要求

| 工具 | 版本要求 | 本机实测 |
|------|---------|---------|
| Go | 1.21+ | 1.26.5（winget 安装） |
| Node.js | 20+ | 24.18.0 |
| Docker | 24+ | 28.4.0 |
| GNU Make | 3.81+ | 3.81（winget 安装，需 msys sh.exe） |

### Windows 环境注意事项

- GNU Make 依赖 `sh.exe`（本机使用 msys64），Makefile 按 POSIX 写法
- MSYS 环境下 npm 需用 `npm.cmd`，Makefile 已通过 `uname -s` 自动识别
- Go 模块名：`portalt`（仓库无远端，未用 github 路径）
- **竞态检测（-race）**：需要原生 MinGW gcc。本机已装 WinLibs（winget 包 `BrechtSanders.WinLibs.MCF.UCRT`）。`make test-race` 会自动探测其路径并通过 `CC` 传给 go；不要依赖 msys64 的 cygwin gcc（报 "don't use the cygwin compiler" 错误）。注意 msys64 位于机器 PATH 中，优先级高于用户 PATH 的 WinLibs，因此必须走 Makefile 的 CC 探测

### Nuxt dev 的全局 CSS 加载 bug（patch-package 修复）

- **症状**：dev 模式首页出现 `Failed to load module script ... MIME type of "text/css"`（针对全局 CSS），经 Cloudflare Tunnel 访问时暴露；hard reload 可缓解、二次加载复现（Nuxt 已知问题，见 nuxt-modules/tailwindcss#613）
- **根因（两层）**：
  1. `@nuxt/vite-builder@3.21.10` `getManifest()`：`nuxt.options.css` 全局 CSS 解析为 Windows 绝对路径（`C:/...`）时原样写入 manifest（Unix 绝对路径 `/...` 本可直接用，盘符路径不行）→ HTML 出现 `/_nuxt/C:/...` 畸形 link；main 分支（Nuxt 4）已用 `toFsUrl()`（加 `/@fs` 前缀）修复
  2. `nuxt/dist/index.mjs` 的 `cssTemplate`：`.nuxt/css.mjs` 虚拟模块 `import "/_nuxt/assets/css/main.css"` 与 HTML 的 `<link>` **同 URL 两种内容形态**（Vite 按请求头返回 CSS 原文或 JS 模块，但响应 `Vary` 只有 `Origin`）→ 浏览器缓存把 link 的 text/css 响应复用给 module import → MIME 报错。`?inline` 让 import 与 link 的 URL 分离（`main.css?inline` 返回 JS 模块、不注入样式，样式仍由 link 加载）
- **修复（frontend/patches/）**：`@nuxt+vite-builder+3.21.10.patch` 绝对路径加 `/@fs` 前缀；`nuxt+3.21.10.patch` 全局 CSS import 追加 `?inline`——**必须仅 dev 生效**（`ctx.nuxt.options.dev` 判断），否则 `nuxt build` 会把全局 CSS 以 `?inline` 打进 JS 字符串且不注入，导致生产样式全丢（`html/body` 不铺满视口、背景/变量缺失）。`package.json` 的 `postinstall` 为 `patch-package && nuxt prepare`，重装依赖后自动应用
- **重打补丁**：改 `node_modules/<pkg>/dist/*.mjs` 后跑 `npx patch-package <pkg>` 重新生成 patches；升级 nuxt 后需删除旧 patch 并确认上游已修复

## Go 依赖镜像（GOPROXY）

本网络环境无法访问 `proxy.golang.org`（2026-08-01 下载 testify 时超时）。
已持久化配置，无需手动处理：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

若 npm 安装遇到 registry 不可达，同样需配置镜像（尚未发生）。

## Makefile 命令

| 命令 | 说明 | 当前状态 |
|------|------|---------|
| `make version` | 输出工具链版本 | ✅ 可用 |
| `make init` | 创建目录骨架 + 下载依赖 | ✅ 可用 |
| `make run` | 启动后端（go run） | ✅ 可用 |
| `make test-domain` | domain 层单测（覆盖率） | ✅ 通过（100%） |
| `make test-repo` | 内存 + gormstore 仓储测试（gormstore 用内存SQLite，无需外部服务） | ✅ 通过 |
| `make test-virt` | Mock 虚拟化提供者 + 工厂测试 | ✅ 通过 |
| `make test-sqlite` | SQLite 适配器集成测试（临时文件库，无需外部服务） | ✅ 通过 |
| `make test-esxi` | ESXi 适配器集成测试（vcsim 内置模拟 vCenter，无需真实 ESXi） | ✅ 通过 |
| `make test-auth` | 认证适配器 + API 层测试（登录/刷新/中间件） | ✅ 通过 |
| `make test-race` | 全部单测 + 竞态检测（需 MinGW） | ✅ 通过 |
| `make test-integration` | 全部适配器集成测试（SQLite 免服务；PostgreSQL 需 `docker compose up -d postgres`） | ✅ 通过 |
| `make test` | 汇总测试（test-domain + test-repo + test-api） | ⏳ 依赖后续阶段 |
| `make build` | 构建后端二进制 `backend/bin/portalt.exe` | ✅ 可用 |
| `make docker-build` | 构建 Docker 镜像 | ⏳ 依赖 Dockerfile |
| `make up/down/logs` | docker compose 便捷命令 | ✅ 可用 |

## 代码规范（Go）

- domain 层**禁止**导入外部框架（Gin/GORM 等），仅标准库
- 每个导出类型/函数必须有注释（中文可接受），包含包注释
- 遵循 `gofmt` 与 `go vet`，提交前必须通过
- 错误处理：返回 `error`，不 panic；仓储层使用 `ports.ErrNotFound` / `ports.ErrInvalidArgument` 哨兵错误
- 测试：使用 `testify` 断言库，公开函数至少一个测试用例
- 并发：内存仓储使用 `sync.RWMutex` + `map`，必须通过 `-race` 检测
- 覆盖率要求：domain 层 > 90%（当前 100%）

## 架构分层约定

- `internal/domain`：领域实体与业务方法，零外部依赖
- `internal/ports`：仓储/虚拟化/认证接口定义 + 错误哨兵，无实现
- `internal/api`：Gin 路由装配（router.go）；`internal/api/response` 统一响应格式（独立叶子包，供处理器与中间件共用，避免循环依赖）
- `internal/api/middleware`：认证等中间件；`internal/api/v1`：接口处理器
- `internal/adapters/auth`：本地密码认证（bcrypt）+ JWT 令牌管理（HS256）+ 管理员引导
- `internal/adapters/memory`：内存仓储（测试/开发用），编译期断言实现接口（`var _ ports.VMRepository = (*VMRepository)(nil)`）
- `internal/adapters/gormstore`：**方言无关**的 GORM 模型与仓储（postgres/sqlite 共用，单一事实来源）
- `internal/adapters/postgres` / `sqlite`：薄包装 + 各自方言的 Open/Migrate；jsonb metadata 空值归一为 nil；upsert 用 `clause.OnConflict`
- `internal/adapters/db.go`：`OpenDB(driver, dsn)` 工厂 + `OpenDBFromEnv()`（读 `DB_DRIVER`/`DB_DSN`/`DB_MIGRATIONS_DIR`，默认 sqlite 便于调试）
- `internal/adapters/esxi`：govmomi 提供者，惰性连接 + 指数退避重试；ListVMs 用 ContainerView 批量拉取；ID 用 VM UUID（MOID 存入 metadata）
- `internal/adapters/mock`：内存态虚拟化模拟器（内置示例 VM/宿主机），开发调试默认
- `internal/adapters/virt_factory.go`：`NewVirtualizationProvider(virtType, config)`，支持 `esxi`/`mock`（默认 mock），esxi 配置键 `url`/`username`/`password`/`insecure`
- `internal/domain/services`：业务编排（如 `VMService.SyncVMs`），只依赖 ports 接口

## 数据库支持矩阵（已实现）

| 驱动 | 依赖 | 迁移脚本 | 用途 |
|------|------|---------|------|
| postgres | GORM + jackc/pgx | `backend/migrations/*.up.sql` | 生产 |
| sqlite | glebarez/sqlite（纯Go，无CGO） | `backend/migrations/sqlite/*.up.sql` | 调试/轻量部署 |

切换示例：`DB_DRIVER=sqlite DB_DSN=./portalt.db make run`（main.go 接线在 Phase 5/6 落地）。

**迁移约定（Phase 9 起）**：
- 已应用迁移记录在 `schema_migrations(version)` 表，启动重复执行自动跳过（幂等）
- 新迁移编号顺延（如 `004_xxx.{up,down}.sql`），postgres 与 sqlite 方言各写一份
- 存量库兼容：postgres 用 `ADD COLUMN IF NOT EXISTS`；sqlite 无此语法，旧库重放报
  "duplicate column name" 由迁移器视为已应用（见 `internal/adapters/sqlite/db.go`）

## 集成测试约定

- PostgreSQL：`docker compose up -d postgres`（默认凭据 portalt/securepassword），`TEST_DATABASE_URL` 覆盖
- SQLite：无需任何服务，使用临时文件数据库
- 测试文件带 `//go:build integration` 标签，`make test-integration`/`make test-sqlite` 才执行
- 每个测试前 `TRUNCATE` 业务表（PostgreSQL）隔离数据
- gormstore 单元测试（无 tag）使用内存/临时 SQLite，常驻 `make test-unit` 中

## 虚拟化集成测试约定

- ESXi 适配器：`//go:build esxi` 标签 + govmomi 内置 vcsim 模拟器（`simulator.VPX()`），无需真实环境；`make test-esxi` 执行
- Mock 提供者与工厂：无 tag 常驻单测，`make test-virt` 执行

## API 测试约定

- 认证适配器单测：内存仓储 + 手工过期/篡改令牌
- API 端到端测试：`internal/api/auth_api_test.go`（完整路由 + 真实认证链），置于 `api` 包以避免 v1↔api 测试循环依赖
- 中间件测试：可编程 TokenManager 桩（`internal/api/middleware/auth_test.go`）
- `make test-auth` 覆盖以上全部；登录成功/失败/刷新/me/错误码均断言

## Docker 环境备注

- **2026-08-01 问题记录**：原 registry 镜像源（hub-mirror.c.163.com 已死、tencentyun 内网专用）导致拉取失败。已改为 `https://docker.m.daocloud.io` + `https://docker.1ms.run`（`~/.docker/daemon.json`，备份在 daemon.json.bak）
- **重要**：修改 daemon.json 后必须**完全退出 Docker Desktop**（托盘 Quit，确保 `com.docker.backend.exe` 进程结束），仅重启 WSL/关窗口无效——backend 进程只在启动时读取一次 daemon.json 并缓存（根因由调试确认）
- 镜像站带宽有限，首次拉取大镜像可能较慢

## 日志与错误处理约定

- 日志使用 `slog` 结构化日志（后续阶段引入）
- 敏感信息（密码）不得出现在日志与 JSON 输出中（`json:"-"` 标签隐藏）

## 配置文件

- 环境变量示例：根目录 `.env.example`（复制为 `.env` 使用）
- 变量带 `PORTALT_` 前缀自动加载（config 层在 Phase 5+ 引入）
