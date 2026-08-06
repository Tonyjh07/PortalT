# PortalT

HomeLab 统一门户系统：在虚拟机中运行的 Web 门户，统一管理 ESXi 上的虚拟机，提供浏览器远程桌面（Guacamole）、ESXi Web 管理界面嵌入与可扩展的插件系统。

> 完整需求与技术架构见 [About.md](./About.md)；构建与部署指南（Debian + ESXi）见
> [docs/how-to-use.md](./docs/how-to-use.md)；开发规范与接口文档见 [docs/](./docs/README.md)。

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go + Gin + GORM |
| 前端 | Nuxt 3 + Vue 3 + Element Plus |
| 数据库 | PostgreSQL 15 / SQLite（零依赖部署） |
| Web 服务器 | Caddy 2 |
| 远程桌面 | Apache Guacamole（guacd） |
| 虚拟化 | govmomi (ESXi) / vmrest (Workstation) / Mock |

## 快速开始

零依赖跑通全栈（SQLite + Mock 虚拟化，无需 Docker/真实环境，适合 Windows/PowerShell 或任意 Linux）：

```bash
# 终端 1：后端（默认 127.0.0.1:8080）
cd backend && go run ./cmd/server

# 终端 2：前端（127.0.0.1:3000，/api 自动代理到后端，含 WebSocket）
cd frontend && npm run dev
```

访问 http://127.0.0.1:3000 ，使用 `admin` / `admin123` 登录（启动自动引导管理员账号）。
后端 curl 验证：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
# {"code":200,"message":"success","data":{"access_token":"...","refresh_token":"...",...}}
```

Windows 提示：Go 可能不在 PATH，先执行 `$env:Path += ";C:\Program Files\Go\bin"`。

**接入真实 ESXi**（门户管理全部 VM + 嵌入管理界面）与**生产部署**（Debian + systemd +
Caddy）见 [docs/how-to-use.md](./docs/how-to-use.md)。

### 环境变量（常用）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_DRIVER` / `DB_DSN` | `sqlite` / 空（内存库） | 数据库；`DB_DSN=portalt.db` 持久化 |
| `VIRT_PROVIDER` | `mock` | `esxi` / `workstation` / `mock` |
| `VIRT_URL` / `VIRT_USERNAME` / `VIRT_PASSWORD` / `VIRT_INSECURE` | - | 平台连接（esxi 时 url 必填；缺省回退 `VIRT_ESXI_*` → `VIRT_WS_*`） |
| `ESXI_WEB_URL` | 自动推导 | ESXi Web 界面地址；反代部署推荐 `/esxi/ui/` |
| `GUACD_URL` | 空 | guacd 地址（如 `127.0.0.1:4822`），远程桌面推荐模式 |
| `JWT_SECRET` | 开发密钥 | **生产必须显式配置** |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | 管理员引导账号，上线前务必改 |

完整清单见 [.env.example](./.env.example) 与 [docs/how-to-use.md](./docs/how-to-use.md) §4。

## 测试

```bash
cd backend && go test ./... -count=1        # 单元 + 仓储 + API 全量（提交/交付前提）
make test-esxi                              # ESXi 适配器（vcsim 模拟 vCenter，免真实环境）
cd frontend && npm run build                # 前端生产构建（产出 .output/）
```

- esxi/integration 两组测试默认被 build tag 排除，需 `-tags esxi` / `-tags integration` 显式执行；
- 真实 ESXi 集成测试（真机验证/回归）约定见 [docs/conventions.md](./docs/conventions.md)。

## 项目结构

```
backend/    Go 后端（domain → ports → adapters 分层，Gin API；cmd/server 为装配点）
frontend/   Nuxt 3 前端（纯 SPA）
caddy/      Caddy 反向代理配置（生产权威版，含 ESXi 反代规则）
docs/       构建部署/开发规范/接口文档
docker-compose.yml  PostgreSQL / guacd / VNC 演示
```

## 开发进度

- ✅ Phase 0-1：基础设施、领域模型（100% 测试覆盖）
- ✅ Phase 2：仓储接口 + 内存实现 + VM 同步服务
- ✅ Phase 3：PostgreSQL 适配器 + SQLite（gormstore 共享层，零依赖调试）
- ✅ Phase 4：ESXi 适配器（govmomi + vcsim 测试；**2026-08-06 真实 ESXi 7.0.3 集成测试
  全链路通过**：ListVMs/GetHostInfo/电源操作/会话复用）+ Mock 提供者 + 工厂
- ✅ Phase 5：认证与 JWT（bcrypt、access/refresh 令牌、管理员引导）
- ✅ Phase 6：核心 API（VM 管理/动态菜单/插件管理/Guacamole WS 代理/RBAC）
- ✅ Phase 7：前端（Nuxt 3 + Element Plus：登录、仪表盘、VM 管理、暗色主题）
- ✅ Phase 8：Guacamole 浏览器远程桌面（guacd 原生隧道 + 配置面板 + RustDesk 一键连接）
- ✅ Phase 9：插件系统（权限/RBAC 矩阵、proxy/native 插件、示例 esxi-admin 嵌入
  ESXi Host Client，Caddy 反代 iframe/控制台已通）
- ⬜ Phase 10：CI/CD 与容器化部署（见 [About.md](./About.md) 进度表）
