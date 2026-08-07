#!/usr/bin/env bash
# PortalT 快速部署脚本
# 用法：git clone <仓库> && cd portalt && bash deploy/deploy.sh
# 自动安装 Go/Node.js → 构建 → 问卷配置 → 部署

set -euo pipefail
trap 'echo -e "\n\033[1;33m[INFO] 部署已取消\033[0m"; exit 1' INT TERM

# ============================================================
# 颜色与辅助函数
# ============================================================
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }
header() { echo -e "\n${BOLD}${CYAN}══ $* ══${NC}\n"; }

# ============================================================
# 路径检测
# ============================================================
header "环境准备"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

[ -f "$REPO_DIR/backend/go.mod" ] || error "未检测到 backend/go.mod，请从仓库根目录运行：bash deploy/deploy.sh"
ok "仓库根目录: $REPO_DIR"

# 检查 sudo 权限（后续大量操作依赖 sudo）
sudo -n true 2>/dev/null || error "当前用户无 sudo 权限，请使用有 sudo 权限的用户运行"

# ============================================================
# 包管理器检测
# ============================================================
PKG_MANAGER=""; PKG_INSTALL=""
if command -v apt &>/dev/null; then
    PKG_MANAGER="apt"; PKG_INSTALL="sudo apt-get install -y"
elif command -v dnf &>/dev/null; then
    PKG_MANAGER="dnf"; PKG_INSTALL="sudo dnf install -y"
elif command -v yum &>/dev/null; then
    PKG_MANAGER="yum"; PKG_INSTALL="sudo yum install -y"
elif command -v apk &>/dev/null; then
    PKG_MANAGER="apk"; PKG_INSTALL="sudo apk add"
else
    error "不支持的包管理器，请手动安装 Go 和 Node.js 后重试"
fi
ok "包管理器: $PKG_MANAGER"

# ============================================================
# 基础工具检查
# ============================================================
for cmd in curl tar sudo git; do
    command -v "$cmd" >/dev/null 2>&1 || { warn "安装 $cmd ..."; $PKG_INSTALL "$cmd"; }
done
# realpath 属于 coreutils（默认安装），不单独检查

# ============================================================
# Go 安装与后端构建
# ============================================================
GO_VER=$(grep '^go ' "$REPO_DIR/backend/go.mod" | awk '{print $2}')
[ -z "$GO_VER" ] && GO_VER="1.22.5"

if command -v go &>/dev/null; then
    ok "Go $(go version | awk '{print $3}') 已安装"
elif [ -f "$REPO_DIR/backend/bin/portalt" ]; then
    warn "Go 未安装，但后端二进制已存在，跳过安装"
else
    read -p "Go 未安装，是否自动安装 Go $GO_VER 并编译后端？(Y/n): " GO_ANS
    if [[ ! "$GO_ANS" =~ ^[Nn] ]]; then
        info "下载 Go $GO_VER ..."
        DOWNLOAD_OK=false
        if curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz" -o /tmp/go.tar.gz && [ -s /tmp/go.tar.gz ]; then
            DOWNLOAD_OK=true
        else
            warn "版本 $GO_VER 不可用，尝试 1.22.5"
            if curl -fsSL "https://go.dev/dl/go1.22.5.linux-amd64.tar.gz" -o /tmp/go.tar.gz && [ -s /tmp/go.tar.gz ]; then
                DOWNLOAD_OK=true
            fi
        fi
        [ "$DOWNLOAD_OK" = true ] || error "Go 下载失败，请检查网络或手动安装"
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf /tmp/go.tar.gz
        export PATH="/usr/local/go/bin:$PATH"
        # 脚本内持久化
        if [ -f "$HOME/.bashrc" ]; then
            echo 'export PATH="/usr/local/go/bin:$PATH"' >> "$HOME/.bashrc"
        fi
        ok "Go $(go version) 已安装"
    else
        error "请手动安装 Go $GO_VER 后重试（https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz）"
    fi
fi

# 构建后端
if [ -f "$REPO_DIR/backend/bin/portalt" ]; then
    BACKEND_SRC="$REPO_DIR/backend/bin/portalt"
    ok "后端二进制已存在"
else
    command -v go &>/dev/null || error "Go 不可用"
    info "编译后端..."
    (cd "$REPO_DIR/backend" && CGO_ENABLED=0 go build -o bin/portalt ./cmd/server)
    BACKEND_SRC="$REPO_DIR/backend/bin/portalt"
    ok "后端编译完成"
