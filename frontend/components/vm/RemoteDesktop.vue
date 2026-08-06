<script setup lang="ts">
import Guacamole from 'guacamole-common-js'
import { ElMessage } from 'element-plus'
import type { VM } from '~/types'

const props = defineProps<{ vm: VM; paused?: boolean }>()

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

function sendKeydown(keysym: number) {
  client?.sendKeyEvent(1, keysym)
}
function sendKeyup(keysym: number) {
  client?.sendKeyEvent(0, keysym)
}

// 键盘事件（监听全局按键，焦点在 iframe/canvas 外也生效）。
// guacamole-common-js 的 Keyboard 只要挂有 handler 就会拦截并 preventDefault
// 全部按键（无输入框豁免），因此页面弹窗（如远程桌面配置对话框）期间必须
// 置空 handler 暂停，否则弹窗内的文本框无法输入。
function setKeyboardActive(active: boolean) {
  if (!keyboard) return
  keyboard.onkeydown = active ? sendKeydown : null
  keyboard.onkeyup = active ? sendKeyup : null
}

watch(
  () => props.paused,
  (paused) => setKeyboardActive(!paused),
  { immediate: true },
)

function notify() {
  emit('state', { connecting: connecting.value, connected: connected.value, error: errorText.value })
}

function wsUrl(): string {
  // nitro routeRules 反代不支持 WebSocket 升级（返回 400），
  // WS 必须直连后端；未配置 apiWsBase 时退回同源（dev 走 wsProxy 模块）。
  // 注意：guacamole-common-js 的 WebSocketTunnel 只认 "/" 开头或相对路径，
  // "http(s)://" 绝对 URL 会被拼成相对路径，因此必须转成 "ws(s)://"。
  const base = (useRuntimeConfig().public.apiWsBase || '').replace(/^http:/, 'ws:')
  const path = `/api/v1/guac/ws/${encodeURIComponent(props.vm.id)}`
  const t = token.value
  return t ? `${base}${path}?token=${encodeURIComponent(t)}` : `${base}${path}`
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

  // 键盘交互（监听全局按键，焦点在 iframe/canvas 外也生效）。
  // 挂载后按当前 paused 状态决定是否激活，避免弹窗打开时新建键盘吞输入。
  keyboard = new Guacamole.Keyboard(document)
  keyboard.onkeydown = sendKeydown
  keyboard.onkeyup = sendKeyup
  setKeyboardActive(!props.paused)

  // 注意：client.onstatechange 收到的是 Client.State（非 Tunnel.State）。
  // WAITING 是 connect() 同步置的状态（WS 未必已通，失败也会经过它），
  // 只有 CONNECTED（首帧已渲染）才代表真的连上。
  client.onstatechange = (state: number) => {
    switch (state) {
      case Guacamole.Client.State.CONNECTED:
        connecting.value = false
        connected.value = true
        fit()
        break
      case Guacamole.Client.State.WAITING:
        connecting.value = true
        connected.value = false
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
  // 必须先清空键盘回调再丢引用：guacamole-common-js 的 Keyboard 把
  // keydown/keyup 监听永久挂在 document 上且无 dispose/removeEventListener，
  // 若不清空 onkeydown/onkeyup，组件卸载后仍会拦截并 preventDefault 全局按键，
  // 导致切到其他页面后所有输入框无法输入。清空后监听闭包虽仍残留于 document
  // （inert，无副作用），长期可改为页面级单例复用避免累积。
  setKeyboardActive(false)
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
  // 进入/退出全屏时容器尺寸变化，重新适配画布（浏览器 resize 事件不可靠）
  document.addEventListener('fullscreenchange', onResize)
  connect()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  document.removeEventListener('fullscreenchange', onResize)
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
  /* guacamole-common-js 的画布是 z-index:-1 的绝对定位元素，
     容器必须创建 stacking context，否则画布会沉到容器黑背景之下
     （桌面内容渲染在位图里却看不见，画面全黑）。 */
  position: relative;
  z-index: 0;
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
