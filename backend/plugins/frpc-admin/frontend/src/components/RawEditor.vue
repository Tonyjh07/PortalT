<script setup lang="ts">
defineProps<{
  content: string
  format?: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:content', v: string): void
  (e: 'dirty'): void
}>()

function onChange(v: string) {
  emit('update:content', v)
  emit('dirty')
}
</script>

<template>
  <div class="raw-editor">
    <el-card shadow="never">
      <template #header>
        <div class="card-head">
          <span>配置文件原文</span>
          <el-tag size="small" :type="format === 'toml' ? 'warning' : 'info'">
            {{ (format || 'auto').toUpperCase() }}
          </el-tag>
        </div>
      </template>
      <div class="hint">
        直接编辑完整配置。保存时服务端会做语法检查（自动检测格式），失败不落盘。
      </div>
      <el-input
        :model-value="content"
        type="textarea"
        :rows="18"
        class="raw-input"
        placeholder="# frpc 配置文件"
        :disabled="disabled"
        spellcheck="false"
        @update:model-value="onChange"
      />
    </el-card>
  </div>
</template>

<style scoped>
.raw-editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 8px;
}

.raw-input :deep(textarea) {
  font-family: var(--el-font-family-mono);
  font-size: 12.5px;
  line-height: 1.6;
  tab-size: 4;
}
</style>
