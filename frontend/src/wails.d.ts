// Wails生成的类型定义
declare global {
  interface Window {
    go: {
      backend: {
        App: {
          // 邮箱账户管理
          GetEmailAccounts(): Promise<EmailAccount[]>
          CreateEmailAccount(account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>): Promise<void>
          UpdateEmailAccount(account: EmailAccount): Promise<void>
          DeleteEmailAccount(id: number): Promise<void>
          TestEmailConnection(account: Omit<EmailAccount, 'id' | 'created_at' | 'updated_at'>): Promise<void>
          
          // 邮件检查
          CheckAllEmails(): Promise<EmailCheckResult[]>
          CheckSingleEmail(accountID: number): Promise<EmailCheckResult>
          StartEmailMonitoring(): Promise<void>
          StopEmailMonitoring(): Promise<void>
          
          // 下载任务管理
          GetDownloadTasks(page: number, pageSize: number): Promise<{ tasks: DownloadTask[], total: number }>
          GetDownloadTasksByStatus(status: string): Promise<DownloadTask[]>
          CreateDownloadTask(task: Omit<DownloadTask, 'id' | 'created_at' | 'updated_at'>): Promise<void>
          PauseDownloadTask(taskID: number): Promise<void>
          ResumeDownloadTask(taskID: number): Promise<void>
          CancelDownloadTask(taskID: number): Promise<void>
          GetActiveDownloads(): Promise<DownloadTask[]>
          
          // 配置管理
          GetConfig(): Promise<AppConfig>
          UpdateConfig(config: AppConfig): Promise<void>
          
          // 统计数据
          GetStatistics(days: number): Promise<DownloadStatistics[]>
          
          // 文件操作
          OpenDownloadFolder(): Promise<void>
          OpenFile(filePath: string): Promise<void>
          SelectDownloadFolder(): Promise<string>
          
          // 应用控制
          MinimizeToTray(): Promise<void>
          RestoreFromTray(): Promise<void>
          QuitApp(): Promise<void>
          ShowNotification(title: string, message: string): Promise<void>
          
          // 系统信息
          GetLogs(lines: number): Promise<string[]>
          GetAppInfo(): Promise<Record<string, any>>
          IsEmailServiceRunning(): Promise<boolean>
          IsDownloadServiceRunning(): Promise<boolean>
          GetServiceStatus(): Promise<Record<string, boolean>>
        }
      }
    }
  }
}

// 邮箱账户接口
export interface EmailAccount {
  id: number
  name: string
  email: string
  password: string
  imap_server: string
  imap_port: number
  use_ssl: boolean
  is_active: boolean
  created_at: string
  updated_at: string
}

// 下载任务接口
export interface DownloadTask {
  id: number
  email_id: number
  email_account: EmailAccount
  subject: string
  sender: string
  file_name: string
  file_size: number
  downloaded_size: number
  status: 'pending' | 'downloading' | 'completed' | 'failed' | 'paused' | 'cancelled'
  type: 'attachment' | 'link'
  source: string
  local_path: string
  error: string
  progress: number
  speed: string
  created_at: string
  updated_at: string
}

// 应用配置接口
export interface AppConfig {
  id: number
  download_path: string
  max_concurrent: number
  check_interval: number
  auto_check: boolean
  minimize_to_tray: boolean
  start_minimized: boolean
  enable_notification: boolean
  theme: 'light' | 'dark' | 'auto'
  language: string
  created_at: string
  updated_at: string
}

// 统计数据接口
export interface DownloadStatistics {
  id: number
  date: string
  total_downloads: number
  success_downloads: number
  failed_downloads: number
  total_size: number
  created_at: string
  updated_at: string
}

// 邮件检查结果接口
export interface EmailCheckResult {
  account: EmailAccount
  new_emails: number
  pdfs_found: number
  error?: string
  success: boolean
}

export {} 