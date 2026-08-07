# 远程桌面使用指南（Phase 8）

> 浏览器远程桌面（VNC/RDP/SSH）使用与配置说明。实现见 `backend/internal/api/v1/guac_tunnel.go` 与 `frontend/components/vm/RemoteDesktop.vue`。

## 架构

```
浏览器 (guacamole-common-js)
   │  WebSocket（子协议 guacamole）
   ▼
PortalT  /api/v1/guac/ws/:vmId
   │  原生 Guacamole 指令流（TCP）
   ▼
guacd :4822
   │  VNC / RDP / SSH 协议
   ▼
目标虚拟机
```

- 协议握手（select/args/size/connect/ready）由 **PortalT 服务端**完成，浏览器只负责渲染与输入；
- 连接参数全部来自 VM 的 `guac.*` 元数据，浏览器侧**无法覆盖**目标主机与凭证（安全边界）；
- 客户机的稳定性 ping 由 PortalT 回显，不转发 guacd；
- **服务端 keepalive**：每 10 秒向 guacd 发送 `3.nop;`，规避 guacd 1.5.x 的用户输入
  15 秒超时（[GUACAMOLE-2233](https://issues.apache.org/jira/browse/GUACAMOLE-2233)）——
  Chrome/Edge 会把 keepalive 降频到 37–60 秒，不做处理会触发
  "User is not responding" 而断连。

## 前提条件

1. guacd 服务可用（默认地址 `127.0.0.1:4822`，可用 `GUACD_URL` 覆盖；未配置时该路由返回 503）。
   ```bash
   docker compose up -d guacd      # docker-compose.yml 已内置 guacamole/guacd:1.5.5
   ```
2. 目标虚拟机已开启对应远程协议：
   - Windows：RDP（3389）
   - Linux：VNC（5900）或 SSH（22）/ Telnet（23）
3. 虚拟机在 PortalT 目录中且 `status = poweredOn`（详情页才会显示远程桌面）。

## 配置连接参数（VM metadata `guac.*` 键）

连接参数存于 VM 的 `metadata`（jsonb）中，由虚拟化平台同步或管理员维护：

| 键 | 必填 | 说明 |
|----|------|------|
| `guac.protocol` | 否（默认 vnc） | 协议：vnc / rdp / ssh / telnet |
| `guac.hostname` | 否 | 目标主机；缺省回退 VM 的 `ip_address` |
| `guac.port` | 否 | 目标端口；缺省按协议：vnc 5900 / rdp 3389 / ssh、telnet 22 |
| `guac.username` / `guac.password` | 视目标而定 | 登录凭证（password 不会展示在前端详情页） |
| `guac.width` / `guac.height` | 否 | 初始分辨率（默认 1280×800） |
| `guac.security` / `guac.domain` / `guac.read-only` / `guac.autoretry` / `guac.color-depth` | 否 | 透传协议参数（rdp security、vnc 只读等） |
| `guac.ignore-cert` | 否 | RDP 忽略证书校验（自签证书环境建议 `true`） |
| `guac.disable-bitmap-caching` / `guac.enable-wallpaper` / `guac.enable-theming` / `guac.enable-font-smoothing` | 否 | 透传 RDP 渲染参数（位图缓存、壁纸/主题/字体平滑；Win10 黑屏时壁纸参数可改善） |

配置方式（二选一）：

1. **前端配置面板（推荐）**：VM 详情页 → 「远程桌面」卡片右上角「配置」按钮（仅管理员）→ 填写协议/目标/端口/用户名/密码 → 保存（写入 metadata，密码为空则清除该键）。
2. **API**：`PUT /api/v1/vms/:id/metadata`（需 `vm:manage` 权限，body 为键值对象，值为 `null` 的键删除；校验：`guac.protocol` 仅限 vnc/rdp/ssh/telnet、`guac.port` 为 1–65535、`guac.hostname` 非空）。示例：

```json
{
  "guac.protocol": "vnc",
  "guac.hostname": "192.168.1.10",
  "guac.port": "5900",
  "guac.password": "vm-password"
}
```

> 注意：`SyncVMs`/状态轮询采用 metadata 合并策略——提供者未提供的键保留库内值，手动配置不会被平台同步覆盖（workstation 适配器 IP 走 `ip_address` 字段，不写 metadata）。
> 安全：API 返回的 VM **不含**键名匹配 `password|passwd|secret|token` 的 metadata（凭证只写不回，仍存于库中供隧道使用）。

## 质量/流畅度模式

远程桌面支持三种**会话级**模式（VM 详情页「远程桌面」卡片顶部切换，选择会记住到本地）：

| 模式 | 说明 |
|------|------|
| 自动（默认） | 连接前按 `navigator.connection` 网络类型初选（2g/saveData → 流畅，其余 → 画质）；连接后持续监测：每秒收到 ≥2 条指令（画面在持续更新）但渲染帧率 < 6fps，连续 6 秒判定为不流畅，**自动重建会话切流畅档**并提示（可撤销回到画质档；撤销后锁定画质，不再自动降档） |
| 画质优先 | 尊重 metadata 分辨率/渲染参数；RDP 会话开启音频（`audio/L16;rate=44100,channels=2`） |
| 流畅优先 | 强制小分辨率（1024×640）+ 低色深（RDP 16 / VNC 8）+ 关闭 RDP 桌面特效（壁纸/主题/字体平滑）+ 静音；位图缓存保留（关闭反而因重复传输更卡） |

- 模式经 WS 查询参数 `?mode=auto|quality|fluency` 传给后端，由服务端在握手时调整
  **会话分辨率与协议参数**（真正降低带宽），前端另按容器尺寸等比缩放显示；
  非法值回退 auto。模式只影响质量档位，**不改变连接目标与凭证**（浏览器仍无法覆盖）。
- 切换模式会重建会话（RDP 分辨率是握手期定的，不能中途改），耗时 1–3 秒。
- 带宽杠杆说明：会话分辨率（握手 `size` 指令）影响最大；`display.scale` 只影响观感；
  视频流始终不启用（guacamole-common-js fork 无 `VideoPlayer`，启用会抛错）。

## 使用步骤

1. 登录 PortalT（默认 `admin/admin123`，可用 `ADMIN_USERNAME`/`ADMIN_PASSWORD` 修改）；
2. 进入「虚拟机」→ 点击目标 VM 进入详情页；
3. 「远程桌面」卡片自动建立连接：
   - 标签依次为 `未连接` → `连接中` → `已连接`（首帧渲染完成）；
   - 页面打开时会按卡片尺寸自动发送 `size` 指令同步会话分辨率；
   - 窗口缩放时自动适配（等比缩放，不拉伸）；
4. 交互：鼠标（点击/拖动/滚轮）作用于桌面画布，键盘事件全局监听（焦点在页面内即可输入）；
5. 点击「全屏」进入浏览器全屏模式（Esc 或再次点击退出）；
6. 连接失败时卡片显示错误信息与「重新连接」按钮。

## 开发 / 演示环境

`docker-compose.yml` 内置演示链路（宿主机零配置可跑通）：

```bash
docker compose up -d            # guacd + vnc-demo 两个服务
```

| 服务 | 说明 |
|------|------|
| `guacd` | guacd 1.5.5，端口 4822；`extra_hosts` 已加 `host.docker.internal:host-gateway` |
| `vnc-demo` | dorowu/ubuntu-desktop-lxde-vnc（Lubuntu 桌面），VNC 端口 5900，密码 `portalt-demo` |

后端以 mock 提供者启动即可连到演示桌面：

```bash
# 后端（Windows PowerShell 示例）
$env:GUACD_URL = "127.0.0.1:4822"
$env:VIRT_PROVIDER = "mock"
go run ./cmd/server

# 前端
npm run dev      # http://localhost:3000
```

mock 提供者内置的 `vm-mock-1` 已带连接参数：

```json
{ "guac.protocol": "vnc", "guac.hostname": "host.docker.internal", "guac.port": "5900", "guac.password": "portalt-demo" }
```

## 前端开发代理（dev 模式 WebSocket）

Nuxt `nitro devProxy` 对 WebSocket 升级的转发并不可靠（见 [nuxt/cli#107](https://github.com/nuxt/cli/issues/107)），仓库内置模块 `frontend/modules/wsProxy.ts` 解决：

- 复用 `nitro.devProxy` 中 `ws: true` 的规则，在 dev 父进程拦截 `upgrade` 事件；
- 用 `httpxy` 直连后端（仅取 target 的 origin，保留原始路径，避免双重前缀）；
- 仅 dev 生效（`nuxt.options.dev` 判断），生产构建不受影响。

## 生产部署（WebSocket 路径）

生产环境（`node .output/server/index.mjs`）**不含** wsProxy，远程桌面 WS 走同源
`/api/v1/guac/ws/:vmId`（地址在 `frontend/components/vm/RemoteDesktop.vue` 按页面
协议动态生成：HTTPS 页面 → `wss://`，HTTP 页面 → `ws://`，不再支持
`NUXT_PUBLIC_API_WS_BASE` 独立直连，避免 HTTPS 页面被浏览器 Mixed Content 拦截），
因此**必须由反向代理支持 WS 升级**：

1. **推荐：Caddy**（仓库 `caddy/Caddyfile` 已内置）：
   - `/api/*` → `backend:8080`（Caddy 原生透传 WS upgrade）；
   - `/*` → `frontend:3000`（preview 或容器）；
   - 浏览器连 `wss://域名/api/v1/guac/ws/...`，全程同源，无需额外配置。
2. **不推荐：nuxt preview 单进程直连隧道**（`routeRules` 反代）：
   - nitro 的 `routeRules.proxy` **不支持 WebSocket 升级**（返回 400），远程桌面不可用；
   - 仅当本地 HTTP 调试时可直连后端 `ws://127.0.0.1:8080`（浏览器限制：HTTPS
     页面不能连 `ws://` 明文地址）。
3. **隧道（cloudflared）**：入口转发到 Caddy（80）而非直接到 3000；cloudflared
   需使用支持 WebSocket 的协议（默认 QUIC 的 h2/h3 端到端不支持
   [RFC 8441](https://datatracker.ietf.org/doc/html/rfc8441) Extended CONNECT，
   会返回 400，须以 `--protocol=http2` 启动，浏览器端才可建 `wss://` 连接。
   注：以上为 2026-08 实测（cloudflared 1.x QUIC 行为），升级 cloudflared 后
   行为可能变化，若 wss 连接受阻优先检查隧道协议。

## RustDesk 一键连接

[RustDesk](https://rustdesk.com/) 作为备选远程方案（内嵌 Guacamole 之外的独立通道），
PortalT 提供**连接信息展示 + 一键唤起本机客户端**：

- 目标机需安装并运行 RustDesk 客户端（官方服务器或自建 hbbs/hbbr 均可）；
- 连接参数存于 VM metadata `rustdesk.*` 键：

| 键 | 必填 | 说明 |
|----|------|------|
| `rustdesk.id` | 是（有该键才显示入口） | 目标机 RustDesk 设备 ID |
| `rustdesk.password` | 否 | 连接密码（**只写不回**：API 脱敏，客户端连接时手动输入） |
| `rustdesk.server` | 否 | 自建服务器地址 `host:port`（如 `rd.example.org:21116`；留空用官方服务器） |
| `rustdesk.key` | 否 | 自建服务器公钥（启用强制校验时填写） |

- 配置：VM 详情页 → 「远程访问配置」→ RustDesk 区块（仅管理员）；
- 使用：详情页「远程桌面」卡片右上角 **RustDesk** 按钮 → 显示设备 ID（可复制）
  与「一键连接」——点击唤起本机 RustDesk 客户端（`rustdesk://<id>[@<server>?key=<key>]`），
  密码在客户端提示时输入。

## 常见问题

| 现象 | 原因与处理 |
|------|-----------|
| 详情页无「远程桌面」卡片 | VM 未开机（`poweredOn` 才显示）；检查状态 |
| 桌面黑屏但有鼠标光标 | **前端 CSS 层叠问题**（非连接问题）：guacamole-common-js 画布为 `z-index:-1` 绝对定位，若容器未创建 stacking context，画布会沉到容器黑背景之下。`.rd-canvas` 已加 `position: relative; z-index: 0`，改动后需重建前端 |
| 连接一段时间后断连（"User is not responding"） | guacd 1.5.x 用户输入 15 秒超时；后端 keepalive 每 10 秒发 `nop` 已规避，确认后端为最新构建 |
| 全屏后画面不占满视口 | `.rd-card:fullscreen` 已配置 flex 布局（卡片 100vw/100vh，画布撑满剩余空间），并监听 `fullscreenchange` 重适配；改动后需重建前端 |
| WS 连接失败 / 401 | 访问令牌过期（15 分钟）；刷新页面重新登录即可 |
| 握手失败（Close 1001 内部错误） | guacd 未启动、`GUACD_URL` 配错、目标主机不可达或凭证错误；查后端日志 |
| 升级被返回 200/404 | dev 模式代理问题：确认 `frontend/modules/wsProxy.ts` 已加载（重启 `npm run dev`） |
| 浏览器报 "no valid credentials" | 连接 URL 被库追加 `?undefined`：后端认证中间件已按 `?`/`&` 截断 token，确认后端为最新构建 |
| 浏览器报 "Sec-WebSocket-Protocol" 错误 | 后端需回显子协议 `guacamole`（`websocket.Upgrader.Subprotocols`），重建并重启后端 |

## 相关接口

- `GET /api/v1/guac/ws/:vmId`（WebSocket，需认证）——契约见 [interfaces.md](./interfaces.md)