fi

# ============================================================
# Node.js 安装与前端构建
# ============================================================
if command -v node &>/dev/null; then
    ok "Node.js $(node --version) 已安装"
    command -v npm &>/dev/null || error "npm 不可用"
elif [ -d "$REPO_DIR/frontend/.output" ]; then
    warn "Node.js 未安装，但前端产物已存在，跳过安装"
else
    read -p "Node.js 未安装，是否自动安装 Node.js 22 LTS 并构建前端？(Y/n): " NODE_ANS
    if [[ ! "$NODE_ANS" =~ ^[Nn] ]]; then
        info "安装 Node.js 22 LTS ..."
        case "$PKG_MANAGER" in
            apt)
                curl -fsSL https://deb.nodesource.com/setup_22.x | sudo bash -
                $PKG_INSTALL nodejs
                ;;
            dnf|yum)
                curl -fsSL https://rpm.nodesource.com/setup_22.x | sudo -E bash -
                $PKG_INSTALL nodejs
                ;;
            apk)
                sudo apk add nodejs npm
                ;;
        esac
        ok "Node.js $(node --version) 已安装"
    else
        error "请手动安装 Node.js 22 LTS 后重试"
    fi
fi

# 构建前端
if [ -d "$REPO_DIR/frontend/.output" ]; then
    FRONTEND_SRC="$REPO_DIR/frontend/.output"
    ok "前端产物已存在"
else
    command -v node &>/dev/null || error "Node.js 不可用"
    command -v npm &>/dev/null || error "npm 不可用"
    info "安装前端依赖..."
    if [ -f "$REPO_DIR/frontend/package-lock.json" ]; then
        (cd "$REPO_DIR/frontend" && npm ci)
    else
        (cd "$REPO_DIR/frontend" && npm install)
    fi
    info "构建前端..."
    (cd "$REPO_DIR/frontend" && npm run build)
    FRONTEND_SRC="$REPO_DIR/frontend/.output"
    ok "前端构建完成"
fi

[ -f "$FRONTEND_SRC/server/index.mjs" ] || error "前端产物不完整：缺少 server/index.mjs"

# ============================================================
# Docker（可选）
# ============================================================
HAS_DOCKER=false
if command -v docker &>/dev/null; then
    HAS_DOCKER=true
    ok "Docker 已安装"
else
    warn "Docker 未安装，基础设施服务（PostgreSQL/guacd）须原生部署"
fi

# ============================================================
# systemd（可选）
# ============================================================
HAS_SYSTEMD=false
if command -v systemctl &>/dev/null; then
    HAS_SYSTEMD=true
    ok "systemd 可用"
else
    warn "systemd 不可用，将跳过服务注册"
fi

# ============================================================
# 问卷
# ============================================================
header "部署配置问卷（直接回车接受默认值）"

# 1. 部署目录
read -p "部署目录 [/opt/portalt]: " DEPLOY_DIR
DEPLOY_DIR="${DEPLOY_DIR:-/opt/portalt}"
DEPLOY_DIR="$(realpath -m "$DEPLOY_DIR")"

# 2. 基础设施运行方式
read -p "基础设施运行方式 (docker/原生) [docker]: " INFRA_MODE
INFRA_MODE="${INFRA_MODE:-docker}"
while [[ "$INFRA_MODE" != "docker" && "$INFRA_MODE" != "原生" ]]; do
    read -p "请输入 docker 或 原生: " INFRA_MODE
    INFRA_MODE="${INFRA_MODE:-docker}"
done

# 3. 数据库
read -p "数据库类型 (postgres/sqlite) [postgres]: " DB_TYPE
DB_TYPE="${DB_TYPE:-postgres}"
while [[ "$DB_TYPE" != "postgres" && "$DB_TYPE" != "sqlite" ]]; do
    read -p "请输入 postgres 或 sqlite: " DB_TYPE
    DB_TYPE="${DB_TYPE:-postgres}"
done

DB_DSN=""
if [ "$DB_TYPE" = "postgres" ]; then
    if [ "$INFRA_MODE" = "docker" ]; then
        DB_DSN="postgres://portalt:securepassword@127.0.0.1:5432/portalt?sslmode=disable"
        info "PostgreSQL 将通过 Docker 运行，DSN: $DB_DSN"
    else
        read -p "PostgreSQL 连接串 [postgres://portalt:securepassword@127.0.0.1:5432/portalt?sslmode=disable]: " DB_DSN
        DB_DSN="${DB_DSN:-postgres://portalt:securepassword@127.0.0.1:5432/portalt?sslmode=disable}"
    fi
