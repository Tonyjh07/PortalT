<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { VM, VMStatus, VMStatusResult } from '~/types'

definePageMeta({ middleware: 'auth' })

const route = useRoute()
const router = useRouter()
const { api } = useApi()

const vm = ref<VM | null>(null)
const loading = ref(false)
const polling = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

async function loadVM() {
  loading.value = true
  try {
    const res = await api<VM>(`/vms/${route.params.id}`)
    vm.value = res
  } catch (err) {
    const status = (err as { statusCode?: number }).statusCode
    if (status === 404) {
      ElMessage.error('虚拟机不存在')
      router.push('/vms')
      return
    }
    ElMessage.error((err as { data?: { message?: string } })?.data?.message || '加载虚拟机失败')
  } finally {
    loading.value = false
  }
}

async function pollStatus() {
  if (!vm.value) return
  try {
    const res = await api<VMStatusResult>(`/vms/${vm.value.id}/status`)
    if (res.status !== vm.value.status) {
      vm.value.status = res.status
    }
  } catch {
    polling.value = false
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }
}

function startPolling() {
  if (timer) return
  polling.value = true
  pollStatus()
  timer = setInterval(pollStatus, 10_000)
}

function stopPolling() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  polling.value = false
}

onMounted(() => {
  loadVM()
  startPolling()
})

onUnmounted(stopPolling)
</script>

<template>
  <div class="page-container" v-loading="loading">
    <template v-if="vm">
      <div class="detail-header">
        <div class="detail-title">
          <IconRenderer icon="mdi:server" :size="26" />
          <h2 class="page-title">{{ vm.name }}</h2>
          <VMStatusTag :status="vm.status" />
          <el-tag v-if="polling" type="info" size="small" effect="plain">自动刷新 10s</el-tag>
        </div>
        <PowerActions :vm="vm" @changed="loadVM" />
      </div>

      <el-row :gutter="16">
        <el-col :xs="24" :lg="8">
          <el-card shadow="never" class="mb-3">
            <template #header><span>基本信息</span></template>
            <el-descriptions :column="1" size="small">
              <el-descriptions-item label="ID">{{ vm.id }}</el-descriptions-item>
              <el-descriptions-item label="IP 地址">{{ vm.ip_address || '-' }}</el-descriptions-item>
              <el-descriptions-item label="宿主机">{{ vm.host || '-' }}</el-descriptions-item>
              <el-descriptions-item label="CPU">{{ vm.cpu }} 核</el-descriptions-item>
              <el-descriptions-item label="内存">
                {{ vm.memory_mb >= 1024 ? `${(vm.memory_mb / 1024).toFixed(1)} GB` : `${vm.memory_mb} MB` }}
              </el-descriptions-item>
            </el-descriptions>
          </el-card>

          <el-card shadow="never" class="mb-3">
            <template #header>
              <span>资源使用</span>
              <span class="meta-hint">由平台提供</span>
            </template>
            <div class="usage-item">
              <span>CPU 核数</span>
              <el-progress :percentage="Math.min(100, vm.cpu * 10)" :stroke-width="10" />
            </div>
            <div class="usage-item">
              <span>内存</span>
              <el-progress
                :percentage="Math.min(100, Math.round((vm.memory_mb / 16384) * 100))"
                :stroke-width="10"
                status="success"
              />
            </div>
          </el-card>
        </el-col>

        <el-col :xs="24" :lg="16">
          <el-card shadow="never" class="mb-3">
            <template #header>
              <div class="card-header">
                <span>远程桌面</span>
                <el-tag type="warning" size="small" effect="plain">Phase 8</el-tag>
              </div>
            </template>
            <div class="rd-placeholder">
              <IconRenderer icon="mdi:desktop" :size="48" />
              <p>浏览器远程桌面将在 Phase 8 提供</p>
              <el-text type="info" size="small">
                WebSocket 入口已就绪：<code>/api/v1/guac/ws/{{ vm.id }}</code>
              </el-text>
            </div>
          </el-card>

          <el-card shadow="never">
            <template #header><span>扩展信息</span></template>
            <el-descriptions :column="1" size="small">
              <el-descriptions-item v-for="(value, key) in vm.metadata" :key="key" :label="key">
                {{ JSON.stringify(value) }}
              </el-descriptions-item>
            </el-descriptions>
            <el-empty v-if="!vm.metadata || !Object.keys(vm.metadata).length" description="无扩展信息" :image-size="60" />
          </el-card>
        </el-col>
      </el-row>
    </template>
  </div>
</template>

<style scoped>
.detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.detail-title {
  display: flex;
  align-items: center;
  gap: 12px;
}

.detail-title .page-title {
  margin: 0;
}

.mb-3 {
  margin-bottom: 16px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.meta-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.usage-item {
  margin-bottom: 12px;
}

.usage-item > span {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.rd-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px 0;
  color: var(--el-text-color-secondary);
}

.rd-placeholder p {
  margin: 0;
}

.rd-placeholder code {
  padding: 2px 6px;
  border-radius: 4px;
  background-color: var(--el-fill-color);
  font-size: 12px;
}
</style>
