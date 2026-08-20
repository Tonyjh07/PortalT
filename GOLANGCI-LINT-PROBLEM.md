# golangci-lint CI 失败问题记录

## 问题描述
CI 流程中的 golangci-lint 检查失败，报告 28 个问题：
- 22 个 errcheck 错误（未检查的错误返回）
- 3 个 staticcheck 错误
- 3 个 unused 错误

## 环境差异分析

### CI 环境
- 使用 golangci-lint-action@v9
- 自动下载并使用最新版本
- 在 GitHub Actions runner 中运行

### 本地环境
- 手动安装 golangci-lint v2.12.2
- Windows + Git Bash 环境
- 配置文件：`backend/.golangci.yml`

## 本地测试结果

### 测试命令
```bash
# 基础测试
golangci-lint run --config=C:/Develop/PortalT/backend/.golangci.yml

# 使用绝对路径
golangci-lint run --config=C:/Develop/PortalT/backend/.golangci.yml --path=C:/Develop/PortalT/backend

# 设置 CGO_ENABLED=0
CGO_ENABLED=0 golangci-lint run --config=C:/Develop/PortalT/backend/.golangci.yml

# 详细输出
golangci-lint run --config=C:/Develop/PortalT/backend/.golangci.yml --verbose

# 启用所有默认 linter
golangci-lint run --config=C:/Develop/PortalT/backend/.golangci.yml --enable-all
```

### 测试结果
所有本地测试均显示 **0 个问题**，与 CI 的 28 个问题形成鲜明对比。

## 可能的原因

### 1. 版本差异
- CI 可能使用比 v2.12.2 更新的版本
- 不同版本间的 linter 规则可能发生变化

### 2. 环境差异
- CI runner 可能有不同的 Go 版本或模块配置
- 环境变量差异（如 GOPROXY、CGO_ENABLED 等）

### 3. 配置差异
- CI 可能使用不同的默认配置
- 路径解析方式可能不同

### 4. 依赖差异
- CI 可能检测到本地未使用的依赖
- 模块下载状态可能不同

## 解决方案尝试

### 1. 检查 CI 配置
查看 `.github/workflows/ci.yml` 中的 golangci-lint-action 配置：
- 确认使用的版本
- 检查是否有额外的配置参数

### 2. 对比 CI 输出
需要获取 CI 的详细输出来确定具体的问题：
- 查看完整的错误信息
- 确定具体哪些文件和行有问题

### 3. 临时解决方案
如果无法复现，可以考虑：
- 暂时放宽 linter 规则
- 排除特定文件或目录
- 使用更保守的配置

## 配置文件内容

```yaml
# backend/.golangci.yml
version: "2"
linters:
  default: standard
  settings:
    staticcheck:
      checks: ["all", "-ST1000", "-ST1003", "-ST1005", "-ST1021"]
```

## 下一步行动

1. 获取 CI 的详细错误日志
2. 尝试在本地使用与 CI 相同的版本
3. 根据具体错误信息逐个修复
4. 考虑调整配置以适应项目实际情况

## 备注

- 本地环境：Windows 10 + Git Bash
- Go 版本：1.26.5
- golangci-lint 版本：v2.12.2
- 测试时间：2026-08-16

---

## 结论（2026-08-20 已定位并修复）

### 根因

**CI 检查的是已推送的 commit `ec7431b`；本地 `golangci-lint run` 检查的是工作区，
而 errcheck/staticcheck/unused 的全部修复（22 个未提交修改文件中的 14 个）从未提交、从未推送。**
「本地 0 问题 vs CI 28 问题」本质是拿工作区（已修复）对比远端提交（未修复），并非版本/平台差异。

### CI 实际报告的 28 项问题（run 31922552472，`golangci-lint v2.12.2` Linux）

**errcheck ×22**（`defer`/裸调用的 error 返回值未检查）：

| 文件:行 | 违规 |
|---|---|
| `internal/adapters/gormstore/plugin_repo.go:103` | `m.FromDomain(p)` |
| `internal/adapters/postgres/db.go:92`、`sqlite/db.go:96` | `defer rows.Close()` |
| `internal/adapters/workstation/provider.go:215,257` | `defer resp.Body.Close()` |
| `internal/api/v1/guac.go:58,75` | `defer clientConn.Close()` / `upstream.Close()` |
| `internal/api/v1/guac_test.go:38,80,83,103` | `defer conn.Close()` ×2、`SetReadDeadline`、`defer resp.Body.Close()` |
| `internal/api/v1/guac_tunnel.go:114,120,136,137` | `defer guacdConn.Close()`、`defer clientConn.Close()`、裸 Close ×2 |
| `internal/api/v1/guac_tunnel_test.go:46,63,214,226` | `ln.Close()`、`conn.Close()`、`SetReadDeadline` ×2 |
| `internal/pluginhost/caddy.go:366,368` | `defer os.Remove()`、`tmp.Close()` |
| `internal/pluginhost/watcher.go:33` | `defer watcher.Close()` |

**staticcheck ×3**：
- `internal/adapters/esxi/provider.go:210` 与 `:214` — QF1008 ×2（`c.Client.RoundTripper` 可简写为 `c.RoundTripper`）
- `internal/api/middleware/auth_test.go:144` — SA4006 ×1（`w` 赋值后未使用）

**unused ×3**：
- `internal/adapters/workstation/provider.go:380` — `ensureDialOK`
- `internal/api/v1/guac_tunnel_test.go:212` — `expectInstruction`
- `internal/domain/services/vm_service_test.go:285` — `(*stubProvider).setStatus`

### 修复方式（随本次提交入库）

- errcheck：`defer x.Close()` → `defer func() { _ = x.Close() }()`、裸调用加 `_ =`、`FromDomain` 错误向上传播（新增中文上下文包装）
- staticcheck：QF1008 简写嵌入字段；SA4006 改 `r, _ :=` + 局部 `w :=`
- unused：删除 3 个无调用者的函数
- 附带：`go mod tidy` 清除误入主模块的 golangci-lint 整棵依赖树（go.mod 273 行 → 72 行）；`.gitignore` 增加 `.zcode/`

### 验证

- 本机 golangci-lint v2.12.2（与 CI 同版本）：`0 issues`
- `go build ./...` 通过；`go test ./... -count=1` 全绿
- 注：`httptest.Server.Close()` 不返回 error，非 errcheck 目标，测试中保留裸 `defer srv.Close()` 不构成违规