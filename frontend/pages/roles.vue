<script setup lang="ts">
import type { PermissionInfo, RoleDefinition } from '~/types'

definePageMeta({ middleware: 'auth' })

const { api } = useApi()

const loading = ref(false)
const saving = ref(false)
const roles = ref<RoleDefinition[]>([])
const permissions = ref<PermissionInfo[]>([])
const editors = reactive<Record<string, { name: string; description: string; permissions: string[] }>>({})

const builtin = ['admin', 'user', 'viewer']

async function load() {
  loading.value = true
  try {
    const [rs, ps] = await Promise.all([api<RoleDefinition[]>('/roles'), api<PermissionInfo[]>('/roles/permissions')])
    roles.value = rs
    permissions.value = ps
    for (const r of rs) {
      editors[r.id] = { name: r.name, description: r.description, permissions: [...r.permissions] }
    }
  } finally {
    loading.value = false
  }
}

async function save(role: RoleDefinition) {
  const e = editors[role.id]
  saving.value = true
  try {
    await api(`/roles/${role.id}`, { method: 'PUT', body: e })
    role.name = e.name
    role.description = e.description
    role.permissions = [...e.permissions]
  } finally {
    saving.value = false
  }
}

async function remove(role: RoleDefinition) {
  await ElMessageBox.confirm(`确定删除角色「${role.name}」？`, '删除确认', { type: 'warning' })
  await api(`/roles/${role.id}`, { method: 'DELETE' })
  await load()
}

onMounted(load)
</script>

<template>
  <div class="page-pad">
    <div class="page-head">
      <div>
        <h2>角色权限</h2>
        <p class="page-sub">配置各角色可用的权限矩阵（保存后立即生效）</p>
      </div>
    </div>

    <div v-loading="loading" class="role-grid">
      <el-card v-for="r in roles" :key="r.id" shadow="never" class="role-card">
        <template #header>
          <div class="role-head">
            <div>
              <el-tag :type="r.id === 'admin' ? 'danger' : r.id === 'user' ? 'primary' : 'info'" effect="dark">
                {{ r.id }}
              </el-tag>
              <el-input v-model="editors[r.id]!.name" class="role-name-input" />
            </div>
            <div v-if="!builtin.includes(r.id)" class="role-actions">
              <el-button link type="danger" @click="remove(r)">删除</el-button>
            </div>
          </div>
        </template>
        <el-input v-model="editors[r.id]!.description" placeholder="角色描述" size="small" class="role-desc" />
        <div class="perm-list">
          <el-checkbox
            v-for="p in permissions"
            :key="p.id"
            v-model="editors[r.id]!.permissions"
            :label="p.id"
          >
            {{ p.id }}
            <span class="perm-desc">{{ p.description }}</span>
          </el-checkbox>
        </div>
        <template #footer>
          <el-button type="primary" :loading="saving" @click="save(r)">保存</el-button>
        </template>
      </el-card>
    </div>
  </div>
</template>

<style scoped>
.role-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 16px;
}

.role-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.role-head > div:first-child {
  display: flex;
  align-items: center;
  gap: 8px;
}

.role-name-input {
  width: 160px;
}

.role-desc {
  margin-bottom: 12px;
}

.perm-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-height: 220px;
}

.perm-desc {
  margin-left: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
