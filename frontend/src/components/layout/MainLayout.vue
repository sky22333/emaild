<template>
  <n-layout has-sider style="height: 100vh;">
    <!-- 侧边栏 -->
    <n-layout-sider
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="240"
      :collapsed="collapsed"
      show-trigger
      @collapse="collapsed = true"
      @expand="collapsed = false"
    >
      <AppSider :collapsed="collapsed" />
    </n-layout-sider>
    
    <!-- 主内容区域 -->
    <n-layout>
      <!-- 顶部导航 -->
      <n-layout-header bordered style="height: 64px; padding: 0 24px;">
        <AppHeader />
      </n-layout-header>
      
      <!-- 内容区域 -->
      <n-layout-content style="padding: 0; overflow: auto;">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { NLayout, NLayoutSider, NLayoutHeader, NLayoutContent } from 'naive-ui'
import AppHeader from './AppHeader.vue'
import AppSider from './AppSider.vue'

// 侧边栏折叠状态
const collapsed = ref(false)
</script>

<style scoped>
.n-layout-header {
  display: flex;
  align-items: center;
}
</style> 