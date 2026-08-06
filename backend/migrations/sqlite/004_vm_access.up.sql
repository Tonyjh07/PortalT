-- PortalT 004_vm_access: 虚拟机资源级授权（SQLite 方言）
-- 记录语义：user_id 被授权访问 vm_id；管理员（vm:manage）不受此表限制。

CREATE TABLE IF NOT EXISTS vm_access (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    vm_id       TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_vm_access_user_vm UNIQUE (user_id, vm_id)
);

CREATE INDEX IF NOT EXISTS idx_vm_access_user ON vm_access (user_id);
