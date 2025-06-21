import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useApi } from '../composables/useApi'
import type { 
  EmailAccount, 
  DownloadTask, 
  AppConfig, 
  DownloadStatistics,
  EmailCheckResult 
} from '../wails'

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
    emailAccounts.value.filter(account => account.is_active)
  )

  const runningTasks = computed(() => 
    downloadTasks.value.filter(task => task.status === 'downloading')
  )

  const completedTasks = computed(() => 
    downloadTasks.value.filter(task => task.status === 'completed')
  )

  const failedTasks = computed(() => 
    downloadTasks.value.filter(task => task.status === 'failed')
  )

  const totalDownloaded = computed(() => 
    completedTasks.value.reduce((sum, task) => sum + task.file_size, 0)
  )

  // 获取全局加载状态
  const isLoading = computed(() => api.isLoading.value)
  const error = computed(() => api.error.value)
  const isWailsReady = computed(() => api.isWailsReady.value)

  // Actions - 配置管理
  const loadConfig = async () => {
    const result = await api.configApi.getConfig()
    config.value = result
    return result
  }

  const updateConfig = async (newConfig: Partial<AppConfig>) => {
    if (config.value) {
      const updatedConfig = { ...config.value, ...newConfig }
      await api.configApi.updateConfig(updatedConfig)
      config.value = updatedConfig
    }
  }

  // Actions - 邮箱账户管理
  const loadEmailAccounts = async () => {
    const result = await api.emailApi.getAccounts()
    emailAccounts.value = result
    return result
  }

  const addEmailAccount = async (account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>) => {
    await api.emailApi.createAccount(account)
    await loadEmailAccounts()
  }

  const updateEmailAccount = async (account: EmailAccount) => {
    await api.emailApi.updateAccount(account)
    await loadEmailAccounts()
  }

  const deleteEmailAccount = async (id: number) => {
    await api.emailApi.deleteAccount(id)
    await loadEmailAccounts()
  }

  const testEmailConnection = async (account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>) => {
    await api.emailApi.testConnection(account)
  }

  // Actions - 邮件检查
  const checkAllEmails = async (): Promise<EmailCheckResult[]> => {
    const result = await api.emailApi.checkAllEmails()
    return result
  }

  const checkSingleEmail = async (accountId: number): Promise<EmailCheckResult> => {
    const result = await api.emailApi.checkSingleEmail(accountId)
    return result
  }

  const startEmailMonitoring = async () => {
    await api.emailApi.startMonitoring()
    await checkServiceStatus()
  }

  const stopEmailMonitoring = async () => {
    await api.emailApi.stopMonitoring()
    await checkServiceStatus()
  }

  // Actions - 下载任务管理
  const loadDownloadTasks = async (page = 1, pageSize = 20) => {
    const result = await api.downloadApi.getTasks(page, pageSize)
    downloadTasks.value = result.tasks
    return result
  }

  const getActiveDownloads = async () => {
    const result = await api.downloadApi.getActiveDownloads()
    return result
  }

  const pauseTask = async (taskId: number) => {
    await api.downloadApi.pauseTask(taskId)
    await loadDownloadTasks()
  }

  const resumeTask = async (taskId: number) => {
    await api.downloadApi.resumeTask(taskId)
    await loadDownloadTasks()
  }

  const cancelTask = async (taskId: number) => {
    await api.downloadApi.cancelTask(taskId)
    await loadDownloadTasks()
  }

  // Actions - 统计数据
  const loadStatistics = async (days = 30) => {
    const result = await api.statisticsApi.getStatistics(days)
    statistics.value = result
    return result
  }

  // Actions - 服务状态
  const checkServiceStatus = async () => {
    const result = await api.systemApi.getServiceStatus()
    serviceStatus.value = {
      email: result.email || false,
      download: result.download || false
    }
    return result
  }

  // Actions - 系统操作
  const openDownloadFolder = async () => {
    await api.systemApi.openDownloadFolder()
  }

  const openFile = async (filePath: string) => {
    await api.systemApi.openFile(filePath)
  }

  const selectDownloadFolder = async (): Promise<string> => {
    const result = await api.systemApi.selectDownloadFolder()
    return result
  }

  const showNotification = async (title: string, message: string) => {
    await api.systemApi.showNotification(title, message)
  }

  const getLogs = async (lines = 100): Promise<string[]> => {
    const result = await api.systemApi.getLogs(lines)
    return result
  }

  const getAppInfo = async () => {
    const result = await api.systemApi.getAppInfo()
    return result
  }

  const minimizeToTray = async () => {
    await api.systemApi.minimizeToTray()
  }

  const restoreFromTray = async () => {
    await api.systemApi.restoreFromTray()
  }

  const quitApp = async () => {
    await api.systemApi.quitApp()
  }

  // 初始化应用数据
  const initialize = async () => {
    await Promise.all([
      loadConfig(),
      loadEmailAccounts(),
      loadDownloadTasks(),
      checkServiceStatus()
    ])
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
    isWailsReady,
    
    // 配置管理
    loadConfig,
    updateConfig,
    
    // 邮箱账户管理
    loadEmailAccounts,
    addEmailAccount,
    updateEmailAccount,
    deleteEmailAccount,
    testEmailConnection,
    
    // 邮件检查
    checkAllEmails,
    checkSingleEmail,
    startEmailMonitoring,
    stopEmailMonitoring,
    
    // 下载任务管理
    loadDownloadTasks,
    getActiveDownloads,
    pauseTask,
    resumeTask,
    cancelTask,
    
    // 统计数据
    loadStatistics,
    
    // 服务状态
    checkServiceStatus,
    
    // 系统操作
    openDownloadFolder,
    openFile,
    selectDownloadFolder,
    showNotification,
    getLogs,
    getAppInfo,
    minimizeToTray,
    restoreFromTray,
    quitApp,
    
    // 初始化
    initialize
  }
})

// 导出类型以供其他地方使用
export type { EmailAccount, DownloadTask, AppConfig, DownloadStatistics, EmailCheckResult } 