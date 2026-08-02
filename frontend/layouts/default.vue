<script setup lang="ts">
const { token, user, fetchMe, logout } = useAuth()
const { load, loaded } = useMenu()
const { init } = useTheme()

const collapsed = ref(false)

onMounted(async () => {
  init()
  if (!token.value) {
    return navigateTo('/login')
  }
  // 刷新/热重载后 token 从 cookie 恢复但 user 为 null：重新拉取当前用户；
  // 令牌已失效时清空登录态并回登录页
  if (!user.value) {
    const me = await fetchMe()
    if (!me) {
      logout()
      return navigateTo('/login')
    }
  }
  try {
    if (!loaded.value) await load()
  } catch {
    logout()
    navigateTo('/login')
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
      <MenuSideMenu :collapsed="collapsed" class="side-menu-scroll" />
      <LayoutSidebarFooter :collapsed="collapsed" class="side-footer" @toggle="collapsed = !collapsed" />
    </el-aside>
    <el-container>
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

.side-menu-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.side-footer {
  flex-shrink: 0;
}

.app-main {
  padding: 0;
  overflow-y: auto;
}
</style>
