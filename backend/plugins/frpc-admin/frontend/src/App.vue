<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  type ConfigResponse,
  type Connection,
  type FrpcConfig,
  type SaveConfigResponse,
  getConfig,
  getConnection,
  saveConfig,
} from './api'
import HostInfoDialog from './components/HostInfoDialog.vue'
import VisualEditor from './components/VisualEditor.vue'
import RawEditor from './components/RawEditor.vue'
import ResultPanel from './components/ResultPanel.vue'
import LogsDialog from './components/LogsDialog.vue'
import BackupsDialog from './components/BackupsDialog.vue'

type EditMode = 'visual' | 'raw'

// 单连接模型：插件管理一台目标主机，不依赖 PortalT 的 VM 列表（与主程序解耦）。
const conn = ref<Connection | null>(null)
const connLoaded = ref(false)
const connLoading = ref(false)
const hostInfoVisible = ref(false)

const mode = ref<EditMode>('visual')
const config = ref<ConfigResponse | null>(null)
const configLoading = ref(false)
const rawContent = ref('')
const saving = ref(false)
const lastResult = ref<SaveConfigResponse | null>(null)
const dirty = ref(false)
const logsVisible = ref(false)
const backupsVisible = ref(false)

const connected = computed(() => !!conn.value)
const connLabel = computed(() => {
  const c = conn.value
  if (!c) return ''
  return `${c.user}@${c.host}:${c.port}`
})

async function loadConnection(quiet = false) {
  connLoading.value = true
  try {
    conn.value = await getConnection()
  } catch (e) {
    const err = e as Error & { status?: number }
    // 404 = 尚未配置连接（正常初装态）
    if (err.status === 404) {
      conn.value = null
    } else {
      conn.value = null
      if (!quiet) ElMessage.error(err.message)
    }
  } finally {
    connLoading.value = false
    connLoaded.value = true
  }
}

let loadSeq = 0

async function loadConfig() {
  if (!conn.value) return
  const seq = ++loadSeq
  configLoading.value = true
  try {
    const res = await getConfig()
    if (seq !== loadSeq) return // 连接已变化，丢弃过期响应
    config.value = res
    rawContent.value = res.content
  } catch (e) {
    if (seq !== loadSeq) return
    const err = e as Error & { detail?: { content?: string; format?: string; path?: string } }
    if (err.detail?.content) {
      // 远端配置无法解析（如被外部手工改坏）：回填原文供文本模式修复，
      // 避免进入可视化空表单把远端配置覆盖成空。
      config.value = null
      rawContent.value = err.detail.content
      mode.value = 'raw'
      ElMessage.warning(`${err.message}，已切换到原文编辑以供修复`)
    } else {
      config.value = null
      rawContent.value = ''
      ElMessage.warning(err.message)
    }
  } finally {
    if (seq === loadSeq) configLoading.value = false
  }
}

function openHostInfo() {
  hostInfoVisible.value = true
}

async function onHostInfoSaved(saved: Connection) {
  conn.value = saved
  ElMessage.success('连接配置已保存')
  dirty.value = false
  config.value = null
  rawContent.value = ''
  lastResult.value = null
  await loadConfig()
}

