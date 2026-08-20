# PortalT 构建与部署指南（Debian + ESXi）

面向 HomeLab 的最简路径：在 ESXi 上开一台 Debian 门户虚拟机，从源码构建出后端二进制与
前端产物，部署为两个 systemd 服务，接入真实 ESXi 统一管理 VM，可选接入 Caddy 反代、
浏览器远程桌面与 ESXi Web 管理界面嵌入。

> 相关文档：[README.md](../../README.md)（项目概览/快速开始）、
> [remote-desktop.md](./remote-desktop.md)（远程桌面）、[external-access.md](./external-access.md)（外网访问/反代）。

## 1. 架构与组件

```
┌─────────────── Debian 门户 VM ───────────────┐
│  浏览器 ──► Caddy（可选，:8808）              │
│               ├─► 前端 preview（:3001, Nuxt SPA）┼─► /api、/native、WS ──► 后端（:8080）
│               └─► /esxi/*、/ui/*、/screen* 等 ──► ESXi Host Client（iframe 嵌入）
│  后端 ──► SQLite / PostgreSQL                 │
│  后端 ──► ESXi（SOAP SDK，govmomi）           │
│  后端 ──► guacd（可选，:4822）                │
└───────────────────────────────────────────────┘
```

- **后端**：Go 单二进制，REST API + VM 同步 + 电源操作 + WebSocket 隧道；
- **前端**：Nuxt 3 纯 SPA，`npm run build` 后由 Node 以 preview 模式运行；
- **数据库**：默认 SQLite（零依赖），可换 PostgreSQL；
- **ESXi**：后端直连 `https://<esxi-host>/sdk` 管理全部 VM（不开放 ESXi 管理端口对外）；
- **远程桌面（可选）**：guacd 守护进程 + VM metadata 连接参数；
- **Caddy（可选但推荐）**：唯一对外入口，反代前后端与 ESXi Web 界面。

## 2. 环境要求

| 组件 | 要求 | 说明 |
|------|------|------|
| 系统 | Debian 12+ (bookworm) | 门户 VM 建议 2 核 / 2GB+ |
| Go | 1.21+ | 仅构建后端需要 |
| Node.js | 20+ | 仅构建前端需要 |
| Caddy | 任意 2.x | 可选；仓库 `caddy/Caddyfile` 直接可用 |
| Docker | 可选 | 仅远程桌面演示/guacd 容器化时需要 |

构建机可以是部署机本身（构建完即可运行），也可以另用一台机器交叉编译后拷入。

## 3. 一键脚本部署（推荐）

仓库 `deploy/` 提供两个脚本（仅依赖 bash + 包管理器，其余依赖自动安装），
以生产环境为标准：systemd 服务 + Caddy 8808 反代 + Docker 运行 guacd/postgres
容器 + `/opt/portalt` 部署布局。

### 3.1 全新安装

```bash
git clone https://github.com/Tonyjh07/PortalT.git && cd PortalT
bash deploy/install.sh          # 交互式（默认值即生产标准，回车全默认）
bash deploy/install.sh --yes    # 全默认非交互（postgres/guacd 容器 + mock 虚拟化）
```

- 自动安装：Go（按 `go.mod` 版本）、Node.js 22、Caddy、Docker（guacd/postgres 容器）；
- 自动完成：后端编译 → 前端 `npm ci && npm run build` → 部署到 `/opt/portalt`
  → 生成 `portalt.env`（随机 JWT/管理员密码，仅展示一次）→ 注册并启动
  `portalt-backend` / `portalt-frontend` → Caddy 反代（:8808）→ 健康检查；
- 可选：ESXi/Workstation 凭据、cloudflared 隧道（http2 协议，WS 兼容）；
- 重复执行安全（幂等）：已安装组件自动跳过，已有配置不被覆盖。

### 3.2 日常更新

```bash
cd PortalT && bash deploy/update.sh
```

- 流程：git pull → 同步数据库迁移文件 → 重编译后端/重构建前端 → 备份旧产物
  → 替换部署 → 重启服务 → 健康检查，**任一步失败自动回滚**；