else
    read -p "SQLite 文件路径 [${DEPLOY_DIR}/data/portalt.db]: " DB_DSN
    DB_DSN="${DB_DSN:-${DEPLOY_DIR}/data/portalt.db}"
fi

# 4. Guacd 地址（仅原生模式）
GUACD_URL="127.0.0.1:4822"
if [ "$INFRA_MODE" = "docker" ]; then
    info "Guacd 将通过 Docker 运行 ($GUACD_URL)"
else
    read -p "Guacd 地址 [127.0.0.1:4822]: " GUACD_URL
    GUACD_URL="${GUACD_URL:-127.0.0.1:4822}"
fi

# 5. VNC 演示容器（仅 Docker 模式）
VNC_DEMO=false
if [ "$INFRA_MODE" = "docker" ]; then
    read -p "运行 VNC 演示容器 (y/N): " VNC_ANS
    [[ "$VNC_ANS" =~ ^[Yy] ]] && VNC_DEMO=true
fi

# 6. 虚拟化平台
read -p "虚拟化平台 (mock/esxi/workstation/none) [mock]: " VIRT_PROVIDER
VIRT_PROVIDER="${VIRT_PROVIDER:-mock}"
while [[ "$VIRT_PROVIDER" != "mock" && "$VIRT_PROVIDER" != "esxi" && "$VIRT_PROVIDER" != "workstation" && "$VIRT_PROVIDER" != "none" ]]; do
    read -p "请输入 mock/esxi/workstation/none: " VIRT_PROVIDER
    VIRT_PROVIDER="${VIRT_PROVIDER:-mock}"
done

# ESXi 配置
VIRT_ESXI_URL=""; VIRT_ESXI_USERNAME="root"; VIRT_ESXI_PASSWORD=""; VIRT_ESXI_INSECURE="true"
ESXI_WEB_URL=""; ESXI_UPSTREAM=""
if [ "$VIRT_PROVIDER" = "esxi" ]; then
    read -p "ESXi SDK URL (如 https://esxi.lan/sdk): " VIRT_ESXI_URL
    while [ -z "$VIRT_ESXI_URL" ]; do
        read -p "ESXi SDK URL 必填: " VIRT_ESXI_URL
    done
    read -p "ESXi 用户名 [root]: " VIRT_ESXI_USERNAME
    VIRT_ESXI_USERNAME="${VIRT_ESXI_USERNAME:-root}"
    read -s -p "ESXi 密码: " VIRT_ESXI_PASSWORD; echo
    while [ -z "$VIRT_ESXI_PASSWORD" ]; do
        read -s -p "ESXi 密码必填: " VIRT_ESXI_PASSWORD; echo
    done
    read -p "跳过证书验证 (true/false) [true]: " VIRT_ESXI_INSECURE
    VIRT_ESXI_INSECURE="${VIRT_ESXI_INSECURE:-true}"
    read -p "ESXi Web 界面地址（Caddy 反代路径 /esxi/ui/，或留空自动推导）[/esxi/ui/]: " ESXI_WEB_URL
    ESXI_WEB_URL="${ESXI_WEB_URL:-/esxi/ui/}"
    # 从 SDK URL 推导 UPSTREAM
    ESXI_UPSTREAM=$(echo "$VIRT_ESXI_URL" | sed -E 's|^https?://([^/]+)/.*|\1|')
    read -p "Caddy 反代 ESXi 上游地址 [$ESXI_UPSTREAM]: " ESXI_UPSTREAM_INPUT
    ESXI_UPSTREAM="${ESXI_UPSTREAM_INPUT:-$ESXI_UPSTREAM}"
fi

# Workstation 配置
VIRT_WS_URL=""; VIRT_WS_USERNAME=""; VIRT_WS_PASSWORD=""
if [ "$VIRT_PROVIDER" = "workstation" ]; then
    read -p "Workstation vmrest URL [http://127.0.0.1:8697]: " VIRT_WS_URL
    VIRT_WS_URL="${VIRT_WS_URL:-http://127.0.0.1:8697}"
    read -p "Workstation 用户名: " VIRT_WS_USERNAME
    read -s -p "Workstation 密码: " VIRT_WS_PASSWORD; echo
