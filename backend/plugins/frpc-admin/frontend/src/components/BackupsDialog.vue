<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { BackupInfo, BackupContentResponse, SaveConfigResponse } from '../api'
import { getBackup, listBackups, restoreBackup } from '../api'
import ResultPanel from './ResultPanel.vue'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})

const loading = ref(false)
const restoring = ref(false)
const backups = ref<BackupInfo[]>([])
const preview = ref<BackupContentResponse | null>(null)
const previewLoading = ref(false)
const result = ref<SaveConfigResponse | null>(null)

function formatSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

function formatTime(ts: string): string {
  const sec = Number(ts)
  if (!Number.isFinite(sec) || sec <= 0) return ts
  const d = new Date(sec * 1000)
  const pad = (x: number) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

async function refresh() {
  loading.value = true
  result.value = null
  preview.value = null
  try {
    backups.value = (await listBackups()).backups
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    loading.value = false
  }
}

async function onPreview(row: BackupInfo) {
  previewLoading.value = true
  preview.value = null
  try {
    preview.value = await getBackup(row.ts)
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    previewLoading.value = false
  }
}

async function onRestore(row: BackupInfo) {
  try {
    await ElMessageBox.confirm(
      `恢复该备份会覆盖远端当前配置并重启 frpc 服务；恢复前将自动备份当前配置，若重启失败会自动回滚。确认恢复 ${formatTime(row.ts)} 的备份吗？`,
      '确认恢复备份',
      { confirmButtonText: '恢复并重启', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  restoring.value = true
  result.value = null
  try {
    const res = await restoreBackup(row.ts)
    result.value = res
    if (res.applied && !res.rolled_back) {
      ElMessage.success('备份已恢复并重启 frpc')
    } else if (res.rolled_back) {
      ElMessage.warning('重启失败，已自动回滚')
    } else {
      ElMessage.error(res.error || '恢复失败')
    }
    await refresh() // 恢复会新增一条"当前配置"备份
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally {
    restoring.value = false
  }
}

watch(visible, (v) => {
  if (v) refresh()
})
</script>

<template>
  <el-dialog v-model="visible" title="历史备份" width="820px" append-to-body>
    <div class="head-row">
      <span class="hint">保存/恢复前都会自动备份当前配置，最多保留 5 份。</span>
      <el-button size="small" :loading="loading" @click="refresh">刷新</el-button>
    </div>

    <el-table
      v-loading="loading"
      :data="backups"
      empty-text="暂无备份"
      size="default"
      row-key="ts"
    >
      <el-table-column label="时间" min-width="170">
        <template #default="{ row }">{{ formatTime(row.ts) }}</template>
      </el-table-column>
      <el-table-column prop="ts" label="时间戳" min-width="130" />
      <el-table-column label="大小" width="90">
        <template #default="{ row }">{{ formatSize(row.size) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="140" align="right">
        <template #default="{ row }">
          <el-button size="small" @click="onPreview(row)">查看内容</el-button>
          <el-button size="small" type="danger" :loading="restoring" @click="onRestore(row)">
            恢复
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-loading="previewLoading" class="preview-area">
      <template v-if="preview">
        <div class="preview-head">
          <span class="hint">备份内容（{{ formatTime(preview.ts) }}）</span>
          <span class="hint">{{ formatSize(preview.size) }}</span>
        </div>
        <pre class="preview-content">{{ preview.content }}</pre>
      </template>
    </div>

    <ResultPanel v-if="result" :result="result" class="result-panel" />
  </el-dialog>
</template>

<style scoped>
.head-row,
.preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}

.hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.preview-area {
  margin-top: 12px;
  min-height: 24px;
}

.preview-content {
  margin: 0;
  background: var(--el-fill-color);
  border-radius: 4px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: var(--el-font-family-mono);
  max-height: 300px;
  overflow: auto;
}

.result-panel {
  margin-top: 12px;
}
</style>