- **无新提交时不更新**（提示「已是最新版本」直接退出）；需强制重建同版本代码加 `--force`；
- 常用参数（可组合）：`--skip-pull` 跳过拉取、`--skip-backend` / `--skip-frontend`
  / `--skip-plugins` 跳过对应重建、`--skip-restart` 不重启、`--skip-health` 跳过健康检查；
- **回滚**：`bash deploy/update.sh --rollback` 回滚到上一版本，
  `--rollback 2` 回滚两个版本（最多 2），来源为更新时自动保留的历史备份；
- 不触碰 `portalt.env` / 数据库 / 容器数据；
- 以下手动流程（§4 起）仅用于自定义改造场景。

## 4. 构建

### 4.1 拉取源码

```bash
git clone https://github.com/Tonyjh07/PortalT.git
cd PortalT
```

### 4.2 后端二进制

```bash
cd backend
go build -o portalt-server ./cmd/server
```

产物 `portalt-server` 是单文件，可直接拷贝部署。国内网络若下载依赖超时，
已配置 `goproxy.cn` 镜像；必要时手动设置：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

### 4.3 前端产物

```bash
cd frontend
npm ci            # 按 lockfile 安装（避免版本漂移）
npm run build     # 产出 .output/（纯 SPA，无需 SSR）
```

产物目录 `frontend/.output/` 整目录拷贝部署。前端以 preview 模式运行：

```bash
PORT=3001 node .output/server/index.mjs
```

## 5. 配置（环境变量）

后端**直接读取系统环境变量**，不加载 `.env` 文件；生产环境用 systemd
`EnvironmentFile` 注入。常用变量（完整清单见仓库 `.env.example`）：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_DRIVER` | `sqlite` | `sqlite`（零依赖）/ `postgres` |
| `DB_DSN` | 空（内存库） | sqlite 文件路径如 `/opt/portalt/portalt.db` |
| `VIRT_PROVIDER` | `mock` | `esxi` 接入真实平台；`mock` 无平台也可演示 |
| `VIRT_URL` | - | ESXi 地址，如 `https://192.168.1.100/sdk` |
| `VIRT_USERNAME` / `VIRT_PASSWORD` | - | ESXi 账号（如 root） |
| `VIRT_INSECURE` | `false` | ESXi 自签证书时设 `true` |
| `JWT_SECRET` | 开发密钥 | **生产必须显式设置**，如 `openssl rand -hex 32` |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / `admin123` | 首次启动引导的管理员账号，**上线前务必改** |
| `GUACD_URL` | 空 | guacd 地址如 `127.0.0.1:4822`（远程桌面，可选） |
| `VM_SYNC_INTERVAL` | `60` | 后端周期同步 VM 状态的间隔（秒），设 `0` 或非法值回落默认 `60`；保持连接并刷新库存 |
| `PORT` | `127.0.0.1:8080` | 后端监听地址（保持 127.0.0.1，由 Caddy 反代） |

配置示例（`/opt/portalt/portalt.env`）：

```bash
DB_DRIVER=sqlite
DB_DSN=/opt/portalt/portalt.db
VIRT_PROVIDER=esxi
VIRT_URL=https://192.168.1.100/sdk
VIRT_USERNAME=root
VIRT_PASSWORD=change-me
VIRT_INSECURE=true
JWT_SECRET=replace-with-openssl-rand-hex-32
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change-me-too
```

`JWT_SECRET` 先在本机生成再填：`openssl rand -hex 32`（`EnvironmentFile` 是纯 key=value，
不支持命令替换）。

安全注意：该文件含 ESXi 明文凭据，权限设为 `600` 且仅 root 可读
（`chmod 600`、目录 `chmod 700`）。

## 6. 部署（systemd）

将二进制与产物放到 `/opt/portalt/`，然后创建两个服务单元。

### 后端 `portalt-backend.service`

```ini
[Unit]
Description=PortalT backend
After=network.target

[Service]
WorkingDirectory=/opt/portalt
ExecStart=/opt/portalt/portalt-server
EnvironmentFile=/opt/portalt/portalt.env
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

### 前端 `portalt-frontend.service`

```ini
[Unit]
Description=PortalT frontend preview
After=network.target portalt-backend.service

