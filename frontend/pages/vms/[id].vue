<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { VM, VMStatus, VMStatusResult } from '~/types'

definePageMeta({ middleware: 'auth' })

const route = useRoute()
const router = useRouter()
const { api } = useApi()
const { hasPerm } = useAuth()

const vm = ref<VM | null>(null)
const loading = ref(false)
const polling = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

const rdCardRef = ref<{ $el: HTMLElement } | null>(null)
const isFullscreen = ref(false)
const rdState = reactive({ connecting: false, connected: false })
// 远程桌面配置对话框打开时暂停全局键盘监听（否则弹窗内无法输入）
const rdConfigOpen = ref(false)

function toggleFullscreen() {
  const el = rdCardRef.value?.$el
  if (!el) return
  if (document.fullscreenElement) {
    void document.exitFullscreen()
    isFullscreen.value = false
  } else {
    void el.requestFullscreen()
    isFullscreen.value = true
  }
}

function onRdState(s: { connecting: boolean; connected: boolean; error: string }) {
  rdState.connecting = s.connecting
  rdState.connected = s.connected
}

// RustDesk 一键连接：rustdesk://<id>[@<server>?key=<key>]
const rdId = computed(() => String(vm.value?.metadata?.['rustdesk.id'] || ''))
const rdServer = computed(() => String(vm.value?.metadata?.['rustdesk.server'] || ''))
const rdKey = computed(() => String(vm.value?.metadata?.['rustdesk.key'] || ''))

function rustdeskLink(): string {
  // id 可能含自定义字符，编码避免破坏 URI 结构（Dart 端会解码 authority）
  let link = `rustdesk://${encodeURIComponent(rdId.value)}`
  if (rdServer.value) {
    link += `@${rdServer.value}`
    if (rdKey.value) {
      link += `?key=${encodeURIComponent(rdKey.value)}`
    }
  }
  return link
}

function openRustDesk() {
  // 用隐藏 iframe 唤起外部协议：未安装 RustDesk 时页面不会跳转错误页，
  // SPA 状态与路由保持不变
  let invoked = false
  const markInvoked = () => {
    invoked = true
  }
  // 唤起本机客户端时窗口会失焦；以此尽力检测是否安装（各浏览器行为有差异）
  window.addEventListener('blur', markInvoked)
  const frame = document.createElement('iframe')
  frame.style.display = 'none'
  frame.src = rustdeskLink()
  document.body.appendChild(frame)
  window.setTimeout(() => frame.remove(), 2000)
  window.setTimeout(() => {
    window.removeEventListener('blur', markInvoked)
    if (!invoked) {
      ElMessage({
        message: '未能唤起 RustDesk，请确认本机已安装客户端（可在 rustdesk.com 下载）；若客户端已打开可忽略此提示',
        type: 'warning',
        duration: 6000,
      })
    }
  }, 1500)
}

async function copyText(text: string) {
  try {
    if (navigator.clipboard) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    }
    ElMessage.success('已复制')
  } catch {
    ElMessage.error('复制失败，请手动复制')
  }
}

