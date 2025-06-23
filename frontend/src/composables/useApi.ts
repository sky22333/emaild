import { ref, computed } from 'vue'
import { useErrorHandler } from './useErrorHandler'
import * as WailsApp from '../wailsjs/go/backend/App'
import { models, backend } from '../wailsjs/go/models'

// 使用Wails生成的类型
type EmailAccount = models.EmailAccount
type DownloadTask = models.DownloadTask
type AppConfig = models.AppConfig
type DownloadStatistics = models.DownloadStatistics
type EmailCheckResult = backend.EmailCheckResult
type GetDownloadTasksResponse = backend.GetDownloadTasksResponse

// 检查Wails运行时是否可用
const isWailsReady = (): boolean => {
  try {
    return !!(window as any)?.go?.backend?.App && typeof (window as any).go.backend.App === 'object'
  } catch {
    return false
  }
}

// 等待Wails运行时准备就绪
const waitForWails = (): Promise<void> => {
  return new Promise((resolve, reject) => {
    if (isWailsReady()) {
      resolve()
      return
    }
    
    let attempts = 0
    const maxAttempts = 50 // 5秒超时
    
    const checkInterval = setInterval(() => {
      attempts++
      
      if (isWailsReady()) {
        clearInterval(checkInterval)
        resolve()
        return
      }
      
      if (attempts >= maxAttempts) {
        clearInterval(checkInterval)
        console.warn('Wails运行时初始化超时')
        reject(new Error('Wails运行时初始化超时'))
        return
      }
    }, 100)
  })
}

