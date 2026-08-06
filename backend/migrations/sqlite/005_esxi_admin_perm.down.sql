-- 回滚：esxi-admin:use 恢复为旧默认 plugin:view（仅本次迁移改过的行）。
UPDATE plugins SET permission = 'plugin:view'
WHERE id = 'esxi-admin' AND permission = 'esxi-admin:use';