fi

# 7. JWT 密钥
read -p "JWT 密钥 (auto=随机生成/manual=手动输入) [auto]: " JWT_MODE
JWT_MODE="${JWT_MODE:-auto}"
JWT_SECRET=""
if [ "$JWT_MODE" = "manual" ]; then
    read -s -p "输入 JWT 密钥: " JWT_SECRET; echo
    while [ -z "$JWT_SECRET" ]; do
        read -s -p "JWT 密钥不能为空: " JWT_SECRET; echo
    done
else
    JWT_SECRET=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
    ok "JWT 密钥已自动生成"
fi

# 8. 管理员密码
read -p "管理员密码 (auto=随机生成/manual=手动输入) [auto]: " PASS_MODE
PASS_MODE="${PASS_MODE:-auto}"
ADMIN_PASSWORD=""
if [ "$PASS_MODE" = "manual" ]; then
    read -s -p "输入管理员密码: " ADMIN_PASSWORD; echo
    while [ -z "$ADMIN_PASSWORD" ]; do
        read -s -p "密码不能为空: " ADMIN_PASSWORD; echo
    done
else
    ADMIN_PASSWORD=$(openssl rand -base64 12 2>/dev/null || head -c 12 /dev/urandom | base64)
    ok "管理员密码已自动生成"
fi

# 9. 后端监听地址
read -p "后端监听地址 [127.0.0.1:8080]: " BACKEND_PORT
BACKEND_PORT="${BACKEND_PORT:-127.0.0.1:8080}"

# 10. 前端监听端口
read -p "前端预览监听端口 [3001]: " FRONTEND_PORT
FRONTEND_PORT="${FRONTEND_PORT:-3001}"

# 提取后端端口号（Caddyfile 反代需要纯端口）
if [[ "$BACKEND_PORT" == *:* ]]; then
    BACKEND_PORT_NUM="${BACKEND_PORT##*:}"
else
    BACKEND_PORT_NUM="$BACKEND_PORT"
fi

# 11. Caddy
read -p "使用 Caddy 反代 (Y/n): " CADDY_ANS
USE_CADDY=true
[[ "$CADDY_ANS" =~ ^[Nn] ]] && USE_CADDY=false

CADDY_PORT="8808"
DOMAIN=""
if $USE_CADDY; then
    read -p "Caddy 入口端口 [8808]: " CADDY_PORT
    CADDY_PORT="${CADDY_PORT:-8808}"
    read -p "配置 HTTPS 域名 (y/N): " HTTPS_ANS
    if [[ "$HTTPS_ANS" =~ ^[Yy] ]]; then
        read -p "域名 (如 portal.yourlab.com): " DOMAIN
        while [ -z "$DOMAIN" ]; do
            read -p "域名必填: " DOMAIN
        done
    fi
fi

# 12. cloudflared
read -p "安装 cloudflared 隧道 (y/N): " CF_ANS
USE_CF=false
CF_TOKEN=""
if [[ "$CF_ANS" =~ ^[Yy] ]]; then
    USE_CF=true
    read -p "Cloudflare 隧道 Token: " CF_TOKEN
    while [ -z "$CF_TOKEN" ]; do
        read -p "Token 必填: " CF_TOKEN
    done
fi

# 13. systemd
USE_SYSTEMD=false
if $HAS_SYSTEMD; then
    read -p "创建 systemd 服务 (Y/n): " SD_ANS
    USE_SYSTEMD=true
    [[ "$SD_ANS" =~ ^[Nn] ]] && USE_SYSTEMD=false
fi

# ============================================================
# 部署摘要预览
# ============================================================
echo -e "\n${BOLD}${YELLOW}即将部署：${NC}"
echo "  部署目录: $DEPLOY_DIR"
echo "  数据库: $DB_TYPE ($DB_DSN)"
echo "  基础设施: $INFRA_MODE（Guacd: $GUACD_URL）"
echo "  虚拟化: $VIRT_PROVIDER"
if [ "$USE_CADDY" = true ]; then echo "  Caddy: 是 (:${CADDY_PORT})"; else echo "  Caddy: 否"; fi
if [ "$USE_SYSTEMD" = true ]; then echo "  systemd: 是"; else echo "  systemd: 否"; fi
if [ "$USE_CF" = true ]; then echo "  cloudflared: 是"; else echo "  cloudflared: 否"; fi

