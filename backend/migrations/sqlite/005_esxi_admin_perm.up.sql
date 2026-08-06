-- 升级 esxi-admin 插件为专属权限 esxi-admin:use
-- 背景与语义同 postgres 方言（005_esxi_admin_perm.up.sql）。
UPDATE plugins SET permission = 'esxi-admin:use'
WHERE id = 'esxi-admin' AND permission = 'plugin:view';
