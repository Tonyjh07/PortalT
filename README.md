# PortalT

HomeLab 统一门户系统：在虚拟机中运行的 Web 门户，统一管理 ESXi 上的虚拟机，提供浏览器远程桌面（Guacamole）、ESXi Web 管理界面嵌入与可扩展的插件系统。

> 完整需求与技术架构见 [About.md](./About.md)；构建、部署与使用指南见 [docs/how-to-use.md](./docs/how-to-use.md)；
> 开发规范与接口文档见 [docs/](./docs/README.md)。

## 功能特性

- **统一 VM 管理**：接入 ESXi / Workstation（Mock 提供者可用于纯演示），列表、状态、电源操作，门户内统一管理；
- **浏览器远程桌面**：Guacamole 网关（VNC/RDP/SSH），连接参数按 VM 配置，画质/流畅度自适应（低带宽自动降档），无需安装客户端；RustDesk 一键唤起作为备选通道；
- **ESXi Web 界面嵌入**：esxi-admin 插件 iframe 嵌入 ESXi Host Client（Caddy 反代 + 控制台 WS 透传），不开放 ESXi 管理端口对外；
- **权限体系**：RBAC 角色矩阵、资源级 VM 授权、插件权限声明；
- **插件系统**：access / native 两类（接入外部资源 / 独立进程插件），动态菜单与权限接入；重构计划见 [plugin-refactor-plan.md](./plugin-refactor-plan.md)；
- **外部访问**：Cloudflare Tunnel（cloudflared）出站隧道 + Caddy 反代，无需公网 IP / 入站端口。

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go + Gin + GORM |
| 前端 | Nuxt 3 + Vue 3 + Element Plus（纯 SPA，暗色主题） |
| 数据库 | PostgreSQL 15 / SQLite（零依赖部署） |
| Web 服务器 | Caddy 2（唯一对外入口） |
| 远程桌面 | Apache Guacamole（guacd） |
| 虚拟化 | govmomi (ESXi) / vmrest (Workstation) / Mock |
| 部署 | bash 脚本一键安装/更新（`deploy/`，systemd + Docker） |

## 快速开始（零依赖开发调试）

SQLite + Mock 虚拟化即可跑通全栈，无需 Docker/真实环境（Windows/PowerShell 或任意 Linux）：

```bash
# 终端 1：后端（默认 127.0.0.1:8080，启动自动引导管理员）
cd backend && go run ./cmd/server

# 终端 2：前端（127.0.0.1:3000，/api 自动代理到后端，含 WebSocket）
cd frontend && npm run dev
```

访问 http://127.0.0.1:3000 ，使用 `admin` / `admin123` 登录（mock 提供者内置 3 台示例 VM）。
后端 curl 验证：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
# {"code":200,"message":"success","data":{"access_token":"...","refresh_token":"...",...}}
```

远程桌面演示（可选）：`docker compose up -d` 启动 guacd + VNC 演示容器，
后端设 `GUACD_URL=127.0.0.1:4822` 后，示例 VM 即可直连桌面。

## 生产部署（一键脚本）

仓库 `deploy/` 提供一键安装/更新脚本，仅依赖 bash + 包管理器（apt/dnf/apk），其余依赖自动安装，
以生产环境为标准：systemd 服务 + Caddy 反代（:8808）+ Docker 容器（guacd/postgres）+ `/opt/portalt` 布局。

```bash
git clone https://github.com/Tonyjh07/PortalT.git && cd PortalT
bash deploy/install.sh          # 交互式（默认值即生产标准，回车全默认）
bash deploy/install.sh --yes    # 全默认非交互（postgres/guacd 容器 + mock 虚拟化）
```

日常更新：`cd PortalT && git pull && bash deploy/update.sh`（自动备份、失败回滚）。

详细说明（含手动部署、接入真实 ESXi、Caddy/远程桌面/隧道配置）见
[docs/how-to-use.md](./docs/how-to-use.md)。

## 环境变量（常用）

后端直接读取系统环境变量，不加载 `.env` 文件；生产环境由 systemd `EnvironmentFile` 注入
（`install.sh` 自动生成 `portalt.env`，见 [.env.example](./.env.example)）：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_DRIVER` / `DB_DSN` | `sqlite` / 空（内存库） | 数据库；`DB_DSN=portalt.db` 持久化 |
| `VIRT_PROVIDER` | `mock` | `esxi` / `workstation` / `mock` |
| `VIRT_URL` / `VIRT_USERNAME` / `VIRT_PASSWORD` / `VIRT_INSECURE` | - | 平台连接（esxi 时 url 必填；缺省回退 `VIRT_ESXI_*` → `VIRT_WS_*`） |
| `ESXI_WEB_URL` | 自动推导 | ESXi Web 界面地址；反代部署推荐 `/esxi/ui/` |
| `GUACD_URL` | 空 | guacd 地址（如 `127.0.0.1:4822`），远程桌面推荐模式 |
| `JWT_SECRET` | 开发密钥 | **生产必须显式配置** |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | 管理员引导账号，上线前务必改 |

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
deploy/     一键安装/更新脚本（install.sh / update.sh / lib/common.sh）
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
- ✅ Phase 8：Guacamole 浏览器远程桌面（guacd 原生隧道 + 配置面板 + 质量/流畅度模式
  + RustDesk 一键连接）
- ✅ Phase 9：插件系统（权限/RBAC 矩阵、proxy/native 插件、示例 esxi-admin 嵌入
  ESXi Host Client，Caddy 反代 iframe/控制台已通）
- 🔄 插件重构 Phase 1-2：access 收敛（type 收敛 access/native、iframe/proxy 合并、
  Caddy 规则落盘交互、esxi-admin 迁移 access、/api/v1/platform），原生插件进程化 Phase 3 规划中
- ✅ Phase 10 CI/CD：CI workflow + 构建文档 + 产物部署文档（2026-08-16
  实施完成；生产部署仍走 deploy/update.sh）
