#!/usr/bin/env bash
# PortalT 一键安装脚本（以生产环境为标准：systemd + Caddy 8808 + Docker(guacd/postgres)）
#
# 用法：
#   git clone https://github.com/Tonyjh07/PortalT.git && cd PortalT
#   bash deploy/install.sh          # 交互式（默认值即生产标准）
#   bash deploy/install.sh --yes    # 全默认非交互（postgres/guacd 容器 + mock 虚拟化）
#
# 仅需 bash + 包管理器（apt/dnf/apk），Go/Node.js/Caddy/Docker 自动安装。
# 重复执行安全（幂等）：已安装组件自动跳过，已存在的配置不会被覆盖。

set -euo pipefail
trap 'echo -e "\n\033[1;33m[INFO] 部署已取消\033[0m"; exit 1' INT TERM

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

NONINTERACTIVE=0
[ "${1:-}" = "--yes" ] && NONINTERACTIVE=1

# ============================================================
# 1. 环境准备
# ============================================================
header "环境准备"
require_sudo
detect_pkg_manager
ensure_systemd

REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
[ -f "$REPO_DIR/backend/go.mod" ] || error "未检测到 backend/go.mod，请从仓库根目录运行：bash deploy/install.sh"
ok "仓库根目录: $REPO_DIR"

# 基础工具（realpath 属 coreutils，默认存在）
ensure_cmds git curl tar openssl

# ============================================================
# 2. Go（按 go.mod 版本安装，已有则跳过）
# ============================================================
header "Go 工具链"
GO_VER=$(grep '^go ' "$REPO_DIR/backend/go.mod" | awk '{print $2}')
[ -n "$GO_VER" ] || GO_VER="1.22.5"
if command -v go >/dev/null 2>&1; then
    ok "Go $(go version | awk '{print $3}') 已安装"
else
    info "安装 Go $GO_VER ..."
    ARCH="amd64"; [ "$(uname -m)" = "aarch64" ] && ARCH="arm64"
    download /tmp/portalt-go.tar.gz "https://go.dev/dl/go${GO_VER}.linux-${ARCH}.tar.gz"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf /tmp/portalt-go.tar.gz
    sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
    ok "Go $(go version | awk '{print $3}') 已安装"
fi
export PATH="/usr/local/go/bin:$PATH"

# ============================================================
# 3. Node.js（官方二进制，已有则跳过）
# ============================================================
header "Node.js 工具链"
if command -v node >/dev/null 2>&1; then
    ok "Node.js $(node --version) 已安装"
else
    NODE_VER="v22.14.0"
    info "安装 Node.js $NODE_VER ..."
    ARCH="x64"; [ "$(uname -m)" = "aarch64" ] && ARCH="arm64"
    download /tmp/portalt-node.tar.xz "https://nodejs.org/dist/${NODE_VER}/node-${NODE_VER}-linux-${ARCH}.tar.xz"
    sudo rm -rf /usr/local/node
    sudo mkdir -p /usr/local/node
    sudo tar -C /usr/local/node --strip-components=1 -xJf /tmp/portalt-node.tar.xz
    sudo ln -sf /usr/local/node/bin/node /usr/local/bin/node
    sudo ln -sf /usr/local/node/bin/npm /usr/local/bin/npm
    sudo ln -sf /usr/local/node/bin/npx /usr/local/bin/npx
    ok "Node.js $(node --version) 已安装"
fi

# ============================================================
# 4. Caddy（系统包优先，缺失则下载官方二进制）
# ============================================================
header "Caddy"
USE_CADDY=1
if command -v caddy >/dev/null 2>&1; then
    ok "Caddy $(caddy version 2>/dev/null) 已安装"
else
    if ! prompt_yes "安装 Caddy 反代（推荐，WebSocket 远程桌面依赖）" "Y"; then
        warn "跳过 Caddy，将无反代入口（远程桌面 WebSocket 不可用）"
        USE_CADDY=0
    else
        CADDY_ARCH="amd64"; [ "$(uname -m)" = "aarch64" ] && CADDY_ARCH="arm64"
        download /tmp/portalt-caddy "https://github.com/caddyserver/caddy/releases/latest/download/caddy-linux-${CADDY_ARCH}"
        sudo mv /tmp/portalt-caddy /usr/local/bin/caddy
        sudo chmod 755 /usr/local/bin/caddy
        ok "Caddy $(caddy version 2>/dev/null) 已安装"
    fi
