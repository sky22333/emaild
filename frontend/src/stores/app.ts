import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useApi } from '../composables/useApi'
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

  // Actions - 配置管理
  const loadConfig = async () => {
    const result = await api.config.getConfig()
    config.value = result
    return result
  }

  const updateConfig = async (newConfig: Partial<AppConfig>) => {
    if (config.value) {
      const updatedConfig = { ...config.value, ...newConfig }
      await api.config.updateConfig(updatedConfig)
      config.value = updatedConfig
    }
  }

  // Actions - 邮箱账户管理
  const loadEmailAccounts = async () => {
    const result = await api.email.getAccounts()
    emailAccounts.value = result || []
    return result || []
  }

  const addEmailAccount = async (account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>) => {
    await api.email.createAccount(account as EmailAccount)
    await loadEmailAccounts()
  }

  const updateEmailAccount = async (account: EmailAccount) => {
    await api.email.updateAccount(account)
    await loadEmailAccounts()
  }

  const deleteEmailAccount = async (id: number) => {
    await api.email.deleteAccount(id)
    await loadEmailAccounts()
  }

  const testEmailConnection = async (account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>) => {
    return await api.email.testConnection(account as EmailAccount)
  }

  // Actions - 邮件检查
  const checkAllEmails = async (): Promise<EmailCheckResult[]> => {
    const result = await api.email.checkAllEmails()
    return result || []
  }

  const checkSingleEmail = async (accountId: number): Promise<EmailCheckResult> => {
    const result = await api.email.checkSingleEmail(accountId)
    return result || { account: null, new_emails: 0, pdfs_found: 0, error: '', success: false }
  }

  const startEmailMonitoring = async () => {
    await api.email.startMonitoring()
    await checkServiceStatus()
  }

  const stopEmailMonitoring = async () => {
    await api.email.stopMonitoring()
    await checkServiceStatus()
  }

  // Actions - 下载任务管理
  const loadDownloadTasks = async (page = 1, pageSize = 20) => {
    const result = await api.download.getTasks(page, pageSize)
    if (result && result.tasks) {
      downloadTasks.value = result.tasks || []
      return result
    } else {
      downloadTasks.value = []
      return { tasks: [], total: 0 }
    }
  }

  const getActiveDownloads = async () => {
    const result = await api.download.getActiveDownloads()
    return result || []
  }

  const pauseTask = async (taskId: number) => {
    await api.download.pauseTask(taskId)
    await loadDownloadTasks()
  }

  const resumeTask = async (taskId: number) => {
    await api.download.resumeTask(taskId)
    await loadDownloadTasks()
  }

  const cancelTask = async (taskId: number) => {
    await api.download.cancelTask(taskId)
    await loadDownloadTasks()
  }

  // Actions - 统计数据
  const loadStatistics = async (days = 30) => {
    const result = await api.stats.getStatistics(days)
    statistics.value = result || []
    return result || []
  }

  // Actions - 服务状态
  const checkServiceStatus = async () => {
    const result = await api.system.getServiceStatus()
    serviceStatus.value = {
      email: result?.email || false,
      download: result?.download || false
    }
    return result || { email: false, download: false }
  }

  // Actions - 系统操作
  const openDownloadFolder = async () => {
    await api.system.openDownloadFolder()
  }

  const openFile = async (filePath: string) => {
    await api.system.openFile(filePath)
  }

  const selectDownloadFolder = async (): Promise<string> => {
    const result = await api.system.selectDownloadFolder()
    return result
  }

  const showNotification = async (title: string, message: string) => {
    await api.system.showNotification(title, message)
  }

  const getAppInfo = async () => {
    const result = await api.system.getAppInfo()
    return result
  }

  const minimizeToTray = async () => {
    await api.system.minimizeToTray()
  }

  const restoreFromTray = async () => {
    await api.system.restoreFromTray()
  }

  const quitApp = async () => {
    await api.system.quitApp()
  }

  // 初始化应用数据
  const initialize = async () => {
    try {
      await Promise.all([
        loadConfig(),
        loadEmailAccounts(),
        loadDownloadTasks(),
        checkServiceStatus()
      ])
    } catch (error) {
      console.error('初始化应用数据失败:', error)
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
    getActiveDownloads,
    pauseTask,
    resumeTask,
    cancelTask,
    loadStatistics,
    checkServiceStatus,
    openDownloadFolder,
    openFile,
    selectDownloadFolder,
    showNotification,
    getAppInfo,
    minimizeToTray,
    restoreFromTray,
    quitApp,
    initialize
  }
})

// 导出类型
export type { EmailAccount, DownloadTask, AppConfig, DownloadStatistics, EmailCheckResult } 