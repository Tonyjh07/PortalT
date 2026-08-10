<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  type ConfigResponse,
  type Connection,
  type FrpcConfig,
  type SaveConfigResponse,
  type VM,
  getConfig,
  listConnections,
  listVMs,
  saveConfig,
} from './api'
import HostInfoDialog from './components/HostInfoDialog.vue'
import VisualEditor from './components/VisualEditor.vue'
import RawEditor from './components/RawEditor.vue'
import ResultPanel from './components/ResultPanel.vue'

type EditMode = 'visual' | 'raw'

const vms = ref<VM[]>([])
const vmLoading = ref(false)
const selectedVM = ref<string>('')
const connections = ref<Connection[]>([])
const connByVm = computed(() => new Map(connections.value.map((c) => [c.vm_id, c])))

const mode = ref<EditMode>('visual')
const config = ref<ConfigResponse | null>(null)
const configLoading = ref(false)
const rawContent = ref('')
const hostInfoVisible = ref(false)
const saving = ref(false)
const lastResult = ref<SaveConfigResponse | null>(null)
const dirty = ref(false)
const connected = computed(() => !!selectedVM.value && connByVm.value.has(selectedVM.value))

async function loadVMs() {
  vmLoading.value = true
  try {
    vms.value = await listVMs()
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    vmLoading.value = false
  }
}

async function loadConnections() {
  try {
    connections.value = await listConnections()
  } catch (e) {
    ElMessage.error((e as Error).message)
  }
}

let loadSeq = 0

async function onVMChange(id: string) {
  if (!id) return
  if (dirty.value) {
    try {
      await ElMessageBox.confirm(
        '当前 VM 有未保存的修改，切换后将丢失。确定切换吗？',
        '切换 VM',
        { confirmButtonText: '切换', cancelButtonText: '取消', type: 'warning' },
      )
    } catch {
      return
    }
  }
  dirty.value = false
  config.value = null
  rawContent.value = ''
  lastResult.value = null
  await loadConfig(id)
}

async function loadConfig(vmId: string) {
  const seq = ++loadSeq
  configLoading.value = true
  try {
    const res = await getConfig(vmId)
    if (seq !== loadSeq) return // 已切换到其它 VM，丢弃过期响应
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

async function onHostInfoSaved(conn: Connection) {
  connections.value = await listConnections()
  ElMessage.success('主机信息已保存')
  // 已有连接配置：刷新当前 VM 配置
  if (selectedVM.value && connByVm.value.has(selectedVM.value)) {
    await loadConfig(selectedVM.value)
  }
}

async function onSave() {
  const vmId = selectedVM.value
  if (!vmId) return
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
    const res = await saveConfig(vmId, req)
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
    if (res.syntax_ok) await loadConfig(vmId)
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

function fmtVmLabel(vm: VM): string {
  return vm.ip_address ? `${vm.name}（${vm.ip_address}）` : vm.name
}

onMounted(async () => {
  await Promise.all([loadVMs(), loadConnections()])
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
        <el-select
          v-model="selectedVM"
          placeholder="选择 VM"
          :loading="vmLoading"
          size="default"
          class="vm-select"
          filterable
          @change="onVMChange"
        >
          <el-option v-for="vm in vms" :key="vm.id" :label="fmtVmLabel(vm)" :value="vm.id" />
        </el-select>
        <el-button :icon="undefined" @click="openHostInfo">
          <span class="btn-icon">⚙</span> 主机信息
        </el-button>
      </div>
    </header>

    <!-- 主体 -->
    <main class="body">
      <template v-if="!selectedVM">
        <el-empty description="请先在上方选择一个 VM 开始管理" />
      </template>

      <template v-else>
        <!-- 操作行：模式切换 + 保存 -->
        <div class="action-bar">
          <el-radio-group :model-value="mode" size="default" @change="onModeChange">
            <el-radio-button value="visual">可视化编辑</el-radio-button>
            <el-radio-button value="raw">配置文件编辑</el-radio-button>
          </el-radio-group>
          <div class="action-right">
            <el-tag v-if="!connected" type="warning" size="default" effect="plain">
              未配置连接，请先打开「主机信息」
            </el-tag>
            <el-tag v-else-if="dirty" type="primary" size="default" effect="plain">有未保存修改</el-tag>
            <el-button type="primary" :loading="saving" :disabled="!connected" @click="onSave">
              保存并重启
            </el-button>
          </div>
        </div>

        <!-- 编辑区 -->
        <div v-loading="configLoading" class="editor-area">
          <VisualEditor
            v-if="mode === 'visual'"
            v-model:config="config"
            :disabled="!connected || configLoading"
            @dirty="onDirty"
          />
          <RawEditor
            v-else
            v-model:content="rawContent"
            :format="config?.format || 'auto'"
            :disabled="!connected || configLoading"
            @dirty="onDirty"
          />
        </div>

        <!-- 结果回显 -->
        <ResultPanel v-if="lastResult" :result="lastResult" class="result-panel" />
      </template>
    </main>

    <!-- 主机信息弹窗 -->
    <HostInfoDialog
      v-model="hostInfoVisible"
      :vm-id="selectedVM"
      :vm-name="vms.find((v) => v.id === selectedVM)?.name"
      :existing="selectedVM ? connByVm.get(selectedVM) : undefined"
      @saved="onHostInfoSaved"
    />
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

.vm-select {
  width: 240px;
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