fi

# ============================================================
# 5. Docker（guacd 必需；postgres 模式也需）
# ============================================================
header "Docker"
HAS_DOCKER=0
DOCKER="sudo docker"
[ "$(id -u)" -eq 0 ] && DOCKER="docker"
if command -v docker >/dev/null 2>&1; then
    HAS_DOCKER=1
    ok "Docker 已安装"
else
    if prompt_yes "安装 Docker（运行 guacd 远程桌面网关）" "Y"; then
        case "$PKG_MANAGER" in
            apt)
                sudo apt-get update -y || true
                sudo apt-get install -y docker.io || error "docker.io 安装失败"
                sudo apt-get install -y docker-compose-plugin >/dev/null 2>&1 || true
                ;;
            dnf|yum)
                sudo dnf install -y docker || error "docker 安装失败"
                ;;
            apk)
                sudo apk add docker || error "docker 安装失败"
                ;;
        esac
        sudo systemctl enable --now docker >/dev/null 2>&1 || sudo service docker start || true
        if command -v docker >/dev/null 2>&1; then
            HAS_DOCKER=1
            ok "Docker 已安装"
        else
            warn "Docker 安装失败，guacd 需另行部署（GUACD_URL 指向可用实例）"
        fi
    else
        warn "跳过 Docker，guacd 需另行部署（GUACD_URL 指向可用实例）"
    fi
fi

# ============================================================
# 6. 构建产物（后端二进制 + 前端 .output）
# ============================================================
header "构建"
BACKEND_BIN="$REPO_DIR/backend/bin/portalt-server"
if [ -x "$BACKEND_BIN" ] && [ -z "$(find "$REPO_DIR/backend" -name '*.go' -newer "$BACKEND_BIN" -print -quit 2>/dev/null)" ]; then
    ok "后端二进制已存在（$BACKEND_BIN）"
else
    info "编译后端（Go 工具链，首次较慢）..."
    (cd "$REPO_DIR/backend" && CGO_ENABLED=0 go build -o bin/portalt-server ./cmd/server) || error "后端编译失败"
    ok "后端编译完成"
fi

FRONTEND_OUT="$REPO_DIR/frontend/.output"
if [ -f "$FRONTEND_OUT/server/index.mjs" ]; then
    ok "前端产物已存在"
else
    command -v npm >/dev/null 2>&1 || error "npm 不可用"
    info "安装前端依赖（npm ci）..."
    if [ -f "$REPO_DIR/frontend/package-lock.json" ]; then
        (cd "$REPO_DIR/frontend" && npm ci) || error "前端依赖安装失败"
    else
        (cd "$REPO_DIR/frontend" && npm install) || error "前端依赖安装失败"
    fi
    info "构建前端..."
    (cd "$REPO_DIR/frontend" && npm run build) || error "前端构建失败"
    ok "前端构建完成"
fi

# ============================================================
# 7. 配置问卷（默认值即生产标准；已部署过则复用既有密钥/密码）
# ============================================================
header "部署配置（默认值即生产标准）"

DEPLOY_DIR="$(prompt "部署目录" "/opt/portalt")"

# 重复安装时读取既有配置作为默认值，避免回车即改变运行形态
OLD_DB_DRIVER=""; OLD_VIRT_PROVIDER=""
if [ -f "$DEPLOY_DIR/portalt.env" ]; then
    OLD_DB_DRIVER="$(grep '^DB_DRIVER=' "$DEPLOY_DIR/portalt.env" | head -1 | cut -d= -f2- || true)"
    OLD_VIRT_PROVIDER="$(grep '^VIRT_PROVIDER=' "$DEPLOY_DIR/portalt.env" | head -1 | cut -d= -f2- || true)"
fi
DB_DRIVER="$(prompt "数据库类型 (postgres/sqlite)" "${OLD_DB_DRIVER:-postgres}")"
VIRT_PROVIDER="$(prompt "虚拟化平台 (mock/esxi/workstation)" "${OLD_VIRT_PROVIDER:-mock}")"
BACKEND_PORT="$(prompt "后端监听地址" "127.0.0.1:8080")"
FRONTEND_PORT="$(prompt "前端预览端口" "3001")"

