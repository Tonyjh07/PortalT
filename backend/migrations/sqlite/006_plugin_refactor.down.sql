-- PortalT 006_plugin_refactor 回滚（SQLite 方言）：重建重构前结构。
DROP TABLE IF EXISTS plugins;

CREATE TABLE plugins (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    icon        TEXT NOT NULL DEFAULT '',
    route       TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL DEFAULT 'iframe',
    iframe_url  TEXT NOT NULL DEFAULT '',
    api_url     TEXT NOT NULL DEFAULT '',
    endpoints   TEXT NOT NULL DEFAULT '[]',
    permission  TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    is_active   INTEGER NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
