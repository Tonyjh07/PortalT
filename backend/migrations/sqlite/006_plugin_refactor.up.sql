-- PortalT 006_plugin_refactor: 插件系统重构（SQLite 方言，破坏性）
-- 重建 plugins 表为 access | native 两类，新增 caddy_rules / status / manifest_json。

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
    is_active     INTEGER NOT NULL DEFAULT 1,
    status        TEXT NOT NULL DEFAULT '',
    manifest_json TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