if [ "$DB_DRIVER" = "postgres" ] && [ "$HAS_DOCKER" = "0" ]; then
    error "postgres 依赖 Docker（本机未就绪）。请先安装 Docker 或改用 sqlite 数据库"
fi

# 重复安装时复用已有 portalt.env 的密钥/密码，避免轮换后连不上既有库
JWT_SECRET=""; ADMIN_PASSWORD=""; DB_PASSWORD=""
if [ -f "$DEPLOY_DIR/portalt.env" ]; then
    info "检测到已有 $DEPLOY_DIR/portalt.env，复用其密钥与数据库密码（如需轮换请手动编辑）"
    JWT_SECRET="$(grep '^JWT_SECRET=' "$DEPLOY_DIR/portalt.env" | head -1 | cut -d= -f2- || true)"
    ADMIN_PASSWORD="$(grep '^ADMIN_PASSWORD=' "$DEPLOY_DIR/portalt.env" | head -1 | cut -d= -f2- || true)"
    if [ "$OLD_DB_DRIVER" = "postgres" ]; then
        DB_PASSWORD="$(grep '^DB_DSN=' "$DEPLOY_DIR/portalt.env" | head -1 | sed -E 's/.*password=([^ ]+).*/\1/' || true)"
    fi
fi
JWT_SECRET="${JWT_SECRET:-$(gen_secret)}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-$(gen_password)}"

if [ "$DB_DRIVER" = "sqlite" ]; then
    DB_DSN="$DEPLOY_DIR/data/portalt.db"
else
    DB_PASSWORD="${DB_PASSWORD:-$(gen_password)}"
    DB_DSN="host=127.0.0.1 port=5432 user=portalt password=${DB_PASSWORD} dbname=portalt sslmode=disable"
fi

VIRT_URL=""; VIRT_USERNAME=""; VIRT_PASSWORD=""; VIRT_INSECURE=""; ESXI_WEB_URL=""
if [ "$VIRT_PROVIDER" = "esxi" ]; then
    VIRT_URL="$(prompt "ESXi SDK URL (如 https://esxi.lan/sdk)" "")"
    [ -n "$VIRT_URL" ] || error "ESXi SDK URL 必填"
    VIRT_USERNAME="$(prompt "ESXi 用户名" "root")"
    read -r -s -p "ESXi 密码: " VIRT_PASSWORD; echo
    [ -n "$VIRT_PASSWORD" ] || error "ESXi 密码必填"
    case "$VIRT_PASSWORD" in *' '*|*'"'*) error "密码含空格/引号（systemd EnvironmentFile 限制），请重新输入" ;; esac
    VIRT_INSECURE="$(prompt "跳过证书验证 (true/false)" "true")"
    ESXI_WEB_URL="$(prompt "ESXi Web 界面路径" "/esxi/ui/")"
elif [ "$VIRT_PROVIDER" = "workstation" ]; then
    VIRT_URL="$(prompt "Workstation vmrest URL" "http://127.0.0.1:8697")"
    VIRT_USERNAME="$(prompt "Workstation 用户名" "")"
    read -r -s -p "Workstation 密码: " VIRT_PASSWORD; echo
    case "$VIRT_PASSWORD" in *' '*|*'"'*) error "密码含空格/引号（systemd EnvironmentFile 限制），请重新输入" ;; esac
fi

GUACD_URL="127.0.0.1:4822"
if [ "$HAS_DOCKER" = "0" ]; then
    GUACD_URL="$(prompt "Guacd 地址" "127.0.0.1:4822")"
fi

CADDY_PORT="8808"
ESXI_UPSTREAM=""
if [ "$USE_CADDY" = "1" ]; then
    CADDY_PORT="$(prompt "Caddy 入口端口" "8808")"
    if [ "$VIRT_PROVIDER" = "esxi" ]; then
        ESXI_UPSTREAM="$(echo "$VIRT_URL" | sed -E 's|^https?://([^/]+).*|\1|')"
        ESXI_UPSTREAM="$(prompt "ESXi 反代上游" "$ESXI_UPSTREAM")"
    fi
fi

USE_CF=0
CF_TOKEN=""
if prompt_yes "安装 cloudflared 隧道（可选）" "N"; then
    USE_CF=1
    CF_TOKEN="$(prompt "Cloudflare 隧道 Token" "")"
    [ -n "$CF_TOKEN" ] || error "Token 必填"
fi

