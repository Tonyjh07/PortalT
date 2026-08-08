-- PortalT 006_plugin_refactor: 插件系统重构（破坏性）
-- plugins 表重建为两类（access | native）：
--   type         access（iframe 嵌入 + API 白名单 + Caddy 规则，可共存）| native（独立进程）
--   新增列       caddy_rules（Caddy 规则片段）、status（native 运行态）、manifest_json（native manifest 缓存）
-- esxi-admin 等默认 access 记录由启动引导 seedDefaultAccessPlugins（cmd/server/main.go）幂等创建（含 Caddy 规则默认值）。

DROP TABLE IF EXISTS plugins;

CREATE TABLE plugins (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    icon          TEXT NOT NULL DEFAULT '',
    route         TEXT NOT NULL UNIQUE,
    type          TEXT NOT NULL DEFAULT 'access',
    iframe_url    TEXT NOT NULL DEFAULT '',
    api_url       TEXT NOT NULL DEFAULT '',
    endpoints     TEXT NOT NULL DEFAULT '[]',
    caddy_rules   TEXT NOT NULL DEFAULT '',
    permission    TEXT NOT NULL DEFAULT '',
    sort_order    INTEGER NOT NULL DEFAULT 0,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    status        TEXT NOT NULL DEFAULT '',
    manifest_json TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
