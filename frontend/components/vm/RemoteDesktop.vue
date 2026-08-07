<script setup lang="ts">
// 静态导入 guacamole-common-js：生产构建下动态 import 经 Nuxt chunk loader
// 包装后 promise 可能永不 settle，且无 catch 导致连接静默卡死在「正在连接」，
// 改为随组件 chunk 静态打包（按路由分包，低带宽场景仍只在桌面页加载）。
import Guacamole from 'guacamole-common-js'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { VM } from '~/types'

export type RdMode = 'auto' | 'quality' | 'fluency'

const props = defineProps<{ vm: VM; paused?: boolean; mode?: RdMode }>()

const emit = defineEmits<{
  (e: 'state', s: { connecting: boolean; connected: boolean; error: string }): void
}>()

const { token } = useAuth()

const containerRef = ref<HTMLElement | null>(null)
const connecting = ref(false)
const connected = ref(false)
const errorText = ref('')

// 会话实际生效的模式：auto 在连接前按网络类型解析为具体档位
const effectiveMode = ref<Exclude<RdMode, 'auto'>>('quality')

let client: Guacamole.Client | null = null
let display: Guacamole.Display | null = null
let displayEl: HTMLElement | null = null
let mouse: Guacamole.Mouse | null = null
let keyboard: Guacamole.Keyboard | null = null
let tunnel: Guacamole.WebSocketTunnel | null = null
let monitorTimer: ReturnType<typeof setInterval> | null = null
// auto 模式状态机：off=监测中；fluency=已自动降档；quality=已撤销（锁定画质，不再自动降档）
let autoState: 'off' | 'fluency' | 'quality' = 'off'
let drawCount = 0
let badSeconds = 0
// 连接代数：重连/模式切换与卸载存在时间窗，仅最新一代的连接被采纳
let connGen = 0
let disposed = false

// auto 初选：按网络类型决定起始档位（4g/无信息 → 画质；2g/saveData → 流畅）
function resolveAuto(): Exclude<RdMode, 'auto'> {
  const conn = (navigator as any).connection
  if (conn?.saveData) return 'fluency'
  const eff: string | undefined = conn?.effectiveType
  if (eff === 'slow-2g' || eff === '2g') return 'fluency'
  return 'quality'
}

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

// 模式切换（手动或父级改动）：重置自动监测状态并重建会话
watch(
  () => props.mode,
  () => {
    autoState = 'off'
    badSeconds = 0
    if (connecting.value || connected.value || errorText.value) connect()
  },
)

function notify() {
  emit('state', { connecting: connecting.value, connected: connected.value, error: errorText.value })
}