# ============================================================
# 8. 部署摘要确认
# ============================================================
echo -e "\n${BOLD}${YELLOW}即将部署：${NC}"
echo "  部署目录: $DEPLOY_DIR    数据库: $DB_DRIVER    虚拟化: $VIRT_PROVIDER"
echo "  后端: $BACKEND_PORT    前端: $FRONTEND_PORT"
[ "$USE_CADDY" = "1" ] && echo "  Caddy: :$CADDY_PORT" || echo "  Caddy: 否"
[ "$USE_CF" = "1" ] && echo "  cloudflared: 是" || echo "  cloudflared: 否"
[ "$VIRT_PROVIDER" = "esxi" ] && echo "  ESXi: $VIRT_URL"
prompt_yes $'\n确认部署' "Y" || { info "已取消"; exit 0; }

# ============================================================
# 9. 基础设施容器（postgres / guacd）
# ============================================================
header "基础设施容器"
if [ "$HAS_DOCKER" = "1" ]; then
    if ! $DOCKER inspect portalt-guacd >/dev/null 2>&1; then
        info "启动 guacd 容器 ..."
        $DOCKER run -d --name portalt-guacd --restart=unless-stopped \
            -p 127.0.0.1:4822:4822 guacamole/guacd:1.5.5 >/dev/null \
            || error "guacd 容器启动失败"
        ok "guacd 容器已启动"
    else
        ok "guacd 容器已存在"
    fi
    if [ "$DB_DRIVER" = "postgres" ] && ! $DOCKER inspect portalt-postgres >/dev/null 2>&1; then
        info "启动 postgres 容器 ..."
        $DOCKER run -d --name portalt-postgres --restart=unless-stopped \
            -e POSTGRES_USER=portalt -e POSTGRES_PASSWORD="$DB_PASSWORD" -e POSTGRES_DB=portalt \
            -p 127.0.0.1:5432:5432 -v portalt-pgdata:/var/lib/postgresql/data \
            postgres:15 >/dev/null \
            || error "postgres 容器启动失败"
        ok "postgres 容器已启动"
    fi
fi

# ============================================================
# 10. 部署文件
# ============================================================
header "部署文件"
sudo mkdir -p "$DEPLOY_DIR"/{frontend,migrations,logs}
sudo chmod 700 "$DEPLOY_DIR"

sudo cp "$BACKEND_BIN" "$DEPLOY_DIR/portalt-server"
sudo chmod 755 "$DEPLOY_DIR/portalt-server"
ok "后端二进制已部署"

sudo rm -rf "$DEPLOY_DIR/frontend/.output"
sudo cp -a "$FRONTEND_OUT" "$DEPLOY_DIR/frontend/.output"
ok "前端产物已部署"

sudo cp -a "$REPO_DIR/backend/migrations/." "$DEPLOY_DIR/migrations/"
ok "数据库迁移文件已部署"

cat > /tmp/portalt.deployed <<EOF
DEPLOY_DIR=$DEPLOY_DIR
REPO_DIR=$REPO_DIR
EOF
chmod 644 /tmp/portalt.deployed
sudo mv /tmp/portalt.deployed "$DEPLOY_DIR/.deployed"

# ============================================================
# 11. 生成 portalt.env（键集与生产一致，600 权限）
# ============================================================
cat > /tmp/portalt.env <<EOF
# PortalT 环境配置（由 install.sh 生成，修改后需重启 portalt-backend）
DB_DRIVER=$DB_DRIVER
DB_DSN=$DB_DSN
DB_MIGRATIONS_DIR=migrations
JWT_SECRET=$JWT_SECRET
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$ADMIN_PASSWORD
PORT=$BACKEND_PORT
VIRT_PROVIDER=$VIRT_PROVIDER
EOF
chmod 600 /tmp/portalt.env
if [ -n "$VIRT_URL" ]; then
    cat >> /tmp/portalt.env <<EOF
VIRT_URL=$VIRT_URL
VIRT_USERNAME=$VIRT_USERNAME
VIRT_PASSWORD=$VIRT_PASSWORD
VIRT_INSECURE=$VIRT_INSECURE
EOF
    [ -n "$ESXI_WEB_URL" ] && echo "ESXI_WEB_URL=$ESXI_WEB_URL" >> /tmp/portalt.env
