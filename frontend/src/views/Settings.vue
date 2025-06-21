<template>
  <div class="settings">
    <n-card title="应用设置">
      <n-tabs type="line" animated>
        <n-tab-pane name="general" tab="⚙️ 常规设置">
          <n-form :model="settings" label-placement="left" label-width="120px">
            <n-form-item label="下载目录">
              <n-input-group>
                <n-input v-model:value="settings.downloadPath" readonly />
                <n-button @click="selectDownloadPath">选择目录</n-button>
              </n-input-group>
            </n-form-item>
            
            <n-form-item label="最大并发数">
              <n-input-number v-model:value="settings.maxConcurrency" :min="1" :max="10" />
            </n-form-item>
            
            <n-form-item label="自动启动">
              <n-switch v-model:value="settings.autoStart" />
            </n-form-item>
            
            <n-form-item label="最小化到托盘">
              <n-switch v-model:value="settings.minimizeToTray" />
            </n-form-item>
          </n-form>
        </n-tab-pane>
        
        <n-tab-pane name="email" tab="📧 邮件设置">
          <n-form :model="settings" label-placement="left" label-width="120px">
            <n-form-item label="检查间隔">
              <n-input-number v-model:value="settings.checkInterval" :min="1" :max="60" />
              <template #feedback>分钟</template>
            </n-form-item>
            
                         <n-form-item label="下载超时">
               <n-input-number v-model:value="settings.downloadTimeout" :min="10" :max="300" />
               <template #feedback>秒</template>
             </n-form-item>
            
            <n-form-item label="重试次数">
              <n-input-number v-model:value="settings.maxRetries" :min="0" :max="10" />
            </n-form-item>
          </n-form>
        </n-tab-pane>
        
        <n-tab-pane name="notification" tab="🔔 通知设置">
          <n-form :model="settings" label-placement="left" label-width="120px">
            <n-form-item label="桌面通知">
              <n-switch v-model:value="settings.enableNotification" />
            </n-form-item>
            
            <n-form-item label="声音提醒">
              <n-switch v-model:value="settings.enableSound" />
            </n-form-item>
            
            <n-form-item label="下载完成通知">
              <n-switch v-model:value="settings.notifyOnComplete" />
            </n-form-item>
            
            <n-form-item label="错误通知">
              <n-switch v-model:value="settings.notifyOnError" />
            </n-form-item>
          </n-form>
        </n-tab-pane>
      </n-tabs>
      
      <n-divider />
      
      <n-space justify="end">
        <n-button @click="resetSettings">重置设置</n-button>
        <n-button type="primary" @click="saveSettings">保存设置</n-button>
      </n-space>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAppStore } from '@/stores/app'
import {
  NCard,
  NTabs,
  NTabPane,
  NForm,
  NFormItem,
  NInput,
  NInputGroup,
  NInputNumber,
  NButton,
  NSwitch,
  NDivider,
  NSpace,
  useMessage
} from 'naive-ui'

const appStore = useAppStore()
const message = useMessage()

const settings = ref({
  downloadPath: '',
  maxConcurrency: 3,
  autoStart: false,
  minimizeToTray: true,
  checkInterval: 5,
  downloadTimeout: 60,
  maxRetries: 3,
  enableNotification: true,
  enableSound: false,
  notifyOnComplete: true,
  notifyOnError: true
})

const selectDownloadPath = async () => {
  try {
    // 调用后端选择目录
    // 选择下载目录功能
  } catch (error) {
    message.error('选择目录失败')
  }
}

const saveSettings = async () => {
  try {
    await appStore.saveSettings(settings.value)
    message.success('设置已保存')
  } catch (error) {
    message.error('保存设置失败')
  }
}

const resetSettings = () => {
  settings.value = {
    downloadPath: '',
    maxConcurrency: 3,
    autoStart: false,
    minimizeToTray: true,
    checkInterval: 5,
    downloadTimeout: 60,
    maxRetries: 3,
    enableNotification: true,
    enableSound: false,
    notifyOnComplete: true,
    notifyOnError: true
  }
}

onMounted(async () => {
  try {
    const config = await appStore.loadSettings()
    if (config) {
      settings.value = { ...settings.value, ...config }
    }
  } catch (error) {
    console.error('加载设置失败:', error)
  }
})
</script>

<style scoped>
.settings {
  padding: 24px;
  max-width: 800px;
  margin: 0 auto;
}
</style> 
