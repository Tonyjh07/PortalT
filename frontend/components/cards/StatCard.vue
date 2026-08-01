<script setup lang="ts">
defineProps<{
  title: string
  value: string | number
  icon?: string
  color?: string
}>()
</script>

<template>
  <el-card shadow="hover">
    <div class="stat-card">
      <div class="stat-icon" :style="{ color: color || 'var(--el-color-primary)' }">
        <IconRenderer :icon="icon || 'mdi:chart-donut'" :size="28" />
      </div>
      <div>
        <div class="stat-value">{{ value }}</div>
        <div class="stat-title">{{ title }}</div>
      </div>
    </div>
  </el-card>
</template>

<style scoped>
.stat-card {
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 12px;
  background-color: var(--el-fill-color-light);
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  line-height: 1.2;
}

.stat-title {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
</style>