function wsUrl(): string {
  // WS 必须同源直连（https 页面连 ws:// 内网会被浏览器 Mixed Content 拦截）：
  // 页面是 https 走 wss://（隧道/反代场景），是 http 走 ws://。
  // 同源时生产经 Caddy/nitro 反代转发到后端，dev 走 wsProxy 模块。
  // guacamole-common-js 的 WebSocketTunnel 只认 "/" 开头或相对路径，
  // 因此必须显式拼成 "ws(s)://" 绝对 URL。
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${window.location.host}/api/v1/guac/ws/${encodeURIComponent(props.vm.id)}`
}

// 连接参数走 client.connect(data)：库内部拼成 URL 的 query 部分。
// 不能把参数拼进 tunnel URL——connect() 无参时会追加 "?undefined"，
// 使最后的参数被污染（后端解析出 "mode=quality?undefined"）。
function connectData(): string {
  const t = token.value
  const parts: string[] = []
  if (t) parts.push(`token=${encodeURIComponent(t)}`)
  // mode 为会话级质量档位（后端按此调整握手分辨率/色深等），与 token 共存
  parts.push(`mode=${effectiveMode.value}`)
  return parts.join('&')
}

// 依据容器尺寸同步会话分辨率（缩放只影响观感；真正降低带宽的是
// 后端按 mode 下发的握手分辨率）。display.scale 是方法，非属性。
function fit() {
  if (!client || !display || !displayEl?.parentElement) return
  const parent = displayEl.parentElement
  const w = parent.clientWidth
  const h = parent.clientHeight
  if (w <= 0 || h <= 0) return
  client.sendSize(w, h)
  const sw = display.getWidth()
  const sh = display.getHeight()
  if (sw > 0 && sh > 0) {
    display.scale(Math.min(1, w / sw, h / sh))
  }
}

// 帧率监测：包装 display 的绘制入口计数。连续 6 秒「每秒 2-5 帧
// （画面在持续渲染但帧率远低于画质档目标）」视为网络不流畅
// → 自动重连为流畅档。空闲桌面（光标闪烁等）低于 2 帧/秒，不会误判。
function startMonitor() {
  if (monitorTimer) return
  monitorTimer = setInterval(() => {
    if (props.mode !== 'auto' || autoState !== 'off' || !connected.value) {
      drawCount = 0
      return
    }
    const frames = drawCount
    drawCount = 0
    if (frames >= 2 && frames < 6) {
      badSeconds++
    } else {
      badSeconds = 0
    }
    if (badSeconds >= 6) {
      degradeToFluency()
    }
  }, 1000)
}

function stopMonitor() {
  if (monitorTimer) {
    clearInterval(monitorTimer)
    monitorTimer = null
  }
}

// 自动降档：立即重建会话为流畅档，并询问是否撤销（撤销则锁定画质档）
function degradeToFluency() {
  autoState = 'fluency'
  effectiveMode.value = 'fluency'
  ElMessage.warning('检测到网络不佳，已自动切换流畅模式')
  ElMessageBox.confirm(
    '远程桌面已切换为流畅模式（分辨率/画质降低）。',
    '流畅模式',
    { confirmButtonText: '撤销，回到画质', cancelButtonText: '继续流畅', type: 'info' },
  )
    .then(() => {
      // 组件可能已卸载（对话框在卸载后才被确认），此时不得复活会话
      if (disposed) return
      autoState = 'quality'
      effectiveMode.value = 'quality'
      connect()
    })
    .catch(() => {})
  connect()
}

function connect() {
  disconnect()
  errorText.value = ''
  connecting.value = true
  connected.value = false
  notify()
  const gen = ++connGen

  // 生效档位：手动模式直取；auto 依次按 已降档/已撤销 → 网络类型初选
  if (props.mode !== 'auto') {
    effectiveMode.value = props.mode
  } else if (autoState !== 'off') {
    effectiveMode.value = autoState
  } else {
    effectiveMode.value = resolveAuto()
  }

  // 库已静态导入随组件 chunk 加载，无需异步等待；gen 快照防止
  // 加载/重连期间旧会话回调建立双会话。任何一步抛错立即进入错误态，
  // 不再静默卡死在「正在连接」（生产构建下动态 import 曾永不 settle）。
  try {
    tunnel = new Guacamole.WebSocketTunnel(wsUrl())
    client = new Guacamole.Client(tunnel)
    display = client.getDisplay()
    displayEl = display.getElement()
    if (containerRef.value) {
      containerRef.value.appendChild(displayEl)
    }

    // 包装绘制入口用于帧率采样
    const origDraw = display.draw.bind(display)
    const origDrawImage = display.drawImage.bind(display)
    const origDrawBlob = display.drawBlob.bind(display)
    display.draw = ((...args: unknown[]) => {
      drawCount++
      return origDraw(...(args as Parameters<typeof display.draw>))
    }) as typeof display.draw
    display.drawImage = ((...args: unknown[]) => {
      drawCount++
      return origDrawImage(...args)
    }) as typeof display.drawImage
    display.drawBlob = ((...args: unknown[]) => {
      drawCount++
      return origDrawBlob(...args)
    }) as typeof display.drawBlob

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

    // 不得覆盖 tunnel.oninstruction：Client 构造函数内部用它做指令分发
    // （sync→CONNECTED 等），覆盖后 Client 收不到任何指令，状态机卡在 WAITING。
    // 掉帧判定由 display.draw 计数承担（见 startMonitor）。

    // 注意：client.onstatechange 收到的是 Client.State（非 Tunnel.State）。
    // WAITING 是 connect() 同步置的状态（WS 未必已通，失败也会经过它），
    // 只有 CONNECTED（首帧已渲染）才代表真的连上。
    client.onstatechange = (state: number) => {
      switch (state) {
        case Guacamole.Client.State.CONNECTED:
          connecting.value = false
          connected.value = true
          fit()
          startMonitor()
          break
        case Guacamole.Client.State.WAITING:
          connecting.value = true
          connected.value = false
          break
        case Guacamole.Client.State.DISCONNECTED:
        case Guacamole.Client.State.IDLE:
          connecting.value = false
          connected.value = false
          stopMonitor()
          break
      }
      notify()
    }
    client.onerror = (error: { message?: string }) => {
      connecting.value = false
      connected.value = false
      errorText.value = friendlyError(error.message || '远程桌面连接失败')
      ElMessage.error(errorText.value)
      notify()
    }

    client.connect(connectData())
  } catch (err) {
    if (gen !== connGen || disposed) return
    // 清理半初始化资源：tunnel 可能已发起 WS 而 client/display 未建完，
    // 直接断开并移除画布，避免悬空连接与残留节点
    disconnect()
    connecting.value = false
    errorText.value = friendlyError(err instanceof Error ? err.message : '远程桌面连接失败')
    ElMessage.error(errorText.value)
    notify()
  }
}

// friendlyError 将 guacd 返回的英文错误消息映射为中文可操作提示
// （guacd 通过 5.error 指令携带，常见于 RDP/VNC 认证与目标连接失败）。
function friendlyError(msg: string): string {
  if (/authentication failure|authentication failed|auth failed|login failed|username or password|credentials/i.test(msg)) {
    return '认证失败：用户名或密码错误，请在远程访问配置中检查'
  }
  if (/ssl\/tls|ssl negotiation|self-signed|untrusted/i.test(msg)) {
    return '目标证书不受信：请在远程访问配置中启用「忽略证书校验」'
  }
  if (/connection failed|unable to connect|timed out|unreachable|refused/i.test(msg)) {
    return '无法连接目标主机：请检查 IP/端口与网络连通性'
  }
  if (/disconnected by other connection|another connection|already connected/i.test(msg)) {
    return '该桌面已被其他会话占用（Windows 远程桌面仅允许一个活动连接），请关闭其他会话后重试'
  }
  return msg
}

function disconnect() {
  // 必须先清空键盘回调再丢引用：guacamole-common-js 的 Keyboard 把
  // keydown/keyup 监听永久挂在 document 上且无 dispose/removeEventListener，
  // 若不清空 onkeydown/onkeyup，组件卸载后仍会拦截并 preventDefault 全局按键，
  // 导致切到其他页面后所有输入框无法输入。清空后监听闭包虽仍残留于 document
  // （inert，无副作用），长期可改为页面级单例复用避免累积。
  setKeyboardActive(false)
  // 代际 +1：使加载中/在途的旧连接回调失效（防双会话泄漏）
  connGen++
  stopMonitor()
  if (client) {
    client.disconnect()
    client = null
  }
  tunnel = null
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
  disposed = true
  window.removeEventListener('resize', onResize)
  document.removeEventListener('fullscreenchange', onResize)
  disconnect()
})
</script>

<template>
  <div class="remote-desktop">
    <div v-if="connecting" class="rd-status rd-connecting">
      <el-icon class="is-loading" :size="20"><Loading /></el-icon>
      <span>正在连接远程桌面…（{{ effectiveMode === 'fluency' ? '流畅' : '画质' }}模式）</span>
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
