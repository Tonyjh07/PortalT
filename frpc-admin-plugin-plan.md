# frpc 配置管理插件（frpc-admin）实施计划

> 状态：**后端核心已完成**（cmd + internal/{configstore,sshx,frc,api}，单测全绿）；
> 待做：§5 的 Makefile/README/static、§3 的 `frontend/` Vue SPA、前端构建与冒烟、
> `docs/` 同步。本文档为实施蓝图，落地后以 `docs/plugins.md`、`docs/interfaces.md`、
> `About.md` 进度表为准。

## 1. 目标与范围

编写一个 **PortalT native 插件**（`frpc-admin`），用于调整指定 VM 客户机内的
**frpc（frp 客户端）配置**：通过 SSH 进入目标 VM，结构化/文本编辑 frpc 配置，
保存时备份原文件、检查语法、应用并重启 frpc 服务，失败自动回滚。

### 范围

- **in scope**：VM 选择、SSH 连接配置（UI 配置并持久化）、frpc 配置读写
  （结构化 + 原文双编辑模式）、保存 = 备份 + 语法检查 + 应用 + 重启 + 失败回滚、
  frp 版本/配置路径探测。
- **out of scope**：frps（服务端）管理；PortalT 后端代码与数据库改动（零改动）；
  cron 类定时同步。

## 2. 决策前提（已与需求方确认）

| 项 | 决策 |
|----|------|
| SSH 连接信息 | 通过 UI 由用户配置，**持久化**（插件运行目录 `data/connections.json`，0600） |
| frpc 配置编辑方式 | **结构化代理管理**为主 + 原始文本编辑（可切换） |
| 保存后是否重启 | **需要**：保存后自动重启 frpc 服务 |
| 配置格式 | **INI + TOML 双支持**，按文件后缀/内容自动检测（可手动强制） |
| 前端实现 | **编译 Vue SPA**（Vite + Vue3 + Element Plus 暗色主题），单页面结构 |
| 安全 | 失败回滚；密码只写不回；未知键保留；SSH 命令不拼接用户输入 |

## 3. 前端设计（单页面结构）

单页布局，顶部工具栏 + 主内容区：

```
┌──────────────────────────────────────────────────────────────┐
│ PortalT │ frpc-admin            [VM 选择 ▼]   [主机信息 ⚙]  │  ← 顶栏
├──────────────────────────────────────────────────────────────┤
│   [可视化编辑]  [配置文件编辑]                    [保存并重启] │  ← 模式切换 + 动作
├──────────────────────────────────────────────────────────────┤
│                                                              │
│   · 可视化：服务端设置表单 + 代理表格（增/删/改）               │
│   · 原文：  文本域显示/编辑完整配置文件（INI/TOML）             │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

- **右上角「主机信息」**：弹窗配置 SSH 连接（主机/端口/用户名/密码/sudo 密码/
  frpc 配置路径/重启命令）+ 「检测配置」按钮（SSH 探测 frp 版本与配置路径）。
  保存到插件持久化存储（按 VM 区分）。
- **顶部「VM 选择」**：调门户 `/api/v1/vms`（读 `access_token` cookie 带 Bearer）列
  出 VM，选中后加载该 VM 的已保存连接与当前配置。
- **主内容区双模式切换**：
  - **可视化编辑**：服务端设置（server_addr/port/token）+ 代理列表（名称、类型
    tcp/udp/http/https/stcp/xtcp/sudp、本地 IP/端口、远端端口、自定义域名，支持
    增/删/改）。
  - **配置文件编辑**：文本域直接编辑完整配置原文；解析失败时给出错误位置提示，
    可一键切回可视化（以已解析结果为准）。
- **「保存并重启」按钮**：统一走后端保存流程（见 §6），结果（备份位置/语法检查/
  重启输出/回滚与否）弹层回显。

### 保存流程（前端交互）

```
点击保存并重启
  → 弹确认（提示将备份原文件并重启 frpc）
  → 调 PUT /api/v1/plugins/native/frpc-admin/api/vms/:vmId/config
  → 回显结果面板：备份路径 / 语法检查通过 / 已应用并重启 / 失败已回滚（含原因）
