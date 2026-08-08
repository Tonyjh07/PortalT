# AGENTS.md — PortalT 原生插件模板

开发 PortalT 原生插件时的约束与快捷方式。主仓库见 `github.com/Tonyjh07/PortalT`。

## 环境 / 工具链

- Go 1.26+ 构建；**非 Go 插件**也可（实现 gRPC 控制面 + HTTP 数据面即可）。
- 构建插件必须 `CGO_ENABLED=0`（PortalT 构建环境无 C 编译器）。
- `make build` / `make install` / `make clean` 是标准命令。
- `make install` 把编译产物和 `manifest.json` 复制到 `$(PLUGINS_DIR)/$(ID)/`。

## 协议（不可协商）

### gRPC 控制面

只有四个 RPC，见 `proto/plugin/v1/plugin.proto`（主仓库）：

| RPC | 方向 | 用途 |
|-----|------|------|
| `Handshake` | 宿主 → 插件（启动后立即） | 插件上报 HTTP 端口 + manifest，宿主返回启用状态 |
| `Health` | 宿主 → 插件（周期） | 返回 healthy 表示正常运行 |
| `Shutdown` | 宿主 → 插件（停用/升级/删除） | 插件清理资源后退出 |
| `Notify` | 宿主 → 插件（事件） | enabled / disabled / config_changed / restarting |

Handshake 响应中的 `enabled` 字段应被插件尊重（禁用时不工作）。

### HTTP 数据面

- 插件自起 HTTP 服务器，端口在 Handshake 中上报（宿主校验回环）。
- **必须提供 `/healthz` 返回 `200 OK`**（宿主握手后探测，验证数据面可达）。
- **仅监听 `127.0.0.1`**（宿主防 SSRF）。
- 其余路径宿主编排到 `/native/<id>/*`（静态）和 `/api/v1/plugins/native/<id>/*`（鉴权 API）。

## go.mod 关键点

- `module github.com/Tonyjh07/PortalT-plugin-template` — fork 后改为你自己的模块路径。
- `replace portalt => ../../../backend` — 作为 submodule 时指向 PortalT 的 `backend/`；**独立开发必须改此路径**（指向本地 PortalT clone 的 `backend/` 目录，或删除 replace 并拷贝 proto 生成代码）。
- 运行 `go mod tidy` 后提交 `go.sum`。

## manifest.json

- **必须是合法 JSON**（无注释行；`$comment` 字段是 JSON 标准字段，合规）。
- `id` 必须等于 `PLUGINS_DIR` 下的目录名，仅允许 `[a-zA-Z0-9._-]`，字母数字开头。
- `permission` 必须是 PortalT 权限字典中的值（管理 API 校验；空串 = 无额外权限）。
- `route` 必须以 `/` 开头。

## 构建与投放

开发期测试：

```bash
# 构建（产物 = plugin/.exe）
make build
# 或手动
CGO_ENABLED=0 go build -o plugin ./cmd/plugin

# 投放（需 PLUGINS_DIR 指向 PortalT 运行时目录）
make install
# 或手动
mkdir -p $PLUGINS_DIR/<id>/
cp plugin manifest.json $PLUGINS_DIR/<id>/
# 可选静态前端
cp -r static $PLUGINS_DIR/<id>/
```

PortalT 管理界面启用插件（默认禁用态），宿主将自动 spawn 进程。

## 模板与集成测试

PortalT 主仓库的集成测试 `go test -tags integration ./internal/pluginhost/...` 会真实 spawn 示例插件 `examples/hello`，验证全生命周期。用此模板开发的插件应遵循相同契约。

## 任意语言实现

模板是 Go，但协议通用。只需：
1. 实现 gRPC server 监听 `PORTALT_PLUGIN_GRPC_PORT`（回环），实现四个 RPC
2. 实现 HTTP server 监听 `PORTALT_PLUGIN_HTTP_PORT`（回环），提供 `/healthz`
3. 把可执行文件 + `manifest.json` 放入 `PLUGINS_DIR/<id>/`（目录名 = manifest id）