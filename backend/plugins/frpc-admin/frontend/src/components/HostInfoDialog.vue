<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  type Connection,
  type ProbeResult,
  probe,
  saveConnection,
} from '../api'

const props = defineProps<{
  modelValue: boolean
  vmId: string
  vmName?: string
  existing?: Connection
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved', conn: Connection): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})

const form = reactive<Partial<Connection>>({
  host: '',
  port: 22,
  user: '',
  password: '',
  sudo_enabled: false,
  sudo_password: '',
  config_path: '',
  format: 'auto',
  restart_cmd: '',
})

const saving = ref(false)
const probing = ref(false)
const probeResult = ref<ProbeResult | null>(null)

watch(visible, (v) => {
  if (v) {
    const e = props.existing
    form.host = e?.host || ''
    form.port = e?.port || 22
    form.user = e?.user || ''
    form.password = ''
    form.sudo_enabled = e?.sudo_enabled ?? false
    form.sudo_password = ''
    form.config_path = e?.config_path || ''
    form.format = e?.format || 'auto'
    form.restart_cmd = e?.restart_cmd || ''
    probeResult.value = null
  }
})

function validate(): string | null {
  if (!form.host) return 'SSH 主机不能为空'
  if (!form.user) return 'SSH 用户名不能为空'
  if (!form.port) return 'SSH 端口不能为空'
  return null
}

async function onProbe() {
  const err = validate()
  if (err) {
    ElMessage.warning(err)
    return
  }
  // 探测基于当前表单值：先保存为草稿再探测（密码为空时后端沿用旧值，不会清空）
  probing.value = true
  probeResult.value = null
  try {
    const conn = await saveConnection(props.vmId, { ...form, vm_id: props.vmId, vm_name: props.vmName })
    emit('saved', conn)
    const res = await probe(props.vmId)
    probeResult.value = res
    if (res.config_path && !form.config_path) form.config_path = res.config_path
    if (res.format_hint && res.format_hint !== '未知' && form.format === 'auto') {
      form.format = res.format_hint
    }
    ElMessage.success('检测完成')
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    probing.value = false
  }
}

async function onSave() {
  const err = validate()
  if (err) {
    ElMessage.warning(err)
    return
  }
  saving.value = true
  try {
    const conn = await saveConnection(props.vmId, { ...form, vm_id: props.vmId, vm_name: props.vmName })
    emit('saved', conn)
    visible.value = false
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" :title="`主机信息 - ${vmName || vmId}`" width="640px" append-to-body>
    <el-form label-width="120px">
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="SSH 主机" required>
            <el-input v-model="form.host" placeholder="192.168.1.10" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="端口" required>
            <el-input-number v-model="form.port" :min="1" :max="65535" :controls="false" style="width: 100%" />
          </el-form-item>
        </el-col>
      </el-row>

      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="用户名" required>
            <el-input v-model="form.user" placeholder="root" />
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="密码">
            <el-input v-model="form.password" type="password" show-password placeholder="登录密码" />
          </el-form-item>
        </el-col>
      </el-row>

      <el-form-item label="sudo">
        <el-switch v-model="form.sudo_enabled" active-text="写文件/重启经 sudo" />
      </el-form-item>
      <el-form-item v-if="form.sudo_enabled" label="sudo 密码">
        <el-input
          v-model="form.sudo_password"
          type="password"
          show-password
          placeholder="留空则使用登录密码"
        />
      </el-form-item>

      <el-form-item label="frpc 配置路径">
        <el-input
          v-model="form.config_path"
          placeholder="如 /etc/frp/frpc.toml（留空自动探测）"
        />
      </el-form-item>
      <el-row :gutter="12">
        <el-col :span="12">
          <el-form-item label="格式">
            <el-select v-model="form.format" style="width: 100%">
              <el-option label="自动检测" value="auto" />
              <el-option label="INI（旧版）" value="ini" />
              <el-option label="TOML（新版）" value="toml" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="重启命令">
            <el-input v-model="form.restart_cmd" placeholder="默认 systemctl restart frpc" />
          </el-form-item>
        </el-col>
      </el-row>
    </el-form>

    <el-alert
      v-if="probeResult"
      type="info"
      :closable="false"
      show-icon
      class="probe-alert"
    >
      <div class="probe-line">
        <span class="probe-k">frp 版本:</span>
        <code>{{ probeResult.version || '未知' }}</code>
      </div>
      <div class="probe-line">
        <span class="probe-k">配置路径:</span>
        <code>{{ probeResult.config_path || '未知（请手动填写）' }}</code>
      </div>
      <div class="probe-line">
        <span class="probe-k">建议格式:</span>
        <code>{{ probeResult.format_hint || '未知' }}</code>
      </div>
      <div v-if="probeResult.raw" class="probe-raw">{{ probeResult.raw }}</div>
      <div class="probe-note">检测到的路径/格式已填入表单，点击「保存」后生效。</div>
    </el-alert>

    <template #footer>
      <el-button :loading="probing" @click="onProbe">检测配置</el-button>
      <el-button type="primary" :loading="saving" @click="onSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.probe-alert {
  margin-bottom: 4px;
}

.probe-line {
  font-size: 13px;
  line-height: 1.9;
}

.probe-k {
  color: var(--el-text-color-secondary);
  margin-right: 6px;
}

.probe-raw {
  margin-top: 6px;
  padding: 6px 10px;
  background: var(--el-fill-color);
  border-radius: 4px;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: var(--el-font-family-mono);
  max-height: 120px;
  overflow: auto;
}

.probe-note {
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
