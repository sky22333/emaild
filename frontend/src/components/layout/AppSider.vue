<template>
  <div class="app-sider">
    <!-- Logo区域 -->
    <div class="sider-header" :class="{ collapsed: collapsed }">
      <div class="logo">
        <n-icon size="28" class="logo-icon">
          📧
        </n-icon>
        <Transition name="fade">
          <span v-if="!collapsed" class="logo-text">PDF下载器</span>
        </Transition>
      </div>
    </div>

    <!-- 导航菜单 -->
    <n-menu
      :collapsed="collapsed"
      :collapsed-width="64"
      :collapsed-icon-size="22"
      :options="menuOptions"
      :value="activeKey"
      @update:value="handleMenuSelect"
      class="sider-menu"
    />

    <!-- 底部状态指示器 -->
    <div class="sider-footer" :class="{ collapsed: collapsed }">
      <div class="service-status">
        <div class="status-item">
          <span class="status-dot" :class="serviceStatus.email ? 'active' : 'inactive'"></span>
          <Transition name="fade">
            <span v-if="!collapsed" class="status-text">邮件服务</span>
          </Transition>
        </div>
        <div class="status-item">
          <span class="status-dot" :class="serviceStatus.download ? 'active' : 'inactive'"></span>
          <Transition name="fade">
            <span v-if="!collapsed" class="status-text">下载服务</span>
          </Transition>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NIcon, NMenu } from 'naive-ui'

interface Props {
  collapsed: boolean
}

defineProps<Props>()

const router = useRouter()
const route = useRoute()

// 模拟服务状态
const serviceStatus = ref({
  email: true,
  download: true
})

// 当前激活的菜单项
const activeKey = computed(() => route.name as string)

// 渲染文本图标的辅助函数
const renderIcon = (emoji: string) => {
  return () => h('span', { style: { fontSize: '18px' } }, emoji)
}

// 菜单选项
const menuOptions = computed(() => [
  {
    label: '仪表盘',
    key: 'dashboard',
    icon: renderIcon('📊')
  },
  {
    label: '邮箱管理',
    key: 'emails',
    icon: renderIcon('📧')
  },
  {
    label: '下载任务',
    key: 'downloads',
    icon: renderIcon('⬇️')
  },
  {
    label: '统计分析',
    key: 'statistics',
    icon: renderIcon('📈')
  },
  {
    type: 'divider',
    key: 'divider'
  },
  {
    label: '应用设置',
    key: 'settings',
    icon: renderIcon('⚙️')
  },
  {
    label: '运行日志',
    key: 'logs',
    icon: renderIcon('📋')
  }
])

// 菜单选择处理
const handleMenuSelect = (key: string) => {
  if (key !== route.name) {
    router.push({ name: key })
  }
}
</script>

<style scoped>
.app-sider {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #fff;
  border-right: 1px solid #f0f0f0;
}

.sider-header {
  height: 64px;
  display: flex;
  align-items: center;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  transition: all 0.3s;
}

.sider-header.collapsed {
  padding: 0 20px;
  justify-content: center;
}

.logo {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-icon {
  color: #1890ff;
  flex-shrink: 0;
}

.logo-text {
  font-size: 18px;
  font-weight: 600;
  color: #262626;
}

.sider-menu {
  flex: 1;
  border-right: none !important;
  padding: 8px 0;
}

.sider-footer {
  padding: 16px 24px;
  border-top: 1px solid #f0f0f0;
  transition: all 0.3s;
}

.sider-footer.collapsed {
  padding: 16px 8px;
}

.service-status {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sider-footer.collapsed .service-status {
  align-items: center;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: #666;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  transition: background-color 0.3s;
}

.status-dot.active {
  background-color: #52c41a;
  box-shadow: 0 0 4px rgba(82, 196, 26, 0.4);
}

.status-dot.inactive {
  background-color: #d9d9d9;
}

.status-text {
  white-space: nowrap;
}

/* 动画效果 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* 菜单样式优化 */
:deep(.n-menu .n-menu-item:hover) {
  background-color: rgba(24, 144, 255, 0.08);
}

:deep(.n-menu .n-menu-item.n-menu-item--selected) {
  background-color: rgba(24, 144, 255, 0.1);
  color: #1890ff;
}

:deep(.n-menu .n-menu-item.n-menu-item--selected::before) {
  background-color: #1890ff;
}

/* 响应式设计 */
@media (max-width: 768px) {
  .sider-header {
    padding: 0 16px;
  }
  
  .sider-footer {
    padding: 12px 16px;
  }
}
</style> 
