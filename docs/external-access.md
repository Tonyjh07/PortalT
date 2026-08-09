# 外部访问指南（Cloudflare Tunnel）

> 从外网访问 PortalT 的推荐方案：`cloudflared` 隧道出站连接 Cloudflare 边缘，
> **无需开放任何入站端口/防火墙规则**，适合 HomeLab 场景（无公网 IP、运营商 NAT）。
> 本指南以 Windows 宿主机 + Caddy 反代（生产构建）为例；**Linux 生产环境请直接用
> `deploy/install.sh` 一键部署**（自动安装 Caddy 系统包 + cloudflared 隧道，
> 见 [how-to-use.md](./how-to-use.md) §3），以下内容供自定义/手工场景参考。

## 架构

```
浏览器 ──https──> Cloudflare 边缘 ──隧道(出站)──> cloudflared(本机)
                                                     │
                                                     ▼
                                         Caddy :8808  (本地反代，原生支持 WS 升级)
                                         ├── /api/*、/native/*、/healthz → 127.0.0.1:8080 (后端)
                                         └── 其余(页面/静态资源) → 127.0.0.1:3001 (Nuxt preview)
                                                     │  guacd 指令流
                                                     ▼
                                         guacd :4822 → VNC/RDP 目标机
```

- cloudflared 主动连出到 Cloudflare，入站 80/443 全部由 CF 边缘接管；
- **必须用 Caddy 反代**：Nuxt 生产构建（`node .output/server/index.mjs`）的 `routeRules`
  反代**不支持 WebSocket 升级**（返回 400），而远程桌面依赖 WS 长连接；
  Caddy 的 `reverse_proxy` 原生透传 WS，是本地补齐该缺口的最简方案；
- 前端 WS 地址是相对路径（`/api/v1/guac/ws/...`），浏览器自动使用
  `wss://<域名>/...` 走隧道 → Caddy → 后端，无需改动代码。

## 一、cloudflared 安装与登录

Windows 客户端（或直接 Docker 运行，见附录）：

```powershell
winget install Cloudflare.cloudflared
cloudflared tunnel login          # 浏览器授权，选择你的域名
cloudflared tunnel create portalt # 生成隧道 ID 与证书，写入 ~/.cloudflared/
```

## 二、配置隧道（~/.cloudflared/config.yml）

```yaml
tunnel: <TUNNEL_ID>                      # cloudflared tunnel list 查看
credentials-file: C:\Users\<你>\.cloudflared\<TUNNEL_ID>.json
ingress:
  - hostname: demo.tonyjh07.dpdns.org    # 换成你的域名
    service: http://127.0.0.1:8808       # Caddy 入口（端口与 Caddy 监听一致，见下节）
  - service: http_status:404             # 兜底
```

## 三、DNS 路由与启动

```powershell
cloudflared tunnel route dns portalt demo.tonyjh07.dpdns.org
cloudflared tunnel run portalt           # 前台运行（生产可用 NSSM/服务方式）
```

> 本机隧道实际为 **Zero Trust token 模式**（服务运行参数
> `cloudflared tunnel run --token-file <token>`），入口规则在 Cloudflare 控制台
> （Zero Trust → Networks → Tunnels）配置，全部流量指向 `http://127.0.0.1:8808`。

## 四、本地服务要求（关键点）

1. **前端 dev server 固定监听 127.0.0.1**（`nuxt.config.ts` 的 `devServer.host`），
   cloudflared 同机回连 127.0.0.1:3000 稳定可用（默认只绑 ::1 时部分客户端连不上）；
2. **放行隧道域名**：cloudflared 转发时保留原始 `Host` 头，Vite 6+ 会拒绝未放行的
   主机名（报 `Blocked request. This host is not allowed`）。已在
   `frontend/nuxt.config.ts` 配置：

   ```ts
   vite: {
     server: {
       allowedHosts: ['demo.tonyjh07.dpdns.org'],   // 替换为你的域名
     },
   },
   ```

3. 后端 `127.0.0.1:8080`（`PORT` 可覆盖）、guacd `127.0.0.1:4822` 均在
   cloudflared 所在宿主机，**只绑 127.0.0.1，对外不可达**；
