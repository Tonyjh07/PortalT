<script setup lang="ts">
import type { Plugin, PluginEndpoint, PluginType } from '~/types'
import { can } from '~/utils/permissions'

definePageMeta({ middleware: 'auth' })

const { api } = useApi()
const { user, hasPerm } = useAuth()

const loading = ref(false)
const plugins = ref<Plugin[]>([])
const dialogVisible = ref(false)
const editing = ref<Plugin | null>(null)

const typeLabels: Record<PluginType, string> = {
  iframe: '嵌入页面',
  proxy: '标准 API 代理',
  native: '原生插件',
}

const form = reactive({
  id: '',
  name: '',
  icon: 'mdi:puzzle',
  route: '',
  type: 'iframe' as PluginType,
  iframe_url: '',
  api_url: '',
  endpoints: [] as PluginEndpoint[],
  permission: '',
  sort_order: 0,
  is_active: true,
})

const canManage = computed(() => {
  const u = user.value
  return u?.role === 'admin' || hasPerm('plugin:manage')
})

function emptyEndpoint(): PluginEndpoint {
  return { method: 'GET', path: '/api/info', name: '', description: '' }
}

async function load() {
  loading.value = true
  try {
    plugins.value = await api<Plugin[]>('/plugins')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, {
    id: '', name: '', icon: 'mdi:puzzle', route: '', type: 'iframe',
    iframe_url: '', api_url: '', endpoints: [emptyEndpoint()],
    permission: '', sort_order: 0, is_active: true,
  })
  dialogVisible.value = true
}

function openEdit(row: Plugin) {
  editing.value = row
  Object.assign(form, {
    id: row.id,
    name: row.name,
    icon: row.icon,
    route: row.route,
    type: row.type ?? 'iframe',
    iframe_url: row.iframe_url ?? '',
    api_url: row.api_url ?? '',
    endpoints: (row.endpoints?.length ? row.endpoints : [emptyEndpoint()]).map((e) => ({ ...e })),
    permission: row.permission ?? '',
    sort_order: row.sort_order ?? 0,
    is_active: row.is_active,
  })
  dialogVisible.value = true
}

function validateForm(): string | null {
  if (!form.id.trim() || !form.name.trim()) return 'ID 与名称必填'
  if (form.type === 'proxy') {
    if (!/^https?:\/\//.test(form.api_url.trim())) return '代理插件必须填写 http(s) API 地址'
    for (const ep of form.endpoints) {
      if (!ep.path.startsWith('/')) return '端点路径须以 / 开头（如 /api/info）'
    }
  }
  if (form.type === 'iframe' && !/^https?:\/\//.test(form.iframe_url.trim())) {
    return '嵌入插件必须填写 http(s) 页面地址'
  }
  return null
}

async function save() {
  const err = validateForm()
  if (err) {
    ElMessage.warning(err)
    return
  }
  const payload = {
    id: form.id,
    name: form.name,
    icon: form.icon,
    route: form.route.startsWith('/') ? form.route : '/' + form.route,
    type: form.type,
    iframe_url: form.iframe_url.trim(),
    api_url: form.api_url.trim(),
    endpoints: form.endpoints.filter((e) => e.path.trim()),
    permission: form.permission,
    sort_order: form.sort_order,
    is_active: form.is_active,
  }
  if (!editing.value) {
    await api('/plugins', { method: 'POST', body: payload })
  } else {
    await api(`/plugins/${editing.value.id}`, { method: 'PUT', body: payload })
  }
  dialogVisible.value = false
  await load()
}

async function remove(row: Plugin) {
  await ElMessageBox.confirm(`确定删除插件「${row.name}」？`, '删除确认', { type: 'warning' })
  await api(`/plugins/${row.id}`, { method: 'DELETE' })
  await load()
}

function addEndpoint() {
  form.endpoints.push(emptyEndpoint())
}

function removeEndpoint(i: number) {
  form.endpoints.splice(i, 1)
}

onMounted(load)
</script>

<template>
  <div class="page-pad">
    <div class="page-head">
      <div>
        <h2>插件管理</h2>
        <p class="page-sub">动态菜单与脚本/原生插件配置（plugin:manage）</p>
      </div>
      <el-button v-if="canManage" type="primary" @click="openCreate">新建插件</el-button>
    </div>

    <el-card shadow="never">
      <el-table :data="plugins" v-loading="loading" stripe>
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="id" label="ID" min-width="120" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <el-tag :type="row.type === 'native' ? 'success' : row.type === 'proxy' ? 'primary' : 'info'">
              {{ typeLabels[row.type as PluginType] ?? row.type }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="route" label="路由" min-width="120" />
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
              {{ row.is_active ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="sort_order" label="排序" width="80" />
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" :disabled="row.type === 'native'" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editing ? `编辑插件：${editing.name}` : '新建插件'" width="640px" top="6vh">
      <el-form label-width="90px">
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="ID">
              <el-input v-model="form.id" :disabled="!!editing" placeholder="唯一标识，如 ha" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="名称">
              <el-input v-model="form.name" placeholder="显示名称" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="图标">
              <el-input v-model="form.icon" placeholder="mdi:home" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="路由">
              <el-input v-model="form.route" placeholder="/ha" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type" :disabled="editing?.type === 'native'">
            <el-radio-button value="iframe">嵌入页面</el-radio-button>
            <el-radio-button value="proxy">标准 API 代理</el-radio-button>
            <el-radio-button value="native" :disabled="true">原生插件</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.type === 'iframe'" label="页面地址">
          <el-input v-model="form.iframe_url" placeholder="https://ha.local" />
        </el-form-item>
        <template v-if="form.type === 'proxy'">
          <el-form-item label="API 地址">
            <el-input v-model="form.api_url" placeholder="http://127.0.0.1:8701" />
          </el-form-item>
          <el-form-item label="API 端点">
            <div class="endpoints-editor">
              <div v-for="(ep, i) in form.endpoints" :key="i" class="endpoint-row">
                <el-select v-model="ep.method" style="width: 96px">
                  <el-option value="GET" label="GET" />
                  <el-option value="POST" label="POST" />
                  <el-option value="PUT" label="PUT" />
                  <el-option value="DELETE" label="DELETE" />
                </el-select>
                <el-input v-model="ep.path" placeholder="/api/info" class="ep-path" />
                <el-input v-model="ep.name" placeholder="端点名称（可选）" class="ep-name" />
                <el-button link type="danger" @click="removeEndpoint(i)">移除</el-button>
              </div>
              <el-button size="small" @click="addEndpoint">+ 添加端点</el-button>
            </div>
          </el-form-item>
        </template>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="权限">
              <el-select v-model="form.permission" clearable style="width: 100%">
                <el-option label="无需额外权限" value="" />
                <el-option label="plugin:view 可访问" value="plugin:view" />
                <el-option label="vm:view 可访问" value="vm:view" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="排序">
              <el-input-number v-model="form.sort_order" :min="0" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="启用">
          <el-switch v-model="form.is_active" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.endpoints-editor {
  width: 100%;
}

.endpoint-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}

.ep-path {
  flex: 1.2;
}

.ep-name {
  flex: 1;
}
</style>
