<script setup lang="ts">
import * as Icons from '@element-plus/icons-vue'
import { computed } from 'vue'

const props = withDefaults(defineProps<{ icon?: string; size?: number }>(), {
  icon: 'mdi:menu',
  size: 16,
})

const ICON_MAP: Record<string, string> = {
  'mdi:home': 'HomeFilled',
  'mdi:view-dashboard': 'Odometer',
  'mdi:server': 'Monitor',
  'mdi:folder': 'Folder',
  'mdi:monitor': 'Monitor',
  'mdi:chart-box': 'TrendCharts',
  'mdi:cog': 'Setting',
  'mdi:tools': 'Tools',
  'mdi:web': 'Grid',
  'mdi:lan': 'Connection',
  'mdi:database': 'Coin',
  'mdi:video': 'VideoCamera',
  'mdi:cloud': 'Cloudy',
  'mdi:rocket': 'Promotion',
  'mdi:package': 'Box',
  'mdi:chart-donut': 'PieChart',
  'mdi:account': 'User',
  'mdi:weather-sunny': 'Sunny',
  'mdi:weather-night': 'Moon',
  'mdi:play': 'VideoPlay',
  'mdi:stop': 'VideoPause',
  'mdi:restart': 'RefreshRight',
  'mdi:logout': 'SwitchButton',
  'mdi:power': 'Power',
  'mdi:desktop': 'Monitor',
  'mdi:memory': 'Cpu',
}

const componentName = computed(() => {
  if (!props.icon) return 'Menu'
  const fromMap = ICON_MAP[props.icon]
  if (fromMap) return fromMap
  const name = props.icon.split(':').pop() || 'Menu'
  const pascal = name.replace(/(^|[-_])(\w)/g, (_, __, c) => c.toUpperCase())
  return pascal in Icons ? pascal : 'Menu'
})
</script>

<template>
  <el-icon :size="size">
    <component :is="Icons[componentName as keyof typeof Icons]" />
  </el-icon>
</template>