4. **Caddy 监听 8808**（隧道/局域网入口，端口可配置，见下节），把 `/api/*`、
   `/native/*`、`/healthz` 转发到后端 8080，其余转发到 Nuxt preview
   （生产构建监听 3001），Caddyfile 见下节。

## 四·五、Caddy 反代（生产入口）

```text
# caddy/Caddyfile（入口端口由 CADDY_PORT 控制，默认 8808）
:{$CADDY_PORT:8808} {
	handle /api/*    { reverse_proxy 127.0.0.1:8080 }
	handle /native/* { reverse_proxy 127.0.0.1:8080 }
	handle /healthz  { reverse_proxy 127.0.0.1:8080 }
	handle           { reverse_proxy 127.0.0.1:3001 }
}
```

启动（Windows，示例路径）：

```powershell
caddy run --config C:\path\to\Caddyfile --adapter caddyfile
```

> - 本机 Caddy 由 Go 源码编译安装（`go install github.com/caddyserver/caddy/v2/cmd/caddy@v2.11.4`，
>   winget 因外网不通不可用），二进制在 `%USERPROFILE%\go\bin\caddy.exe`；
> - 前端预览监听 3001：`node .output/server/index.mjs`（nitro 默认 3000，
>   用 `PORT=3001` 或 nitro 配置覆盖）；远程桌面 WS 同源动态生成
>   （`wss://<域名>/api/...` 走隧道 → Caddy → 后端，详见 `remote-desktop.md`）；
> - 验证命令（本机模拟隧道）：
>   - 页面：`Invoke-WebRequest http://127.0.0.1:8808/`（200）
>   - WS：`ws://127.0.0.1:8808/api/v1/guac/ws/<vmId>?token=<token>`（应 OPENED 并收到渲染指令）

### Caddy 入口端口配置

- **默认 8808**：避开 80/443/8080/3000 等易被封禁、易被扫描的常见端口；
  同时绑定全部接口（v4+v6），局域网与公网 v6 直连、cloudflared 隧道回连均走此端口；
- 修改端口二选一（改后**必须同步** `~/.cloudflared/config.yml` 或 Cloudflare
  控制台的 ingress 指向）：
  1. 环境变量注入（不改文件）：`$env:CADDY_PORT = "xxxx"; caddy run ...`，
     Caddyfile 的 `{$CADDY_PORT:8808}` 自动读取；
  2. 直接编辑 `caddy/Caddyfile` 顶级监听端口；
- 未安装 RustDesk 客户端不影响；启用 HTTPS 域名（80/443）见 Caddyfile 尾部
  注释示例（需域名 DNS 可解析，Caddy 自动签发证书）。

## 四·六、ESXi Web 管理界面嵌入（esxi-admin 插件）

`esxi-admin` 插件在门户内 iframe 嵌入 ESXi Host Client（`https://<esxi-host>/ui/`）。
**直连内网 IP 不可行**（外部设备不可达 + `X-Frame-Options: DENY`），必须经 Caddy 反代：

### 反代规则（由 esxi-admin 插件提供，已带门户鉴权）

ESXi 反代规则**由 `esxi-admin` 插件的 `caddy_rules` 默认值**（`DefaultESXIAdminCaddyRules`，
见 `backend/internal/pluginhost/caddy.go`）提供，后端启动时落盘 `<PLUGIN_CADDY_DIR>/<id>.caddy`
并 reload；**内置 `caddy/Caddyfile` 不再包含任何 ESXi handle**。因此**停用/删除该插件即移除规则，
`/esxi/*` 等路径不再反代，访问自然收回**。

**鉴权**：每个 handle 先经 Caddy `forward_auth` 回调门户鉴权闸口
`GET /api/v1/auth/gate?perm=esxi-admin:use`（校验请求 cookie 中的 access/refresh 令牌，
再用角色矩阵校验 `esxi-admin:use` 权限）——未登录返回 401、无权限返回 403（iframe 内显示中文
提示页），放行后才反代到 ESXi。插件页在门户内加载时每 5 分钟静默续期 access cookie
（`pages/plugins/[...slug].vue`），长会话不中断。

