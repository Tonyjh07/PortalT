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
2. **API**：`PUT /api/v1/vms/:id/metadata`（需 `vm:manage` 权限，body 为键值对象，值为 `null` 的键删除）。示例：

```json
{
  "guac.protocol": "vnc",
  "guac.hostname": "192.168.1.10",
  "guac.port": "5900",
  "guac.password": "vm-password"
}
```

> 注意：`SyncVMs`/状态轮询采用 metadata 合并策略——提供者未提供的键保留库内值，手动配置不会被平台同步覆盖（workstation 适配器 IP 走 `ip_address` 字段，不写 metadata）。

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
