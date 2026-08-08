# PortalT 项目文档

本目录存放项目开发规范与接口文档，随实现进度持续更新。

> 总体需求与技术架构见仓库根目录 `About.md`（本文档不重复）。

| 文档 | 内容 |
|------|------|
| [conventions.md](./conventions.md) | 开发规范：工具链、目录、Makefile、代码风格、测试、配置 |
| [how-to-use.md](./how-to-use.md) | 构建与部署指南（Debian + ESXi）：§3 一键脚本部署（`deploy/install.sh` + `update.sh`）→ 手工构建 → systemd 部署 → 接入 ESXi → Caddy 反代 → 远程桌面 → 验证/排错 |
| [interfaces.md](./interfaces.md) | 接口文档：HTTP 接口、错误码、领域模型 JSON 契约 |
| [plugins.md](./plugins.md) | 插件开发指南：access / native 两型、access 与 Caddy 交互、manifest 契约 |
| [remote-desktop.md](./remote-desktop.md) | 远程访问指南（Phase 8）：架构、guac.*/rustdesk.* 连接参数、质量/流畅度模式、使用步骤、FAQ |
| [external-access.md](./external-access.md) | 外部访问指南（Cloudflare Tunnel）：隧道配置、allowedHosts、WS 透传、验证方法 |

## 文档更新约定

- 仅记录**已实现**的内容；未实现的部分标注"规划中"或直接省略
- 每个 Phase 完成时同步更新对应文档
- 接口变更必须同步修改本目录文档
