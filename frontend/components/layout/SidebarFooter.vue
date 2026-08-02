<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import type { Role } from '~/types'

defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{ toggle: [] }>()

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
  <div class="sidebar-footer">
    <div class="footer-row">
      <el-tooltip content="展开/收起侧栏" placement="right">
        <el-button text circle @click="emit('toggle')">
          <IconRenderer :icon="collapsed ? 'mdi:menu-open' : 'mdi:menu' " :size="20" />
        </el-button>
      </el-tooltip>
      <span v-if="!collapsed" class="footer-brand">PortalT</span>
      <el-tooltip :content="isDark ? '切换亮色' : '切换暗色'" placement="right">
        <el-button text circle @click="toggle">
          <IconRenderer :icon="isDark ? 'mdi:weather-sunny' : 'mdi:weather-night'" :size="18" />
        </el-button>
      </el-tooltip>
    </div>
    <el-dropdown class="footer-user" :disabled="collapsed" trigger="click">
      <span class="user-chip">
        <el-avatar :size="30">{{ user?.username?.slice(0, 1).toUpperCase() }}</el-avatar>
        <span v-if="!collapsed" class="user-meta">
          <span class="user-name">{{ user?.username }}</span>
          <el-tag v-if="currentRole" :type="currentRole.type" size="small" class="user-role">
            {{ currentRole.text }}
          </el-tag>
        </span>
      </span>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item @click="handleLogout">退出登录</el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </div>
</template>

<style scoped>
.sidebar-footer {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 10px;
  padding: 10px 12px;
  border-top: 1px solid var(--el-border-color-lighter);
  margin-top: auto;
}

.footer-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.footer-brand {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
}

.footer-user {
  width: 100%;
}

.user-chip {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  padding: 4px 6px;
  border-radius: 8px;
  cursor: pointer;
  box-sizing: border-box;
}

.user-chip:hover {
  background: var(--el-fill-color-light);
}

.user-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
}

.user-name {
  font-size: 13px;
  line-height: 1.3;
}

.user-role {
  margin-top: 2px;
}
</style>
