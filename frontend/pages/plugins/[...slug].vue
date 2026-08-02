<script setup lang="ts">
import type { MenuItem, PluginEndpoint } from '~/types'

definePageMeta({ middleware: 'auth' })

const route = useRoute()
const { items, load, findByRoute } = useMenu()

const plugin = ref<MenuItem | null>(null)
const notFound = ref(false)

const selectedEndpoint = ref<PluginEndpoint | null>(null)
const methodBody = ref('')
const methodResult = ref<{ status: number; body: string } | null>(null)
const calling = ref(false)

async function sync() {
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
watch(() => route.params.slug, sync)
watch(selectedEndpoint, () => {
  methodResult.value = null
  methodBody.value = ''
})
</script>

<template>
  <div class="plugin-page">
    <template v-if="plugin">
      <!-- iframe 类型：嵌入外部页面 -->
      <iframe
        v-if="plugin.type === 'iframe'"
        :src="plugin.iframe_url"
        class="vm-iframe"
        title="plugin"
      />
      <!-- native 类型：嵌入后端托管的内嵌静态页 -->
      <iframe
        v-else-if="plugin.type === 'native'"
        :src="nativeSrc(plugin)"
        class="vm-iframe"
        title="plugin"
      />
      <!-- proxy 类型：标准 API 调用面板 -->
      <div v-else class="proxy-panel">
        <div v-if="!plugin.endpoints?.length" class="proxy-empty">
          <el-empty description="该插件未声明任何 API 端点" />
        </div>
        <template v-else>
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
        </template>
      </div>
    </template>
    <div v-else class="plugin-empty">
      <el-empty :description="notFound ? '插件不存在或无权访问' : '正在加载...'" />
    </div>
  </div>
</template>

<style scoped>
.plugin-page {
  height: 100%;
}

.plugin-empty,
.proxy-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
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
