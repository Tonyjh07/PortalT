# PortalT

HomeLab 统一门户系统：在虚拟机中运行的 Web 门户，统一管理 ESXi 上的虚拟机，提供浏览器远程桌面（Guacamole）与可扩展的插件系统。

> 完整需求与技术架构见 [About.md](./About.md)，开发规范与接口文档见 [docs/](./docs/README.md)。

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go + Gin + GORM |
| 前端 | Nuxt 3 + Vue 3 + Element Plus（Phase 7） |
| 数据库 | PostgreSQL 15 / SQLite（调试部署） |
| Web 服务器 | Caddy 2 |
| 远程桌面 | Apache Guacamole |
| 虚拟化 | govmomi (ESXi) |

## 快速开始

```bash
make version   # 检查工具链 (Go 1.21+, Node 20+, Docker 24+)
make init      # 初始化目录结构与依赖
make run       # 启动后端服务 (http://localhost:8080)
make test      # 运行测试（全量）
```

### 认证

启动时自动引导管理员账号（环境变量 `ADMIN_USERNAME`/`ADMIN_PASSWORD`，默认 `admin`/`admin123`）：

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -d '{"username":"admin","password":"admin123"}'
# {"code":200,"message":"success","data":{"access_token":"...","refresh_token":"...",...}}
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_DRIVER` | `sqlite` | `postgres` / `sqlite`（sqlite 零依赖调试） |
| `DB_DSN` | 空（内存库） | postgres 连接串 / sqlite 文件路径 |
| `DB_MIGRATIONS_DIR` | `migrations` | 迁移脚本目录 |
| `VIRT_PROVIDER` | `mock` | `esxi` / `mock`（mock 无需真实环境） |
| `VIRT_ESXI_URL` 等 | - | esxi 连接配置（url/username/password/insecure） |
| `JWT_SECRET` | 开发密钥 | 生产必须显式配置 |
| `JWT_ACCESS_TTL` / `JWT_REFRESH_TTL` | `900` / `604800`（秒） | 令牌有效期 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | 管理员引导账号 |

完整列表见 [.env.example](./.env.example)。

## 测试

```bash
make test-unit        # domain / 仓储 / 虚拟化 mock / API 单测
make test-auth        # 认证适配器 + API 层（登录/刷新/中间件）
make test-sqlite      # SQLite 集成测试（免服务）
make test-esxi        # ESXi 适配器（vcsim 模拟 vCenter）
make test-integration # 全部适配器集成测试（PostgreSQL 需 docker compose up -d postgres）
make test-race        # 竞态检测（需 MinGW）
```

## 项目结构

```
backend/    Go 后端（domain → ports → adapters 分层，Gin API）
frontend/   Nuxt 3 前端
caddy/      Caddy 反向代理配置
docs/       开发规范与接口文档
```

## 开发进度

- ✅ Phase 0-1：基础设施、领域模型（100% 测试覆盖）
- ✅ Phase 2：仓储接口 + 内存实现 + VM 同步服务
- ✅ Phase 3：PostgreSQL 适配器 + SQLite（gormstore 共享层，零依赖调试）
- ✅ Phase 4：ESXi 适配器（govmomi + vcsim 测试）+ Mock 提供者 + 工厂
- ✅ Phase 5：认证与 JWT（bcrypt 本地认证、access/refresh 令牌、管理员引导）
- ⬜ Phase 6+：核心 API（VM 管理/菜单/Guacamole）、前端等（见 About.md 进度表）
