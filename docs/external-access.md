# 外部访问指南（Cloudflare Tunnel）

> 从外网访问 PortalT 的推荐方案：`cloudflared` 隧道出站连接 Cloudflare 边缘，
> **无需开放任何入站端口/防火墙规则**，适合 HomeLab 场景（无公网 IP、运营商 NAT）。
> 本指南以 Windows 宿主机 + Nuxt dev 模式为例；生产部署见文末。

## 架构

```
浏览器 ──https──> Cloudflare 边缘 ──隧道(出站)──> cloudflared(本机)
                                                    │
                                                    ▼
                                        http://127.0.0.1:3000 (Nuxt dev)
                                                    │  /api 代理(含 WS)
                                                    ▼
                                        http://127.0.0.1:8080 (PortalT 后端)
                                                    │  guacd 指令流
                                                    ▼
                                        guacd :4822 → VNC/RDP 目标机
```

- cloudflared 主动连出到 Cloudflare，入站 80/443 全部由 CF 边缘接管；
- 页面/API/WebSocket 全部经同一条隧道（cloudflared 原生透传 WS，仅需在
  Cloudflare 后台确认该域名的 **WebSockets 已启用**，免费版默认开启）；
- 前端 WS 地址是相对路径（`/api/v1/guac/ws/...`），浏览器自动使用
  `wss://<域名>/...`，无需改动代码。

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
   `/api` 与 WS 由 dev 模式 `nitro.devProxy` + `frontend/modules/wsProxy.ts` 转发。

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

- `npm run build` 产物（`.output/`）运行时不带 devProxy，需由反向代理拆分
  `/api` 与页面流量。仓库 `caddy/Caddyfile` 已具备该路由（`/api/* → backend:8080`，
  `/* → frontend:3000`），Caddy 原生支持 WebSocket 升级；
- 生产路径：`cloudflared → http://127.0.0.1:80(caddy) → frontend/backend`，
  caddy 容器需 `extra_hosts` 指向宿主机，或全部容器化后走 compose 内部网络；
- `docker-compose.yml` 中 caddy 已绑定 80/443，与隧道无冲突（隧道不占入站端口）。
