# PortalT 使用教程

从零开始：克隆仓库 → 安装依赖 → 配置 → 运行 → 验证 → 接入真实虚拟化与远程桌面。

> 相关文档：[README.md](../../README.md)（快速开始）、[conventions.md](./conventions.md)（工具链/测试规范）、[remote-desktop.md](./remote-desktop.md)（远程桌面）、[external-access.md](./external-access.md)（外网访问）。

## 1. 环境要求

| 工具 | 版本 | 说明 |
|------|------|------|
| Git | 任意 | 克隆仓库 |
| Go | 1.21+ | 后端（本机实测 1.26.5） |
| Node.js | 20+ | 前端（本机实测 24.x） |
| Docker | 24+ | 可选：PostgreSQL / guacd / VNC 演示（零依赖模式不需要） |
| GNU Make | 3.81+ | 可选：便捷命令（Windows 需 msys sh.exe） |

Windows PowerShell 用户注意：Go 可能不在 PATH，每个新会话先执行一次：

```powershell
$env:Path += ";C:\Program Files\Go\bin"
```

## 2. 克隆仓库

```bash
git clone https://github.com/Tonyjh07/PortalT.git
cd PortalT
```

## 3. 目录结构

```
backend/     Go 后端（domain → ports → adapters 分层，Gin API）
frontend/    Nuxt 3 前端（纯 SPA）
caddy/       Caddy 反向代理配置
docs/        文档（本文件所在目录）
Makefile     便捷命令（init/run/test/build/up/down 等）
.env.example 环境变量参考模板（复制为 .env 后仅作备忘，见 §4）
docker-compose.yml  PostgreSQL / guacd / VNC 演示 / Caddy
```

## 4. 配置

### 4.1 重要说明：环境变量的加载方式

- 后端（Go）**直接读取系统环境变量**，**不会自动加载 `.env` 文件**。
- `.env.example` 是参考模板：复制为 `.env` 记录你的配置即可，但要让配置生效必须写到 shell 环境变量（或部署时的系统服务/Docker 注入）。
- PowerShell 中设置方式（对当前会话生效，`go run` / `make` 均继承）：

```powershell
$env:DB_DRIVER = "sqlite"
$env:DB_DSN = "portalt.db"
$env:VIRT_PROVIDER = "mock"
$env:GUACD_URL = "127.0.0.1:4822"
```

### 4.2 环境变量一览

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_DRIVER` | `sqlite` | `postgres` / `sqlite`（sqlite 零依赖调试） |
| `DB_DSN` | 空（内存库） | postgres 连接串 / sqlite 文件路径（如 `portalt.db`） |
| `DB_MIGRATIONS_DIR` | `migrations` | 迁移脚本目录 |
| `VIRT_PROVIDER` | `mock` | 虚拟化平台：`mock` / `esxi` / `workstation` |
| `VIRT_URL` / `VIRT_USERNAME` / `VIRT_PASSWORD` / `VIRT_INSECURE` | 空 | 通用虚拟化配置（缺省回退 `VIRT_ESXI_*` → `VIRT_WS_*`） |
| `GUACD_URL` | 空 | guacd 原生隧道地址（如 `127.0.0.1:4822`），远程桌面推荐模式 |
| `GUAC_URL` | 空 | 旧模式：Guacamole Web 应用 WS 代理（与 `GUACD_URL` 二选一） |
| `GUAC_SECRET` | 空 | 旧模式签名密钥 |
| `JWT_SECRET` | 开发密钥 | 生产必须显式配置 |
| `JWT_ACCESS_TTL` / `JWT_REFRESH_TTL` | `900` / `604800`（秒） | 令牌有效期 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | 首次启动自动引导的管理员账号 |
| `DOMAIN` | - | Caddy 部署域名（见 `caddy/Caddyfile`） |

## 5. 安装依赖

二选一：

```bash
# 方式 A：Makefile（会同时处理目录骨架）
make init

# 方式 B：手动
cd backend && go mod download && cd ..
cd frontend && npm install --no-fund --no-audit && cd ..
```

网络提示：Go 依赖已走 `goproxy.cn` 镜像（全局配置，勿改回）；若 npm 安装超时可配置 npm 镜像源。

## 6. 运行（零依赖快速开始）

无需 Docker、无需真实虚拟化环境，直接跑通全栈：

```powershell
# 终端 1：后端（sqlite 内存库 + mock 虚拟化）
$env:Path += ";C:\Program Files\Go\bin"
$env:GUACD_URL = "127.0.0.1:4822"   # 远程桌面需要，可后补
go run ./cmd/server

