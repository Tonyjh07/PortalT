<script setup lang="ts">
import { ElMessage } from 'element-plus'
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
    ElMessage.success('角色已保存')
    role.name = e.name
    role.description = e.description
    role.permissions = [...e.permissions]
  } catch (err) {
    ElMessage.error((err as { data?: { message?: string } })?.data?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function remove(role: RoleDefinition) {
  await ElMessageBox.confirm(`确定删除角色「${role.name}」？`, '删除确认', { type: 'warning' })
  await api(`/roles/${role.id}`, { method: 'DELETE' })
  await load()
}

// ---- 新建角色 ----
const createVisible = ref(false)
const creating = ref(false)
const createForm = reactive({ id: '', name: '', description: '', permissions: [] as string[] })

function openCreate() {
  Object.assign(createForm, { id: '', name: '', description: '', permissions: [] })
  createVisible.value = true
}

async function createRole() {
  creating.value = true
  try {
    await api('/roles', { method: 'POST', body: createForm })
    ElMessage.success('角色已创建')
    createVisible.value = false
    await load()
  } catch (err) {
    ElMessage.error((err as { data?: { message?: string } })?.data?.message || '创建失败')
  } finally {
    creating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page-pad">
    <div class="page-head">
      <div>
        <h2>角色权限</h2>
        <p class="page-sub">配置各角色可用的权限矩阵（保存后立即生效）；插件声明的权限在 API 层强制校验</p>
      </div>
      <el-button type="primary" @click="openCreate">新建角色</el-button>
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

    <el-dialog v-model="createVisible" title="新建角色" width="480px">
      <el-form label-width="80px">
        <el-form-item label="角色 ID">
          <el-input v-model="createForm.id" placeholder="小写字母/数字/下划线/连字符（如 ops）" />
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="createForm.name" placeholder="如：运维" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="createForm.description" placeholder="可选" />
        </el-form-item>
        <el-form-item label="权限">
          <div class="perm-list create-perms">
            <el-checkbox
              v-for="p in permissions"
              :key="p.id"
              v-model="createForm.permissions"
              :label="p.id"
            >
              {{ p.id }}
              <span class="perm-desc">{{ p.description }}</span>
            </el-checkbox>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="createRole">创建</el-button>
      </template>
    </el-dialog>
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

.create-perms {
  min-height: 120px;
  max-height: 260px;
  overflow-y: auto;
}

.perm-desc {
  margin-left: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
