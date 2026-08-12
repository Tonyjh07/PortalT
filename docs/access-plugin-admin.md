# access 插件管理手册（创建 / 管理 / 修改 / 删除）

面向运维与管理员的实操指南：通过管理界面（`/plugins-admin`）或管理 API 维护 access 插件。
原理、字段语义与开发视角见 [plugins.md](./plugins.md)；HTTP 接口与错误码汇总见 [interfaces.md](./interfaces.md)。

## 1. 概念速览

access 插件是**纯配置型**插件（无进程、无代码），一份插件可同时提供三种能力，**任意共存**：

| 能力 | 字段 | 作用 |
|------|------|------|
| 嵌入页面 | `iframe_url` | 门户内 iframe 嵌入外部页面（绝对地址）或门户内相对路径（配套 Caddy 反代） |
| API 代理 | `api_url` + `endpoints` | 白名单 API 代理：门户只转发声明过的端点，注入调用者身份头 |
| Caddy 反代 | `caddy_rules` | 原始 Caddy `handle` 片段，落盘 `plugins.d/<id>.caddy` 并 reload 热生效 |

关键行为：

- 管理操作需 `plugin:manage` 权限（默认仅 admin）。
- 保存/删除含 `caddy_rules` 的插件时，后端**自动落盘规则文件并 reload Caddy**，热生效。
- **停用插件（`is_active=false`）或清空 `caddy_rules` 会移除对应规则文件**——反代路径立即收回。
- 若 `PLUGIN_CADDY_DIR` 未配置（本地 dev 无 Caddy），规则仅存数据库，保存返回告警而非静默成功。

## 2. 字段与校验规则

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | 创建时可省 | 1-64 位字母数字与 `._-`，须以字母/数字开头；省略时自动生成 UUID。**创建后不可改**（路径/规则文件名都依赖它） |
| `name` | ✅ | 菜单显示名 |
| `icon` | | MDI 图标标识，如 `mdi:nas` |
| `route` | ✅ | 菜单路由，如 `/omv-admin` |
| `type` | | `access`（默认）或 `native`；**native 不可手动创建**，由宿主按 manifest 自动注册 |
| `iframe_url` | 见下 | 外部 `http(s)` 地址，或门户内相对路径（`/` 开头且非 `//`） |
| `api_url` | | 目标服务地址，仅允许 `http(s)` |
| `endpoints` | | `[{method, path, name, description}]`，`path` 必须以 `/` 开头（如 `/api/info`） |
| `caddy_rules` | | 原始 Caddy `handle` 片段（见 §3.2） |
| `permission` | | 访问该插件所需权限，**必须存在于权限字典**（`GET /api/v1/roles/permissions` 可查）；空 = 无需额外权限 |
| `sort_order` | | 菜单排序，越小越靠前 |
| `is_active` | | 是否启用 |

必填组合：access 必须提供 `iframe_url`，或 `api_url` + **至少一个**端点，二者至少其一。

## 3. 配置编写

### 3.1 iframe_url：两种形态

- **外部地址**：`https://ha.local` —— 浏览器直接嵌入目标页面。注意：门户为 https 时**不能嵌 http 地址**（mixed content 会被浏览器拦截），必须走下方反代形态。
- **门户内相对路径**：`/omv/` —— 由插件自己的 `caddy_rules` 反代到目标，浏览器与门户同源。
  **必须带尾斜杠**：`/omv/` 下页面的相对资源（`main.js`、API 请求）会解析为 `/omv/main.js`、`/omv/api/...`；若配成 `/omv`（无斜杠），资源会解析到根路径 `/main.js` 而 404。
- 目标页面带 `X-Frame-Options` / CSP 禁嵌响应头时，外部直嵌会被浏览器拒绝，同样应改走反代并剥头（见 §3.2）。

### 3.2 caddy_rules：反代规则

**只写本插件的 `handle` 块**；不要写站点监听、全局指令（会破坏 Caddyfile 语法导致 reload 失败）。
保存时若环境存在 `caddy` 且规则不含 `{env.*}` 占位符，后端会先 `caddy validate` 校验片段，**校验失败不落盘**。

最小反代模板（目标为内网 http 服务）：

```
handle /omv/* {
	uri strip_prefix /omv
	reverse_proxy 192.168.2.22:80
}
```

目标页面禁 iframe 时，剥掉相关响应头（否则嵌入仍被拒）：

```
handle /omv/* {
	uri strip_prefix /omv
	reverse_proxy 192.168.2.22:80 {
		header_down -X-Frame-Options
	}
}
```

需要门户鉴权的反代（参考 esxi-admin：每个 handle 先回调鉴权闸口，未登录 401、无权限 403，放行才反代）：

```
handle /esxi/* {
	forward_auth 127.0.0.1:8080 {
		uri /api/v1/auth/gate?perm=esxi-admin:use
	}
	uri strip_prefix /esxi
	reverse_proxy {env.ESXI_UPSTREAM}:443 {
		transport http {
			tls
			tls_insecure_skip_verify
		}
		header_down -X-Frame-Options
		header_down -Content-Security-Policy
	}
}
```

无尾斜杠访问兜底（地址栏少打斜杠时 301 跳到正路径）：

```
handle /omv {
	redir https://{http.request.host}/omv/?{query} permanent
}
```

> **坑**：`redir` 的第一个参数若是路径形式（`/` 开头）会被 Caddy 当作 **matcher** 而非重定向目标——
> `redir /omv/ permanent` 实际解析为"匹配 `/omv/` 路径、重定向到字面 `permanent`"，无斜杠访问
> 不匹配任何处理器、返回空响应而非 301。目标地址必须以 `https://...`/`/` 之外的形式给出
> （如上例用 `{http.request.host}` 拼完整 URL）。

