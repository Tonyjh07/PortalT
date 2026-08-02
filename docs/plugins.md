# PortalT 插件开发指南

PortalT 的插件机制共三种类型，均为门户侧"菜单项 + 页面"的扩展点：

| 类型 | 创建方式 | 页面 | API | 适用场景 |
|------|----------|------|-----|----------|
| `iframe` | 管理界面/API | 门户 iframe 嵌入 `iframe_url` | 无（浏览器直连目标） | 内嵌已有 Web 界面（如 ESXi Web UI、Grafana） |
| `proxy` | 管理界面/API | 门户 iframe 嵌入自身页面 | 标准 API 代理转发到外部服务 | 自带脚本前端、需要服务端转发的应用 |
| `native` | 代码注册（编译进二进制） | `/native/<id>/` 内嵌静态页 | `/api/v1/plugins/native/<id>/...`（Go 实现） | 需要后端能力（调度、平台交互、鉴权数据）的插件 |

本文重点讲 **native 原生插件**（`iframe`/`proxy` 无后端代码，配置见 `interfaces.md` 插件管理一节）。

## 原生插件：目录与文件约定

```
backend/internal/plugins/examples/<name>/
├── plugin.go        # 插件实现（接口 + API + 可选后台任务）
├── plugin_test.go   # 路由级测试
└── static/
    └── index.html   # 内嵌静态前端（自包含单页，css/js 内联）
```

- 包注释须说明：演示的能力、数据是否持久化（示例默认内存、重启重置）
- 包名与目录名一致（如 `package cron`）

## 原生插件：Plugin 接口

实现 `internal/plugins.Plugin` 的三个方法：

```go
type Plugin interface {
    Info() domain.Plugin              // 元信息：ID/Name/Icon/Route/SortOrder/IsActive
    Mount(rt *gin.RouterGroup, deps Deps) // 注册 API（rt 已带鉴权与启用闸门）
    StaticFS() fs.FS                  // 内嵌静态前端（fs.Sub 风格），无前端返回 nil
}
```

约定：

- `Info()`：`ID` 必须全局唯一（与 `Route` 一致，格式 `"/"+ID`，如 `"cron"` → `/cron`）；
  `SortOrder` 控制侧栏顺序，不设置时同步默认 100
- `Mount()`：所有路由都位于 `/api/v1/plugins/native/<id>/` 之下，**不要再加前缀**；
  路由挂载前已通过 `nativeGate`——插件在 plugins 表中不存在或已停用 → 404
- 路由组/分组本身已带 `plugin:view` 权限；需要更细粒度（如仅管理员）时，
  在单个路由上加 `middleware.RequirePermission(domain.PERM_XXX)`
- API 响应一律用统一信封：`response.OK(c, data)` / `response.Error(c, httpStatus, code, msg)`，
  错误码见 `docs/interfaces.md`（常用：`CodeNotFound` 4006、`CodeInvalidOperation` 4007、`CodeBadRequest` 4004）

## 原生插件：依赖注入 Deps

```go
type Deps struct {
    Provider string // 当前虚拟化平台：esxi / workstation / mock
    WebURL   string // 平台 Web 管理界面地址（如 https://esxi.lan/ui/），空=未配置
}
```

- 只读注入，**勿持有跨请求状态**
- 需要虚拟化能力（ListVMs/电源操作/GetHostInfo）时向 `plugins` 包扩展新的门面接口，
  与 `VMServiceFacade`（Phase 9 示例期接口，已随 esxi-admin 简化移除）同模式：
  在 `ports` 层定义小接口，`main.go` 装配时注入实现

## 原生插件：注册与菜单同步

1. `backend/cmd/server/main.go` 的 `builtinPlugins()` 追加 `xxx.New()`
2. 启动时 `Registry.Register` 校验 ID 冲突，随后 `services.SyncNativePlugins` 按 `Info()`
   对 plugins 表做 upsert：
   - 新插件：插入记录（`type=native`）
   - 已存在：只更新技术字段（类型/名称/图标/路由），**保留管理员在界面设置的权限与启用状态**
3. 因此 `native` 类型不能通过管理 API 创建，也**不建议删除**（重启后会被代码重新 upsert）

## 原生插件：静态前端约定

- 公开托管于 `/native/<id>/`（无鉴权中间件），所以**页面本身不得包含敏感数据**，
  数据一律通过鉴权 API 获取
- 认证：从 cookie `access_token` 读取令牌，调用 API 时带 `Authorization: Bearer <token>`：

  ```js
  function token() {
    const m = document.cookie.match(/(?:^|;\s*)access_token=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }
  ```

- 深浅色主题与门户同步（门户主题键为 `localStorage['portalt-theme']`，`html.dark` class）：

  ```js
  function applyTheme() {
    const saved = localStorage.getItem('portalt-theme');
    const dark = saved === 'dark'
      || (!saved && window.matchMedia('(prefers-color-scheme: dark)').matches);
    document.documentElement.classList.toggle('dark', dark);
  }
  applyTheme();
  window.addEventListener('storage', e => { if (e.key === 'portalt-theme') applyTheme(); });
  // 同源兜底：观察父窗口 html.dark
  try {
    const parentEl = window.parent.document.documentElement;
    if (parentEl && parentEl !== document.documentElement) {
      new MutationObserver(() =>
        document.documentElement.classList.toggle('dark', parentEl.classList.contains('dark'))
      ).observe(parentEl, { attributes: true, attributeFilter: ['class'] });
    }
  } catch (e) { /* 跨域忽略 */ }
  ```

- 样式基准：CSS 变量 + `html.dark` 覆盖双套主题（`--bg/--card/--text/--accent` 等），
  渐变光晕背景、毛玻璃卡片，参考现有示例 `esxiadmin` / `cron` 的 `static/index.html`
- 轮询类刷新建议 5s 起步，避免频繁请求；操作类交互后局部刷新

## 原生插件：后台任务

示例（`cron` 插件）模式：`New()` 时创建调度器并 `go` 启动后台循环，
插件随进程生命周期存活；共享状态必须用 `sync.Mutex` 保护；内存数据不持久化，
需持久化时自行对接 `ports` 仓储并在文档注明。

## 原生插件：测试约定

- 路由级测试：`gin.SetMode(gin.TestMode)` + auth 桩中间件 + `New().Mount(...)`
- 覆盖点：列表返回、状态切换、执行/副作用（如日志增长）、404 分支、静态页关键内容
  （`fs.ReadFile(fsys, "index.html")` 断言含 `esxiFrame`、`portalt-theme` 等标记）
- 运行：`go test ./internal/plugins/... -count=1`（提交前须全绿）

## 示例索引

| 插件 | 位置 | 演示能力 |
|------|------|----------|
| esxi-admin | `internal/plugins/examples/esxiadmin` | iframe 嵌入 ESXi Web 管理界面：`/config` 返回 provider/connected/web_url，现代占位页三态（未连接/已连接未配置/可嵌入） |
| cron | `internal/plugins/examples/cron` | 内存定时任务：后台调度 goroutine + 任务管理 API（jobs/toggle/run/logs）+ 现代化管理页 |

## 常见坑

- `ID` 冲突会注册失败并导致启动报错——保持唯一
- 静态页忘了主题同步 → 深色门户下 iframe 内一片白（或反之）
- `Mount` 里手写权限中间件时注意顺序：`RequirePermission` 应作用于具体路由而非整组
- 内存态插件（如 cron）文档与页面都要注明"重启后重置"
