<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import type { Role } from '~/types'

const emit = defineEmits<{ toggle: [] }>()

const route = useRoute()
const router = useRouter()
const { user, logout } = useAuth()
const { isDark, toggle } = useTheme()

const roleLabel: Record<Role, { text: string; type: 'success' | 'primary' | 'info' }> = {
  admin: { text: '管理员', type: 'success' },
  user: { text: '普通用户', type: 'primary' },
  viewer: { text: '访客', type: 'info' },
}

const currentRole = computed(() => (user.value ? roleLabel[user.value.role] : null))

async function handleLogout() {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '退出',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  logout()
  ElMessage.success('已退出登录')
  router.push('/login')
}
</script>

<template>
  <header class="app-header">
    <div class="header-left">
      <el-button text circle @click="emit('toggle')">
        <IconRenderer icon="mdi:menu" :size="20" />
      </el-button>
      <el-breadcrumb separator="/">
        <el-breadcrumb-item>PortalT</el-breadcrumb-item>
        <el-breadcrumb-item v-if="route.path !== '/dashboard'">
          {{ route.meta.title || route.name }}
        </el-breadcrumb-item>
      </el-breadcrumb>
    </div>
    <div class="header-right">
      <el-tooltip :content="isDark ? '切换亮色' : '切换暗色'">
        <el-button text circle @click="toggle">
          <IconRenderer :icon="isDark ? 'mdi:weather-sunny' : 'mdi:weather-night'" :size="18" />
        </el-button>
      </el-tooltip>
      <el-dropdown>
        <span class="user-chip">
          <el-avatar :size="30">{{ user?.username?.slice(0, 1).toUpperCase() }}</el-avatar>
          <span class="user-name">{{ user?.username }}</span>
          <el-tag v-if="currentRole" :type="currentRole.type" size="small">
            {{ currentRole.text }}
          </el-tag>
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item @click="handleLogout">退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </header>
</template>

<style scoped>
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--portalt-header-height);
  padding: 0 16px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  background-color: var(--el-bg-color);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.user-name {
  font-size: 14px;
}
</style>
