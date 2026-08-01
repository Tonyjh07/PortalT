<script setup lang="ts">
import type { VMStatus } from '~/types'

const props = defineProps<{ status: VMStatus }>()

const STATUS_META: Record<VMStatus, { label: string; type: 'success' | 'info' | 'warning' | 'danger' }> = {
  poweredOn: { label: '运行中', type: 'success' },
  poweredOff: { label: '已关机', type: 'info' },
  suspended: { label: '已挂起', type: 'warning' },
  unknown: { label: '未知', type: 'danger' },
}

const meta = computed(() => STATUS_META[props.status] || STATUS_META.unknown)
</script>

<template>
  <el-tag :type="meta.type" size="small" effect="light">
    <span class="status-dot" :class="`is-${status}`" />
    {{ meta.label }}
  </el-tag>
</template>

<style scoped>
.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  vertical-align: middle;
}

.status-dot.is-poweredOn {
  background-color: #67c23a;
  box-shadow: 0 0 6px #67c23a;
}

.status-dot.is-poweredOff {
  background-color: #909399;
}

.status-dot.is-suspended {
  background-color: #e6a23c;
}

.status-dot.is-unknown {
  background-color: #f56c6c;
}
</style>
