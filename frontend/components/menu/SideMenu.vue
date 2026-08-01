<script setup lang="ts">
defineProps<{ collapsed: boolean }>()

const route = useRoute()
const { items } = useMenu()
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
  </el-menu>
</template>

<style scoped>
.side-menu {
  border-right: none;
  height: 100%;
  overflow-y: auto;
}
</style>