[Service]
WorkingDirectory=/opt/portalt/frontend
Environment=PORT=3001
ExecStart=/usr/bin/node .output/server/index.mjs
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

> 生产部署**必须经过 Caddy**（见 §8）：nuxt preview 的 `/api` 反代走 `routeRules`，
> 不支持 WebSocket 升级，远程桌面/ESXi 控制台的 WS 通道由 Caddy 负责透传。
>
> 手动单元与 `install.sh` 生成的版本一致，区别仅在：后者在 postgres 模式下额外带
> `After=network.target docker.service`、`Wants=docker.service` 与等待数据库就绪的
> `ExecStartPre`（`docker exec portalt-postgres pg_isready`），前端单元设
> `HOST=127.0.0.1`（仅本机可达，由 Caddy 反代）。

### 启动与验证

```bash
systemctl daemon-reload
systemctl enable --now portalt-backend portalt-frontend

# 验证后端
curl http://127.0.0.1:8080/healthz
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"change-me-too"}'
# 返回 {"code":200,...,"data":{"access_token":"...",...}} 即正常
```

## 7. 接入真实 ESXi

1. 在 `portalt.env` 中设置 `VIRT_PROVIDER=esxi` 与 `VIRT_URL`/`VIRT_USERNAME`/
   `VIRT_PASSWORD`/`VIRT_INSECURE`（见 §5）；
2. 重启后端：`systemctl restart portalt-backend`——启动时会执行一次 `SyncVMs`
   全量同步，把 ESXi 上所有 VM 拉入门户目录（平台侧已删除的 VM 会被移除，属预期）；
3. 验证：

```bash
# jq 未安装先装：apt install -y jq
TOKEN=$(curl -s -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"change-me-too"}' | jq -r .data.access_token)
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/vms | jq
# data[] 应包含真实 VM（id/name/status/cpu/memory 等），带 Metadata.moid
```

4. 门户内即可对 VM 执行 开机/关机/重启（受状态规则约束：仅 poweredOff/suspended
   可开机）。VM 的 IP/控制台等均无需开放 ESXi 对外端口。

> 备用：未配置凭据时 `VIRT_PROVIDER=mock` 仍可完整演示门户（内置 3 台示例 VM）。

## 8. Caddy 反向代理（可选但推荐）

仓库 `caddy/Caddyfile` 是生产权威配置，复制即用、无需改内容（入口端口 `CADDY_PORT`、
目标 ESXi `ESXI_UPSTREAM` 由环境变量控制）。Debian 包自带 systemd 单元，直接接管：

```bash
apt install -y caddy
cp caddy/Caddyfile /etc/caddy/Caddyfile
systemctl edit caddy          # 注入环境变量（drop-in 配置）
systemctl restart caddy
```

`systemctl edit caddy` 写入：

```ini
[Service]
Environment=CADDY_PORT=8808
Environment=ESXI_UPSTREAM=esxi.lan
```

（快速验证也可前台跑：`caddy run --config caddy/Caddyfile`，注意先停掉包自带的
systemd 单元避免 admin 端口冲突：`systemctl stop caddy`。）

- `/api`、`/native`、WS 升级 → 后端 8080；页面 → 前端 3001；
- `/esxi/*`、`/ui/*`、`/screen*` 等 → ESXi Host Client（iframe 嵌入管理界面，
  含 VM 控制台预览截图；`/ticket` WS 控制台）；
- **ESXi Web 界面嵌入还要求 https 入口**（ESXi 前端硬编码 `https://`/`wss://`，
  http 页面下无法登录/开控制台）：隧道域名本身是 https 可直接用；本机/局域网访问
  按 `./external-access.md` §四·六 生成自签 RSA 证书并解开 `https://:8443`
  注释块，`tls` 指令里的证书路径改为部署机实际路径（勿用 `tls internal`，
  ECC 证书在 Windows 客户端握手失败）；
- 外网暴露（Cloudflare Tunnel/域名 TLS）完整流程见 `./external-access.md`。

## 9. 浏览器远程桌面（可选）

