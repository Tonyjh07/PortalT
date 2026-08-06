-- 升级 esxi-admin 插件为专属权限 esxi-admin:use
-- 背景：Phase 9 首次 seed 时 esxi-admin 声明 plugin:view（非空），
-- SyncNativePlugins 遵循"管理员调整优先"不回填新声明；此处一次性迁移
-- 覆盖"仍为旧默认值 plugin:view"的记录（管理员手动改过的其他值不受影响）。
UPDATE plugins SET permission = 'esxi-admin:use'
WHERE id = 'esxi-admin' AND permission = 'plugin:view';