fi
echo "GUACD_URL=$GUACD_URL" >> /tmp/portalt.env
sudo mv /tmp/portalt.env "$DEPLOY_DIR/portalt.env"
sudo chmod 600 "$DEPLOY_DIR/portalt.env"
ok "portalt.env 已生成"

# ============================================================
# 12. systemd 服务
# ============================================================
header "systemd 服务"

cat > /tmp/portalt-backend.service <<EOF
[Unit]
Description=PortalT Backend
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
WorkingDirectory=$DEPLOY_DIR
EOF
if [ "$DB_DRIVER" = "postgres" ]; then
    cat >> /tmp/portalt-backend.service <<EOF
ExecStartPre=/usr/bin/env bash -c "for i in \$(seq 1 30); do docker exec portalt-postgres pg_isready -U portalt -d portalt && exit 0; sleep 2; done; exit 1"
EOF
fi
cat >> /tmp/portalt-backend.service <<EOF
ExecStart=$DEPLOY_DIR/portalt-server
EnvironmentFile=$DEPLOY_DIR/portalt.env
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
sudo mv /tmp/portalt-backend.service /etc/systemd/system/portalt-backend.service
sudo chmod 644 /etc/systemd/system/portalt-backend.service

cat > /tmp/portalt-frontend.service <<EOF
[Unit]
Description=PortalT Frontend
After=network.target

[Service]
Type=simple
WorkingDirectory=$DEPLOY_DIR/frontend
Environment=HOST=127.0.0.1
Environment=PORT=$FRONTEND_PORT
ExecStart=$(command -v node) .output/server/index.mjs
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
sudo mv /tmp/portalt-frontend.service /etc/systemd/system/portalt-frontend.service
sudo chmod 644 /etc/systemd/system/portalt-frontend.service

sudo systemctl daemon-reload
service_enable_now portalt-backend
service_enable_now portalt-frontend
ok "portalt-backend / portalt-frontend 已注册并启动"

# ============================================================
# 13. Caddy 配置（仓库 Caddyfile 直接使用，端口/上游走环境变量）
# ============================================================
if [ "$USE_CADDY" = "1" ]; then
    header "Caddy 配置"
    [ -f "$REPO_DIR/caddy/Caddyfile" ] || error "仓库缺少 caddy/Caddyfile"
    sudo mkdir -p /etc/caddy
    sudo cp "$REPO_DIR/caddy/Caddyfile" /etc/caddy/Caddyfile

    if systemctl list-unit-files caddy.service >/dev/null 2>&1; then
        # 系统包自带 caddy.service：drop-in 注入环境变量
        sudo mkdir -p /etc/systemd/system/caddy.service.d
        cat > /tmp/portalt-caddy.conf <<EOF
[Service]
Environment=CADDY_PORT=$CADDY_PORT
EOF
        [ -n "$ESXI_UPSTREAM" ] && echo "Environment=ESXI_UPSTREAM=$ESXI_UPSTREAM" >> /tmp/portalt-caddy.conf
        sudo mv /tmp/portalt-caddy.conf /etc/systemd/system/caddy.service.d/portalt.conf
        sudo systemctl daemon-reload
        service_enable_now caddy
    else
        # 二进制安装：生成独立服务
        cat > /tmp/portalt-caddy.service <<EOF
[Unit]
Description=PortalT Caddy
After=network.target

[Service]
Type=simple
User=root
Environment=CADDY_PORT=$CADDY_PORT
EOF
        [ -n "$ESXI_UPSTREAM" ] && echo "Environment=ESXI_UPSTREAM=$ESXI_UPSTREAM" >> /tmp/portalt-caddy.service
        cat >> /tmp/portalt-caddy.service <<EOF
ExecStart=/usr/local/bin/caddy run --config /etc/caddy/Caddyfile
WorkingDirectory=/etc/caddy
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
        sudo mv /tmp/portalt-caddy.service /etc/systemd/system/portalt-caddy.service
        sudo systemctl daemon-reload
        service_enable_now portalt-caddy
    fi
    ok "Caddy 已配置（:${CADDY_PORT}）"
else
    if [ -f /etc/caddy/Caddyfile ] || systemctl list-unit-files portalt-caddy.service >/dev/null 2>&1; then
        warn "检测到既有 Caddy 配置未清理（本次未选择 Caddy）。如需停用请手动处理"
    fi
fi