export function useApi() {
  const errorHandler = useErrorHandler()
  
  // 安全的API调用包装器
  const safeApiCall = async <T>(
    apiCall: () => Promise<T>,
    context: string
  ): Promise<T> => {
    // 等待Wails运行时准备就绪
    await waitForWails()
    
    if (!isWailsReady()) {
      throw new Error('Wails运行时未准备就绪')
    }
    
    const result = await errorHandler.wrapAsync(apiCall, context)
    return result as T
  }

  // 配置相关API
  const configApi = {
    async getConfig(): Promise<AppConfig> {
      return safeApiCall(
        () => WailsApp.GetConfig(),
        '获取配置'
      )
    },

    async updateConfig(config: AppConfig): Promise<void> {
      return safeApiCall(
        () => WailsApp.UpdateConfig(config),
        '更新配置'
      )
    }
  }

  // 邮箱账户相关API
  const emailApi = {
    async getAccounts(): Promise<EmailAccount[]> {
      return safeApiCall(
        () => WailsApp.GetEmailAccounts(),
        '获取邮箱账户'
      )
    },

    async createAccount(account: EmailAccount): Promise<void> {
      return safeApiCall(
        () => WailsApp.CreateEmailAccount(account),
        '创建邮箱账户'
      )
    },

    async updateAccount(account: EmailAccount): Promise<void> {
      return safeApiCall(
        () => WailsApp.UpdateEmailAccount(account),
        '更新邮箱账户'
      )
    },

    async deleteAccount(id: number): Promise<void> {
      return safeApiCall(
        () => WailsApp.DeleteEmailAccount(id),
        '删除邮箱账户'
      )
    },

    async testConnection(account: EmailAccount): Promise<void> {
      return safeApiCall(
        () => WailsApp.TestEmailConnection(account),
        '测试邮箱连接'
      )
    },

    async checkAllEmails(): Promise<EmailCheckResult[]> {
      return safeApiCall(
        () => WailsApp.CheckAllEmails(),
        '检查所有邮件'
      )
    },

    async checkSingleEmail(accountId: number): Promise<EmailCheckResult> {
      return safeApiCall(
        () => WailsApp.CheckSingleEmail(accountId),
        '检查单个邮箱'
      )
    },

    async startMonitoring(): Promise<void> {
      return safeApiCall(
        () => WailsApp.StartEmailMonitoring(),
        '启动邮件监控'
      )
    },

    async stopMonitoring(): Promise<void> {
      return safeApiCall(
        () => WailsApp.StopEmailMonitoring(),
        '停止邮件监控'
      )
    }
  }

  // 下载任务相关API
  const downloadApi = {
    async getTasks(page = 1, pageSize = 20): Promise<GetDownloadTasksResponse> {
      return safeApiCall(
        () => WailsApp.GetDownloadTasks(page, pageSize),
        '获取下载任务'
      )
    },

    async getTasksByStatus(status: string): Promise<DownloadTask[]> {
      return safeApiCall(
        () => WailsApp.GetDownloadTasksByStatus(status),
        '按状态获取下载任务'
      )
    },

    async getActiveDownloads(): Promise<DownloadTask[]> {
      return safeApiCall(
        () => WailsApp.GetActiveDownloads(),
        '获取活跃下载'
      )
    },

    async pauseTask(taskId: number): Promise<void> {
      return safeApiCall(
        () => WailsApp.PauseDownloadTask(taskId),
        '暂停下载任务'
      )
    },

    async resumeTask(taskId: number): Promise<void> {
      return safeApiCall(
        () => WailsApp.ResumeDownloadTask(taskId),
        '恢复下载任务'
      )
    },

    async cancelTask(taskId: number): Promise<void> {
      return safeApiCall(
        () => WailsApp.CancelDownloadTask(taskId),
        '取消下载任务'
      )
    }
  }

  // 统计数据相关API
  const statsApi = {
    async getStatistics(days = 30): Promise<DownloadStatistics[]> {
      return safeApiCall(
        () => WailsApp.GetStatistics(days),
        '获取统计数据'
      )
    }
  }

  // 系统相关API
  const systemApi = {
    async openDownloadFolder(): Promise<void> {
      return safeApiCall(
        () => WailsApp.OpenDownloadFolder(),
        '打开下载文件夹'
      )
    },

    async openFile(filePath: string): Promise<void> {
      return safeApiCall(
        () => WailsApp.OpenFile(filePath),
        '打开文件'
      )
    },

    async selectDownloadFolder(): Promise<string> {
      return safeApiCall(
        () => WailsApp.SelectDownloadFolder(),
        '选择下载文件夹'
      )
    },

    async showNotification(title: string, message: string): Promise<void> {
      return safeApiCall(
        () => WailsApp.ShowNotification(title, message),
        '显示通知'
      )
    },

    async getServiceStatus(): Promise<Record<string, boolean>> {
      return safeApiCall(
        () => WailsApp.GetServiceStatus(),
        '获取服务状态'
      )
    },

    async minimizeToTray(): Promise<void> {
      return safeApiCall(
        () => WailsApp.MinimizeToTray(),
        '最小化到托盘'
      )
    },

    async restoreFromTray(): Promise<void> {
      return safeApiCall(
        () => WailsApp.RestoreFromTray(),
        '从托盘恢复'
      )
    },

    async quitApp(): Promise<void> {
      return safeApiCall(
        () => WailsApp.QuitApp(),
        '退出应用'
      )
    },

    async getAppInfo(): Promise<Record<string, any>> {
      return safeApiCall(
        () => WailsApp.GetAppInfo(),
        '获取应用信息'
      )
    },

    async isEmailServiceRunning(): Promise<boolean> {
      return safeApiCall(
        () => WailsApp.IsEmailServiceRunning(),
        '检查邮件服务状态'
      )
    },

    async getActiveDownloadsCount(): Promise<number> {
      return safeApiCall(
        () => WailsApp.GetActiveDownloadsCount(),
        '获取活跃下载数量'
      )
    }
  }

  return {
    // API 分组
    config: configApi,
    email: emailApi,
    download: downloadApi,
    stats: statsApi,
    system: systemApi,
    
    // 状态
    isLoading: computed(() => errorHandler.isRetrying.value),
    error: computed(() => errorHandler.lastError.value)
  }
}

// 导出类型以供其他组件使用
export type { EmailAccount, DownloadTask, AppConfig, DownloadStatistics, EmailCheckResult, GetDownloadTasksResponse } 