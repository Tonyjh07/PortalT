#!/usr/bin/env bash
# PortalT 更新脚本（生产日常更新：git pull → 构建 → 部署 → 重启 → 健康检查）
#
# 用法：cd <仓库根目录> && bash deploy/update.sh [参数]
#
# 参数（可组合）：
#   --skip-pull|-sp        跳过 git pull
#   --skip-backend|-sbe    跳过重建后端
#   --skip-frontend|-sfr   跳过重建前端
#   --skip-plugins|-spl    跳过重建官方插件
#   --skip-restart|-sr     跳过服务重启
#   --skip-health|-sh      跳过健康检查
#   --help|-h              显示此帮助
#
# 自更新：脚本自身在 git pull 后若发生变化，自动 exec 新版本（保留参数）。
#
# 前置：已通过 install.sh 部署（/opt/portalt 存在）、仓库已 clone。
# 安全：不触碰 portalt.env / 数据库 / 容器；前后端均先备份，失败自动回滚。

set -euo pipefail
trap 'echo -e "\n\033[1;33m[INFO] 更新已取消\033[0m"; exit 1' INT TERM

# ---- 参数解析 ----
SKIP_PULL=0
SKIP_BACKEND=0
SKIP_FRONTEND=0
SKIP_PLUGINS=0
SKIP_RESTART=0
SKIP_HEALTH=0
while [ $# -gt 0 ]; do
    case "$1" in
        --skip-pull|-sp)     SKIP_PULL=1; shift ;;
        --skip-backend|-sbe) SKIP_BACKEND=1; shift ;;
        --skip-frontend|-sfr) SKIP_FRONTEND=1; shift ;;
        --skip-plugins|-spl) SKIP_PLUGINS=1; shift ;;
        --skip-restart|-sr)  SKIP_RESTART=1; shift ;;
        --skip-health|-sh)   SKIP_HEALTH=1; shift ;;
        --help|-h)
            sed -n '/^# 用法/,/^$/p' "$0" | sed 's/^# //;s/^#$//'
            exit 0 ;;
        *) error "未知参数: $1（用 --help 查看用法）" ;;
    esac
done

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
# 2. 拉取最新代码（--skip-pull 跳过）
# ============================================================
if [ "$SKIP_PULL" = "0" ]; then
    header "拉取最新代码"
    if ! git -C "$REPO_DIR" diff --quiet HEAD; then
        error "仓库有未提交的本地改动，请先处理（git status 查看）"
    fi
    if [ -n "$(git -C "$REPO_DIR" ls-files --others --exclude-standard)" ]; then
        error "仓库有未跟踪文件，请先处理（git status 查看）"
    fi
    # 保存脚本自身哈希，用于自更新检测
    SCRIPT_HASH="$(md5sum "$0" 2>/dev/null | cut -d' ' -f1)"
    git -C "$REPO_DIR" pull --ff-only || error "git pull 失败（网络或冲突）"
    CURRENT_HEAD="$(git -C "$REPO_DIR" log -1 --oneline)"
    ok "已更新到: $CURRENT_HEAD"

    # 自更新检测：脚本内容变化则用 bash 显式重新执行新版本。
    # 不用 exec "$0"：$0 可能是相对路径，且脚本未必有可执行权限；
    # 追加 --skip-pull 避免二次拉取。
    NEW_HASH="$(md5sum "$0" 2>/dev/null | cut -d' ' -f1)"
    if [ "$NEW_HASH" != "$SCRIPT_HASH" ]; then
        info "更新脚本已更新，重新执行新版本 ..."
        exec bash "$SCRIPT_DIR/update.sh" --skip-pull "$@"
    fi
else
    # 跳过 pull 时从现有 HEAD 获取版本号
    CURRENT_HEAD="$(git -C "$REPO_DIR" log -1 --oneline)"
    ok "跳过 git pull（当前: $CURRENT_HEAD）"
fi

# 同步数据库迁移文件（新版本可能新增迁移）
sudo cp -a "$REPO_DIR/backend/migrations/." "$DEPLOY_DIR/migrations/"
ok "数据库迁移文件已同步"

# Caddyfile 差异自动同步：备份旧文件 → 替换为版本库版本 → reload
# 手动修改的域名等自定义内容请从旧备份中恢复（路径见摘要）
BACKUP_CADDY=""
if [ -f /etc/caddy/Caddyfile ] && ! sudo diff -q "$REPO_DIR/caddy/Caddyfile" /etc/caddy/Caddyfile >/dev/null 2>&1; then
    BACKUP_CADDY="/etc/caddy/Caddyfile.bak.$(date +%Y%m%d%H%M%S)"
    info "备份旧 Caddyfile 到 $BACKUP_CADDY ..."
    sudo cp /etc/caddy/Caddyfile "$BACKUP_CADDY"
    sudo cp "$REPO_DIR/caddy/Caddyfile" /etc/caddy/Caddyfile
    # 确保 plugins.d 目录存在（新版 Caddyfile import 此处）
    sudo mkdir -p /etc/caddy/plugins.d
    if command -v caddy >/dev/null 2>&1; then
        if sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1; then
            if [ "$SKIP_RESTART" = "0" ]; then
                service_enable_now caddy || true
                ok "Caddyfile 已更新并 reload"
            else
                ok "Caddyfile 已更新（--skip-restart，未 reload）"
            fi
        else
            warn "新版 Caddyfile 校验失败，回滚到旧版本 ..."
            sudo cp "$BACKUP_CADDY" /etc/caddy/Caddyfile
            if [ "$SKIP_RESTART" = "0" ]; then
                sudo systemctl reload caddy 2>/dev/null || true
            fi
            warn "Caddyfile 已回滚（新版语法错误），旧备份保留在 $BACKUP_CADDY"
        fi
    else
        # 无 caddy 可执行文件：只落盘不 reload，依赖下次系统重启生效
        warn "caddy 命令不可用，Caddyfile 已更新但未 reload（下次重启生效）"
    fi