路径要点（与插件规则一致）：

- `/esxi/*` → 剥前缀后指向 ESXi（页面与相对路径静态资源；iframe 地址
  `ESXI_WEB_URL=/esxi/ui/` 同源反代，本地 http 与隧道 https 均可用）；
- ESXi 资源/API 走**绝对路径**，需全部反代：`/ui/*`、`/sdk*`、`/sts*`、`/ticket*`、
  `/vfeed/*`、`/converter/*`、`/eam/*`、`/pbm/*`、`/sms/*`、`/vsan/*`；
- `/screen*` 是 VM **控制台预览截图**端点（`/screen?id=<moid>&ts=<时间戳>`，相对路径，
  轮询刷新）——漏配会落到前端 3001 兜底，预览图报 `blob:... ERR_FILE_NOT_FOUND`；
- **必须剥除的响应头**：
  - `X-Frame-Options: DENY` —— 否则浏览器拒绝 iframe 加载（报"拒绝了我们的连接请求"）；
  - `Content-Security-Policy: upgrade-insecure-requests` —— 否则 http 页面下
    子资源被强制升级 https 导致登录失败（`ERR_SSL_PROTOCOL_ERROR`）；
- `/ticket*` 是 VM 控制台 **WebSocket 端点**（`wss://<host>/ticket/<ticket>`），
  Caddy `forward_auth`（恒发 GET 子请求）+ `reverse_proxy` 原生透传 WS；
- 目标 ESXi 用 `ESXI_UPSTREAM` 环境变量设置（**只填主机名/IP，不带 scheme 与端口**，上游固定走 443；
  **仓库不预设地址**；未设置时 Caddy 仍正常启动，仅访问 `/esxi/*`、`/ui/*` 等路径请求期报错，
  避免静默反代到错误主机；该变量由插件规则经 `{env.ESXI_UPSTREAM}` 在运行时解析）。

### 本机/局域网 https 入口（必须）

ESXi Host Client 的 JS **硬编码 `https://` 与 `wss://`** 构造 SDK/控制台地址
（`$location.host():$location.port()`），因此 **http 页面下 iframe 无法登录/打开控制台**，
须有 https 入口（隧道域名本身是 https，无此问题）。Caddyfile 中已提供
`https://:8443` 配置（默认注释，`tls` 指令指定证书）：

```powershell
# 1) 生成自签证书（CN=127.0.0.1，含 localhost SAN）
openssl req -x509 -newkey rsa:2048 -keyout C:\portalt\local\portalt-local.key `
  -out C:\portalt\local\portalt-local.crt -days 3650 -nodes `
  -subj "/CN=127.0.0.1" -addext "subjectAltName=IP:127.0.0.1,DNS:localhost"
# 2) 装入系统信任库（无警告），需管理员权限
certutil -addstore -f Root C:\portalt\local\portalt-local.crt
# 3) 取消 Caddyfile 中 https://:8443 块注释后重启
```

> **坑**：`tls internal`（Caddy 自动内部 CA）在 Windows schannel 握手失败
> （`SEC_E_INTERNAL_ERROR`，ECC 证书兼容问题），必须用 openssl 生成的 RSA 证书显式指定。
> 前端 `ESXI_WEB_URL=/esxi/ui/`（相对路径，同源）——经 http 入口的页面
> **不会自动变成 https**，因此门户本机访问请用 `https://127.0.0.1:8443`（或隧道域名），
> 详见下表"访问入口"。

### 验证

```powershell
# 1) 未登录直连 → 401（鉴权闸口拒绝，若得 200 说明 ESXi 已暴露）
curl -k -o /dev/null -w "%{http_code}\n" https://127.0.0.1:8443/esxi/ui/
# 2) 带上门户 cookie（登录后在浏览器复制 access_token）→ 应放行（200/301）
curl -k -o /dev/null -w "%{http_code}\n" -H "Cookie: access_token=<门户access令牌>" https://127.0.0.1:8443/esxi/ui/
# 3) 登录后控制台：浏览器 DevTools → Network → WS，观察 wss://<入口>/ticket/<ticket> 连接成功
```

