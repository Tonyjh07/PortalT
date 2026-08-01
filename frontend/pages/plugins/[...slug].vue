<script setup lang="ts">
import type { MenuItem } from '~/types'

definePageMeta({ middleware: 'auth' })

const route = useRoute()
const { items, load, findByRoute } = useMenu()

const plugin = ref<MenuItem | null>(null)
const notFound = ref(false)

async function sync() {
  const path = '/' + (route.params.slug as string[]).join('/')
  if (!items.value.length) {
    try {
      await load()
    } catch {
      notFound.value = true
      return
    }
  }
  const found = findByRoute(path)
  if (found) {
    plugin.value = found
    notFound.value = false
  } else {
    notFound.value = true
  }
}

onMounted(sync)
watch(() => route.params.slug, sync)
</script>

<template>
  <div class="plugin-page">
    <iframe
      v-if="plugin"
      :src="plugin.iframe_url"
      class="vm-iframe"
      title="plugin"
    />
    <div v-else class="plugin-empty">
      <el-empty :description="notFound ? '插件不存在或无权访问' : '正在加载...'" />
    </div>
  </div>
</template>

<style scoped>
.plugin-page {
  height: 100%;
}

.plugin-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
</style>