async function onSave() {
  // 可视化模式需已成功解析出结构化配置（否则会用空模板覆盖远端）
  if (mode.value === 'visual' && !config.value) {
    ElMessage.warning('当前配置未成功解析，无法可视化保存；请切换到「配置文件编辑」修复后保存')
    return
  }
  try {
    await ElMessageBox.confirm(
      '保存后将备份远端原配置、应用新配置并重启 frpc 服务；若重启失败会自动回滚。',
      '确认保存并重启',
      { confirmButtonText: '保存并重启', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  saving.value = true
  lastResult.value = null
  try {
    const req =
      mode.value === 'visual'
        ? { structured: buildStructured(), format: config.value?.format || 'auto' }
        : { content: rawContent.value, format: 'auto' }
    const res = await saveConfig(req)
    lastResult.value = res
    dirty.value = false
    if (res.syntax_ok && res.applied && !res.rolled_back) {
      ElMessage.success('配置已应用并重启 frpc')
    } else if (res.rolled_back) {
      ElMessage.warning('重启失败，已自动回滚')
    } else {
      ElMessage.error(res.error || res.syntax_error || '保存失败')
    }
    // 保存成功后刷新原文（回显服务端最终内容，含重新序列化结果）
    if (res.syntax_ok) await loadConfig()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    saving.value = false
  }
}

function buildStructured(): FrpcConfig {
  return {
    format: (config.value?.format || 'auto') as FrpcConfig['format'],
    server: { ...(config.value?.server || { server_addr: '', server_port: 0, token: '' }) },
    proxies: [...(config.value?.proxies || [])],
  }
}

function onDirty() {
  dirty.value = true
}

async function onModeChange(next: EditMode) {
  if (next === mode.value) return
  // 有未保存修改时切模式会丢失另一视图的编辑（两视图各自维护数据源），
  // 提醒用户先保存，避免静默丢弃。
  if (dirty.value) {
    try {
      await ElMessageBox.confirm(
        '当前有未保存的修改。切换模式不会携带这些修改（可视化与原文各自独立维护），确定继续切换吗？',
        '切换编辑模式',
        { confirmButtonText: '继续切换', cancelButtonText: '取消', type: 'warning' },
      )
    } catch {
      return
    }
  }
  mode.value = next
}

onMounted(async () => {
  await loadConnection(true)
  if (conn.value) await loadConfig()
})
</script>

<template>
  <div class="app">
    <!-- 顶栏 -->
    <header class="topbar">
      <div class="brand">
        <span class="brand-title">frpc-admin</span>
        <span class="brand-sub">frpc 配置管理</span>
      </div>
      <div class="topbar-right">
        <el-tag v-if="connected" type="success" size="default" effect="plain">{{ connLabel }}</el-tag>
        <el-button :icon="undefined" @click="openHostInfo">
          <span class="btn-icon">⚙</span> 连接配置
        </el-button>
      </div>
    </header>

    <!-- 主体 -->
    <main class="body">
      <template v-if="!connLoaded">
        <el-empty description="正在加载连接配置..." />
      </template>

      <template v-else-if="!connected">
        <el-empty description="尚未配置 SSH 连接，请先点击「连接配置」设置目标主机">
          <template #default>
            <el-button type="primary" @click="openHostInfo">去配置连接</el-button>
          </template>
        </el-empty>
      </template>

      <template v-else>
        <!-- 操作行：模式切换 + 日志/备份 + 保存 -->
        <div class="action-bar">
          <div class="action-left">
            <el-radio-group :model-value="mode" size="default" @change="onModeChange">
              <el-radio-button value="visual">可视化编辑</el-radio-button>
              <el-radio-button value="raw">配置文件编辑</el-radio-button>
            </el-radio-group>
            <el-button size="default" @click="logsVisible = true">
              日志
            </el-button>
            <el-button size="default" @click="backupsVisible = true">
              历史备份
            </el-button>
          </div>
          <div class="action-right">
            <el-tag v-if="dirty" type="primary" size="default" effect="plain">有未保存修改</el-tag>
            <el-button type="primary" :loading="saving" @click="onSave">
              保存并重启
            </el-button>
          </div>
        </div>

        <!-- 编辑区 -->
        <div v-loading="configLoading" class="editor-area">
          <VisualEditor
            v-if="mode === 'visual'"
            v-model:config="config"
            :disabled="configLoading"
            @dirty="onDirty"
          />
          <RawEditor
            v-else
            v-model:content="rawContent"
            :format="config?.format || 'auto'"
            :disabled="configLoading"
            @dirty="onDirty"
          />
        </div>

        <!-- 结果回显 -->
        <ResultPanel v-if="lastResult" :result="lastResult" class="result-panel" />
      </template>
    </main>

    <!-- 连接配置弹窗 -->
    <HostInfoDialog
      v-model="hostInfoVisible"
      :existing="conn || undefined"
      @saved="onHostInfoSaved"
    />

    <!-- 日志 / 历史备份弹窗 -->
    <LogsDialog v-model="logsVisible" />
    <BackupsDialog v-model="backupsVisible" />
  </div>
</template>

<style scoped>
.app {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 16px;
  height: 56px;
  background: var(--el-bg-color);
  border-bottom: 1px solid var(--el-border-color-light);
}

.brand {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.brand-title {
  font-size: 16px;
  font-weight: 600;
}

.brand-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.btn-icon {
  margin-right: 4px;
}

.body {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 16px;
  gap: 12px;
  min-height: 0;
  overflow-y: auto;
}

.action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 10px;
}

.action-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.action-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.editor-area {
  min-height: 0;
}

.result-panel {
  margin-top: 4px;
}
</style>
