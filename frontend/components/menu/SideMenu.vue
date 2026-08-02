<script setup lang="ts">
import { can } from '~/utils/permissions'

defineProps<{ collapsed: boolean }>()

const route = useRoute()
const { items } = useMenu()
const { user } = useAuth()

const canManage = computed(() => {
  const u = user.value
  return u?.role === 'admin' || can(u, 'user:manage')
})
</script>

<template>
  <el-menu
    :default-active="route.path"
    :collapse="collapsed"
    :collapse-transition="false"
    router
    class="side-menu"
  >
    <el-menu-item index="/dashboard">
      <IconRenderer icon="mdi:view-dashboard" />
      <template #title><span>仪表盘</span></template>
    </el-menu-item>
    <el-menu-item index="/vms">
      <IconRenderer icon="mdi:server" />
      <template #title><span>虚拟机</span></template>
    </el-menu-item>
    <MenuItem v-for="item in items" :key="item.id" :item="item" />
    <el-sub-menu v-if="canManage" index="/admin">
      <template #title>
        <IconRenderer icon="mdi:shield-account" />
        <span>权限管理</span>
      </template>
      <el-menu-item index="/users">
        <IconRenderer icon="mdi:account-group" />
        <template #title><span>用户管理</span></template>
      </el-menu-item>
      <el-menu-item index="/roles">
        <IconRenderer icon="mdi:shield-key" />
        <template #title><span>角色权限</span></template>
      </el-menu-item>
      <el-menu-item index="/plugins-admin">
        <IconRenderer icon="mdi:puzzle" />
        <template #title><span>插件管理</span></template>
      </el-menu-item>
    </el-sub-menu>
  </el-menu>
</template>

<style scoped>
.side-menu {
  border-right: none;
  height: 100%;
  overflow-y: auto;
}
</style>
