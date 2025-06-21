<template>
  <div class="app-header">
    <div class="header-left">
      <h2 class="app-title">邮件附件下载器</h2>
    </div>
    
    <div class="header-center">
      <n-space>
        <div class="status-indicator">
          <span class="status-dot" :class="{ active: serviceStatus.running }"></span>
          <span class="status-text">{{ serviceStatus.running ? '运行中' : '已停止' }}</span>
        </div>
      </n-space>
    </div>
    
    <div class="header-right">
      <n-space>
        <n-button quaternary circle @click="toggleTheme">
          <template #icon>
            <span style="font-size: 18px;">{{ isDark ? '☀️' : '🌙' }}</span>
          </template>
        </n-button>
        
        <n-button quaternary circle @click="showSettings">
          <template #icon>
            <span style="font-size: 18px;">⚙️</span>
          </template>
        </n-button>
        
        <n-button quaternary circle @click="minimizeWindow">
          <template #icon>
            <span style="font-size: 18px;">➖</span>
          </template>
        </n-button>
        
        <n-button quaternary circle @click="closeWindow">
          <template #icon>
            <span style="font-size: 18px;">✖️</span>
          </template>
        </n-button>
      </n-space>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { NSpace, NButton } from 'naive-ui'

const router = useRouter()

// 响应式数据
const isDark = ref(false)

// 模拟服务状态
const serviceStatus = ref({ running: true })

// 方法
const toggleTheme = () => {
  isDark.value = !isDark.value
  // 这里可以切换主题
  // 主题切换逻辑
}

const showSettings = () => {
  router.push({ name: 'settings' })
}

  const minimizeWindow = async () => {
    // 调用Wails API最小化窗口
    await appStore.minimizeToTray()
  }

  const closeWindow = async () => {
    // 调用Wails API关闭窗口
    await appStore.quitApp()
  }
</script>

<style scoped>
.app-header {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  background: #fff;
  border-bottom: 1px solid #f0f0f0;
  -webkit-app-region: drag;
}

.header-left {
  display: flex;
  align-items: center;
}

.app-title {
  font-size: 18px;
  font-weight: 600;
  color: #262626;
  margin: 0;
}

.header-center {
  display: flex;
  align-items: center;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px;
  background: #f5f5f5;
  border-radius: 16px;
  font-size: 12px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #d9d9d9;
  transition: background-color 0.3s;
}

.status-dot.active {
  background: #52c41a;
  box-shadow: 0 0 4px rgba(82, 196, 26, 0.4);
}

.status-text {
  color: #666;
  font-weight: 500;
}

.header-right {
  display: flex;
  align-items: center;
  -webkit-app-region: no-drag;
}

.header-right .n-button {
  width: 32px;
  height: 32px;
}

@media (max-width: 768px) {
  .app-header {
    padding: 0 16px;
  }
  
  .app-title {
    font-size: 16px;
  }
  
  .status-indicator {
    display: none;
  }
}
</style> 