> 注意：`https://:8443` 入口（本机/局域网，未启用域名 HTTPS 时）也要能访问插件规则，
> Caddyfile 中该块的 `import plugins.d/*.caddy` 须与主站一致（已内置）。

## 五、验证

```powershell
# 1) 页面与 API（带上隧道域名 Host 模拟 cloudflared 转发）
curl -H "Host: demo.tonyjh07.dpdns.org" http://127.0.0.1:8808/
curl -X POST -H "Host: demo.tonyjh07.dpdns.org" -H "Content-Type: application/json" `
  -d '{"username":"admin","password":"admin123"}' http://127.0.0.1:8808/api/v1/auth/login

# 2) WebSocket 隧道（外部 Origin + 隧道 Host）
node ws-host-test.cjs   # 预期：WS OPEN → 收到 VNC 渲染指令 → 连接保持活跃

# 3) 真机：手机/其他电脑浏览器打开 https://demo.tonyjh07.dpdns.org 登录并打开 VM 远程桌面
```

## 常见问题

| 现象 | 处理 |
|------|------|
| 页面提示 `Blocked request. This host is not allowed` | `nuxt.config.ts` 的 `vite.server.allowedHosts` 加入域名后重启 dev server |
| 页面能开但 WS 连不上 | Cloudflare 后台域名 → Network → WebSockets 开启 |
| 登录后 401 / 令牌失效 | HTTPS 下 cookie 正常（SameSite=Lax）；注意 CF 缓存不会命中带 Set-Cookie 的 POST |
| 远程桌面握手失败 | guacd 未启动 / `GUACD_URL` 错误 / 目标机不可达；另注意演示容器
  `host.docker.internal` 仅对同宿主机目标有效，外部 VM 需在 metadata 配可达的 `guac.hostname` |

## 生产端口分配

混合部署（推荐）：本机运行 Caddy/后端/前端 preview，**Caddy 是唯一对外出口**，
其余服务只绑 127.0.0.1（仅 Caddy 所在宿主机可达）。

| 端口 | 服务 | 绑定 | 说明 |
|------|------|------|------|
| **8808** | Caddy 入口 | 全部接口（v4+v6） | 隧道/LAN/v6 统一入口；`CADDY_PORT` 可改 |
| **8443** | Caddy https（ESXi 控制台） | 全部接口（v4+v6） | 自签 RSA 证书；ESXi Host Client 硬编码 https，http 页无法登录/开控制台。证书 SAN 仅 127.0.0.1/localhost，局域网他机访问需另签含对应 IP 的证书 |
| 80/443 | Caddy HTTPS（可选） | 全部接口 | 域名 TLS 就绪后启用，见 `caddy/Caddyfile` 注释 |
| 3001 | 前端 preview | 127.0.0.1 | `PORT=3001`（`node .output/server/index.mjs`） |
| 8080  | 后端 | 127.0.0.1 | `PORT` 可覆盖（默认 `127.0.0.1:8080`） |
| 4822  | guacd | 127.0.0.1 | compose 已收紧；后端经此连 guacd |
| 5432  | postgres | 127.0.0.1 | compose 已收紧；仅宿主机/后端用 |
| 5900  | vnc-demo | 127.0.0.1 | 演示容器；生产指向真实 VM 后移除 |
| 3000 | dev server | 127.0.0.1 | 仅开发；生产不占用（Caddy 与他端口错开） |

## 生产部署说明

> 生产（Linux）推荐直接 `bash deploy/install.sh`（见 [how-to-use.md](./how-to-use.md) §3）：
> 自动安装 Caddy 系统包/Docker/cloudflared、生成 systemd 服务与 `portalt.env`、
> 写入 `/etc/caddy/Caddyfile` 与 Caddy drop-in 环境变量、健康检查。以下为手工场景说明。

- 生产构建不带 devProxy，`/api` 反代依赖 `routeRules`（不支持 WS 升级），因此
  **入口必须经过 Caddy**（见上节）：
  `cloudflared → http://127.0.0.1:{CADDY_PORT} → Caddy → 前端 3001 / 后端 8080`；
