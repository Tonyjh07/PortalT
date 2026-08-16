# PortalT 构建指南

本节描述项目各组件的构建方式、Docker 镜像、CI 流水线。
部署指南（install/update 脚本、systemd 配置）见 [how-to-use.md](./how-to-use.md)。

## 1. 工具链与版本

| 组件 | 版本 | 来源 | 说明 |
|------|------|------|------|
| Go | 1.26.5（`backend/go.mod`） | go.dev 或系统包 | 仅构建后端需要；install.sh 按 go.mod 自动安装 |
| Node.js | 22（`deploy/install.sh` 固定 v22.14.0 / `frontend/Dockerfile` 用 node:22-alpine） | nodejs.org 或系统包 | 仅构建前端需要 |
| Docker | 24+（可选） | Docker Engine | 镜像构建或运行 guacd/postgres 容器 |

关键特性：
- 后端**纯 Go 无 cgo 依赖**（SQLite 驱动为 `glebarez/sqlite`，基于 modernc.org 纯 Go 实现），构建一律 `CGO_ENABLED=0`；
- 前端依赖 `patch-package`（`frontend/patches/`），`npm install` / `npm ci` 的 postinstall 阶段自动应用，**仅 dev 生效**（补丁修复 Windows 绝对路径与全局 CSS MIME 的 dev-only bug）；
- 国内网络下 Go 依赖已配置 `GOPROXY=https://goproxy.cn,direct`（已持久化到用户环境，CI runner 在海外用默认源）。

## 2. 后端构建

### 手工构建

```bash
cd backend
CGO_ENABLED=0 go build -o bin/portalt-server ./cmd/server
```

产物：`backend/bin/portalt-server`（Linux/macOS）/ `portalt-server.exe`（Windows）。

### Makefile

```bash
make build-backend     # 产物 backend/bin/portalt（注意与 install.sh 的 portalt-server 名不同）
```

### 交叉编译（构建机 ≠ 部署机）

```bash
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o portalt-server ./cmd/server
# aarch64（如 ARM 服务器）
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o portalt-server ./cmd/server
```

纯 Go 无 cgo，交叉编译零额外依赖。

## 3. 前端构建

```bash
cd frontend
npm ci              # 按 lockfile 安装（避免版本漂移）；postinstall 自动 patch-package
npm run build       # 产出 frontend/.output/（纯 SPA，ssr: false）
```

产物目录 `frontend/.output/` 整目录拷贝部署。Preview 运行：

```bash
cd frontend
PORT=3001 node .output/server/index.mjs
```

## 4. 插件构建

### frpc-admin（独立 Go 模块 + Vue 前端）

frpc-admin 是独立 Go module（`portalt-plugins/frpc-admin`），通过 `replace portalt => ../../../backend` 引用主模块。

```bash
# 后端二进制
cd backend/plugins/frpc-admin
CGO_ENABLED=0 go build -o plugin ./cmd/frpc-admin

# 前端 SPA（构建产物按约定提交到 static/）
cd backend/plugins/frpc-admin/frontend
npm ci && npm run build    # build 含 vue-tsc --noEmit 类型检查
```

### 其他插件

| 目录 | 类型 | 构建方式 |
|------|------|----------|
| `backend/plugins/examples/hello` | 主模块子包 | `cd backend && go build -o plugin ./plugins/examples/hello/cmd/hello` |
| `backend/plugins/template` | 独立模块，含 Makefile | `cd backend/plugins/template && make build` |

## 5. Docker 镜像

生产部署默认走 `deploy/update.sh`（systemd + 二进制），镜像用于容器化运行与 CI 构建校验。

```bash
# Makefile（VERSION 默认 v0.1）
make docker-build          # 构建全部：backend + frontend

# 手工构建
docker build -t portalt/backend:ci  backend/
docker build -t portalt/frontend:ci frontend/
```

### backend 镜像（多阶段构建）

- builder：`golang:1.26-alpine`，`CGO_ENABLED=0 go build`；
- runner：`alpine:3`（ca-certificates + tzdata），拷入二进制与 `migrations/`。

运行要点：

```bash
docker run -d \
  -e DB_DRIVER=sqlite -e DB_DSN=/data/portalt.db \
  -e JWT_SECRET=xxx -e ADMIN_PASSWORD=xxx \
  -e GUACD_URL=host.docker.internal:4822 \
  -e PLUGINS_DIR=/plugins \
  -v portalt-data:/data \
  -p 8080:8080 \
  portalt/backend:ci
```

- 默认 `PORT=0.0.0.0:8080`（容器内必须监听全接口）；
- postgres 模式需额外连 postgres 容器（设 `DB_DRIVER=postgres` + `DB_DSN`）；
- guacd 若同 docker network，`GUACD_URL` 用容器名。

### frontend 镜像（两阶段构建）

- builder：`node:22-alpine`，`npm install` → `npm run build`；
- runner：`node:22-alpine`，拷入 `.output/` + `node_modules`，`node .output/server/index.mjs`。

```bash
docker run -d -p 3000:3000 portalt/frontend:ci
```

## 6. CI 流水线（GitHub Actions）

配置文件：`.github/workflows/ci.yml`。

### 触发条件

| 事件 | 条件 |
|------|------|
| push | main 分支；paths 忽略 `*.md` 和 `docs/**` |
| pull_request | 任意目标分支 |

同分支新推送自动取消旧跑（concurrency）。

### Job 结构

| Job | 内容 | 本地等价命令 |
|-----|------|------------|
| **lint** | golangci-lint（默认 linters 集） | `cd backend && golangci-lint run` |
| **backend** | build + vet + 单测 + vcsim + pluginhost 集成 + race | `go build ./... && go vet ./... && go test ./... -count=1 && go test -tags esxi ./internal/adapters/esxi/... && go test -tags integration ./internal/pluginhost/... && go test -race ./...` |
| **backend-integration** | postgres 服务容器 + adapters 集成测试 | `TEST_DATABASE_URL=postgres://... go test -tags integration ./internal/adapters/...` |
| **frontend** | npm ci + build | `cd frontend && npm ci && npm run build` |
| **plugins** | frpc-admin 二进制 + 前端构建 | `cd backend/plugins/frpc-admin && CGO_ENABLED=0 go build ./cmd/frpc-admin && cd frontend && npm ci && npm run build` |
| **docker** | buildx 构建两个镜像（不推送） | `docker build backend/ && docker build frontend/` |

docker job 依赖前述全部通过（needs）。

### 本地运行 golangci-lint

```bash
cd backend
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run
```

配置文件：`backend/.golangci.yml`（启用默认 linters 集，ST1000/ST1003/ST1005/ST1021 因仓库命名惯例豁免）。

## 7. 测试

测试命令与约定详见 [conventions.md](./conventions.md)。

| 命令 | 覆盖范围 | 外部依赖 |
|------|---------|---------|
| `go test ./... -count=1` | 单测 + 仓储 + API（排除 esxi/integration tag） | 无 |
| `go test -tags esxi ./internal/adapters/esxi/...` | vcsim 进程内模拟 ESXi | 无 |
| `go test -tags integration ./internal/pluginhost/...` | 插件宿主全生命周期（自编 hello 插件） | Go 工具链 |
| `go test -tags integration ./internal/adapters/...` | postgres + sqlite 集成（真实 ESXi 无凭据 skip） | postgres（`TEST_DATABASE_URL`） |
| `go test -race ./...` | 全量竞态检测 | gcc |