1. 启动 guacd：`docker compose up -d guacd`（或二进制方式安装 guacd 1.5.x）；
2. 后端配置 `GUACD_URL=127.0.0.1:4822` 并重启；
3. **连接参数按 VM 各自配置**，存于该 VM 的 metadata `guac.*` 键（门户 VM 详情页
   有配置面板，仅管理员）：
   ```json
   {
     "guac.protocol": "vnc",
     "guac.hostname": "192.168.1.50",
     "guac.port": "5900",
     "guac.password": "vnc-pass"
   }
   ```
   缺省：hostname 用 VM.IPAddress；port 按协议 vnc 5900 / rdp 3389 / ssh 22；
4. VM 卡片点「远程桌面」即连。演示/调试：compose 提供 `portalt-vnc-demo`
   （VNC 5900，密码 `portalt-demo`），mock VM 的 metadata 已指向它。

详细参数与排错见 [remote-desktop.md](./remote-desktop.md)。

## 10. 产物清单与部署布局

以下为 `install.sh` / `update.sh` 使用的标准部署布局（默认 `/opt/portalt`，
可通过 install.sh 交互配置修改）。

### 10.1 产物清单

| 源 | 部署目标 | 说明 |
|----|---------|------|
| `backend/bin/portalt-server` | `/opt/portalt/portalt-server` | 后端单二进制，755 |
| `frontend/.output/` | `/opt/portalt/frontend/.output/` | Nuxt SPA 产物（整目录拷贝） |
| `backend/migrations/` | `/opt/portalt/migrations/` | SQL 迁移文件（`DB_MIGRATIONS_DIR`） |
| `backend/plugins/<id>/plugin` | `/opt/portalt/plugins/<id>/plugin` | 官方 native 插件二进制 |
| `backend/plugins/<id>/manifest.json` | `/opt/portalt/plugins/<id>/manifest.json` | 插件清单（宿主按此扫描） |
| `backend/plugins/<id>/static/` | `/opt/portalt/plugins/<id>/static/` | 插件内置前端 SPA（如有） |
| `caddy/Caddyfile` | `/etc/caddy/Caddyfile` | Caddy 反代配置（install.sh 首次部署，update.sh 差异同步） |

> 构建细节见 [build.md](./build.md)。

### 10.2 部署目录树

```
/opt/portalt/                        # DEPLOY_DIR（install.sh 可自定义）
├── portalt-server                   # 后端二进制（由 systemd 启动）
├── portalt.env                      # 环境配置（600 权限，仅 root 可读）
├── .deployed                        # 部署标记（DEPLOY_DIR=... REPO_DIR=...）
├── frontend/
│   └── .output/                     # Nuxt SPA 产物（node server 运行）
├── migrations/                      # SQL 迁移文件（启动时自动执行）
├── plugins/                         # PLUGINS_DIR（native 插件运行时目录）
│   ├── frpc-admin/
│   │   ├── plugin                   # 插件二进制
│   │   ├── manifest.json            # 清单
│   │   └── static/                  # 内置前端 SPA
│   └── <用户自定义插件>/
├── logs/                            # 日志目录（备用）
└── data/                            # SQLite 数据库文件（DB_DSN 指向此目录时）
```

### 10.3 systemd 单元

由 `install.sh` 生成并注册（§6 有手工版本说明），与 §6 的区别：
- 后端单元始终带 `After=docker.service`、`Wants=docker.service`（`Wants` 为软依赖，
  docker 未装时仅告警不影响启动）；postgres 模式额外加等待数据库就绪的
  `ExecStartPre`（`docker exec portalt-postgres pg_isready`）；
- 前端单元设 `HOST=127.0.0.1`（仅本机可达，由 Caddy 反代）；
- Caddy 使用系统包自带单元或生成独立单元，drop-in 注入 `CADDY_PORT`/`ESXI_UPSTREAM`。

### 10.4 用户自定义插件投放

除 `install.sh` / `update.sh` 循环构建的**官方插件**外，用户可直接将预编译
产物投放 `PLUGINS_DIR`（`/opt/portalt/plugins/`）：

1. 创建子目录 `PLUGINS_DIR/<插件ID>/`；
2. 放入 `plugin`（二进制）、`manifest.json`（必需，宿主按此扫描发现插件）；
3. 可选放入 `static/`（内置前端 SPA）；
4. 后端启动或运行期间（fsnotify 热加载）自动发现并加载。

