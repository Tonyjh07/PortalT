<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { VM } from '~/types'

const props = defineProps<{ vm: VM }>()
const emit = defineEmits<{ changed: []; open: []; close: [] }>()

const { user } = useAuth()
const { api } = useApi()

const visible = ref(false)
const saving = ref(false)

const form = reactive({
  protocol: 'vnc',
  hostname: '',
  port: '',
  username: '',
  password: '',
  rdId: '',
  rdPassword: '',
  rdServer: '',
  rdKey: '',
})

const PROTOCOLS = [
  { value: 'vnc', label: 'VNC', defaultPort: 5900 },
  { value: 'rdp', label: 'RDP', defaultPort: 3389 },
  { value: 'ssh', label: 'SSH', defaultPort: 22 },
]

function defaultPort(protocol: string): string {
  return String(PROTOCOLS.find((p) => p.value === protocol)?.defaultPort || '')
}

function open() {
  const md = props.vm.metadata || {}
  form.protocol = String(md['guac.protocol'] || 'vnc')
  form.hostname = String(md['guac.hostname'] || props.vm.ip_address || '')
  form.port = String(md['guac.port'] || defaultPort(form.protocol))
  form.username = String(md['guac.username'] || '')
  form.password = String(md['guac.password'] || '')
  form.rdId = String(md['rustdesk.id'] || '')
  // 密码经 API 脱敏不回显（只写不回），打开时保持为空
  form.rdPassword = ''
  form.rdServer = String(md['rustdesk.server'] || '')
  form.rdKey = String(md['rustdesk.key'] || '')
  visible.value = true
}

watch(visible, (v) => {
  // 通知页面暂停/恢复远程桌面全局键盘监听，否则弹窗内无法输入
  emit(v ? 'open' : 'close')
})

function onProtocolChange(v: string) {
  // 端口为空或仍为旧协议默认值时跟随切换
  if (!form.port || PROTOCOLS.some((p) => String(p.defaultPort) === form.port)) {
    form.port = defaultPort(v)
  }
}

async function save() {
  saving.value = true
  try {
    const patch: Record<string, unknown> = {
      'guac.protocol': form.protocol,
      'guac.hostname': form.hostname.trim() || null,
      'guac.port': form.port.trim() || null,
      'guac.username': form.username.trim() || null,
      'guac.password': form.password || null,
      'rustdesk.id': form.rdId.trim() || null,
      'rustdesk.password': form.rdPassword || null,
      'rustdesk.server': form.rdServer.trim() || null,
      'rustdesk.key': form.rdKey.trim() || null,
    }
    await api<VM>(`/vms/${props.vm.id}/metadata`, { method: 'PUT', body: patch })
    ElMessage.success('远程访问配置已保存')
    visible.value = false
    emit('changed')
  } catch (err) {
    const message = (err as { data?: { message?: string } })?.data?.message || '保存失败'
    ElMessage.error(message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <el-button
    v-if="can(user, 'vm:manage')"
    size="small"
    plain
    :title="'配置远程访问参数（Guacamole / RustDesk）'"
    @click="open"
  >
    <IconRenderer icon="mdi:settings-outline" /> 配置
  </el-button>

  <el-dialog v-model="visible" title="远程访问配置" width="480px" destroy-on-close>
    <el-form label-width="90px" @submit.prevent>
      <el-divider content-position="left">Guacamole 内嵌远程桌面</el-divider>
      <el-form-item label="协议">
        <el-select v-model="form.protocol" class="w-full" @change="onProtocolChange">
          <el-option v-for="p in PROTOCOLS" :key="p.value" :label="p.label" :value="p.value" />
        </el-select>
      </el-form-item>
      <el-form-item label="目标主机">
        <el-input v-model="form.hostname" placeholder="IP 或域名，留空使用虚拟机 IP" />
      </el-form-item>
      <el-form-item label="端口">
        <el-input v-model="form.port" placeholder="留空使用协议默认端口" />
      </el-form-item>
      <el-form-item label="用户名">
        <el-input v-model="form.username" placeholder="RDP: 如 Administrator" autocomplete="off" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input v-model="form.password" type="password" show-password placeholder="RDP/VNC 登录密码" autocomplete="new-password" />
      </el-form-item>

      <el-divider content-position="left">RustDesk 一键连接</el-divider>
      <el-form-item label="设备 ID">
        <el-input v-model="form.rdId" placeholder="目标机 RustDesk 客户端显示的 ID" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input v-model="form.rdPassword" type="password" show-password placeholder="连接密码（不回显，留空则客户端输入）" autocomplete="new-password" />
      </el-form-item>
      <el-form-item label="服务器">
        <el-input v-model="form.rdServer" placeholder="自建 hbbs 地址，如 rd.example.org:21116（留空用官方服务器）" />
      </el-form-item>
      <el-form-item label="公钥">
        <el-input v-model="form.rdKey" placeholder="自建服务器公钥（启用强制校验时需要）" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存</el-button>
    </template>
  </el-dialog>
</template>
