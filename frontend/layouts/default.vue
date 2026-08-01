<script setup lang="ts">
const { token } = useAuth()
const { load, loaded } = useMenu()
const { init } = useTheme()

const collapsed = ref(false)

onMounted(async () => {
  init()
  if (!token.value) {
    return navigateTo('/login')
  }
  try {
    if (!loaded.value) await load()
  } catch {
    if (process.client) {
      const { logout } = useAuth()
      logout()
      navigateTo('/login')
    }
  }
})
</script>

<template>
  <el-container class="app-layout">
    <el-aside :width="collapsed ? 'var(--portalt-sidebar-collapsed-width)' : 'var(--portalt-sidebar-width)'" class="app-aside">
      <div class="logo">
        <IconRenderer icon="mdi:server" :size="24" />
        <span v-if="!collapsed" class="logo-text">PortalT</span>
      </div>
      <SideMenu :collapsed="collapsed" />
    </el-aside>
    <el-container>
      <AppHeader @toggle="collapsed = !collapsed" />
      <el-main class="app-main">
        <slot />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.app-layout {
  height: 100%;
}

.app-aside {
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--el-border-color-lighter);
  background-color: var(--el-bg-color);
  transition: width 0.2s;
  overflow: hidden;
}

.logo {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: var(--portalt-header-height);
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  color: var(--el-color-primary);
}

.app-main {
  padding: 0;
  overflow-y: auto;
}
</style>
