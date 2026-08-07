# AGENTS.md

PortalT：HomeLab 统一门户（Go 后端 + Nuxt 3 前端）。详见 `About.md`（架构/进度）、`docs/conventions.md`（工具链/代码规范/测试约定）——本文件只记录文档没写、但踩过坑才知道的上下文。

## 环境（Windows + PowerShell）

- Go 不在 PATH：每次新会话跑 Go 命令前先 `$env:Path += ";C:\Program Files\Go\bin"`。
- `rg` 未安装；命令行搜代码用 `Select-String`，搜文件用 glob/grep 工具。
- GOPROXY 已持久化为 `goproxy.cn`（外网 registry 不通），勿改回。
- 用文件工具（Write/Edit）写含中文的文件；若必须用 PowerShell 写文件须 UTF-8 无 BOM（`[System.IO.File]::WriteAllText`），否则乱码。

## 常用命令（均在对应子目录执行）

- 后端测试：`go test ./... -count=1`（backend/ 下；各包首跑偏慢属正常）。构建前先 `go build ./...` 或 `go vet ./...` 自查。
- esxi/集成测试默认被 build tag 排除：`go test -tags esxi ./internal/adapters/esxi/...`、`-tags integration ./internal/adapters/...`。
- 前端构建：`npm.cmd run build`（frontend/ 下）；无 lint 脚本。
- 后端入口：`go run ./cmd/server`（默认 `127.0.0.1:8080`，管理员 `admin/admin123`）。
- 生产部署（Linux，目标机拉仓库后）：`bash deploy/install.sh`（一键安装，`--yes` 非交互）；日常更新 `git pull && bash deploy/update.sh`（自动备份/回滚）。`deploy/lib/common.sh` 是两脚本公共库（require_sudo 交互模式走 `sudo -v`）。

## 架构接线（非文件名能直接看出的部分）

- 分层：`internal/domain`（零外部依赖）→ `internal/ports`（接口+哨兵错误）→ `internal/adapters` → `internal/api`（Gin）。`cmd/server/main.go` 是唯一装配点：DB → 认证 → 虚拟化工厂 → VM 服务 → 路由。
- 虚拟化工厂 `adapters.NewVirtualizationProvider`：`VIRT_PROVIDER=mock|esxi|workstation`。配置键 url/username/password/insecure，env 优先 `VIRT_*` 通用变量，回退 `VIRT_ESXI_*` → `VIRT_WS_*`。新增环境变量后必须同步 `.env.example`。
- `VMService.SyncVMs` 会删除平台侧已不存在的 VM（全量对齐）；提供者报错时整体不动。电源操作受 domain 状态规则约束（`CanStart/CanStop/CanRestart`：仅 poweredOff/suspended 可 start）。
- 提供者 ID 约定各平台不同（esxi=VM UUID、workstation=vmrest 的 id 串），UI 与远程桌面只看 `domain.VM` 字段，勿跨平台假设。
- 新虚拟化适配器：实现 `ports.VirtualizationProvider`（ListVMs/StartVM/StopVM/RestartVM/GetHostInfo），测试用 httptest mock 目标平台 API（参考 `adapters/workstation/provider_test.go`）。

## 远程桌面（Guacamole）要点

- 模式一（推荐）：`GUACD_URL=127.0.0.1:4822`，后端与 guacd 走原生指令（`internal/api/v1/guac_tunnel.go`）；WS 升级必须设 `Subprotocols: ["guacamole"]`，否则浏览器握手失败。
- 连接参数全部来自 VM metadata `guac.*`：hostname/port/protocol/username/password 等；hostname 缺省回退 `VM.IPAddress`，port 按协议默认 vnc 5900 / rdp 3389 / ssh 22。桌面打不开先查 metadata 是否指对目标。
- 前端用 `guacamole-common-js`，其类型声明在 `frontend/types/guacamole-common-js.d.ts`（手工维护，缺枚举/方法时补在此处）。判断连接状态用 `Client.State`（WAITING/CONNECTED），勿用 `Tunnel.State`（一直不变）。
- compose 里 `portalt-guacd`(4822) 与 `portalt-vnc-demo`(5900, VNC_PASSWORD=portalt-demo) 是 mock VM 的演示目标。

## 前端（Nuxt 3）注意

- `ssr: false`（纯 SPA）；Element Plus 暗色主题；组件全自动导入，**同名冲突**时须带目录前缀引用（如 `VmRemoteDesktop`，页面组件定义在 `frontend/components/`）。
- dev 模式固定监听 `127.0.0.1:3000`，仅本机或 cloudflared 同机回连可访问；经 CF 隧道（`demo.tonyjh07.dpdns.org`）访问时报 `Blocked request. This host is not allowed` 就到 `nuxt.config.ts` 的 `vite.server.allowedHosts` 加域名。
- `/api` 走 nitro devProxy 到 8080；WS 升级经 `frontend/modules/wsProxy.ts`（httpxy，仅 dev，target 只取 origin 避免 `/api/api` 双重前缀）。生产部署不走此路径。
- 修改 `nuxt.config.ts` 的 host/allowedHosts 后必须重启 dev server 才生效。
- dev 全局 CSS 加载有两个上游 bug（Windows 畸形 URL + link/import 缓存冲突报 MIME 错），已用 patch-package 修补（`frontend/patches/`）；`?inline` 补丁**必须仅 dev 生效**，否则生产构建全局样式全丢；遇到 `Failed to load module script ... text/css` 先看 patch 是否还在（`npx patch-package --check`）。详见 `docs/conventions.md`。
- 对外（隧道）访问用生产产物而非 dev：`npm run build && node .output/server/index.mjs`（routeRules 已内置 /api 反代）；dev 冷加载经隧道又慢又可能触发 CSS MIME bug；dev↔preview 切换后浏览器需硬刷新。

## 协作约定

- 代码注释、错误消息、commit 均为中文；commit 用 Conventional Commits 风格（如 `fix(frontend): ...`、`Phase N: ...`），直接在 main 分支工作，提交须用户明确要求。
- 文档约定（`docs/README.md`）：只记录已实现内容，Phase 完成或接口变更必须同步更新文档与索引表；`About.md` 进度表同步打勾。
- 测试用 testify（assert/require）；后端 `go test ./... -count=1` 全绿是提交/交付的前提。
- **提交/交付前必须调用 subagent 审查代码**：先用 `general` subagent 对未提交改动做静态 review（安全问题、正确性、与仓库约定一致性），无阻断问题方可提交；阻断/重要建议先修复再提交。