> 提示：目标服务若以**绝对路径**引用资源（如 `/javascript/...`、`/api/...`），子路径反代会 404，
> 需评估目标是否支持子目录部署（参考：ESXi Host Client 用根路径 handle `/ui/*` 反代；OMV/MCSManager
> 页面资源为相对路径，可子路径反代）。`/api/*`、`/native/*`、`/healthz` 是门户自身路由，
> 反代规则避免占用同名前缀。

### 3.3 endpoints：API 白名单

```json
"api_url": "http://127.0.0.1:8701",
"endpoints": [
  { "method": "GET",  "path": "/api/info",      "name": "服务信息" },
  { "method": "POST", "path": "/api/restart",   "name": "重启服务" }
]
```

门户按 方法+路径 精确匹配转发到 `/api/v1/plugin-proxy/:pluginId/*path`，并注入
`X-PortalT-User / X-PortalT-Role / X-PortalT-Perms` 身份头；未声明端点一律拒绝。
插件页"API 面板"可直接在线调用这些端点。

## 4. 管理 API 操作

### 4.0 前置：登录获取令牌

```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}'
# 响应 data.access_token 即后续请求的 Bearer 令牌（15 分钟有效）
TOKEN=<上面返回的 access_token>
```

以下所有请求带 `-H "Authorization: Bearer $TOKEN"`，需 `plugin:manage` 权限。

### 4.1 列表：GET /api/v1/plugins

返回全部插件（含停用、native），access 行附 `caddy_applied`（规则文件当前是否已落盘）。

### 4.2 创建：POST /api/v1/plugins

请求体示例（相对路径反代插件，本仓库生产实例形态）：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "omv-admin",
    "name": "OMV 管理",
    "icon": "mdi:nas",
    "route": "/omv-admin",
    "type": "access",
    "iframe_url": "/omv/",
    "caddy_rules": "handle /omv/* {\n\turi strip_prefix /omv\n\treverse_proxy 192.168.2.22:80 {\n\t\theader_down -X-Frame-Options\n\t}\n}",
    "permission": "",
    "sort_order": 100,
    "is_active": true
  }'
```

响应 `data` 为完整插件对象。成功时 `message` 为 `success`；**`message` 非 success 时是告警**（如
"Caddy reload 失败，规则已落盘将随下次 reload 生效"），插件本身已保存成功，`data` 仍可用。

### 4.3 修改：PUT /api/v1/plugins/:id

access 为**全字段覆盖**（请求体与创建相同，需包含所有字段）。典型用途：

```bash
# 停用（移除反代规则文件，访问立即收回）
curl -X PUT http://127.0.0.1:8080/api/v1/plugins/omv-admin \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":"omv-admin","name":"OMV 管理","icon":"mdi:nas","route":"/omv-admin","type":"access","iframe_url":"/omv/","caddy_rules":"...","permission":"","sort_order":100,"is_active":false}'
```

native 插件仅允许改 `permission` 与 `is_active`。

### 4.4 删除：DELETE /api/v1/plugins/:id

```bash
curl -X DELETE http://127.0.0.1:8080/api/v1/plugins/omv-admin \
  -H "Authorization: Bearer $TOKEN"
```

删除插件并**移除其规则文件 + reload**；native 记录由宿主管理，不可删除（删插件目录即标记 missing）。

### 4.5 重载 Caddy：POST /api/v1/plugins/caddy-reload

以数据库为准**全量对齐**规则文件（补写未落盘、清理孤儿文件）并 reload——用于规则保存后 reload 失败、
或手工改盘后的一次性修复。`PLUGIN_CADDY_DIR` 未配置时返回 503；reload 命令未配置时返回
200 + `message` 告警（`data.reloaded=false`）。

## 5. 管理界面操作（/plugins-admin）

| 操作 | 入口 |
|------|------|
| 新建 | "新建插件"按钮 → 填 ID/名称/图标/路由/类型/页面地址/API 端点/Caddy 规则/权限/排序/启用 |
| 编辑 | 行内"编辑"→ 对话框全字段 |
| 启停 | 行内开关（停用即移除反代规则） |
| 删除 | 行内"删除"（二次确认） |
| 修复规则 | 页头"重载 Caddy"按钮（同 §4.5） |
| 状态 | 列表"状态"列：已生效 / 待重载 / 停用 / 无规则；native 行为运行态 |

## 6. 常见问题排查

| 现象 | 排查 |
|------|------|
| 插件页显示"插件不存在或无权访问" | 插件未启用、`permission` 未授予当前用户、未登录 |
| 页面空白 / 404 | ① 反代未生效：列表状态"待重载"或 `caddy_applied=false` → 点"重载 Caddy" ② iframe 目标禁嵌未剥 `X-Frame-Options` ③ 目标资源为绝对路径，子路径反代 404（见 §3.2 提示）④ 相对路径缺尾斜杠（`/omv` 不匹配 `handle /omv/*`，落到门户 404；应配 redir 兜底，见 §3.2） |
| https 门户嵌 http 页面无响应 | mixed content 被浏览器拦截，改走 Caddy 反代 |
| 保存报"未知权限" | `permission` 不在权限字典（`GET /api/v1/roles/permissions` 查字典，或用空值） |
| "重载 Caddy" 503 | 部署机未配置 `PLUGIN_CADDY_DIR`，规则不会落盘 |
| 保存返回告警"未落盘/未热生效" | `PLUGIN_CADDY_DIR` 或 `CADDY_RELOAD_CMD` 未配置；配置后重启后端，再点"重载 Caddy" |
| 规则保存失败 | `caddy_rules` 语法错误（后端 `caddy validate` 拦截），或写了站点/全局指令 |
| 菜单不显示 | 检查 `is_active`、`sort_order`、组级 `plugin:view` 与 `permission` |