# 终端 2：前端（127.0.0.1:3000，/api 自动代理到 8080）
npm run dev
```

访问 http://127.0.0.1:3000 ，使用 `admin` / `admin123` 登录。

> 想让数据持久化：`$env:DB_DSN = "portalt.db"`（生成 sqlite 文件）。

### 验证后端

```powershell
curl.exe -X POST http://127.0.0.1:8080/api/v1/auth/login -H "Content-Type: application/json" -d '{\"username\":\"admin\",\"password\":\"admin123\"}'
# 返回 {"code":200,...,"data":{"access_token":"...","refresh_token":"...",...}}
```

```powershell
curl.exe http://127.0.0.1:8080/api/v1/vms
# {"code":200,"message":"success","data":[{"id":...,"name":"mock-web-01",...}]}
```

## 7. 完整环境（Docker）

启动 PostgreSQL + guacd（远程桌面守护进程）+ VNC 演示目标 + Caddy：

```bash
docker compose up -d
```

| 容器 | 端口 | 用途 |
|------|------|------|
| portalt-postgres | 5432 | 生产数据库 |
| portalt-guacd | 4822 | Guacamole 协议守护进程 |
| portalt-vnc-demo | 5900 | VNC 演示桌面（密码 `portalt-demo`） |
| portalt-caddy | 80/443 | 反向代理 + 自动 HTTPS |

使用 PostgreSQL 运行后端：

```powershell
$env:DB_DRIVER = "postgres"
$env:DB_DSN = "postgres://portalt:securepassword@127.0.0.1:5432/portalt?sslmode=disable"
go run ./cmd/server
```

## 8. 接入真实虚拟化平台

### 8.1 ESXi

```powershell
$env:VIRT_PROVIDER = "esxi"
$env:VIRT_ESXI_URL = "https://esxi.lan/sdk"
$env:VIRT_ESXI_USERNAME = "root"
$env:VIRT_ESXI_PASSWORD = "password"
$env:VIRT_ESXI_INSECURE = "true"
go run ./cmd/server
```

启动时后端会调用 `SyncVMs` 全量对齐平台 VM 目录（平台侧已删除的 VM 会从本系统移除）。

### 8.2 VMware Workstation（vmrest，本机调试）

适合本机跑 Workstation 做开发调试。前置步骤（Windows，需管理员权限）：

```powershell
cd "C:\Program Files (x86)\VMware\VMware Workstation"
.\vmrest.exe -C        # 设置凭证（保存到 %USERPROFILE%\vmrest.cfg）
.\vmrest               # 启动 REST 服务（HTTPS 需 -c 证书 -k 私钥）
```

然后：

```powershell
$env:VIRT_PROVIDER = "workstation"
$env:VIRT_WS_URL = "http://127.0.0.1:8697"
$env:VIRT_WS_USERNAME = "<vmrest 用户名>"
$env:VIRT_WS_PASSWORD = "<vmrest 密码>"
go run ./cmd/server
```

> 适配器对 vmrest 各版本字段差异做了容错（状态大小写变体、CPU/内存新旧字段名），若仍解析异常，可先 `curl.exe http://127.0.0.1:8697/api/vms` 对比实际返回。

## 9. 远程桌面（Guacamole）

- 后端设置 `GUACD_URL=127.0.0.1:4822`（或 Docker 内 `guacd:4822`）后，前端 VM 卡片点「远程桌面」即可用。
- 连接参数**全部来自 VM metadata 的 `guac.*` 字段**（数据库 `vms.metadata` jsonb / sqlite json 列），不是全局配置：

```json
{
  "guac.protocol": "vnc",
  "guac.hostname": "127.0.0.1",
  "guac.port": "5900",
  "guac.username": "",
  "guac.password": "portalt-demo",
  "guac.width": "1280",
  "guac.height": "800"
}
```

- 缺省规则：hostname 缺省用 `VM.IPAddress`；port 按协议默认 vnc 5900 / rdp 3389 / ssh 22。
- 演示环境零配置即可用：mock 虚拟机的 metadata 已指向 compose 里的 `portalt-vnc-demo`（需 Docker 侧启动）。

详细排错见 [remote-desktop.md](./remote-desktop.md)。

## 10. 外网访问（Cloudflare Tunnel）

dev 模式前端固定监听 `127.0.0.1:3000`，经 `cloudflared` 隧道（域名 `demo.tonyjh07.dpdns.org`）可暴露到公网：

1. `~/.cloudflared/config.yml` 配置 ingress → `http://127.0.0.1:3000`；
2. 若浏览器报 `Blocked request. This host is not allowed`，把域名加进 `frontend/nuxt.config.ts` 的 `vite.server.allowedHosts` 并**重启 dev server**；
3. WS 升级走 `frontend/modules/wsProxy.ts`（仅 dev），无需额外配置。

完整流程与生产部署（Caddy）见 [external-access.md](./external-access.md)。

## 11. 测试

```bash
make test          # 单元测试汇总（domain + 仓储 + 虚拟化 + API）
make test-sqlite   # SQLite 集成测试（免服务）
make test-esxi     # ESXi 适配器（vcsim 模拟，免真实环境）
make test-integration  # 全部集成测试（PostgreSQL 需先 docker compose up -d postgres）
make test-race     # 竞态检测（Windows 需 MinGW，Makefile 自动探测）
```

或直接（推荐日常使用）：

```bash
cd backend
go test ./... -count=1
```

> 注意：esxi 与集成测试默认被 build tag 排除（`-tags esxi` / `-tags integration` 才跑）；`go test ./...` 默认不含它们。提交前须保证上述命令全绿。

## 12. 常见问题

| 现象 | 处理 |
|------|------|
| `go: command not found` / PowerShell 找不到 go | `$env:Path += ";C:\Program Files\Go\bin"` |
| 依赖下载超时 | Go 已配 `goproxy.cn`；npm 失败则配置 npm registry 镜像 |
| 前端 `Blocked request. This host is not allowed` | `nuxt.config.ts` 的 `vite.server.allowedHosts` 加域名后重启 dev server |
| 远程桌面一直 WAITING / 打不开 | 查 VM metadata `guac.*` 是否指向可达目标；guacd 容器是否 healthy（`docker compose ps`） |
| 修改 `nuxt.config.ts` host/allowedHosts 不生效 | dev server 必须重启 |
| 拉取 Docker 镜像失败 | 本机已配 `docker.m.daocloud.io` 镜像源；改 `~/.docker/daemon.json` 后必须完全退出 Docker Desktop 再启动 |
| 登录后 401 | 管理员账号由环境变量引导；后端日志会打印「管理员账号已就绪」 |
