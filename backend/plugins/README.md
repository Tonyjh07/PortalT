# PortalT 官方插件

本目录存放**官方原生插件**源码（native 类型），每个插件为独立 git 仓库的
submodule，目录名 = 插件 ID，构建产物投放运行时插件目录 `PLUGINS_DIR`
（部署机默认 `<app>/plugins`，见 `docs/plugins.md`）。

```
backend/plugins/
├── <id>/            # 插件源码（git submodule，指向独立仓库）
│   ├── cmd/...      # 插件可执行文件入口（监听 gRPC 控制面 + HTTP 数据面）
│   ├── manifest.json
│   └── static/      # 插件静态前端（可选，挂载 /native/<id>/）
└── README.md
```

## 登记新官方插件

1. 官方插件仓库就绪后执行：
   ```bash
   git submodule add <url> backend/plugins/<id>
   ```
   `.gitmodules` 自动登记条目。
2. 无网络 / 仓库未就绪时，先用本地目录占位（含 `manifest.json` 模板），
   待就绪后替换为 submodule。

## 与用户插件的区别

- **官方插件**：源码以 submodule 引入本仓库，`deploy/install.sh` 构建产物
  投放 `PLUGINS_DIR`。
- **用户插件**：本地自行维护源码与产物，直接投放预编译产物到 `PLUGINS_DIR`
  即可（任意语言），不经过本目录。
