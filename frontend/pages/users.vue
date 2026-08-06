<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import type { Role, RoleDefinition, User, VM } from '~/types'

definePageMeta({ middleware: 'auth' })

const { api } = useApi()
const { user: currentUser } = useAuth()

const loading = ref(false)
const users = ref<User[]>([])
const roles = ref<RoleDefinition[]>([])
const dialogVisible = ref(false)
const editing = ref<User | null>(null)
const form = reactive({ username: '', password: '', email: '', role: 'user' as Role })

const roleLabels = computed<Record<string, string>>(() => {
  const labels: Record<string, string> = { admin: '管理员', user: '普通用户', viewer: '访客' }
  for (const r of roles.value) {
    if (!labels[r.id]) labels[r.id] = r.name
  }
  return labels
})

async function load() {
  loading.value = true
  try {
    const [us, rs] = await Promise.all([api<User[]>('/users'), api<RoleDefinition[]>('/roles')])
    users.value = us
    roles.value = rs
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, { username: '', password: '', email: '', role: 'user' })
  dialogVisible.value = true
}

function openEdit(row: User) {
  editing.value = row
  Object.assign(form, { username: row.username, password: '', email: row.email, role: row.role })
  dialogVisible.value = true
}

async function save() {
  const payload: Record<string, unknown> = { email: form.email, role: form.role }
  if (!editing.value) {
    payload.username = form.username
    payload.password = form.password
  } else if (form.password) {
    payload.password = form.password
  }
  try {
    if (!editing.value) {
      await api('/users', { method: 'POST', body: payload })
    } else {
      await api(`/users/${editing.value.id}`, { method: 'PUT', body: payload })
    }
    ElMessage.success(editing.value ? '用户已更新' : '用户已创建')
  } catch (err) {
    ElMessage.error((err as { data?: { message?: string } })?.data?.message || '保存失败')
    return
  }
  dialogVisible.value = false
  await load()
}

async function remove(row: User) {
  try {
    await ElMessageBox.confirm(`确定删除用户「${row.username}」？`, '删除确认', { type: 'warning' })
    await api(`/users/${row.id}`, { method: 'DELETE' })
    await load()
  } catch {
    /* 取消或失败 */
  }
}

// ---- 虚拟机资源授权分配 ----
const accessDialogVisible = ref(false)
const accessUser = ref<User | null>(null)
const accessVms = ref<VM[]>([])
const accessLoading = ref(false)
const accessSelected = ref<string[]>([])
const accessSaving = ref(false)

async function openAccess(row: User) {
  accessUser.value = row
  accessDialogVisible.value = true
  accessLoading.value = true
  accessSelected.value = []
  try {
    const [vms, granted] = await Promise.all([
      api<VM[]>('/vms'),
      api<{ vm_ids: string[] }>(`/users/${row.id}/vm-access`),
    ])
    accessVms.value = vms
    accessSelected.value = granted.vm_ids
  } catch (err) {
    ElMessage.error((err as { data?: { message?: string } })?.data?.message || '查询虚拟机授权失败')
  } finally {
    accessLoading.value = false
  }
}

async function saveAccess() {
  if (!accessUser.value) return
  accessSaving.value = true
  try {
    await api(`/users/${accessUser.value.id}/vm-access`, {
      method: 'PUT',
      body: { vm_ids: accessSelected.value },
    })
    ElMessage.success('虚拟机授权已更新')
    accessDialogVisible.value = false
  } catch (err) {
    ElMessage.error((err as { data?: { message?: string } })?.data?.message || '更新授权失败')
  } finally {
    accessSaving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page-pad">
    <div class="page-head">
      <div>
        <h2>用户管理</h2>
        <p class="page-sub">管理门户账号、角色分配与虚拟机资源授权（user:manage）</p>
      </div>
      <el-button type="primary" @click="openCreate">新建用户</el-button>
    </div>

    <el-card shadow="never">
      <el-table :data="users" v-loading="loading" stripe>
        <el-table-column prop="username" label="用户名" min-width="140" />
        <el-table-column prop="email" label="邮箱" min-width="180" />
        <el-table-column label="角色" width="130">
          <template #default="{ row }">
            <el-tag :type="row.role === 'admin' ? 'danger' : row.role === 'user' ? 'primary' : 'info'">
              {{ roleLabels[row.role] || row.role }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="primary" @click="openAccess(row)">资源授权</el-button>
            <el-button link type="danger" :disabled="row.id === currentUser?.id" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editing ? `编辑用户：${editing.username}` : '新建用户'" width="460px">
      <el-form label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="form.username" :disabled="!!editing" placeholder="登录名" />
        </el-form-item>
        <el-form-item :label="editing ? '新密码' : '密码'">
          <el-input v-model="form.password" type="password" show-password :placeholder="editing ? '留空则不修改' : '初始密码'" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="form.email" placeholder="可选" />
        </el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role" style="width: 100%">
            <el-option v-for="(label, key) in roleLabels" :key="key" :label="label" :value="key" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="accessDialogVisible"
      :title="`虚拟机资源授权：${accessUser?.username ?? ''}`"
      width="520px"
      destroy-on-close
    >
      <p class="page-sub" style="margin-bottom: 12px">
        勾选该用户可见/可操作的虚拟机；持有 vm:manage 权限的用户不受此限制（全量可见）。
      </p>
      <el-checkbox-group v-model="accessSelected" v-loading="accessLoading" class="access-list">
        <el-checkbox v-for="vm in accessVms" :key="vm.id" :label="vm.id">
          {{ vm.name }}
          <span class="perm-desc">{{ vm.status }}</span>
        </el-checkbox>
      </el-checkbox-group>
      <el-empty v-if="!accessLoading && accessVms.length === 0" description="暂无虚拟机" :image-size="60" />
      <template #footer>
        <el-button @click="accessDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="accessSaving" @click="saveAccess">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.access-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 320px;
  overflow-y: auto;
}

.perm-desc {
  margin-left: 6px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}
</style>
