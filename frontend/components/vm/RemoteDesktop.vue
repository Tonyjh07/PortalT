<script setup lang="ts">
import Guacamole from 'guacamole-common-js'
import { ElMessage } from 'element-plus'
import type { VM } from '~/types'

const props = defineProps<{ vm: VM }>()

const emit = defineEmits<{
  (e: 'state', s: { connecting: boolean; connected: boolean; error: string }): void
}>()

const { token } = useAuth()

const containerRef = ref<HTMLElement | null>(null)
const connecting = ref(false)
const connected = ref(false)
const errorText = ref('')

let client: Guacamole.Client | null = null
let display: Guacamole.Display | null = null
let displayEl: HTMLElement | null = null
let mouse: Guacamole.Mouse | null = null
let keyboard: Guacamole.Keyboard | null = null

function notify() {
  emit('state', { connecting: connecting.value, connected: connected.value, error: errorText.value })
}

function wsUrl(): string {
  const base = `/api/v1/guac/ws/${encodeURIComponent(props.vm.id)}`
  const t = token.value
  return t ? `${base}?token=${encodeURIComponent(t)}` : base
}

// 依据容器尺寸同步会话分辨率（DPI 96，与物理像素一致）
function fit() {
  if (!client || !display || !displayEl?.parentElement) return
  const parent = displayEl.parentElement
  const w = parent.clientWidth
  const h = parent.clientHeight
  if (w <= 0 || h <= 0) return
  client.sendSize(w, h, 96)
  const sw = display.getWidth()
  const sh = display.getHeight()
  if (sw > 0 && sh > 0) {
    display.scale = Math.min(1, w / sw, h / sh)
  }
}

function connect() {
  disconnect()
  errorText.value = ''
  connecting.value = true
  connected.value = false
  notify()

  const tunnel = new Guacamole.WebSocketTunnel(wsUrl())
  client = new Guacamole.Client(tunnel)
  display = client.getDisplay()
  displayEl = display.getElement()
  if (containerRef.value) {
    containerRef.value.appendChild(displayEl)
  }

  // 鼠标交互（点击/移动/滚轮）
  mouse = new Guacamole.Mouse(displayEl)
  const sendMouse = (state: Guacamole.MouseState) => client?.sendMouseState(state, true)
  mouse.onmousedown = sendMouse
  mouse.onmouseup = sendMouse
  mouse.onmousemove = sendMouse

  // 键盘交互（监听全局按键，焦点在 iframe/canvas 外也生效）
  keyboard = new Guacamole.Keyboard(document)
  keyboard.onkeydown = (keysym: number) => client?.sendKeyEvent(1, keysym)
  keyboard.onkeyup = (keysym: number) => client?.sendKeyEvent(0, keysym)

  // 注意：client.onstatechange 收到的是 Client.State（非 Tunnel.State），
  // WAITING 表示隧道已通但首帧未到，CONNECTED 表示首帧已渲染。
  client.onstatechange = (state: number) => {
    switch (state) {
      case Guacamole.Client.State.CONNECTED:
      case Guacamole.Client.State.WAITING:
        connecting.value = false
        connected.value = true
        fit()
        break
      case Guacamole.Client.State.DISCONNECTED:
      case Guacamole.Client.State.IDLE:
        connecting.value = false
        connected.value = false
        break
    }
    notify()
  }
  client.onerror = (error: { message?: string }) => {
    connecting.value = false
    connected.value = false
    errorText.value = error.message || '远程桌面连接失败'
    ElMessage.error(errorText.value)
    notify()
  }

  client.connect()
}

function disconnect() {
  if (client) {
    client.disconnect()
    client = null
  }
  display = null
  if (displayEl?.parentElement) {
    displayEl.remove()
  }
  displayEl = null
  mouse = null
  keyboard = null
  connecting.value = false
  connected.value = false
  notify()
}

function onResize() {
  if (connected.value) fit()
}

onMounted(() => {
  window.addEventListener('resize', onResize)
  connect()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  disconnect()
})
</script>

<template>
  <div class="remote-desktop">
    <div v-if="connecting" class="rd-status rd-connecting">
      <el-icon class="is-loading" :size="20"><Loading /></el-icon>
      <span>正在连接远程桌面…</span>
    </div>
    <div v-else-if="errorText && !connected" class="rd-status rd-error">
      <el-icon :size="20"><WarningFilled /></el-icon>
      <span>{{ errorText }}</span>
      <el-button size="small" type="primary" plain @click="connect">重新连接</el-button>
    </div>
    <div ref="containerRef" class="rd-canvas" :class="{ hidden: !connected && !connecting }" />
  </div>
</template>

<style scoped>
.remote-desktop {
  position: relative;
}

.rd-canvas {
  min-height: 480px;
  border-radius: 4px;
  background-color: #000;
  overflow: hidden;
  touch-action: none;
}

.rd-canvas :deep(.guac-display) {
  margin: 0 auto;
}

.rd-canvas.hidden {
  display: none;
}

.rd-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 480px;
  color: var(--el-text-color-secondary);
}

.rd-status.rd-connecting {
  color: var(--el-color-primary);
}

.rd-status.rd-error {
  flex-direction: column;
  color: var(--el-color-danger);
}
</style>