// 扩展信息展示时隐藏敏感键（密码/令牌），仅展示连接参数
const displayMetadata = computed(() => {
  const md = vm.value?.metadata || {}
  return Object.entries(md)
    .filter(([key]) => !/password|passwd|secret|token/i.test(key))
    .map(([key, value]) => ({ key, value: typeof value === 'string' ? value : JSON.stringify(value) }))
})

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
        <VmPowerActions :vm="vm" @changed="loadVM" />
      </div>

      <el-row :gutter="16">
        <el-col :xs="24" :lg="8">
          <el-card shadow="never" class="mb-3">
            <template #header><span>基本信息</span></template>
            <el-descriptions :column="1" size="small">
              <el-descriptions-item label="ID">{{ vm.id }}</el-descriptions-item>
              <el-descriptions-item label="IP 地址">{{ vm.ip_address || vm.metadata?.['guac.hostname'] || '-' }}</el-descriptions-item>
              <el-descriptions-item label="宿主机">{{ vm.host || '-' }}</el-descriptions-item>
              <el-descriptions-item label="CPU">{{ vm.cpu }} 核</el-descriptions-item>
              <el-descriptions-item label="内存">
                {{ vm.memory_mb >= 1024 ? `${(vm.memory_mb / 1024).toFixed(1)} GB` : `${vm.memory_mb} MB` }}
              </el-descriptions-item>
            </el-descriptions>
          </el-card>

          <el-card shadow="never">
            <template #header><span>扩展信息</span></template>
            <el-descriptions :column="1" size="small">
              <el-descriptions-item v-for="item in displayMetadata" :key="item.key" :label="item.key">
                {{ item.value }}
              </el-descriptions-item>
            </el-descriptions>
            <el-empty v-if="!displayMetadata.length" description="无扩展信息" :image-size="60" />
          </el-card>
        </el-col>

        <el-col :xs="24" :lg="16">
          <el-card ref="rdCardRef" shadow="never" class="mb-3 rd-card">
            <template #header>
              <div class="card-header">
                <span>远程桌面</span>
                <div class="rd-actions">
                  <el-tag v-if="rdState.connected" type="success" size="small" effect="plain">已连接</el-tag>
                  <el-tag v-else-if="rdState.connecting" type="info" size="small" effect="plain">连接中</el-tag>
                  <el-tag v-else type="warning" size="small" effect="plain">未连接</el-tag>
                  <el-popover v-if="rdId && hasPerm('vm:console')" placement="bottom-end" :width="300" trigger="click">
                    <template #reference>
                      <el-button size="small" plain :title="'RustDesk 一键连接'">
                        <IconRenderer icon="mdi:monitor" /> RustDesk
                      </el-button>
                    </template>
                    <div class="rustdesk-pop">
                      <div class="rustdesk-row">
                        <span>设备 ID</span>
                        <el-text tag="b">{{ rdId }}</el-text>
                        <el-button size="small" text type="primary" @click="copyText(rdId)">复制</el-button>
                      </div>
                      <div v-if="rdServer" class="rustdesk-row">
                        <span>服务器</span>
                        <el-text>{{ rdServer }}</el-text>
                      </div>
                      <div class="rustdesk-actions">
                        <el-button size="small" type="primary" @click="openRustDesk">
                          <IconRenderer icon="mdi:monitor" /> 一键连接
                        </el-button>
                        <el-button
                          size="small"
                          tag="a"
                          href="https://rustdesk.com/"
                          target="_blank"
                          rel="noopener"
                        >
                          下载客户端
                        </el-button>
                      </div>
                      <p class="rustdesk-hint">
                        连接密码在客户端提示时输入；本机需已安装 RustDesk，
                        目标机需安装并运行 RustDesk 客户端（ID 在其界面左上角查看）
                      </p>
                    </div>
                  </el-popover>
                  <VmRemoteDesktopConfig
                    :vm="vm"
                    @changed="loadVM"
                    @open="rdConfigOpen = true"
                    @close="rdConfigOpen = false"
                  />
                </div>
              </div>
            </template>
            <div v-if="vm.status === 'poweredOn' && hasPerm('vm:console')" class="rd-conn-info">
              <el-text size="small" type="info">
                协议 {{ vm.metadata?.['guac.protocol'] || '未配置' }} ·
                目标 {{ vm.metadata?.['guac.hostname'] || vm.ip_address || '-' }}:{{
                  vm.metadata?.['guac.port'] || '默认'
                }}
              </el-text>
              <el-button size="small" type="primary" plain @click="toggleFullscreen">全屏</el-button>
            </div>
            <div v-if="vm.status !== 'poweredOn'" class="rd-placeholder">
              <IconRenderer icon="mdi:desktop" :size="48" />
              <p>虚拟机未开机，启动后可打开浏览器远程桌面</p>
            </div>
            <div v-else-if="!hasPerm('vm:console')" class="rd-placeholder">
              <IconRenderer icon="mdi:shield-lock" :size="48" />
              <p>无远程桌面权限（vm:console），请联系管理员</p>
            </div>
            <template v-else>
              <VmRemoteDesktop
                :vm="vm"
                :paused="rdConfigOpen"
                @state="onRdState"
              />
            </template>
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

/* 全屏模式：卡片占满视口，画布区域撑满剩余空间 */
.rd-card:fullscreen {
  display: flex;
  flex-direction: column;
  width: 100vw;
  height: 100vh;
  margin: 0;
  border-radius: 0;
  overflow: hidden;
}

.rd-card:fullscreen :deep(.el-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 8px;
  overflow: hidden;
}

.rd-card:fullscreen .rd-conn-info {
  flex: none;
  margin-bottom: 8px;
}

.rd-card:fullscreen :deep(.remote-desktop) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.rd-card:fullscreen :deep(.rd-canvas) {
  flex: 1;
  min-height: 0;
  height: auto;
}

.rd-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px 0;
  color: var(--el-text-color-secondary);
}

.rd-conn-info {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.rd-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rustdesk-pop {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rustdesk-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rustdesk-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rustdesk-row > span:first-child {
  flex: none;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.rustdesk-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

@media (max-width: 767px) {
  .detail-header {
    flex-wrap: wrap;
    gap: 10px;
    align-items: flex-start;
  }

  .detail-title {
    flex-wrap: wrap;
    gap: 8px;
  }

  .detail-title .page-title {
    overflow-wrap: anywhere;
  }

  .card-header {
    flex-wrap: wrap;
    gap: 8px;
  }

  .rd-actions {
    flex-wrap: wrap;
  }

  .rd-conn-info {
    flex-wrap: wrap;
    gap: 8px;
  }
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
