-- PortalT 003_plugin_types: 插件类型扩展（SQLite 方言）

ALTER TABLE plugins ADD COLUMN type TEXT NOT NULL DEFAULT 'iframe';
ALTER TABLE plugins ADD COLUMN api_url TEXT NOT NULL DEFAULT '';
ALTER TABLE plugins ADD COLUMN endpoints TEXT NOT NULL DEFAULT '[]';
