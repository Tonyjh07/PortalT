<script setup lang="ts">
import type { Role, User } from '~/types'

definePageMeta({ middleware: 'auth' })

const { api } = useApi()
const { user: currentUser } = useAuth()

const loading = ref(false)
const users = ref<User[]>([])
const dialogVisible = ref(false)
const editing = ref<User | null>(null)
const form = reactive({ username: '', password: '', email: '', role: 'user' as Role })

const roleLabels: Record<Role, string> = { admin: '管理员', user: '普通用户', viewer: '访客' }

async function load() {
  loading.value = true
  try {
    users.value = await api<User[]>('/users')
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
  if (!editing.value) {
    await api('/users', { method: 'POST', body: payload })
  } else {
    await api(`/users/${editing.value.id}`, { method: 'PUT', body: payload })
  }
  dialogVisible.value = false
  await load()
}

async function remove(row: User) {
  await ElMessageBox.confirm(`确定删除用户「${row.username}」？`, '删除确认', { type: 'warning' })
  await api(`/users/${row.id}`, { method: 'DELETE' })
  await load()
}

onMounted(load)
</script>

<template>
  <div class="page-pad">
    <div class="page-head">
      <div>
        <h2>用户管理</h2>
        <p class="page-sub">管理门户账号与角色分配（user:manage）</p>
      </div>
      <el-button type="primary" @click="openCreate">新建用户</el-button>
    </div>

    <el-card shadow="never">
      <el-table :data="users" v-loading="loading" stripe>
        <el-table-column prop="username" label="用户名" min-width="140" />
        <el-table-column prop="email" label="邮箱" min-width="180" />
        <el-table-column label="角色" width="120">
          <template #default="{ row }">
            <el-tag :type="row.role === 'admin' ? 'danger' : row.role === 'user' ? 'primary' : 'info'">
              {{ roleLabels[row.role as Role] }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
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
  </div>
</template>
