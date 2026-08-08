#!/usr/bin/env bash
# PortalT 更新脚本（生产日常更新：git pull → 构建 → 部署 → 重启 → 健康检查）
#
# 用法：cd <仓库根目录> && bash deploy/update.sh
# 前置：已通过 install.sh 部署（/opt/portalt 存在）、仓库已 clone。
# 安全：不触碰 portalt.env / 数据库 / 容器；前后端均先备份，失败自动回滚。

set -euo pipefail
trap 'echo -e "\n\033[1;33m[INFO] 更新已取消\033[0m"; exit 1' INT TERM

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

REPO_DIR="${REPO_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/portalt}"
if [ -f "$DEPLOY_DIR/.deployed" ]; then
    # install.sh 写入的部署标记：自定义部署目录时据此定位
    DEPLOY_DIR="$(grep '^DEPLOY_DIR=' "$DEPLOY_DIR/.deployed" | head -1 | cut -d= -f2-)"
fi

# ============================================================
# 1. 前置检查
# ============================================================
header "前置检查"
require_sudo
detect_pkg_manager
[ -f "$DEPLOY_DIR/portalt.env" ] || error "未检测到 $DEPLOY_DIR/portalt.env，请先运行 bash deploy/install.sh"
[ -d "$REPO_DIR/.git" ] || error "未检测到 git 仓库，请先 git clone 本仓库"
ok "部署目录: $DEPLOY_DIR    仓库: $REPO_DIR"

# ============================================================
# 2. 拉取最新代码
# ============================================================
header "拉取最新代码"
if ! git -C "$REPO_DIR" diff --quiet HEAD; then
    error "仓库有未提交的本地改动，请先处理（git status 查看）"
fi
if [ -n "$(git -C "$REPO_DIR" ls-files --others --exclude-standard)" ]; then
    error "仓库有未跟踪文件，请先处理（git status 查看）"
fi
git -C "$REPO_DIR" pull --ff-only || error "git pull 失败（网络或冲突）"
CURRENT_HEAD="$(git -C "$REPO_DIR" log -1 --oneline)"
ok "已更新到: $CURRENT_HEAD"

# 同步数据库迁移文件（新版本可能新增迁移）
sudo cp -a "$REPO_DIR/backend/migrations/." "$DEPLOY_DIR/migrations/"
ok "数据库迁移文件已同步"

# Caddyfile 有差异时提示手动同步（可能含用户手工修改的域名）
if [ -f /etc/caddy/Caddyfile ] && ! sudo diff -q "$REPO_DIR/caddy/Caddyfile" /etc/caddy/Caddyfile >/dev/null 2>&1; then
    warn "仓库 caddy/Caddyfile 与 /etc/caddy/Caddyfile 有差异，请检查后手动同步（sudo cp + systemctl reload caddy）"
fi

BACKEND_PORT_NUM="8080"
if grep -q "^PORT=" "$DEPLOY_DIR/portalt.env"; then
    BACKEND_PORT_NUM="$(grep "^PORT=" "$DEPLOY_DIR/portalt.env" | cut -d= -f2 | sed 's/.*://')"
fi

# ============================================================
# 3. 更新后端（构建 → 备份 → 替换 → 重启 → 健康检查，失败回滚）
# ============================================================
header "更新后端"
BACKUP_BIN=""
if [ -f "$DEPLOY_DIR/portalt-server" ]; then
    BACKUP_BIN="$DEPLOY_DIR/portalt-server.bak.$(date +%Y%m%d%H%M%S)"
    sudo cp "$DEPLOY_DIR/portalt-server" "$BACKUP_BIN"
    ok "已备份旧二进制 ($(basename "$BACKUP_BIN"))"
fi

info "编译后端 ..."
(cd "$REPO_DIR/backend" && CGO_ENABLED=0 go build -o /tmp/portalt-server-new ./cmd/server) \
    || error "后端编译失败（已跳过部署，旧版本继续运行）"

sudo mv /tmp/portalt-server-new "$DEPLOY_DIR/portalt-server"
sudo chmod 755 "$DEPLOY_DIR/portalt-server"
sudo systemctl restart portalt-backend || true

if wait_http "http://127.0.0.1:${BACKEND_PORT_NUM}/healthz"; then
    ok "后端健康检查通过"
else
    warn "后端健康检查失败，回滚到旧版本 ..."
    if [ -n "$BACKUP_BIN" ]; then
        sudo mv "$BACKUP_BIN" "$DEPLOY_DIR/portalt-server"
        sudo systemctl restart portalt-backend || error "回滚后 portalt-backend 仍无法启动"
    fi
    error "后端更新失败（已回滚，查看: journalctl -u portalt-backend -n 50）"
fi

# ============================================================
# 4. 更新前端（构建 → 备份 → 替换 → 重启 → 检查，失败回滚）
# ============================================================
header "更新前端"
command -v npm >/dev/null 2>&1 || error "npm 不可用"
BACKUP_OUT=""

if [ -d "$DEPLOY_DIR/frontend/.output" ]; then
    BACKUP_OUT="$DEPLOY_DIR/frontend/.output.bak.$(date +%Y%m%d%H%M%S)"
    sudo mv "$DEPLOY_DIR/frontend/.output" "$BACKUP_OUT"
    ok "已备份旧前端产物"