fi

BACKEND_PORT_NUM="8080"
if grep -q "^PORT=" "$DEPLOY_DIR/portalt.env"; then
    BACKEND_PORT_NUM="$(grep "^PORT=" "$DEPLOY_DIR/portalt.env" | cut -d= -f2 | sed 's/.*://')"
fi

# ============================================================
# 3. 更新后端（--skip-backend 跳过）
# ============================================================
if [ "$SKIP_BACKEND" = "0" ]; then
    header "更新后端"
    BACKUP_BIN=""
    if [ -f "$DEPLOY_DIR/portalt-server" ]; then
        BACKUP_BIN="$DEPLOY_DIR/portalt-server.bak.$(date +%Y%m%d%H%M%S)"
        sudo cp "$DEPLOY_DIR/portalt-server" "$BACKUP_BIN"
        ok "已备份旧二进制 ($(basename "$BACKUP_BIN"))"
    fi

    info "编译后端 ..."
    (cd "$REPO_DIR/backend" && CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct go build -o /tmp/portalt-server-new ./cmd/server) \
        || error "后端编译失败（已跳过部署，旧版本继续运行）"

    sudo mv /tmp/portalt-server-new "$DEPLOY_DIR/portalt-server"
    sudo chmod 755 "$DEPLOY_DIR/portalt-server"
    if [ "$SKIP_RESTART" = "0" ]; then
        sudo systemctl restart portalt-backend || true
    else
        ok "后端二进制已替换（--skip-restart，未重启）"
    fi

    if [ "$SKIP_HEALTH" = "0" ]; then
        if wait_http "http://127.0.0.1:${BACKEND_PORT_NUM}/healthz"; then
            ok "后端健康检查通过"
        else
            warn "后端健康检查失败，回滚到旧版本 ..."
            if [ -n "$BACKUP_BIN" ]; then
                sudo mv "$BACKUP_BIN" "$DEPLOY_DIR/portalt-server"
                if [ "$SKIP_RESTART" = "0" ]; then
                    sudo systemctl restart portalt-backend || error "回滚后 portalt-backend 仍无法启动"
                fi
            fi
            error "后端更新失败（已回滚，查看: journalctl -u portalt-backend -n 50）"
        fi
    else
        ok "后端二进制已部署（--skip-health，未验证）"
    fi
else
    ok "跳过后端更新"
fi

# ============================================================
# 4. 更新前端（--skip-frontend 跳过）
# ============================================================
if [ "$SKIP_FRONTEND" = "0" ]; then
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
        if [ "$SKIP_RESTART" = "0" ]; then
            sudo systemctl restart portalt-frontend || true
        else
            ok "前端产物已部署（--skip-restart，未重启）"
        fi

        if [ "$SKIP_HEALTH" = "0" ]; then
            if wait_http "http://127.0.0.1:$(grep -oP 'PORT=\K[0-9]+' /etc/systemd/system/portalt-frontend.service 2>/dev/null || echo 3001)/"; then
                ok "前端健康检查通过"
            else
                warn "前端健康检查失败"
                FAILED_FRONTEND=1
            fi
        else
            ok "前端产物已部署（--skip-health，未验证）"
        fi
    else
        warn "前端构建失败"
        FAILED_FRONTEND=1
    fi

    if [ "${FAILED_FRONTEND:-0}" = "1" ] && [ -n "$BACKUP_OUT" ]; then
        warn "回滚前端产物 ..."
        sudo rm -rf "$DEPLOY_DIR/frontend/.output"
        sudo mv "$BACKUP_OUT" "$DEPLOY_DIR/frontend/.output"
        if [ "$SKIP_RESTART" = "0" ]; then
            sudo systemctl restart portalt-frontend || true
        fi
        error "前端更新失败（已回滚到旧产物）"
    fi
    if [ "${FAILED_FRONTEND:-0}" = "1" ]; then
        error "前端更新失败（无备份可回滚）"
    fi
else
    ok "跳过前端更新"
fi

# 清理旧备份（各保留最近 2 份）
ls -1t "$DEPLOY_DIR"/portalt-server.bak.* 2>/dev/null | tail -n +3 | xargs -r sudo rm -f || true
ls -1dt "$DEPLOY_DIR"/frontend/.output.bak.* 2>/dev/null | tail -n +3 | xargs -r sudo rm -rf || true

# ============================================================
# 4.5 插件目录（--skip-plugins 跳过重建，仅确保目录存在）
# ============================================================
PLUGINS_DIR="$DEPLOY_DIR/plugins"
if grep -q '^PLUGINS_DIR=' "$DEPLOY_DIR/portalt.env"; then
    PLUGINS_DIR="$(grep '^PLUGINS_DIR=' "$DEPLOY_DIR/portalt.env" | head -1 | cut -d= -f2-)"
fi

if [ "$SKIP_PLUGINS" = "0" ]; then
    header "插件目录"
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
    [ "$id" = "template" ] && continue
    if [ -f "$pdir/manifest.json" ] && command -v go >/dev/null 2>&1; then
        info "重建官方插件 $id ..."
        sudo mkdir -p "$PLUGINS_DIR/$id"
        if ! (cd "$pdir" && CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct go build -o "$PLUGINS_DIR/$id/plugin" ./cmd); then
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

else
    # 跳过重建时仅确保目录存在
    sudo mkdir -p "$PLUGINS_DIR"
    ok "跳过插件重建（目录已就绪: $PLUGINS_DIR）"
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
