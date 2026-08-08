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
  access: '访问接入',
  native: '原生插件',
}

const statusLabels: Record<string, string> = {
  running: '运行中',
  stopped: '已停止',
  error: '异常',
  missing: '未安装',
}

const statusTypes: Record<string, 'success' | 'info' | 'danger' | 'warning'> = {
  running: 'success',
  stopped: 'info',
  error: 'danger',
  missing: 'warning',
}

const form = reactive({
  id: '',
  name: '',
  icon: 'mdi:puzzle',
  route: '',
  type: 'access' as PluginType,
  iframe_url: '',
  api_url: '',
  endpoints: [] as PluginEndpoint[],
  caddy_rules: '',
  permission: '',
  sort_order: 0,
  is_active: true,
})

const canManage = computed(() => {
  const u = user.value
  return u?.role === 'admin' || hasPerm('plugin:manage')
})

// native 插件由运行时安装/启停，仅只读展示
const readonlyNative = computed(() => editing.value?.type === 'native')

const manifest = computed<Record<string, any> | null>(() => {
  if (!editing.value?.manifest_json) return null
  try {
    return JSON.parse(editing.value.manifest_json)
  } catch {
    return null
  }
})

const hasNative = computed(() => plugins.value.some((p) => p.type === 'native'))

function caddyStatus(row: Plugin): { text: string; type: string } {
  if (row.type !== 'access') return { text: '-', type: 'info' }
  if (row.caddy_applied) return { text: '已生效', type: 'success' }
  if (row.caddy_rules && !row.is_active) return { text: '停用', type: 'info' }
  if (row.caddy_rules) return { text: '待重载', type: 'warning' }
  return { text: '无规则', type: 'info' }
}

function emptyEndpoint(): PluginEndpoint {
  return { method: 'GET', path: '/api/info', name: '', description: '' }
}

function resetForm() {
  Object.assign(form, {
    id: '', name: '', icon: 'mdi:puzzle', route: '', type: 'access',
    iframe_url: '', api_url: '', endpoints: [emptyEndpoint()],
    caddy_rules: '', permission: '', sort_order: 0, is_active: true,
  })
}

async function load() {
  loading.value = true
  try {
    plugins.value = await api<Plugin[]>('/plugins')
  } finally {
    loading.value = false
  }
}

let pollTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  load()
  pollTimer = setInterval(() => {
    if (hasNative.value) load()
  }, 15000)
})
onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})

async function toggleActive(row: Plugin) {
  const next = !row.is_active
  await api(`/plugins/${row.id}`, { method: 'PUT', body: { is_active: next } })
  ElMessage.success(next ? '已启用' : '已停用')
  await load()
}

async function restartNative(row: Plugin) {
  await api(`/plugins/${row.id}/restart`, { method: 'POST' })
  ElMessage.success('重启指令已发送')
  await load()
}

function openCreate() {
  editing.value = null
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: Plugin) {
  editing.value = row
  resetForm()
  Object.assign(form, {
    id: row.id,
    name: row.name,
    icon: row.icon,
    route: row.route,
    type: row.type,
    iframe_url: row.iframe_url ?? '',
    api_url: row.api_url ?? '',
    endpoints: (row.endpoints?.length ? row.endpoints : [emptyEndpoint()]).map((e) => ({ ...e })),
    caddy_rules: row.caddy_rules ?? '',
    permission: row.permission ?? '',
    sort_order: row.sort_order ?? 0,
    is_active: row.is_active,
  })
  dialogVisible.value = true
}

