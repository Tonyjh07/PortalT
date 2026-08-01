<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import type { VM } from '~/types'

const props = defineProps<{ vm: VM }>()
const emit = defineEmits<{ changed: [] }>()

const { user } = useAuth()
const { api } = useApi()

const canStart = computed(() => can(user.value, 'vm:start') && (props.vm.status === 'poweredOff' || props.vm.status === 'suspended'))
const canStop = computed(() => can(user.value, 'vm:stop') && props.vm.status === 'poweredOn')
const canRestart = computed(() => can(user.value, 'vm:restart') && props.vm.status === 'poweredOn')

const VERBS: Record<string, { label: string; verb: string }> = {
  start: { label: '启动', verb: 'start' },
  stop: { label: '停止', verb: 'stop' },
  restart: { label: '重启', verb: 'restart' },
}

async function doPower(action: 'start' | 'stop' | 'restart') {
  const { label, verb } = VERBS[action]
  try {
    await ElMessageBox.confirm(`确定要${label}虚拟机「${props.vm.name}」吗？`, '操作确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await api<VM>(`/vms/${props.vm.id}/${verb}`, { method: 'POST' })
    ElMessage.success(`${label}成功`)
    emit('changed')
  } catch (err) {
    const message = (err as { data?: { message?: string } })?.data?.message || `${label}失败`
    ElMessage.error(message)
  }
}
</script>

<template>
  <div class="power-actions">
    <el-button
      size="small"
      type="success"
      :disabled="!canStart"
      :title="canStart ? '启动' : '仅关机/挂起状态可启动'"
      @click="doPower('start')"
    >
      <IconRenderer icon="mdi:play" /> 启动
    </el-button>
    <el-button
      size="small"
      type="danger"
      plain
      :disabled="!canStop"
      :title="canStop ? '停止' : '仅运行中可停止'"
      @click="doPower('stop')"
    >
      <IconRenderer icon="mdi:stop" /> 停止
    </el-button>
    <el-button
      size="small"
      type="warning"
      plain
      :disabled="!canRestart"
      :title="canRestart ? '重启' : '仅运行中可重启'"
      @click="doPower('restart')"
    >
      <IconRenderer icon="mdi:restart" /> 重启
    </el-button>
  </div>
</template>

<style scoped>
.power-actions {
  display: inline-flex;
  gap: 8px;
}
</style>
