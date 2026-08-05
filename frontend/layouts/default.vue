<script setup lang="ts">
const { token, user, fetchMe, logout } = useAuth()
const { load, loaded } = useMenu()
const { init } = useTheme()

const collapsed = ref(false)
const { isMobile } = useIsMobile()
const drawerOpen = ref(false)
const route = useRoute()

// 路由切换时自动收起移动端抽屉
watch(() => route.fullPath, () => {
  drawerOpen.value = false
})

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
    <el-aside
      v-if="!isMobile"
      :width="collapsed ? 'var(--portalt-sidebar-collapsed-width)' : 'var(--portalt-sidebar-width)'"
      class="app-aside"
    >
      <div class="logo">
        <IconRenderer icon="mdi:server" :size="24" />
        <span v-if="!collapsed" class="logo-text">PortalT</span>
      </div>
      <MenuSideMenu :collapsed="collapsed" class="side-menu-scroll" />
      <LayoutSidebarFooter :collapsed="collapsed" class="side-footer" @toggle="collapsed = !collapsed" />
    </el-aside>

    <template v-else>
      <header class="mobile-topbar">
        <el-button text circle class="menu-btn" @click="drawerOpen = true">
          <IconRenderer icon="mdi:menu" :size="22" />
        </el-button>
        <div class="mobile-logo">
          <IconRenderer icon="mdi:server" :size="20" color="var(--el-color-primary)" />
          <span>PortalT</span>
        </div>
      </header>
      <el-drawer v-model="drawerOpen" direction="ltr" :size="240" :with-header="false" class="mobile-drawer">
        <div class="drawer-inner">
          <div class="logo">
            <IconRenderer icon="mdi:server" :size="24" />
            <span class="logo-text">PortalT</span>
          </div>
          <MenuSideMenu :collapsed="false" class="side-menu-scroll" />
          <LayoutSidebarFooter :collapsed="false" class="side-footer" @toggle="drawerOpen = false" />
        </div>
      </el-drawer>
    </template>

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

/* 移动端：固定侧栏由顶栏 + 抽屉替代，整体改纵向排列，
   否则 el-container 默认横向，顶栏会被挤成左侧竖条 */
@media (max-width: 767px) {
  .app-layout {
    flex-direction: column;
  }
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

.mobile-topbar {
  display: flex;
  align-items: center;
  gap: 4px;
  height: var(--portalt-header-height);
  padding: 0 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  background-color: var(--el-bg-color);
  flex-shrink: 0;
}

.mobile-topbar .menu-btn {
  font-size: 20px;
}

.mobile-logo {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 700;
  color: var(--el-color-primary);
}

.mobile-drawer :deep(.el-drawer__body) {
  padding: 0;
}

.drawer-inner {
  display: flex;
  flex-direction: column;
  height: 100%;
}
</style>
