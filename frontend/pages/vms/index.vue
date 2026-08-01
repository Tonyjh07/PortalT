<script setup lang="ts">
import { ElMessage } from 'element-plus'
import type { VM } from '~/types'

definePageMeta({ middleware: 'auth' })

const { api } = useApi()

const vms = ref<VM[]>([])
const loading = ref(false)

async function loadVMs() {
  loading.value = true
  try {
    const res = await api<VM[]>('/vms')
    vms.value = res
  } catch (err) {
    ElMessage.error((err as { data?: { message?: string } })?.data?.message || '加载虚拟机列表失败')
  } finally {
    loading.value = false
  }
}

function fmtMemory(mb: number): string {
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`
}

onMounted(loadVMs)
</script>

<template>
  <div class="page-container">
    <div class="list-header">
      <h2 class="page-title">虚拟机</h2>
      <el-button :icon="undefined" @click="loadVMs">
        <IconRenderer icon="mdi:restart" /> 刷新
      </el-button>
    </div>

    <el-card shadow="never" v-loading="loading">
      <el-table
        :data="vms"
        @row-click="(row: VM) => navigateTo(`/vms/${row.id}`)"
        class="vm-table"
      >
        <el-table-column label="" width="56">
          <template #default="{ row }">
            <IconRenderer :icon="'mdi:server'" :size="20" color="var(--el-color-primary)" />
          </template>
        </el-table-column>
        <el-table-column prop="name" label="名称" min-width="180" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <VMStatusTag :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="CPU" width="90">
          <template #default="{ row }">{{ row.cpu }} 核</template>
        </el-table-column>
        <el-table-column label="内存" width="110">
          <template #default="{ row }">{{ fmtMemory(row.memory_mb) }}</template>
        </el-table-column>
        <el-table-column prop="ip_address" label="IP 地址" min-width="140">
          <template #default="{ row }">
            <span v-if="row.ip_address">{{ row.ip_address }}</span>
            <el-text v-else type="info">-</el-text>
          </template>
        </el-table-column>
        <el-table-column prop="host" label="宿主机" min-width="120" />
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <PowerActions :vm="row" @changed="loadVMs" />
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && !vms.length" description="暂无虚拟机" />
    </el-card>
  </div>
</template>

<style scoped>
.list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.vm-table {
  cursor: pointer;
}
</style>
