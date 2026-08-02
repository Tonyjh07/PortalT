-- PortalT 003_plugin_types: 插件类型扩展
-- plugins 表新增 type / api_url / endpoints 列：
--   type      iframe（嵌入）| proxy（脚本标准API代理）| native（Go原生插件）
--   api_url   proxy 类型插件的 API 服务地址
--   endpoints proxy 类型插件的端点白名单（JSON 数组）

ALTER TABLE plugins ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'iframe';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS api_url TEXT NOT NULL DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS endpoints TEXT NOT NULL DEFAULT '[]';
