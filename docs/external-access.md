# 外部访问指南（Cloudflare Tunnel）

> 从外网访问 PortalT 的推荐方案：`cloudflared` 隧道出站连接 Cloudflare 边缘，
> **无需开放任何入站端口/防火墙规则**，适合 HomeLab 场景（无公网 IP、运营商 NAT）。
> 本指南以 Windows 宿主机 + Caddy 反代（生产构建）为例。

## 架构

```
浏览器 ──https──> Cloudflare 边缘 ──隧道(出站)──> cloudflared(本机)
                                                     │
                                                     ▼
                                        Caddy :3000  (本地反代，原生支持 WS 升级)
                                        ├── /api/*、/native/* → 127.0.0.1:8080 (后端)
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
    service: http://127.0.0.1:3000       # Nuxt dev / 生产前端
  - service: http_status:404             # 兜底
```

## 三、DNS 路由与启动

```powershell
cloudflared tunnel route dns portalt demo.tonyjh07.dpdns.org
cloudflared tunnel run portalt           # 前台运行（生产可用 NSSM/服务方式）
```

> 本机隧道实际为 **Zero Trust token 模式**（服务运行参数
> `cloudflared tunnel run --token-file <token>`），入口规则在 Cloudflare 控制台
> （Zero Trust → Networks → Tunnels）配置，全部流量指向 `http://127.0.0.1:3000`。

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

3. 后端 `:8080`、guacd `:4822` 均在 cloudflared 所在宿主机，无需对外暴露；
4. **Caddy 监听 3000**（隧道入口），把 `/api/*`、`/native/*` 转发到后端 8080，
   其余转发到 Nuxt preview（生产构建监听 3001），Caddyfile 见下节。

## 四·五、Caddy 反代（推荐，生产入口）

```text
# Caddyfile（监听 3000 作为隧道入口；Caddy 原生支持 WebSocket 升级）
:3000 {
	handle /api/* {
		reverse_proxy 127.0.0.1:8080
	}
	handle /native/* {
		reverse_proxy 127.0.0.1:8080
	}
	handle {
		reverse_proxy 127.0.0.1:3001
	}
}
```

启动（Windows，示例路径）：

```powershell
caddy run --config C:\path\to\Caddyfile --adapter caddyfile
```

> - 本机 Caddy 由 Go 源码编译安装（`go install github.com/caddyserver/caddy/v2/cmd/caddy@v2.11.4`，
>   winget 因外网不通不可用），二进制在 `%USERPROFILE%\go\bin\caddy.exe`；
> - 前端预览监听 3001：`node .output/server/index.mjs`（nitro 默认 3000，
>   用 `PORT=3001` 或 nitro 配置覆盖）；`NUXT_PUBLIC_API_WS_BASE` **留空**
>   （同源 `wss://<域名>/api/...` 走隧道 → Caddy → 后端）；
> - 验证命令（本机模拟隧道）：
>   - 页面：`Invoke-WebRequest http://127.0.0.1:3000/`（200）
>   - WS：`ws://127.0.0.1:3000/api/v1/guac/ws/<vmId>?token=<token>`（应 OPENED 并收到渲染指令）

## 五、验证

```powershell
# 1) 页面与 API（带上隧道域名 Host 模拟 cloudflared 转发）
curl -H "Host: demo.tonyjh07.dpdns.org" http://127.0.0.1:3000/
curl -X POST -H "Host: demo.tonyjh07.dpdns.org" -H "Content-Type: application/json" `
  -d '{"username":"admin","password":"admin123"}' http://127.0.0.1:3000/api/v1/auth/login

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

## 生产部署说明

- 生产构建不带 devProxy，`/api` 反代依赖 `routeRules`（不支持 WS 升级），因此
  **隧道入口必须经过 Caddy**（见上节）：`cloudflared → http://127.0.0.1:3000(caddy)`
  → 前端 3001 / 后端 8080；
- 容器化时：Caddyfile 中上游改为容器名（`backend:8080`、`frontend:3000`），
  全部服务进 compose 内部网络即可，Caddy 无需绑定宿主端口；
- `docker-compose.yml` 中 caddy 已预留 80/443 映射，与隧道不冲突
  （隧道不占入站端口，Cloudflare 边缘直连本机出站链路）。

## dev 模式经隧道的已知问题

- **不要用 dev 模式对外提供隧道访问**（本地调试例外）：
  1. **慢**：dev 冷加载需实时 transform 上千个模块，经隧道逐个下载可达分钟级，期间白屏；
  2. **全局 CSS MIME 报错**：`Failed to load module script ... MIME type of "text/css"`，
     由 Vite 对同一 CSS URL 的 link/import 双形态 + 浏览器缓存引起（详见
     `docs/conventions.md`「Nuxt dev 的全局 CSS 加载 bug」），已用 patch-package 修补；
  3. **切换 dev ↔ preview 后**：浏览器缓存了旧 HTML，需**硬刷新（Ctrl+Shift+R）**，
     否则会请求已不存在的 `/_nuxt/@vite/client`、`/_nuxt/C:/...` 等 dev 资源（404）。
- 对外验证请用生产产物：`npm run build && node .output/server/index.mjs`（监听 3000）。
