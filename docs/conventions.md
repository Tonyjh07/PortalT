# PortalT 开发规范

> 仅记录当前已落地执行的规范（截至 Phase 1）。

## 工具链要求

| 工具 | 版本要求 | 本机实测 |
|------|---------|---------|
| Go | 1.21+ | 1.26.5（winget 安装） |
| Node.js | 20+ | 24.18.0 |
| Docker | 24+ | 28.4.0 |
| GNU Make | 3.81+ | 3.81（winget 安装，需 msys sh.exe） |

### Windows 环境注意事项

- GNU Make 依赖 `sh.exe`（本机使用 msys64），Makefile 按 POSIX 写法
- MSYS 环境下 npm 需用 `npm.cmd`，Makefile 已通过 `uname -s` 自动识别
- Go 模块名：`portalt`（仓库无远端，未用 github 路径）
- **竞态检测（-race）**：需要原生 MinGW gcc。本机已装 WinLibs（winget 包 `BrechtSanders.WinLibs.MCF.UCRT`）。`make test-race` 会自动探测其路径并通过 `CC` 传给 go；不要依赖 msys64 的 cygwin gcc（报 "don't use the cygwin compiler" 错误）。注意 msys64 位于机器 PATH 中，优先级高于用户 PATH 的 WinLibs，因此必须走 Makefile 的 CC 探测

## Go 依赖镜像（GOPROXY）

本网络环境无法访问 `proxy.golang.org`（2026-08-01 下载 testify 时超时）。
已持久化配置，无需手动处理：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

若 npm 安装遇到 registry 不可达，同样需配置镜像（尚未发生）。

## Makefile 命令

| 命令 | 说明 | 当前状态 |
|------|------|---------|
| `make version` | 输出工具链版本 | ✅ 可用 |
| `make init` | 创建目录骨架 + 下载依赖 | ✅ 可用 |
| `make run` | 启动后端（go run） | ✅ 可用 |
| `make test-domain` | domain 层单测（覆盖率） | ✅ 通过（100%） |
| `make test-repo` | 内存仓储测试 | ✅ 通过（100%） |
| `make test-race` | 全部单测 + 竞态检测（需 MinGW） | ✅ 通过 |
| `make test` | 汇总测试（test-domain + test-repo + test-api） | ⏳ 依赖后续阶段 |
| `make build` | 构建后端二进制 `backend/bin/portalt.exe` | ✅ 可用 |
| `make docker-build` | 构建 Docker 镜像 | ⏳ 依赖 Dockerfile |
| `make up/down/logs` | docker compose 便捷命令 | ✅ 可用 |

## 代码规范（Go）

- domain 层**禁止**导入外部框架（Gin/GORM 等），仅标准库
- 每个导出类型/函数必须有注释（中文可接受），包含包注释
- 遵循 `gofmt` 与 `go vet`，提交前必须通过
- 错误处理：返回 `error`，不 panic；仓储层使用 `ports.ErrNotFound` / `ports.ErrInvalidArgument` 哨兵错误
- 测试：使用 `testify` 断言库，公开函数至少一个测试用例
- 并发：内存仓储使用 `sync.RWMutex` + `map`，必须通过 `-race` 检测
- 覆盖率要求：domain 层 > 90%（当前 100%）

## 架构分层约定

- `internal/domain`：领域实体与业务方法，零外部依赖
- `internal/ports`：仓储/虚拟化/认证接口定义 + 错误哨兵，无实现
- `internal/adapters/memory`：内存仓储（测试/开发用），编译期断言实现接口（`var _ ports.VMRepository = (*VMRepository)(nil)`）
- `internal/domain/services`：业务编排（如 `VMService.SyncVMs`），只依赖 ports 接口

## 日志与错误处理约定

- 日志使用 `slog` 结构化日志（后续阶段引入）
- 敏感信息（密码）不得出现在日志与 JSON 输出中（`json:"-"` 标签隐藏）

## 配置文件

- 环境变量示例：根目录 `.env.example`（复制为 `.env` 使用）
- 变量带 `PORTALT_` 前缀自动加载（config 层在 Phase 5+ 引入）
