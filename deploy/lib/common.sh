#!/usr/bin/env bash
# PortalT 部署公共函数库（install.sh / update.sh 共用）
# 仅依赖 bash + 包管理器（apt/dnf/apk），其余依赖由脚本自动安装。
# 约定：日志中文；外部下载带重试；任何失败立即退出（set -euo pipefail）。

set -euo pipefail

# 调用方可用 NONINTERACTIVE=1 进入非交互模式
NONINTERACTIVE="${NONINTERACTIVE:-0}"

# ============================================================
# 颜色与日志
# ============================================================
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }
header() { echo -e "\n${BOLD}${CYAN}══ $* ══${NC}\n"; }

# ============================================================
# 权限与包管理器
# ============================================================
require_sudo() {
    # 交互模式允许提示输密码；--yes 非交互模式静默探测
    if [ "$NONINTERACTIVE" = "1" ]; then
        sudo -n true 2>/dev/null || error "当前用户无免密 sudo 权限，非交互安装前请配置 NOPASSWD 或以 root 运行"
    else
        sudo -v 2>/dev/null || error "当前用户无 sudo 权限，请使用有 sudo 权限的用户运行"
    fi
    ok "sudo 权限可用"
}

detect_pkg_manager() {
    if command -v apt >/dev/null 2>&1; then
        PKG_MANAGER="apt"; PKG_INSTALL="sudo apt-get install -y"
    elif command -v dnf >/dev/null 2>&1; then
        PKG_MANAGER="dnf"; PKG_INSTALL="sudo dnf install -y"
    elif command -v apk >/dev/null 2>&1; then
        PKG_MANAGER="apk"; PKG_INSTALL="sudo apk add"
    else
        error "不支持的包管理器（仅支持 apt/dnf/apk），请手动安装后重试"
    fi
    ok "包管理器: $PKG_MANAGER"
}

# 安装缺失的基础工具（参数为命令名列表，包名默认同名）
ensure_cmds() {
    local missing=()
    for cmd in "$@"; do
        command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
    done
    if [ ${#missing[@]} -gt 0 ]; then
        info "安装基础工具: ${missing[*]}"
        $PKG_INSTALL "${missing[@]}" || error "基础工具安装失败（可能需先更新软件源：sudo apt-get update）"
    fi
}

ensure_systemd() {
    command -v systemctl >/dev/null 2>&1 || error "未检测到 systemd，本脚本以 systemd 环境为标准"
}

# ============================================================
# 下载（带重试；参数: 输出文件 URL）
# ============================================================
download() {
    local dest="$1" url="$2"
    info "下载 ${url##*/} ..."
    curl -fsSL --retry 5 --retry-all-errors --connect-timeout 15 -o "$dest" "$url" \
        || error "下载失败: $url（请检查网络后重试）"
    [ -s "$dest" ] || error "下载产物为空: $url"
}

# ============================================================
# 随机密钥
# ============================================================
gen_secret() {
    openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
}

gen_password() {
    openssl rand -base64 12 2>/dev/null || head -c 12 /dev/urandom | base64
}

# ============================================================
# 交互输入：$1 提示语 $2 默认值；NONINTERACTIVE=1 时直接用默认值
# ============================================================
prompt() {
    local label="$1" default="$2" ans
    if [ "$NONINTERACTIVE" = "1" ]; then
        printf '%s' "$default"
        return 0
    fi
    read -r -p "$label [$default]: " ans
    printf '%s' "${ans:-$default}"
}

# 布尔确认：$1 提示语 $2 默认（Y/N），输出 0=是 1=否
prompt_yes() {
    local label="$1" default="$2" ans
    if [ "$NONINTERACTIVE" = "1" ]; then
        [[ "$default" =~ ^[Yy] ]] && return 0
        return 1
    fi
    read -r -p "$label [$default]: " ans
    [[ "${ans:-$default}" =~ ^[Yy] ]] && return 0
    return 1
}

# ============================================================
# systemd 服务管理
# ============================================================
service_enable_now() {
    local name="$1"
    sudo systemctl enable "$name" >/dev/null 2>&1 || true
    sudo systemctl restart "$name" || error "服务 $name 启动失败（查看: journalctl -u $name -n 50）"
    ok "$name 已启动"
}

# 等待 HTTP 就绪：$1 URL $2 秒数（默认 30）
wait_http() {
    local url="$1" timeout="${2:-30}" i
    for ((i = 0; i < timeout; i += 2)); do
        if curl -fsS -o /dev/null "$url" 2>/dev/null; then
            return 0
        fi
        sleep 2
    done
    return 1
}
