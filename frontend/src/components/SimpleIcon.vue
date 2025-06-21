<template>
  <span :style="{ fontSize: size + 'px' }">{{ iconMap[name] || '•' }}</span>
</template>

<script setup lang="ts">
interface Props {
  name: string
  size?: number
}

withDefaults(defineProps<Props>(), {
  size: 16
})

// 图标映射表
const iconMap: Record<string, string> = {
  // 导航图标
  dashboard: '📊',
  home: '🏠',
  email: '📧',
  mail: '📧',
  download: '⬇️',
  settings: '⚙️',
  statistics: '📈',
  chart: '📈',
  logs: '📋',
  document: '📋',
  
  // 操作图标
  add: '➕',
  edit: '✏️',
  delete: '🗑️',
  save: '💾',
  cancel: '❌',
  refresh: '🔄',
  search: '🔍',
  filter: '🔽',
  
  // 状态图标
  success: '✅',
  error: '❌',
  warning: '⚠️',
  info: 'ℹ️',
  loading: '⏳',
  
  // 文件图标
  file: '📄',
  folder: '📁',
  pdf: '📄',
  
  // 网络图标
  wifi: '📶',
  sync: '🔄',
  
  // 其他
  menu: '☰',
  close: '✖️',
  minimize: '➖',
  notification: '🔔',
  account: '👤',
  play: '▶️',
  pause: '⏸️',
  stop: '⏹️'
}
</script> 