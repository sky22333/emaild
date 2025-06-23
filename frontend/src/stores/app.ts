import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useApi } from '../composables/useApi'
import { backend } from '../wailsjs/go/models'
import type { 
  EmailAccount, 
  DownloadTask, 
  AppConfig, 
  DownloadStatistics,
  EmailCheckResult 
} from '../composables/useApi'

// 应用状态管理
export const useAppStore = defineStore('app', () => {
  // 使用API组合式函数
  const api = useApi()
  
  // 状态
  const config = ref<AppConfig | null>(null)
  const emailAccounts = ref<EmailAccount[]>([])
  const downloadTasks = ref<DownloadTask[]>([])
  const statistics = ref<DownloadStatistics[]>([])
  const serviceStatus = ref({
    email: false,
    download: false
  })

  // 计算属性
  const activeEmailAccounts = computed(() => 
    (emailAccounts.value || []).filter(account => account.is_active)
  )

  const runningTasks = computed(() => 
    (downloadTasks.value || []).filter(task => task.status === 'downloading')
  )

  const completedTasks = computed(() => 
    (downloadTasks.value || []).filter(task => task.status === 'completed')
  )

  const failedTasks = computed(() => 
    (downloadTasks.value || []).filter(task => task.status === 'failed')
  )

  const totalDownloaded = computed(() => 
    (completedTasks.value || []).reduce((sum, task) => sum + (task.file_size || 0), 0)
  )

  // 获取全局加载状态
  const isLoading = computed(() => api.isLoading.value)
  const error = computed(() => api.error.value)

  // 简化的错误处理包装器
  const safeCall = async <T>(operation: () => Promise<T>, throwOnError = false): Promise<T | null> => {
    try {
      return await operation()
    } catch (error) {
      console.error('API调用失败:', error)
      if (throwOnError) {
        throw error
      }
      return null
    }
  }

  // Actions - 配置管理
  const loadConfig = async () => {
    const result = await safeCall(() => api.config.getConfig())
    if (result) {
      config.value = result
    }
    return result
  }

  const updateConfig = async (newConfig: Partial<AppConfig>) => {
    if (config.value) {
      const updatedConfig = { ...config.value, ...newConfig }
      const result = await safeCall(() => api.config.updateConfig(updatedConfig))
      if (result !== null) {
        config.value = updatedConfig
      }
    }
  }

  // Actions - 邮箱账户管理
  const loadEmailAccounts = async () => {
    const result = await safeCall(() => api.email.getAccounts())
    emailAccounts.value = result || []
    return result || []
  }

  const addEmailAccount = async (account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>) => {
    const result = await safeCall(() => api.email.createAccount(account as EmailAccount))
    if (result !== null) {
      await loadEmailAccounts()
    }
  }

  const updateEmailAccount = async (account: EmailAccount) => {
    const result = await safeCall(() => api.email.updateAccount(account))
    if (result !== null) {
      await loadEmailAccounts()
    }
  }

  const deleteEmailAccount = async (id: number) => {
    const result = await safeCall(() => api.email.deleteAccount(id))
    if (result !== null) {
      await loadEmailAccounts()
    }
  }

  const testEmailConnection = async (account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>) => {
    return await safeCall(() => api.email.testConnection(account as EmailAccount), true)
  }

  const checkAllEmails = async (): Promise<EmailCheckResult[]> => {
    const result = await safeCall(() => api.email.checkAllEmails())
    return result || []
  }

  const checkSingleEmail = async (accountId: number): Promise<EmailCheckResult> => {
    const result = await safeCall(() => api.email.checkSingleEmail(accountId))
    return result || backend.EmailCheckResult.createFrom({ account: null, new_emails: 0, pdfs_found: 0, error: '', success: false })
  }

  const startEmailMonitoring = async () => {
    await safeCall(() => api.email.startMonitoring())
  }

  const stopEmailMonitoring = async () => {
    await safeCall(() => api.email.stopMonitoring())
  }

  // Actions - 下载任务管理
  const loadDownloadTasks = async (page = 1, pageSize = 20) => {
    const result = await safeCall(() => api.download.getTasks(page, pageSize))
    if (result && result.tasks) {
      downloadTasks.value = result.tasks || []
      return result
    } else {
      downloadTasks.value = []
      return { tasks: [], total: 0 }
    }
  }

  const pauseTask = async (taskId: number) => {
    const result = await safeCall(() => api.download.pauseTask(taskId))
    if (result !== null) {
      await loadDownloadTasks()
    }
  }

  const resumeTask = async (taskId: number) => {
    const result = await safeCall(() => api.download.resumeTask(taskId))
    if (result !== null) {
      await loadDownloadTasks()
    }
  }

  const cancelTask = async (taskId: number) => {
    const result = await safeCall(() => api.download.cancelTask(taskId))
    if (result !== null) {
      await loadDownloadTasks()
    }
  }

  // Actions - 统计数据
  const loadStatistics = async (days = 30) => {
    const result = await safeCall(() => api.stats.getStatistics(days))
    statistics.value = result || []
    return result || []
  }

  // Actions - 服务状态
  const checkServiceStatus = async () => {
    const result = await safeCall(() => api.system.getServiceStatus())
    if (result) {
      serviceStatus.value = {
        email: result.email || false,
        download: result.download || false
      }
    }
    return result || { email: false, download: false }
  }

  // Actions - 系统操作
  const openDownloadFolder = async () => {
    return await safeCall(() => api.system.openDownloadFolder())
  }

  const selectDownloadFolder = async (): Promise<string> => {
    const result = await safeCall(() => api.system.selectDownloadFolder())
    return result || ''
  }

  // 设置管理的便捷方法
  const saveSettings = async (settings: any) => {
    const configToSave: Partial<AppConfig> = {
      download_path: settings.downloadPath || '',
      max_concurrent: settings.maxConcurrency || 3,
      check_interval: settings.checkInterval || 5,
      auto_check: settings.autoStart || false,
      minimize_to_tray: settings.minimizeToTray !== undefined ? settings.minimizeToTray : true,
      start_minimized: settings.startMinimized || false,
      enable_notification: settings.enableNotification !== undefined ? settings.enableNotification : true,
      theme: settings.theme || 'light',
      language: settings.language || 'zh-CN'
    }
    
    await updateConfig(configToSave)
  }

  const loadSettings = async () => {
    const config = await loadConfig()
    if (!config) {
      return {
        downloadPath: '',
        maxConcurrency: 3,
        checkInterval: 5,
        autoStart: false,
        minimizeToTray: true,
        startMinimized: false,
        enableNotification: true,
        theme: 'light',
        language: 'zh-CN'
      }
    }
    
    return {
      downloadPath: config.download_path || '',
      maxConcurrency: config.max_concurrent || 3,
      checkInterval: config.check_interval || 5,
      autoStart: config.auto_check || false,
      minimizeToTray: config.minimize_to_tray !== undefined ? config.minimize_to_tray : true,
      startMinimized: config.start_minimized || false,
      enableNotification: config.enable_notification !== undefined ? config.enable_notification : true,
      theme: config.theme || 'light',
      language: config.language || 'zh-CN'
    }
  }

  return {
    // 状态
    config,
    emailAccounts,
    downloadTasks,
    statistics,
    serviceStatus,
    
    // 计算属性
    activeEmailAccounts,
    runningTasks,
    completedTasks,
    failedTasks,
    totalDownloaded,
    isLoading,
    error,
    
    // Actions
    loadConfig,
    updateConfig,
    loadEmailAccounts,
    addEmailAccount,
    updateEmailAccount,
    deleteEmailAccount,
    testEmailConnection,
    checkAllEmails,
    checkSingleEmail,
    startEmailMonitoring,
    stopEmailMonitoring,
    loadDownloadTasks,
    pauseTask,
    resumeTask,
    cancelTask,
    loadStatistics,
    checkServiceStatus,
    openDownloadFolder,
    selectDownloadFolder,
    saveSettings,
    loadSettings
  }
})

// 导出类型
export type { EmailAccount, DownloadTask, AppConfig, DownloadStatistics, EmailCheckResult } 