read -p $'\n确认部署？(Y/n): ' CONFIRM
[[ "$CONFIRM" =~ ^[Nn] ]] && { info "已取消"; exit 0; }

# ============================================================
# 执行部署
# ============================================================
header "开始部署"

# 创建目录结构
mkdir -p "$DEPLOY_DIR"/{bin,data,logs,caddy,frontend}

# 复制后端二进制
cp "$BACKEND_SRC" "$DEPLOY_DIR/bin/portalt"
chmod 755 "$DEPLOY_DIR/bin/portalt"
ok "后端二进制已部署"

# 复制前端产物
info "复制前端产物（可能较大，请稍候）..."
cp -a "$FRONTEND_SRC"/. "$DEPLOY_DIR/frontend/"
ok "前端产物已部署"

# ============================================================
# 生成 .env（后端环境变量）
# ============================================================
header "生成配置文件"

cat > "$DEPLOY_DIR/.env" <<EOF
# PortalT 环境配置（由部署脚本生成）
# 修改后需重启 portalt-backend 服务生效

# 数据库
DB_DRIVER=$DB_TYPE
DB_DSN=$DB_DSN
DB_MIGRATIONS_DIR=migrations

# JWT
JWT_SECRET=$JWT_SECRET
JWT_ACCESS_TTL=900
JWT_REFRESH_TTL=604800

# 管理员初始账号
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$ADMIN_PASSWORD

# HTTP 监听
PORT=$BACKEND_PORT

# 虚拟化
VIRT_PROVIDER=$VIRT_PROVIDER
EOF

# 追加 ESXi 配置
if [ "$VIRT_PROVIDER" = "esxi" ]; then
    cat >> "$DEPLOY_DIR/.env" <<EOF
VIRT_ESXI_URL=$VIRT_ESXI_URL
VIRT_ESXI_USERNAME=$VIRT_ESXI_USERNAME
VIRT_ESXI_PASSWORD=$VIRT_ESXI_PASSWORD
VIRT_ESXI_INSECURE=$VIRT_ESXI_INSECURE
ESXI_WEB_URL=$ESXI_WEB_URL
ESXI_UPSTREAM=$ESXI_UPSTREAM
EOF
fi

# 追加 Workstation 配置
if [ "$VIRT_PROVIDER" = "workstation" ]; then
    cat >> "$DEPLOY_DIR/.env" <<EOF
VIRT_WS_URL=$VIRT_WS_URL
VIRT_WS_USERNAME=$VIRT_WS_USERNAME
VIRT_WS_PASSWORD=$VIRT_WS_PASSWORD
EOF
fi

# Guacd
cat >> "$DEPLOY_DIR/.env" <<EOF

# Guacamole 远程桌面
GUACD_URL=$GUACD_URL
EOF

# Caddy 环境变量（仅用于 Caddyfile 引用）
if $USE_CADDY; then
    cat >> "$DEPLOY_DIR/.env" <<EOF

# Caddy
CADDY_PORT=$CADDY_PORT
EOF
    if [ "$VIRT_PROVIDER" = "esxi" ]; then
        echo "ESXI_UPSTREAM=$ESXI_UPSTREAM" >> "$DEPLOY_DIR/.env"
    fi
fi

chmod 600 "$DEPLOY_DIR/.env"
ok ".env 已生成"

# ============================================================
# 生成 Caddyfile
# ============================================================
if $USE_CADDY; then
    # 检查是否有现成 Caddyfile
    if [ -f "$SCRIPT_DIR/Caddyfile" ]; then
        cp "$SCRIPT_DIR/Caddyfile" "$DEPLOY_DIR/caddy/Caddyfile"
        info "使用提供的 Caddyfile"
    elif [ -f "$REPO_DIR/caddy/Caddyfile" ]; then
        cp "$REPO_DIR/caddy/Caddyfile" "$DEPLOY_DIR/caddy/Caddyfile"
        # 替换占位符（模式单引号保持 {$...} 字面量，防止 shell 展开变量后 sed 误读 \N 为反向引用）
        sed -i 's/{\$CADDY_PORT:8808}/'"$CADDY_PORT"'/g' "$DEPLOY_DIR/caddy/Caddyfile"
        if [ -n "$ESXI_UPSTREAM" ]; then
            sed -i 's/{\$ESXI_UPSTREAM:192.168.118.129}/'"$ESXI_UPSTREAM"'/g' "$DEPLOY_DIR/caddy/Caddyfile"
        fi
        sed -i "s|127.0.0.1:3001|127.0.0.1:$FRONTEND_PORT|g" "$DEPLOY_DIR/caddy/Caddyfile"
        sed -i "s|127.0.0.1:8080|127.0.0.1:$BACKEND_PORT_NUM|g" "$DEPLOY_DIR/caddy/Caddyfile"
        info "使用仓库 Caddyfile 并替换端口/IP"
    else
        # 用占位符生成简洁 Caddyfile（使用命名块，确保 import 始终可用）
        cat > "$DEPLOY_DIR/caddy/Caddyfile" <<'CADDYEOF'