else
    warn "无旧前端产物可备份"
fi

info "安装前端依赖 ..."
if [ -f "$REPO_DIR/frontend/package-lock.json" ]; then
    (cd "$REPO_DIR/frontend" && npm ci) || { warn "npm ci 失败（可能 lockfile 与依赖声明不一致），回退 npm install ..."; (cd "$REPO_DIR/frontend" && npm install) || true; }
else
    (cd "$REPO_DIR/frontend" && npm install) || true
fi

info "构建前端 ..."
if (cd "$REPO_DIR/frontend" && npm run build); then
    sudo cp -a "$REPO_DIR/frontend/.output" "$DEPLOY_DIR/frontend/.output"
    sudo systemctl restart portalt-frontend || true

    if wait_http "http://127.0.0.1:$(grep -oP 'PORT=\K[0-9]+' /etc/systemd/system/portalt-frontend.service 2>/dev/null || echo 3001)/"; then
        ok "前端健康检查通过"
    else
        warn "前端健康检查失败"
        FAILED_FRONTEND=1
    fi
else
    warn "前端构建失败"
    FAILED_FRONTEND=1
fi

if [ "${FAILED_FRONTEND:-0}" = "1" ] && [ -n "$BACKUP_OUT" ]; then
    warn "回滚前端产物 ..."
    sudo rm -rf "$DEPLOY_DIR/frontend/.output"
    sudo mv "$BACKUP_OUT" "$DEPLOY_DIR/frontend/.output"
    sudo systemctl restart portalt-frontend || true
    error "前端更新失败（已回滚到旧产物）"
fi
if [ "${FAILED_FRONTEND:-0}" = "1" ]; then
    error "前端更新失败（无备份可回滚）"
fi

# 清理旧备份（各保留最近 2 份）
ls -1t "$DEPLOY_DIR"/portalt-server.bak.* 2>/dev/null | tail -n +3 | xargs -r sudo rm -f || true
ls -1dt "$DEPLOY_DIR"/frontend/.output.bak.* 2>/dev/null | tail -n +3 | xargs -r sudo rm -rf || true

# ============================================================
# 4.5 插件目录（备份 → 重建官方插件 → 失败回滚）
# ============================================================
header "插件目录"
PLUGINS_DIR="$DEPLOY_DIR/plugins"
if grep -q '^PLUGINS_DIR=' "$DEPLOY_DIR/portalt.env"; then
    PLUGINS_DIR="$(grep '^PLUGINS_DIR=' "$DEPLOY_DIR/portalt.env" | head -1 | cut -d= -f2-)"
fi
sudo mkdir -p "$PLUGINS_DIR"

# 备份现有插件目录（官方插件重建前；失败时回滚，用户插件不丢失）
PLUGINS_BACKUP="$PLUGINS_DIR.bak.$(date +%Y%m%d%H%M%S)"
if [ -n "$(sudo ls -A "$PLUGINS_DIR" 2>/dev/null | head -1)" ]; then
    sudo cp -a "$PLUGINS_DIR" "$PLUGINS_BACKUP"
    ok "已备份插件目录 ($(basename "$PLUGINS_BACKUP"))"
fi

BUILD_FAILED=0
for pdir in "$REPO_DIR"/backend/plugins/*/; do
    [ -d "$pdir" ] || continue
    id="$(basename "$pdir")"
    [ "$id" = "examples" ] && continue
    if [ -f "$pdir/manifest.json" ] && command -v go >/dev/null 2>&1; then
        info "重建官方插件 $id ..."
        sudo mkdir -p "$PLUGINS_DIR/$id"
        if ! (cd "$pdir" && CGO_ENABLED=0 go build -o "$PLUGINS_DIR/$id/plugin" ./cmd); then
            warn "官方插件 $id 构建失败"
            BUILD_FAILED=1
        fi
    fi
done

if [ "$BUILD_FAILED" = "1" ]; then
    if [ -d "$PLUGINS_BACKUP" ]; then
        warn "插件重建失败，回滚插件目录 ..."
        sudo rm -rf "$PLUGINS_DIR"
        sudo mv "$PLUGINS_BACKUP" "$PLUGINS_DIR"
        error "插件目录已回滚（查看构建输出）"
    else
        error "插件构建失败（无备份可回滚）"
    fi
fi
# 清理旧插件备份（保留最近 2 份），无论构建成功与否均执行
ls -1dt "$DEPLOY_DIR"/plugins.bak.* 2>/dev/null | tail -n +3 | xargs -r sudo rm -rf || true

if [ "$BUILD_FAILED" != "1" ]; then
    ok "插件目录就绪: $PLUGINS_DIR"
fi

# ============================================================
# 5. 摘要
# ============================================================
header "更新完成"
echo -e "${BOLD}${GREEN}══ PortalT 已更新（${CURRENT_HEAD}） ══${NC}"
echo ""
echo -e "  ${BOLD}后端:${NC}        systemctl status portalt-backend"
echo -e "  ${BOLD}前端:${NC}        systemctl status portalt-frontend"
echo -e "  ${BOLD}日志:${NC}        journalctl -u portalt-backend -n 50"
echo -e "  ${BOLD}入口:${NC}        http://127.0.0.1:${BACKEND_PORT_NUM}/（经 Caddy）"
echo ""