```

## 4. 插件 API 设计（`/api/v1/plugins/native/frpc-admin/api/*`）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 宿主健康探测 |
| GET | `/connections` | 已保存的 SSH 连接列表（密码脱敏） |
| PUT | `/connections/:vmId` | 保存/更新某 VM 的 SSH 连接（主机/端口/用户名/密码/sudo 密码/frpc 路径/重启命令/格式强制） |
| DELETE | `/connections/:vmId` | 删除连接 |
| GET | `/connections/:vmId/probe` | SSH 探测：frpc 版本、配置路径、格式检测建议 |
| GET | `/vms/:vmId/config` | SSH 读取并解析 frpc 配置 → 结构化 JSON + 原文 |
| PUT | `/vms/:vmId/config` | **保存（核心流程，见 §6）**：结构化或原文 → 备份 → 语法检查 → 应用 → 重启 → 失败回滚 |

请求体（PUT config）：
```json
{
  "content": "…完整配置原文…",        // 二选一
  "structured": { "server": {...}, "proxies": [...] },  // 二选一
  "format": "auto|ini|toml"
}
```

响应：`{ backup_path, syntax_ok, syntax_error, applied, restart_output, rolled_back, rollback_error }`

## 5. 后端目录结构（新模块，独立 go.mod，仿 template）

```
backend/plugins/frpc-admin/
├── manifest.json          # id=frpc-admin, route=/frpc-admin, permission=frpc-admin:manage（默认仅 admin）
├── go.mod / go.sum        # 独立模块，replace portalt => ../../../backend
├── cmd/frpc-admin/main.go # 装配：gRPC 控制面（照搬 template）+ HTTP 数据面
├── internal/
│   ├── configstore/       # SSH 连接配置持久化（JSON 文件，增删改查，密码脱敏）
│   ├── sshx/              # SSH 客户端：连接/执行/读文件/写文件(sudo)/重启服务
│   ├── frc/               # frpc 配置模型 + INI/TOML 解析/序列化 + 语法检查 + 备份/回滚
│   └── api/               # HTTP handler（/healthz + /api/* 端点）
├── static/                # Vue SPA 构建产物（make install 投放）
├── frontend/              # Vue 3 + Vite + Element Plus SPA 源码
├── Makefile               # build / install（含前端构建）/ test / clean
└── README.md              # 使用说明 + 部署 + 「怎么看配置路径/格式」
```

## 6. frpc 配置结构化模型与保存流程（`internal/frc`）

### 模型

```go
type Config struct {
    Server  ServerConfig   // INI: [common] 段 / TOML: serverAddr, serverPort, auth.token
    Proxies []Proxy        // 有序，保持文件原始顺序
    Format  string         // "ini" | "toml"
}
type ServerConfig struct {
    ServerAddr string
    ServerPort int
    Token      string
    Extra      map[string]any   // 未知键保留（round-trip 不丢）
}
type Proxy struct {
    Name          string
    Type          string        // tcp/udp/http/https/stcp/xtcp/sudp
    LocalIP       string
    LocalPort     int
    RemotePort    int
    CustomDomains []string
    Extra         map[string]any // 未知键保留
}
```

- **INI** 用 `gopkg.in/ini.v1`（frp 自身所用解析库，语义一致）；**TOML** 用
  `github.com/BurntSushi/toml`。
- 格式检测：后缀 `.toml` → TOML；`.ini/.conf` → INI；无后缀探测内容；支持手动强制。
- **未知键一律保留**，避免"改一个代理清空其他配置"。

### 保存流程（`PUT /vms/:vmId/config`，事务化）

```
1. SSH 连接目标 VM，读取当前配置原文（读失败 → 直接报错，不进行后续）
2. 语法检查：解析请求体（结构化 → 序列化；原文 → 校验）→ 失败返回 syntax_ok=false，不落盘
3. 备份：远端 cp 原文件 → <path>.bak.<timestamp>（保留最近 N 份，如 5）
4. 应用：写入新配置（先写临时文件再 mv，规避半写损坏）
5. 重启：执行重启命令（默认 systemctl restart frpc，支持 sudo -S 喂密码）
6. 重启失败 → 回滚：mv 备份回原文件 + 再次重启 → 返回 rolled_back=true + 原失败原因
7. 成功 → 返回 backup_path + restart_output
```

关键点：
- **第 3 步备份成功后才动原文件**；任何失败在回滚后状态一致。
- 语法检查先于备份（不产生无效备份）。
- 重启命令失败（如服务名不对）与配置错误（frpc 起不来）都走回滚，并把远端
  stderr 一并回显给用户排查。
- 回滚后若 frpc 仍未恢复（如回滚本身失败），明确报 `rollback_error` 并提示人工介入。

### frp 版本/配置路径探测（`probe`）

```bash
which frpc && frpc --version            # 版本 → 判断默认格式偏好
systemctl cat frpc 2>/dev/null | grep ExecStart  # -c 参数 → 配置路径
ps aux | grep frpc | grep -v grep       # 运行参数兜底
ls -l /etc/frp/ /etc/frpc* 2>/dev/null # 常见路径兜底
```

## 7. SSH 层（`internal/sshx`）

- `golang.org/x/crypto/ssh`，密码认证，连接超时。
- 读文件：`session cat <path>`；写文件：先 `cat > /tmp/…` 临时文件 → `sudo -S cp` 到目标
  （无 sudo 密码则直接写）；重启：`systemctl restart frpc` 或用户自定义命令。
- 每次操作短连接（不维护长会话）；操作失败回显远端 stderr。
- **命令构造不拼接用户输入进 shell**：写文件用 stdin 流，路径参数校验后传参，防注入。

## 8. 安全与边界

- 凭据只存插件数据目录（0600），API 返回脱敏（密码只写不回）。
- 反代层由宿主保证：仅回环、插件启用闸门、身份头注入（`X-PortalT-User/Role/Perms`）。
- **访问控制**：插件 manifest 声明专属权限 `frpc-admin:manage`（权限字典已注册，
  默认仅 admin 持有），宿主 API 反代据此校验；插件侧写操作不再信任任何用户身份头。
- 不触碰门户 backend 代码与数据库（仅新增权限字典条目）；不依赖门户新增权限点。
- 插件放置：`backend/plugins/frpc-admin/` 仓库内源码管理，构建产物手动投放
  `PLUGINS_DIR`，不进 `deploy/install.sh` 官方 submodule 循环（如需纳入部署再议）。

## 9. 实施步骤（落地顺序）

1. **骨架**：`backend/plugins/frpc-admin/` 目录 + manifest.json + go.mod（replace）+ 
   `cmd/frpc-admin/main.go`（gRPC 控制面照搬 template + 空 HTTP 数据面 + /healthz）。
2. **frc 核心**：配置模型 + INI/TOML 解析/序列化 + 语法检查 + 未知键保留（配单测：
   往返一致、损坏输入报错、格式自动检测）。
3. **sshx**：SSH 客户端（连接/执行/读/写/重启），用内置 SSH test server 写单测。
4. **configstore**：连接配置持久化（增删改查 + 脱敏 + 单测）。
5. **api**：HTTP 端点（connections CRUD / probe / config GET / config PUT 保存流程）。
6. **前端 SPA**：单页面（顶栏 VM 选择 + 右上主机信息弹窗 + 双模式切换 + 保存并重启
   + 结果回显），构建产物进 `static/`。
7. **Makefile / README**：build/install/test/clean；部署与使用说明。
8. **验证**：
   - 插件单测：`go test ./...`（frc / sshx / configstore）
   - 构建：`go build ./...`、`CGO_ENABLED=0 go build ./cmd/frpc-admin`、`npm run build`
   - 后端回归：`go test ./... -count=1`（backend，确认嵌套模块不影响）+ `go build/vet`
   - 冒烟：投放 `PLUGINS_DIR` → 门户启用 → iframe 打开 → 连接 mock Linux VM 实测
     读写 + 备份/回滚/重启链路
9. **文档同步**：`docs/plugins.md`、`docs/interfaces.md`、`docs/README.md`、`About.md`
   进度表；提交前 subagent 静态 review。

## 10. 待确认

- 插件放置路径（`backend/plugins/frpc-admin/` 仓库内、非官方 submodule 循环）——
  若无异议按此实施。
- 重启命令默认值：`systemctl restart frpc`（可被用户连接配置覆盖）。
