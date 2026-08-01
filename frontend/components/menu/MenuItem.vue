<script setup lang="ts">
import type { MenuItem } from '~/types'

defineProps<{ item: MenuItem }>()
</script>

<template>
  <el-menu-item v-if="!item.children?.length" :index="item.route">
    <IconRenderer :icon="item.icon" class="menu-icon" />
    <template #title>
      <span>{{ item.name }}</span>
    </template>
  </el-menu-item>
  <el-sub-menu v-else :index="item.route">
    <template #title>
      <IconRenderer :icon="item.icon" class="menu-icon" />
      <span>{{ item.name }}</span>
    </template>
    <MenuItem v-for="child in item.children" :key="child.id" :item="child" />
  </el-sub-menu>
</template>

<style scoped>
.menu-icon {
  margin-right: 4px;
}
</style>
