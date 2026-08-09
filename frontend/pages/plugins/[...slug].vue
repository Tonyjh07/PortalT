<script setup lang="ts">
import type { MenuItem, PluginEndpoint } from '~/types'

definePageMeta({ middleware: 'auth' })

const route = useRoute()
const { items, load, findByRoute } = useMenu()
const { api } = useApi()
const { refresh, logout } = useAuth()

// 门户内相对路径 iframe（如 esxi-admin /esxi/ui/）为长会话：access_token 仅 15 分钟，
// 页面挂载期间每 5 分钟静默续期一次（useAuth.refresh 写回 cookie），保证 iframe 内
// 大量子请求（经 Caddy forward_auth 闸口校验）与门户 API 的令牌保持新鲜。
// 仅在续期确认失败（refresh token 失效，refresh() 返回 'expired'）时登出并停止定时器；
// 网络抖动/后端短暂不可用（'error'）跳过本轮，避免把用户踢出登录态。
const KEEPALIVE_INTERVAL = 5 * 60 * 1000
let keepaliveTimer: ReturnType<typeof setInterval> | null = null

function startKeepalive() {
  const tick = async () => {
    if ((await refresh()) === 'expired') {
      stopKeepalive()
      logout()
    }
  }
  void tick()
  keepaliveTimer = setInterval(tick, KEEPALIVE_INTERVAL)
}

function stopKeepalive() {
  if (keepaliveTimer) {
    clearInterval(keepaliveTimer)
    keepaliveTimer = null
  }
}

const plugin = ref<MenuItem | null>(null)
const notFound = ref(false)

// 平台接入状态（仅门户内相对路径 iframe 的 access 插件需要，如 esxi-admin）
interface PlatformInfo {
  provider: string
  web_url: string
  connected: boolean
}
const platform = ref<PlatformInfo | null>(null)
const platformErr = ref(false)
const iframeFailed = ref(false)

const selectedEndpoint = ref<PluginEndpoint | null>(null)
const methodBody = ref('')
const methodResult = ref<{ status: number; body: string } | null>(null)
const calling = ref(false)

// 是否为门户内相对路径 iframe（由 Caddy 规则反代），需要平台接入状态判断可嵌入性
function isPortalIframe(p: MenuItem): boolean {
  return !!p.iframe_url && p.iframe_url.startsWith('/')
}

async function sync() {
  stopKeepalive()
  const path = '/' + (route.params.slug as string[]).join('/')
  if (!items.value.length) {
    try {
      await load()
    } catch {
      notFound.value = true
      return
    }
  }
  const found = findByRoute(path)
  if (found) {
    plugin.value = found
    notFound.value = false
    iframeFailed.value = false
    if (isPortalIframe(found)) {
      startKeepalive()
      platform.value = null
      platformErr.value = false
      try {
        platform.value = await api<PlatformInfo>('/platform')
      } catch {
        platformErr.value = true
      }
    }
  } else {
    notFound.value = true
  }
}

function nativeSrc(p: MenuItem) {
  return `/native/${p.id}/`
}

async function callEndpoint() {
  const p = plugin.value
  const ep = selectedEndpoint.value
  if (!p || !ep || calling.value) return
  calling.value = true
  methodResult.value = null
  try {
    const { token } = useAuth()
    const config = useRuntimeConfig()
    const url = `${config.public.apiBase}/plugin-proxy/${p.id}/${ep.path.replace(/^\/+/, '')}`
    const opts: RequestInit = { method: ep.method, headers: { Authorization: `Bearer ${token.value}` } }
    if (ep.method !== 'GET') {
      opts.headers = { ...opts.headers, 'Content-Type': 'application/json' }
      opts.body = methodBody.value || '{}'
    }
    const res = await fetch(url, opts)
    const text = await res.text()
    methodResult.value = { status: res.status, body: text }
  } catch (e: any) {
    methodResult.value = { status: 0, body: String(e?.message ?? e) }
  } finally {
    calling.value = false
  }
}

onMounted(sync)
onUnmounted(stopKeepalive)
watch(() => route.params.slug, sync)
watch(selectedEndpoint, () => {
  methodResult.value = null
  methodBody.value = ''
})
</script>

