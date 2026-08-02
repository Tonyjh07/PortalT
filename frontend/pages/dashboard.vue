<script setup lang="ts">
import type { Plugin, VM, VMStatus } from '~/types'

definePageMeta({ middleware: 'auth' })

const { user } = useAuth()
const { load, items } = useMenu()

const vms = ref<VM[]>([])
const loading = ref(false)
const quickLinks = ref<Plugin[]>([])

const totalCpu = computed(() => vms.value.reduce((sum, vm) => sum + vm.cpu, 0))
const totalMemory = computed(() => vms.value.reduce((sum, vm) => sum + vm.memory_mb, 0))
const runningCount = computed(
  () => vms.value.filter((vm) => vm.status === ('poweredOn' as VMStatus)).length,
)

function fmtMemory(mb: number): string {
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`
}

// 插件一律导航到 /plugins<route>，由 pages/plugins/[...slug].vue 渲染（与侧栏一致）
function pluginNav(route: string) {
  return route.startsWith('/plugins') ? route : `/plugins${route}`
}

onMounted(async () => {
  loading.value = true
  try {
    const { api } = useApi()
    const res = await api<VM[]>('/vms')
    vms.value = res
  } finally {
    loading.value = false
  }
  if (!items.value.length) {
    try {
      await load()
    } catch {
      /* viewer 无菜单权限 */
    }
  }
  quickLinks.value = items.value
    .flatMap((item) => (item.children?.length ? item.children : [item]))
    .filter((p) => !p.permission || can(user.value, p.permission))
    .slice(0, 6)
})
</script>

<template>
  <div class="page-container">
    <h2 class="page-title">仪表盘</h2>
    <el-row :gutter="16" v-loading="loading">
      <el-col :xs="12" :sm="6">
        <CardsStatCard title="虚拟机总数" :value="vms.length" icon="mdi:server" />
      </el-col>
      <el-col :xs="12" :sm="6">
        <CardsStatCard title="运行中" :value="runningCount" icon="mdi:play" color="#67c23a" />
      </el-col>
      <el-col :xs="12" :sm="6">
        <CardsStatCard title="总 CPU 核数" :value="totalCpu" icon="mdi:memory" color="#e6a23c" />
      </el-col>
      <el-col :xs="12" :sm="6">
        <CardsStatCard title="总内存" :value="fmtMemory(totalMemory)" icon="mdi:database" color="#f56c6c" />
      </el-col>
    </el-row>

    <el-row :gutter="16" class="mt-4">
      <el-col :xs="24" :lg="14">
        <el-card shadow="never">
          <template #header>
            <div class="card-header">
              <span>最近虚拟机</span>
              <el-button text type="primary" @click="navigateTo('/vms')">查看全部</el-button>
            </div>
          </template>
          <el-table :data="vms.slice(0, 5)" size="small" @row-click="(row: VM) => navigateTo(`/vms/${row.id}`)">
            <el-table-column prop="name" label="名称" min-width="160" />
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <VMStatusTag :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column prop="ip_address" label="IP 地址" min-width="130" />
            <el-table-column label="内存" width="100">
              <template #default="{ row }">{{ fmtMemory(row.memory_mb) }}</template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!loading && !vms.length" description="暂无虚拟机" :image-size="80" />
        </el-card>
      </el-col>
      <el-col :xs="24" :lg="10">
        <el-card shadow="never">
          <template #header>
            <div class="card-header"><span>快速启动</span></div>
          </template>
          <div v-if="quickLinks.length" class="quick-grid">
            <div v-for="p in quickLinks" :key="p.id" class="quick-item" @click="navigateTo(pluginNav(p.route))">
              <IconRenderer :icon="p.icon" :size="22" />
              <span>{{ p.name }}</span>
            </div>
          </div>
          <el-empty v-else description="暂无可用插件" :image-size="80" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.mt-4 {
  margin-top: 16px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.quick-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}

.quick-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 16px 8px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
}

.quick-item:hover {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}
</style>
