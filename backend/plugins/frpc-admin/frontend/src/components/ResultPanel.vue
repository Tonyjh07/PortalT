<script setup lang="ts">
import type { SaveConfigResponse } from '../api'

defineProps<{
  result: SaveConfigResponse
}>()

function statusTag(result: SaveConfigResponse) {
  if (result.error) return { type: 'danger' as const, text: '失败' }
  if (!result.syntax_ok) return { type: 'danger' as const, text: '语法错误' }
  if (result.rolled_back) return { type: 'warning' as const, text: '已回滚' }
  if (result.applied) return { type: 'success' as const, text: '已应用' }
  return { type: 'danger' as const, text: '失败' }
}
</script>

<template>
  <el-card shadow="never" class="result-card">
    <template #header>
      <div class="result-head">
        <span>保存结果</span>
        <el-tag :type="statusTag(result).type" size="small">
          {{ statusTag(result).text }}
        </el-tag>
      </div>
    </template>
    <div class="result-body">
      <template v-if="result.error">
        <el-alert type="error" :closable="false" show-icon :title="result.error" />
      </template>

      <template v-if="result.syntax_ok">
        <div class="row"><span class="k">语法检查</span><span class="v ok">通过</span></div>
        <div class="row" v-if="result.backup_path">
          <span class="k">备份位置</span><code class="v">{{ result.backup_path }}</code>
        </div>
        <div class="row" v-if="result.applied">
          <span class="k">应用</span><span class="v ok">已写入并重启 frpc</span>
        </div>
        <div class="row" v-if="result.rolled_back">
          <span class="k">回滚</span>
          <span class="v warn">
            已恢复备份{{ result.rollback_error ? '，但' : '，重启恢复成功' }}
          </span>
        </div>
        <div class="row" v-if="result.restart_output">
          <span class="k">重启输出</span>
          <pre class="out">{{ result.restart_output }}</pre>
        </div>
        <div class="row" v-if="result.rollback_error">
          <span class="k">回滚错误</span>
          <el-alert type="warning" :closable="false" show-icon :title="result.rollback_error" class="full" />
        </div>
      </template>

      <template v-if="!result.syntax_ok && result.syntax_error">
        <el-alert type="error" :closable="false" show-icon :title="result.syntax_error" class="full" />
      </template>
    </div>
  </el-card>
</template>

<style scoped>
.result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.result-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  font-size: 13px;
}

.k {
  min-width: 64px;
  color: var(--el-text-color-secondary);
  flex-shrink: 0;
}

.v {
  word-break: break-all;
}

.v.ok {
  color: var(--el-color-success);
}

.v.warn {
  color: var(--el-color-warning);
}

.out {
  margin: 0;
  flex: 1;
  background: var(--el-fill-color);
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  font-family: var(--el-font-family-mono);
  max-height: 140px;
  overflow: auto;
}

.full {
  flex: 1;
}
</style>
