<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { LogsResponse } from '../api'
import { getLogs } from '../api'

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

const linesOptions = [100, 200, 500, 1000]
const lines = ref(200)
const loading = ref(false)
const result = ref<LogsResponse | null>(null)

async function refresh() {
  loading.value = true
  try {
    result.value = await getLogs(lines.value)
  } catch (e) {
    result.value = {
      source: 'journal',
      lines: lines.value,
      error: (e as Error).message,
    }
  } finally {
    loading.value = false
  }
}

watch(visible, (v) => {
  if (v) {
    result.value = null
    refresh()
  }
})
</script>

<template>
  <el-dialog v-model="visible" title="frpc 日志" width="760px" append-to-body>
    <div class="toolbar">
      <div class="toolbar-left">
        <span class="source-label">来源</span>
        <el-tag v-if="result" :type="result.source === 'file' ? 'primary' : 'info'" size="small">
          {{ result.source === 'file' ? '日志文件' : 'systemd journal' }}
        </el-tag>
        <code v-if="result?.path" class="source-path">{{ result.path }}</code>
      </div>
      <div class="toolbar-right">
        <span class="lines-label">行数</span>
        <el-select v-model="lines" size="small" style="width: 120px">
          <el-option v-for="n in linesOptions" :key="n" :label="`最近 ${n} 行`" :value="n" />
        </el-select>
        <el-button size="small" :loading="loading" @click="refresh">刷新</el-button>
      </div>
    </div>

    <el-alert
      v-if="result?.error"
      type="warning"
      :closable="false"
      show-icon
      :title="result.error"
      class="error-alert"
    />

    <el-empty v-if="!loading && result && !result.content && !result.error" description="暂无日志输出" />
    <pre v-else-if="result?.content" class="log-output">{{ result.content }}</pre>
    <div v-loading="loading" class="load-mask" />
  </el-dialog>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.source-label,
.lines-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.source-path {
  font-size: 12px;
  word-break: break-all;
}

.log-output {
  margin: 0;
  background: var(--el-fill-color);
  border-radius: 4px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: var(--el-font-family-mono);
  max-height: 60vh;
  overflow: auto;
  min-height: 200px;
}

.error-alert {
  margin-bottom: 10px;
}

.load-mask {
  min-height: 24px;
}
</style>