# ============================================================
# 14. cloudflared（可选，生产标准：http2 协议 + token 文件）
# ============================================================
if [ "$USE_CF" = "1" ]; then
    header "cloudflared 隧道"
    if ! command -v cloudflared >/dev/null 2>&1; then
        ARCH="amd64"; [ "$(uname -m)" = "aarch64" ] && ARCH="arm64"
        download /tmp/portalt-cloudflared "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${ARCH}"
        sudo mv /tmp/portalt-cloudflared /usr/local/bin/cloudflared
        sudo chmod 755 /usr/local/bin/cloudflared
    fi
    sudo mkdir -p /etc/cloudflared
    echo -n "$CF_TOKEN" | sudo tee /etc/cloudflared/token >/dev/null
    sudo chmod 600 /etc/cloudflared/token
    cat > /tmp/cloudflared.service <<EOF
[Unit]
Description=Cloudflare Tunnel client
After=network-online.target
Wants=network-online.target

[Service]
TimeoutStartSec=120
Type=notify
ExecStart=/usr/local/bin/cloudflared --no-autoupdate --protocol=http2 tunnel run --token-file /etc/cloudflared/token
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF
    sudo mv /tmp/cloudflared.service /etc/systemd/system/cloudflared.service
    sudo chmod 644 /etc/systemd/system/cloudflared.service
    sudo systemctl daemon-reload
    service_enable_now cloudflared
    ok "cloudflared 隧道已配置（--protocol=http2，WS 兼容）"
else
    if [ -f /etc/cloudflared/token ] || systemctl list-unit-files cloudflared.service >/dev/null 2>&1; then
        warn "检测到既有 cloudflared 配置未清理（本次未选择隧道）。如需停用请手动处理"
    fi
fi

# ============================================================
# 15. 健康检查与摘要
# ============================================================
header "健康检查"
BACKEND_PORT_NUM="${BACKEND_PORT##*:}"
if wait_http "http://127.0.0.1:${BACKEND_PORT_NUM}/healthz"; then
    ok "后端健康检查通过 (http://127.0.0.1:${BACKEND_PORT_NUM}/healthz)"
else
    error "后端健康检查超时（查看: journalctl -u portalt-backend -n 50）"
fi
if [ "$USE_CADDY" = "1" ] && wait_http "http://127.0.0.1:${CADDY_PORT}/" 20; then
    ok "Caddy 入口可访问 (http://127.0.0.1:${CADDY_PORT}/)"
elif [ "$USE_CADDY" = "1" ]; then
    error "Caddy 入口访问失败（查看: journalctl -u caddy -n 50）"
fi

header "部署完成"
echo -e "${BOLD}${GREEN}══════════════════════════════════════════════${NC}"
echo -e "${BOLD}${GREEN}   PortalT 已部署到 ${DEPLOY_DIR}${NC}"
echo -e "${BOLD}${GREEN}══════════════════════════════════════════════${NC}"
echo ""
if [ "$USE_CADDY" = "1" ]; then
    echo -e "  ${BOLD}入口地址:${NC}      http://127.0.0.1:${CADDY_PORT}"
else
    echo -e "  ${BOLD}入口地址:${NC}      http://127.0.0.1:${FRONTEND_PORT}（未安装 Caddy）"
fi
echo -e "  ${BOLD}管理账号:${NC}      admin / ${ADMIN_PASSWORD}"
echo -e "  ${BOLD}JWT 密钥:${NC}      ${JWT_SECRET}"
echo -e "  ${BOLD}数据库:${NC}        ${DB_DRIVER}"
[ "$VIRT_PROVIDER" != "mock" ] && echo -e "  ${BOLD}虚拟化:${NC}        ${VIRT_PROVIDER} (${VIRT_URL})"
echo -e "  ${BOLD}配置文件:${NC}      ${DEPLOY_DIR}/portalt.env"
echo -e "  ${BOLD}服务:${NC}         portalt-backend / portalt-frontend"
echo ""
echo -e "${RED}请立即记录管理员密码和 JWT 密钥！${NC}"
echo -e "${YELLOW}查看: grep -E 'JWT_SECRET|ADMIN_PASSWORD' ${DEPLOY_DIR}/portalt.env${NC}"
echo ""
echo -e "${BOLD}后续更新:${NC} cd $REPO_DIR && git pull && bash deploy/update.sh"
echo ""
