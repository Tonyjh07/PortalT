-- PortalT 006_plugin_refactor 回滚：重建重构前结构（iframe/proxy/native 模型）。
-- 注意：重建后 access/native 记录将丢失（破坏性迁移，无法保留数据）。

DROP TABLE IF EXISTS plugins;

CREATE TABLE plugins (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    icon        TEXT NOT NULL DEFAULT '',
    route       TEXT NOT NULL UNIQUE,
    iframe_url  TEXT NOT NULL DEFAULT '',
    permission  TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE plugins ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'iframe';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS api_url TEXT NOT NULL DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS endpoints TEXT NOT NULL DEFAULT '[]';