<template>
  <div class="plugin-page">
    <template v-if="plugin">
      <!-- ============ access：一页双区块（iframe 嵌入 + API 面板，可共存） ============ -->
      <template v-if="plugin.type === 'access'">
        <!-- 区块一：iframe 嵌入 -->
        <template v-if="plugin.iframe_url">
          <!-- 门户内相对路径（如 esxi-admin /esxi/ui/）：按平台接入状态渲染三态 -->
          <div v-if="isPortalIframe(plugin)" class="access-embed">
            <template v-if="platform?.connected">
              <iframe
                :src="plugin.iframe_url"
                class="vm-iframe"
                title="plugin"
                @error="iframeFailed = true"
              />
              <el-alert
                v-if="iframeFailed"
                type="warning"
                :closable="false"
                show-icon
                title="嵌入页面加载失败"
                description="若页面空白，请确认 Caddy 已配置目标上游（ESXI_UPSTREAM）并已重载。"
              />
            </template>
            <div v-else class="embed-empty">
              <el-empty
                :description="platformErr ? '无法获取平台接入状态' : '当前未接入 ESXi 虚拟化平台，该插件不可用'"
              >
                <template #default>
                  <p class="embed-hint">接入 ESXi 平台（配置 VIRT_PROVIDER=esxi）并重启服务后，此处将嵌入 ESXi 管理界面。</p>
                </template>
              </el-empty>
            </div>
          </div>
          <!-- 外部地址（http/https）：直接嵌入 -->
          <iframe v-else :src="plugin.iframe_url" class="vm-iframe" title="plugin" />
        </template>

        <!-- 区块二：标准 API 面板（proxy 能力） -->
        <div v-if="plugin.endpoints?.length" class="proxy-panel">
          <el-row :gutter="16">
            <el-col :span="10">
              <el-card shadow="never">
                <template #header>API 端点</template>
                <div
                  v-for="ep in plugin.endpoints"
                  :key="ep.method + ep.path"
                  class="endpoint-item"
                  :class="{ active: selectedEndpoint === ep }"
                  @click="selectedEndpoint = ep"
                >
                  <span class="endpoint-method" :class="ep.method.toLowerCase()">{{ ep.method }}</span>
                  <span class="endpoint-path">{{ ep.path }}</span>
                  <span class="endpoint-name">{{ ep.name }}</span>
                </div>
              </el-card>
            </el-col>
            <el-col :span="14">
              <el-card shadow="never">
                <template #header>调用</template>
                <el-empty v-if="!selectedEndpoint" description="选择一个端点开始调用" />
                <template v-else>
                  <el-form label-width="70px" size="default">
                    <el-form-item label="路径">
                      <el-input :model-value="selectedEndpoint.path" readonly />
                    </el-form-item>
                    <el-form-item v-if="selectedEndpoint.method !== 'GET'" label="JSON">
                      <el-input
                        v-model="methodBody"
                        type="textarea"
                        :rows="4"
                        placeholder='{"key": "value"}'
                      />
                    </el-form-item>
                    <el-form-item>
                      <el-button type="primary" :loading="calling" @click="callEndpoint">
                        {{ selectedEndpoint.method }} 调用
                      </el-button>
                    </el-form-item>
                  </el-form>
                  <div v-if="methodResult" class="method-result">
                    <div class="method-status">
                      状态码: <el-tag size="small" :type="methodResult.status >= 200 && methodResult.status < 300 ? 'success' : 'danger'">
                        {{ methodResult.status }}
                      </el-tag>
                    </div>
                    <pre class="method-body">{{ methodResult.body }}</pre>
                  </div>
                </template>
              </el-card>
            </el-col>
          </el-row>
        </div>
        <!-- 双区块均无内容时（理论不出现，API 校验已拦截） -->
        <div v-if="!plugin.iframe_url && !plugin.endpoints?.length" class="embed-empty">
          <el-empty description="插件未配置嵌入页面或 API 端点" />
        </div>
      </template>

      <!-- native 类型：嵌入后端托管的内嵌静态页 -->
      <iframe
        v-else-if="plugin.type === 'native'"
        :src="nativeSrc(plugin)"
        class="vm-iframe"
        title="plugin"
      />
    </template>
    <div v-else class="plugin-empty">
      <el-empty :description="notFound ? '插件不存在或无权访问' : '正在加载...'" />
    </div>
  </div>
</template>

<style scoped>
.plugin-page {
  height: 100%;
  overflow-y: auto;
}

.plugin-empty,
.embed-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.embed-hint {
  margin-top: 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.access-embed {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 4px 8px;
}

.access-embed .vm-iframe {
  flex: 1;
}

.proxy-panel {
  padding: 4px 8px;
}

.endpoint-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.endpoint-item:hover {
  background: var(--el-fill-color-light);
}

.endpoint-item.active {
  background: var(--el-color-primary-light-9);
  outline: 1px solid var(--el-color-primary-light-7);
}

.endpoint-method {
  min-width: 52px;
  text-align: center;
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 600;
  font-size: 12px;
  color: #fff;
}

.endpoint-method.get {
  background: #47b881;
}

.endpoint-method.post {
  background: #3b82f6;
}

.endpoint-method.put {
  background: #d97706;
}

.endpoint-method.delete {
  background: #e5484d;
}

.endpoint-path {
  font-family: var(--el-font-family-mono);
  color: var(--el-text-color-primary);
}

.endpoint-name {
  color: var(--el-text-color-secondary);
}

.method-result {
  margin-top: 12px;
}

.method-status {
  margin-bottom: 8px;
}

.method-body {
  background: #1e1e1e;
  color: #d4d4d4;
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  max-height: 260px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