# PortalT Caddy 配置（由部署脚本生成）
(portalt_routes) {
	handle /api/* { reverse_proxy 127.0.0.1:__BACKEND_PORT__ }
	handle /native/* { reverse_proxy 127.0.0.1:__BACKEND_PORT__ }
	handle /healthz { reverse_proxy 127.0.0.1:__BACKEND_PORT__ }
__ESXI_BLOCK_START__
	handle /esxi/* {
		uri strip_prefix /esxi
		reverse_proxy https://__ESXI_UPSTREAM__ {
			transport http { tls_insecure_skip_verify }
			header_down -X-Frame-Options
			header_down -Content-Security-Policy
		}
	}
	handle /ui/* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -X-Frame-Options header_down -Content-Security-Policy } }
	handle /screen* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /sdk* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /sts* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /ticket* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /vfeed/* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /converter/* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /eam/* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /pbm/* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /sms/* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
	handle /vsan/* { reverse_proxy https://__ESXI_UPSTREAM__ { transport http { tls_insecure_skip_verify } header_down -Content-Security-Policy } }
__ESXI_BLOCK_END__
	handle { reverse_proxy 127.0.0.1:__FRONTEND_PORT__ }
}

:__CADDY_PORT__ {
	import portalt_routes
}
CADDYEOF

        # 基础替换（纯端口）
        sed -i "s/__CADDY_PORT__/$CADDY_PORT/g" "$DEPLOY_DIR/caddy/Caddyfile"
        sed -i "s/__BACKEND_PORT__/$BACKEND_PORT_NUM/g" "$DEPLOY_DIR/caddy/Caddyfile"
        sed -i "s/__FRONTEND_PORT__/$FRONTEND_PORT/g" "$DEPLOY_DIR/caddy/Caddyfile"

        if [ -n "$ESXI_UPSTREAM" ]; then
            sed -i '/__ESXI_BLOCK_START__/d' "$DEPLOY_DIR/caddy/Caddyfile"
            sed -i '/__ESXI_BLOCK_END__/d' "$DEPLOY_DIR/caddy/Caddyfile"
            sed -i "s/__ESXI_UPSTREAM__/$ESXI_UPSTREAM/g" "$DEPLOY_DIR/caddy/Caddyfile"
        else
            sed -i '/__ESXI_BLOCK_START__/,/__ESXI_BLOCK_END__/d' "$DEPLOY_DIR/caddy/Caddyfile"
        fi

        # 域名 HTTPS 入口（可选）：portalt_routes 始终存在，import 安全
        if [ -n "$DOMAIN" ]; then
            cat >> "$DEPLOY_DIR/caddy/Caddyfile" <<CADDYEOF

$DOMAIN {
	import portalt_routes
}
CADDYEOF
        fi
    fi
    ok "Caddyfile 已生成"
fi

# ============================================================
# Docker Compose
# ============================================================
if [ "$INFRA_MODE" = "docker" ] && $HAS_DOCKER; then
    if [ -f "$REPO_DIR/docker-compose.yml" ]; then
        cp "$REPO_DIR/docker-compose.yml" "$DEPLOY_DIR/docker-compose.yml"
    else
        # 生成最小 docker-compose.yml
        cat > "$DEPLOY_DIR/docker-compose.yml" <<COMPOSEEOF
# PortalT 基础设施（由部署脚本生成）
name: portalt

services:
  postgres:
    image: postgres:15
    container_name: portalt-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: portalt
      POSTGRES_PASSWORD: securepassword
      POSTGRES_DB: portalt
    ports:
      - "127.0.0.1:5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U portalt -d portalt"]
      interval: 10s
      timeout: 5s
      retries: 5

  guacd:
    image: guacamole/guacd:1.5.5
    container_name: portalt-guacd
    restart: unless-stopped
    ports:
      - "127.0.0.1:4822:4822"
    extra_hosts:
      - "host.docker.internal:host-gateway"
COMPOSEEOF

        if $VNC_DEMO; then
            cat >> "$DEPLOY_DIR/docker-compose.yml" <<COMPOSEEOF

  vnc-demo:
    image: dorowu/ubuntu-desktop-lxde-vnc
    container_name: portalt-vnc-demo
    restart: unless-stopped
    environment:
      VNC_PASSWORD: portalt-demo
      VNC_RESOLUTION: 1280x800
    ports:
      - "127.0.0.1:5900:5900"
COMPOSEEOF
        fi

        cat >> "$DEPLOY_DIR/docker-compose.yml" <<COMPOSEEOF

volumes:
  pgdata:
COMPOSEEOF
    fi

    ok "docker-compose.yml 已生成"
fi

# ============================================================
# systemd 服务
# ============================================================
if $USE_SYSTEMD; then
    header "注册 systemd 服务"

    # 后端服务
    cat > /tmp/portalt-backend.service <<SERVICEEOF
[Unit]
Description=PortalT Backend
After=network.target
Wants=$([ "$INFRA_MODE" = "docker" ] && echo "docker.service" || echo "")

[Service]
Type=simple
ExecStart=$DEPLOY_DIR/bin/portalt
WorkingDirectory=$DEPLOY_DIR
EnvironmentFile=$DEPLOY_DIR/.env
Restart=on-failure
RestartSec=5
StandardOutput=append:$DEPLOY_DIR/logs/backend.log
StandardError=append:$DEPLOY_DIR/logs/backend.log

[Install]
WantedBy=multi-user.target
SERVICEEOF

    sudo mv /tmp/portalt-backend.service /etc/systemd/system/portalt-backend.service
    sudo chmod 644 /etc/systemd/system/portalt-backend.service
    ok "portalt-backend.service 已注册"

    # 前端服务
    cat > /tmp/portalt-frontend.service <<SERVICEEOF
[Unit]
Description=PortalT Frontend
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/node $DEPLOY_DIR/frontend/server/index.mjs
WorkingDirectory=$DEPLOY_DIR/frontend
Environment=PORT=$FRONTEND_PORT
Restart=on-failure
RestartSec=5
StandardOutput=append:$DEPLOY_DIR/logs/frontend.log
StandardError=append:$DEPLOY_DIR/logs/frontend.log

[Install]
WantedBy=multi-user.target
SERVICEEOF

    sudo mv /tmp/portalt-frontend.service /etc/systemd/system/portalt-frontend.service
    sudo chmod 644 /etc/systemd/system/portalt-frontend.service
    ok "portalt-frontend.service 已注册"

    # Caddy 服务（可选）
    if $USE_CADDY && command -v caddy &>/dev/null; then
        cat > /tmp/portalt-caddy.service <<SERVICEEOF
[Unit]
Description=PortalT Caddy
After=network.target

[Service]
Type=simple
ExecStart=$(command -v caddy) run --config $DEPLOY_DIR/caddy/Caddyfile
WorkingDirectory=$DEPLOY_DIR/caddy
EnvironmentFile=$DEPLOY_DIR/.env
Restart=on-failure
RestartSec=5
StandardOutput=append:$DEPLOY_DIR/logs/caddy.log
StandardError=append:$DEPLOY_DIR/logs/caddy.log

[Install]
WantedBy=multi-user.target
SERVICEEOF

        sudo mv /tmp/portalt-caddy.service /etc/systemd/system/portalt-caddy.service
        sudo chmod 644 /etc/systemd/system/portalt-caddy.service
        ok "portalt-caddy.service 已注册"
    fi

    sudo systemctl daemon-reload
fi

# ============================================================
# 启动服务
# ============================================================
header "启动服务"

# 启动 Docker 容器
if [ "$INFRA_MODE" = "docker" ] && $HAS_DOCKER; then
    pushd "$DEPLOY_DIR" >/dev/null
    docker compose up -d
    popd >/dev/null
    ok "Docker 容器已启动"
fi

# 启动 systemd 服务
if $USE_SYSTEMD; then
    sudo systemctl enable portalt-backend.service
    sudo systemctl start portalt-backend.service
    ok "portalt-backend 已启动"

    sudo systemctl enable portalt-frontend.service
    sudo systemctl start portalt-frontend.service
    ok "portalt-frontend 已启动"

    if $USE_CADDY && [ -f /etc/systemd/system/portalt-caddy.service ]; then
        sudo systemctl enable portalt-caddy.service
        sudo systemctl start portalt-caddy.service
        ok "portalt-caddy 已启动"
    fi
fi

# ============================================================
# cloudflared
# ============================================================
if $USE_CF; then
    header "配置 cloudflared"
    if ! command -v cloudflared &>/dev/null; then
        warn "cloudflared 未安装，正在下载..."
        curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /tmp/cloudflared
        sudo mv /tmp/cloudflared /usr/local/bin/cloudflared
        sudo chmod 755 /usr/local/bin/cloudflared
        ok "cloudflared 已安装"
    else
        ok "cloudflared 已存在"
    fi

    # 创建系统服务或输出启动命令
    if $USE_SYSTEMD; then
        sudo cloudflared service install "$CF_TOKEN" || {
            warn "cloudflared service install 失败，请手动运行："
            echo "  sudo cloudflared service install $CF_TOKEN"
        }
        ok "cloudflared 隧道服务已安装"
    else
        info "启动 cloudflared 隧道："
        echo "  cloudflared tunnel run --token $CF_TOKEN"
    fi
fi

# ============================================================
# 部署摘要
# ============================================================
header "部署完成"

echo -e "${BOLD}${GREEN}══════════════════════════════════════════════${NC}"
echo -e "${BOLD}${GREEN}   PortalT 已部署到 ${DEPLOY_DIR}${NC}"
echo -e "${BOLD}${GREEN}══════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${BOLD}后端地址:${NC}      http://${BACKEND_PORT}"
echo -e "  ${BOLD}前端地址:${NC}      http://127.0.0.1:${FRONTEND_PORT}"
if $USE_CADDY; then
    echo -e "  ${BOLD}Caddy 入口:${NC}   http://127.0.0.1:${CADDY_PORT}"
    [ -n "$DOMAIN" ] && echo -e "  ${BOLD}HTTPS 域名:${NC}    https://${DOMAIN}"
fi
echo ""
echo -e "  ${BOLD}管理账号:${NC}      admin / ${ADMIN_PASSWORD}"
echo -e "  ${BOLD}JWT 密钥:${NC}      ${JWT_SECRET}"
echo ""
echo -e "  ${BOLD}日志目录:${NC}      ${DEPLOY_DIR}/logs/"
echo -e "  ${BOLD}配置文件:${NC}      ${DEPLOY_DIR}/.env"
echo ""

# 后续步骤
echo -e "${BOLD}${YELLOW}后续步骤：${NC}"

if $USE_CADDY && ! command -v caddy &>/dev/null; then
    echo "  1. 安装 Caddy（反代入口必需，否则 WebSocket 不可用）："
    echo "     sudo apt install caddy  # Debian/Ubuntu"
    echo "     或下载: curl -fsSL https://caddyserver.com/download/linux/amd64 | tar -xz"
    echo "     配置: caddy run --config ${DEPLOY_DIR}/caddy/Caddyfile"
    echo "     systemd: 部署脚本已生成服务文件，安装 Caddy 后即可启用"
fi

if $USE_CF && ! $USE_SYSTEMD; then
    echo "  2. 启动 cloudflared："
    echo "     cloudflared tunnel run --token $CF_TOKEN"
elif $USE_CF; then
    echo "  2. cloudflared 隧道服务已安装，检查状态："
    echo "     sudo systemctl status cloudflared"
fi

echo "  3. 验证部署："
echo "     curl http://127.0.0.1:${BACKEND_PORT}/healthz"
if $USE_CADDY; then
    echo "     curl http://127.0.0.1:${CADDY_PORT}/"
fi
echo ""
echo "  4. 打开浏览器访问门户，使用 admin / ${ADMIN_PASSWORD} 登录"
echo ""

# 密码提示
echo -e "${RED}请立即记录管理员密码和 JWT 密钥！它们仅在本次部署中显示一次。${NC}"
echo -e "${YELLOW}以后查看密码：cat ${DEPLOY_DIR}/.env | grep -E 'JWT_SECRET|ADMIN_PASSWORD'${NC}"
echo ""