- `docker-compose.yml` 仅提供数据库与远程桌面演示容器（postgres/guacd/vnc-demo），
  端口一律收紧到 127.0.0.1，不参与对外暴露；后端/前端/Caddy 在宿主机以
  二进制运行（或按需再容器化并接入同一内部网络）；
- RustDesk 为一键唤起本机客户端（`rustdesk://`），不部署 hbbs/hbbr 服务器，
  无需额外对外开放端口；
- **对外暴露前务必**：修改默认管理员密码（`admin/admin123`，见
  `ADMIN_PASSWORD`）、配置强 `JWT_SECRET`，并确认 Caddy 仅暴露在可信网络/隧道内。

### Caddyfile 快速配置（生产）

仓库 `caddy/Caddyfile` 是唯一权威配置（**本机调试用的临时版不部署**，两者差异仅
`CADDY_PORT` 变量/`/healthz` handle/注释态的 8443、443 块）。生产配置只需三步：

1. 复制 `caddy/Caddyfile` 到部署机（如 `C:\portalt\caddy\Caddyfile`），**无需改内容**；
   ESXi 反代规则由后端插件落盘（`plugins.d/<id>.caddy`），**后端须先启动并完成规则写盘 + reload**，
   `/esxi/*` 才会被反代；
2. 环境变量注入（可写入服务/计划任务环境）：
   ```powershell
   $env:CADDY_PORT = "8808"                   # 入口端口（默认 8808，改后同步隧道 ingress）
   $env:ESXI_UPSTREAM = "esxi.lan"           # 目标 ESXi，只填主机名/IP（必填，仓库无默认；多 ESXi 时必配）
   ```
   `ESXI_UPSTREAM` 由**插件规则**经 `{env.ESXI_UPSTREAM}` 在运行时解析；
3. 启动：`caddy run --config C:\portalt\caddy\Caddyfile`；
   需要 ESXi 管理界面嵌入时另起 https 入口：按 §四·六 生成自签 RSA 证书并解开
   `https://:8443` 注释块（证书路径改为部署机实际路径）。

> **升级顺序**（本特性涉及后端与 Caddy 两处）：先部署新后端（seed 将插件规则升级为带鉴权闸口
> 的新默认值并写盘 reload），再同步新 Caddyfile（update.sh 自动校验/回滚）。反序时旧 Caddyfile
> 的 ESXi handle 无鉴权闸口，`/esxi/*` 会**短暂无鉴权暴露**（而非仅 5xx），请尽量缩短该窗口、
> 避免在窗口内公布入口地址。

> **"拉仓库改了就能跑"需要的前置**：Caddyfile 本身 `caddy validate` 即可用，但完整门户
> 还需——后端二进制（`go build ./cmd/server`，配 `VIRT_PROVIDER`/ESXi 凭据/DB/JWT
> 等环境变量，见 `.env.example`）、前端产物（`npm run build` 后 `node .output/server/index.mjs`，
> 端口 3001）、远程桌面需 guacd（`docker compose up -d guacd`）、外部依赖：ESXi
> 可达、cloudflared 隧道指向 `CADDY_PORT`、8443 证书已生成并信任。

## dev 模式经隧道的已知问题

- **不要用 dev 模式对外提供隧道访问**（本地调试例外）：
  1. **慢**：dev 冷加载需实时 transform 上千个模块，经隧道逐个下载可达分钟级，期间白屏；
  2. **全局 CSS MIME 报错**：`Failed to load module script ... MIME type of "text/css"`，
     由 Vite 对同一 CSS URL 的 link/import 双形态 + 浏览器缓存引起（详见
     `docs/conventions.md`「Nuxt dev 的全局 CSS 加载 bug」），已用 patch-package 修补；
  3. **切换 dev ↔ preview 后**：浏览器缓存了旧 HTML，需**硬刷新（Ctrl+Shift+R）**，
     否则会请求已不存在的 `/_nuxt/@vite/client`、`/_nuxt/C:/...` 等 dev 资源（404）。
- 对外验证请用生产产物：`npm run build && node .output/server/index.mjs`
  （监听 3001，`PORT=3001` 覆盖，避开 dev 的 3000）。