function validateForm(): string | null {
  if (!form.id.trim() || !form.name.trim()) return 'ID 与名称必填'
  const hasCapability =
    !!form.iframe_url.trim() ||
    (!!form.api_url.trim() && form.endpoints.some((e) => e.path.trim()))
  if (!hasCapability) return '访问接入插件必须配置页面地址，或 API 地址 + 至少一个端点'
  if (form.iframe_url.trim() && !/^(https?:\/\/|\/)/.test(form.iframe_url.trim())) {
    return '页面地址须为 http(s) 或门户内相对路径（如 /esxi/ui/）'
  }
  if (form.api_url.trim() && !/^https?:\/\//.test(form.api_url.trim())) {
    return 'API 地址必须为 http(s)'
  }
  for (const ep of form.endpoints) {
    if (ep.path.trim() && !ep.path.startsWith('/')) return '端点路径须以 / 开头（如 /api/info）'
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
    caddy_rules: form.caddy_rules.trim(),
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

</script>

<template>
  <div class="page-pad">
    <div class="page-head">
      <div>
        <h2>插件管理</h2>
        <p class="page-sub">动态菜单 / 嵌入访问 / 原生插件配置（plugin:manage）</p>
      </div>
      <el-button v-if="canManage" type="primary" @click="openCreate">新建插件</el-button>
    </div>

    <el-card shadow="never">
      <el-table :data="plugins" v-loading="loading" stripe>
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="id" label="ID" min-width="120" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <el-tag :type="row.type === 'native' ? 'success' : 'primary'">
              {{ typeLabels[row.type as PluginType] ?? row.type }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <template v-if="row.type === 'native'">
              <el-tag :type="statusTypes[row.status] ?? 'info'" size="small">
                {{ statusLabels[row.status] ?? row.status }}
              </el-tag>
            </template>
            <span v-else class="text-muted">-</span>
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
        <el-table-column label="Caddy 规则" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.type === 'access'" :type="caddyStatus(row).type" size="small">{{ caddyStatus(row).text }}</el-tag>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="sort_order" label="排序" width="80" />
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <template v-if="row.type === 'native' && canManage">
              <el-button link :type="row.is_active ? 'warning' : 'success'" @click="toggleActive(row)">{{ row.is_active ? '停用' : '启用' }}</el-button>
              <el-button link type="primary" :disabled="!row.is_active" @click="restartNative(row)">重启</el-button>
            </template>
            <template v-else>
              <el-button link type="primary" @click="openEdit(row)">{{ row.type === 'native' ? '查看' : '查看 / 编辑' }}</el-button>
              <el-button link type="danger" :disabled="row.type === 'native'" @click="remove(row)">删除</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? `编辑插件：${editing.name}` : '新建插件'"
      width="660px"
      top="6vh"
    >
      <!-- native 插件：只读展示运行状态与 manifest 信息 -->
      <template v-if="readonlyNative">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="ID">{{ editing!.id }}</el-descriptions-item>
          <el-descriptions-item label="类型">
            <el-tag type="success">原生插件</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="运行状态">
            <el-tag :type="statusTypes[editing!.status] ?? 'info'" size="small">
              {{ statusLabels[editing!.status] ?? editing!.status }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item v-if="manifest?.name" label="manifest 名称">{{ manifest.name }}</el-descriptions-item>
          <el-descriptions-item v-if="manifest?.version" label="版本">{{ manifest.version }}</el-descriptions-item>
          <el-descriptions-item v-if="manifest?.description" label="描述">{{ manifest.description }}</el-descriptions-item>
          <el-descriptions-item label="路由">{{ editing!.route }}</el-descriptions-item>
          <el-descriptions-item label="启用">{{ editing!.is_active ? '是' : '否' }}</el-descriptions-item>
        </el-descriptions>
        <el-alert
          class="native-tip"
          type="info"
          :closable="false"
          show-icon
          title="原生插件由运行时安装与启停，此处仅展示状态信息"
        />
      </template>

      <!-- access 插件：表单编辑 -->
      <el-form v-else label-width="90px">
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
          <el-radio-group v-model="form.type">
            <el-radio-button value="access">访问接入</el-radio-button>
            <el-radio-button value="native" :disabled="true">原生插件（运行时安装）</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="页面地址">
          <el-input v-model="form.iframe_url" placeholder="https://ha.local 或 /esxi/ui/（门户内相对路径）" />
        </el-form-item>
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
        <el-form-item label="Caddy 规则">
          <el-input
            v-model="form.caddy_rules"
            type="textarea"
            :rows="4"
            placeholder="需反代到本插件的 Caddy 站点规则，多行。例如：
route /* {
    reverse_proxy 127.0.0.1:8701
}"
          />
        </el-form-item>
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
        <el-button v-if="!readonlyNative" type="primary" @click="save">保存</el-button>
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

.native-tip {
  margin-top: 16px;
}

.text-muted {
  color: var(--el-text-color-secondary);
}
</style>