详细 manifest 契约与开发指南见 [plugins.md](./plugins.md)。

### 10.5 update.sh 备份与回滚

每次 `update.sh` 更新前自动备份，失败时自动回滚。备份命名与轮转规则如下表。

| 备份项 | 命名格式 | 回滚恢复目标 |
|--------|---------|------------|
| 后端二进制 | `portalt-server.bak.YYYYMMDDHHMMSS` | `/opt/portalt/portalt-server` |
| 前端产物 | `frontend/.output.bak.YYYYMMDDHHMMSS` | `/opt/portalt/frontend/.output/`（目录） |
| 插件目录 | `plugins.bak.YYYYMMDDHHMMSS` | `/opt/portalt/plugins/`（目录） |
| Caddyfile | `/etc/caddy/Caddyfile.bak.YYYYMMDDHHMMSS` | `/etc/caddy/Caddyfile` |

> 前三项（二进制/前端/插件）每次更新后保留最近 2 份，旧份自动清理；
> **Caddyfile 备份不参与轮转**——仅在 `caddy validate` 失败时用于回滚，成功路径下
> 每次同步均新增一份（建议定期手工清理 `/etc/caddy/Caddyfile.bak.*`）。

回滚用法：

```bash
bash deploy/update.sh --rollback        # 回滚到上一版本
bash deploy/update.sh --rollback 2      # 回滚两个版本（最多 2）
```

回滚前会把当前版本备份（可用 `--rollback 1` 撤销本次回滚），详见 `--help`。

## 11. 验证清单（部署完成）

| 项 | 方法 |
|----|------|
| 后端健康 | `curl http://127.0.0.1:8080/healthz` → 200 |
| 登录 | 浏览器访问入口 → `admin` 登录成功 |
| VM 目录 | VM 列表显示真实 ESXi VM（含状态/资源） |
| 电源操作 | 对测试 VM 执行 开机/关机/重启，状态轮询生效 |
| ESXi 管理界面 | esxi-admin 插件页：加载、登录、VM 控制台可打开 |
| 远程桌面（可选） | VM 详情页「远程桌面」连上并渲染画面 |

## 12. 常见问题

| 现象 | 处理 |
|------|------|
| 登录返回 401 | 管理员账号由 `ADMIN_USERNAME/ADMIN_PASSWORD` 引导，检查 `portalt.env` 与实际值 |
| VM 列表为空 | 看后端日志 `journalctl -u portalt-backend`；检查 ESXi 地址/凭据/自签（`VIRT_INSECURE`） |
| 电源操作报错 | 状态规则限制（如已开机的 VM 不能再次开机）；ESXi 会话/权限问题看日志 |
| 端口被占用 | `ss -ltnp` 查 8080/3001/8808 占用；改 `PORT`/`CADDY_PORT` 后需同步 Caddy/隧道 |
| ESXi 管理界面打不开 | 确认 https 入口（8443 或隧道域名）；metadata/`ESXI_WEB_URL` 指向 `/esxi/ui/` |
| 远程桌面 WAITING | guacd 是否运行；VM metadata `guac.*` 是否指向可达目标（见 §9） |
| 构建下载超时 | Go：`go env -w GOPROXY=https://goproxy.cn,direct`；npm：配置镜像 registry |

## 13. 测试（开发/交付前）

```bash
cd backend && go test ./... -count=1     # 单元+仓储+API 全量（首跑偏慢属正常）
go test -tags esxi ./internal/adapters/esxi/...   # vcsim 模拟 vCenter（免真实环境）
go test -tags integration ./internal/adapters/... # 真实环境集成（需自备环境，见 conventions.md）
```

- 默认 `go test ./...` 不含 esxi/integration 两组（build tag 排除）；
- 真实 ESXi 集成测试需 `TEST_ESXI_URL/USERNAME/PASSWORD` 环境变量，见
  `./conventions.md`「虚拟化集成测试约定」。

---

本地 Windows 快速开发（PowerShell + mock）见 [README.md](../../README.md)「快速开始」。
