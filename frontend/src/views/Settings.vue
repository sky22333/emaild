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
            
            <n-form-item label="自动检查">
              <n-switch v-model:value="settings.autoStart" />
            </n-form-item>
            
            <n-form-item label="最小化到托盘">
              <n-switch v-model:value="settings.minimizeToTray" />
            </n-form-item>
            
            <n-form-item label="启动时最小化">
              <n-switch v-model:value="settings.startMinimized" />
            </n-form-item>
          </n-form>
        </n-tab-pane>
        
        <n-tab-pane name="email" tab="📧 邮件设置">
          <n-form :model="settings" label-placement="left" label-width="120px">
            <n-form-item label="检查间隔">
              <n-input-number v-model:value="settings.checkInterval" :min="1" :max="60" />
              <template #feedback>分钟</template>
            </n-form-item>
          </n-form>
        </n-tab-pane>
        
        <n-tab-pane name="notification" tab="🔔 通知设置">
          <n-form :model="settings" label-placement="left" label-width="120px">
            <n-form-item label="桌面通知">
              <n-switch v-model:value="settings.enableNotification" />
            </n-form-item>
          </n-form>
        </n-tab-pane>
      </n-tabs>
      
      <n-divider />
      
      <n-space justify="end">
        <n-button @click="resetSettings">重置设置</n-button>
        <n-button type="primary" @click="saveSettings" :loading="saving">保存设置</n-button>
      </n-space>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAppStore } from '../stores/app'
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
const saving = ref(false)

const settings = ref({
  downloadPath: '',
  maxConcurrency: 3,
  autoStart: false,
  minimizeToTray: true,
  startMinimized: false,
  checkInterval: 5,
  enableNotification: true
})

const selectDownloadPath = async () => {
  try {
    const selectedPath = await appStore.selectDownloadFolder()
    if (selectedPath) {
      settings.value.downloadPath = selectedPath
    }
  } catch (error) {
    message.error('选择目录失败')
  }
}

const saveSettings = async () => {
  if (saving.value) return
  
  try {
    saving.value = true
    await appStore.saveSettings(settings.value)
    message.success('设置已保存')
  } catch (error) {
    console.error('保存设置失败:', error)
    const errorMessage = error instanceof Error ? error.message : '保存设置失败'
    message.error(errorMessage)
  } finally {
    saving.value = false
  }
}

const resetSettings = async () => {
  if (saving.value) return
  
  try {
    const defaultSettings = await appStore.loadSettings()
    if (defaultSettings) {
      settings.value = { ...defaultSettings }
      message.info('设置已重置为默认值')
    }
  } catch (error) {
    console.error('重置设置失败:', error)
    message.error('重置设置失败')
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
    message.warning('加载设置失败，使用默认设置')
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
