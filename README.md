# PortalT

HomeLab 统一门户系统：在虚拟机中运行的 Web 门户，统一管理 ESXi 上的虚拟机，提供浏览器远程桌面（Guacamole）与可扩展的插件系统。

> 完整需求与技术架构见 [About.md](./About.md)，开发规范与接口文档见 [docs/](./docs/README.md)。

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go + Gin + GORM |
| 前端 | Nuxt 3 + Vue 3 + Element Plus |
| 数据库 | PostgreSQL 15 |
| Web 服务器 | Caddy 2 |
| 远程桌面 | Apache Guacamole |
| 虚拟化 | govmomi (ESXi) |

## 快速开始

```bash
make version   # 检查工具链 (Go 1.21+, Node 20+, Docker 24+)
make init      # 初始化目录结构与依赖
make run       # 启动后端服务 (http://localhost:8080)
make test      # 运行测试
```

## 项目结构

```
backend/    Go 后端（domain → ports → adapters 分层）
frontend/   Nuxt 3 前端
caddy/      Caddy 反向代理配置
docs/       开发规范与接口文档
```

## 开发进度

- ✅ Phase 0-2：基础设施、领域模型、仓储接口与内存实现
- ⬜ Phase 3+：PostgreSQL 适配器、ESXi 驱动、认证、API、前端等（见 About.md 进度表）
