package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"emaild/backend/database"
	"emaild/backend/models"
	"emaild/backend/services"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/skratchdot/open-golang/open"
)

// 使用models包中的EmailCheckResult定义
// 避免重复定义

// App 主应用结构体
type App struct {
	ctx             context.Context
	cancel          context.CancelFunc
	db              *database.Database
	downloadService *services.DownloadService
	emailService    *services.EmailService
	trayService     *services.TrayService
	logger          *logrus.Logger
	
	// 服务状态
	isInitialized   bool
	initMutex       sync.RWMutex
	
	// 优雅关闭相关
	shutdownOnce    sync.Once
	isShuttingDown  bool
	shutdownMutex   sync.RWMutex
}

// NewApp 创建应用实例
func NewApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	
	// 初始化日志
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	return &App{
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		isInitialized:  false,
		isShuttingDown: false,
	}
}

// OnStartup 应用启动时的回调
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	
	// 异步初始化服务，避免阻塞启动
	go func() {
		if err := a.initializeServices(); err != nil {
			a.logger.Errorf("服务初始化失败: %v", err)
			// 显示用户友好的错误对话框
			a.showErrorDialog("服务初始化失败", fmt.Sprintf("无法启动应用服务: %v", err))
		}
	}()
}

// OnShutdown 应用关闭时的回调
func (a *App) OnShutdown(ctx context.Context) {
	a.logger.Info("应用关闭中...")

	// 停止服务
	if a.emailService != nil {
		a.emailService.Stop()
	}

	if a.downloadService != nil {
		a.downloadService.Stop()
	}

	if a.trayService != nil {
		a.trayService.Stop()
	}

	// 关闭数据库连接
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.logger.Errorf("关闭数据库失败: %v", err)
		}
	}

	a.logger.Info("应用已关闭")
}

// OnDomReady 前端DOM准备完成时的回调
func (a *App) OnDomReady(ctx context.Context) {
	// 检查是否需要在启动时最小化
	config, err := a.GetConfig()
	if err == nil && config.StartMinimized {
		runtime.WindowMinimise(ctx)
	}
}

// getOrCreateDefaultConfig 获取或创建默认配置
func (a *App) getOrCreateDefaultConfig() (*models.AppConfig, error) {
	config, err := a.GetConfig()
	if err != nil {
		// 创建默认配置
		homeDir, _ := os.UserHomeDir()
		now := time.Now()
		defaultConfig := models.AppConfig{
			DownloadPath:       filepath.Join(homeDir, "Downloads", "EmailPDFs"),
			MaxConcurrent:      3,
			CheckInterval:      300, // 5分钟
			AutoCheck:          false,
			MinimizeToTray:     true,
			StartMinimized:     false,
			EnableNotification: true,
			Theme:              "auto",
			Language:           "zh-CN",
			CreatedAt:          models.TimeToString(now),
			UpdatedAt:          models.TimeToString(now),
		}
		
		if err := a.CreateConfig(defaultConfig); err != nil {
			return nil, err
		}
		return &defaultConfig, nil
	}
	return &config, nil
}

// ====================
// 邮箱账户管理 API
// ====================

// GetEmailAccounts 获取所有邮箱账户
func (a *App) GetEmailAccounts() ([]models.EmailAccount, error) {
	if err := a.ensureServicesReady(); err != nil {
		return nil, err
	}
	
	return a.db.GetEmailAccounts()
}

// CreateEmailAccount 创建邮箱账户
func (a *App) CreateEmailAccount(account models.EmailAccount) error {
	// 验证邮箱格式
	if account.Email == "" || account.Password == "" || account.IMAPServer == "" {
		return fmt.Errorf("邮箱地址、密码和IMAP服务器不能为空")
	}

	// 测试连接
	if err := a.emailService.TestConnection(&account); err != nil {
		return fmt.Errorf("邮箱连接测试失败: %v", err)
	}

	// 保存邮箱账户
	if err := a.db.CreateEmailAccount(&account); err != nil {
		return err
	}

	// 如果账户是激活状态，立即触发一次邮件检查
	if account.IsActive && a.emailService != nil {
		go func() {
			// 等待一秒钟确保数据库操作完成
			time.Sleep(1 * time.Second)
			// 检查新添加的账户
			a.emailService.CheckAccountWithResult(&account)
		}()
	}

	return nil
}

// UpdateEmailAccount 更新邮箱账户
func (a *App) UpdateEmailAccount(account models.EmailAccount) error {
	// 验证数据
	if account.Email == "" || account.Password == "" || account.IMAPServer == "" {
		return fmt.Errorf("邮箱地址、密码和IMAP服务器不能为空")
	}

	// 测试连接（如果邮箱设置有变化）
	oldAccount, err := a.db.GetEmailAccountByID(account.ID)
	if err != nil {
		return fmt.Errorf("获取原账户信息失败: %v", err)
	}

	if oldAccount.Email != account.Email || oldAccount.Password != account.Password || 
	   oldAccount.IMAPServer != account.IMAPServer || oldAccount.IMAPPort != account.IMAPPort {
		if err := a.emailService.TestConnection(&account); err != nil {
			return fmt.Errorf("邮箱连接测试失败: %v", err)
		}
	}

	return a.db.UpdateEmailAccount(&account)
}

// DeleteEmailAccount 删除邮箱账户
func (a *App) DeleteEmailAccount(id uint) error {
	return a.db.DeleteEmailAccount(id)
}

// TestEmailConnection 测试邮箱连接
func (a *App) TestEmailConnection(account models.EmailAccount) error {
	return a.emailService.TestConnection(&account)
}

// TestEmailConnectionByID 根据ID测试邮箱连接
func (a *App) TestEmailConnectionByID(accountID uint) error {
	account, err := a.db.GetEmailAccountByID(accountID)
	if err != nil {
		return fmt.Errorf("获取账户信息失败: %v", err)
	}
	return a.emailService.TestConnection(account)
}

// ====================
// 邮件检查 API
// ====================

// CheckAllEmails 检查所有邮箱
func (a *App) CheckAllEmails() ([]models.EmailCheckResult, error) {
	if err := a.ensureServicesReady(); err != nil {
		return nil, err
	}

	accounts, err := a.db.GetEmailAccounts()
	if err != nil {
		return nil, fmt.Errorf("获取邮箱账户失败: %v", err)
	}

	results := make([]models.EmailCheckResult, 0, len(accounts))
	
	for _, account := range accounts {
		if !account.IsActive {
			continue
		}
		
		// 调用实际的邮件检查逻辑
		serviceResult := a.emailService.CheckAccountWithResult(&account)
		results = append(results, serviceResult)
	}
	
	return results, nil
}

// CheckSingleEmail 检查单个邮箱
func (a *App) CheckSingleEmail(accountID uint) (models.EmailCheckResult, error) {
	if err := a.ensureServicesReady(); err != nil {
		return models.EmailCheckResult{
			Error:   err.Error(),
			Success: false,
		}, err
	}
	
	account, err := a.db.GetEmailAccountByID(accountID)
	if err != nil {
		return models.EmailCheckResult{
			Error:   fmt.Sprintf("获取邮箱账户失败: %v", err),
			Success: false,
		}, err
	}

	// 调用实际的邮件检查逻辑
	serviceResult := a.emailService.CheckAccountWithResult(account)
	return serviceResult, nil
}

// StartEmailMonitoring 启动邮件监控
func (a *App) StartEmailMonitoring() error {
	if a.emailService == nil {
		return fmt.Errorf("邮件服务未初始化")
	}
	return a.emailService.StartEmailMonitoring()
}

// StopEmailMonitoring 停止邮件监控
func (a *App) StopEmailMonitoring() {
	if a.emailService != nil {
		a.emailService.StopEmailMonitoring()
	}
}

// ====================
// 下载任务管理 API
// ====================

// GetDownloadTasksResponse 下载任务列表响应
type GetDownloadTasksResponse struct {
	Tasks []models.DownloadTask `json:"tasks"`
	Total int                   `json:"total"`
}

// GetDownloadTasks 获取下载任务列表
func (a *App) GetDownloadTasks(page, pageSize int) (GetDownloadTasksResponse, error) {
	offset := (page - 1) * pageSize
	tasks, total, err := a.db.GetDownloadTasks(pageSize, offset)
	if err != nil {
		return GetDownloadTasksResponse{}, err
	}

	return GetDownloadTasksResponse{
		Tasks: tasks,
		Total: int(total),
	}, nil
}

// GetDownloadTasksByStatus 根据状态获取下载任务
func (a *App) GetDownloadTasksByStatus(status models.DownloadStatus) ([]models.DownloadTask, error) {
	return a.db.GetDownloadTasksByStatus(status)
}

// CreateDownloadTask 创建下载任务
func (a *App) CreateDownloadTask(task models.DownloadTask) error {
	if err := a.ensureServicesReady(); err != nil {
		return err
	}

	// 设置任务状态和时间
	task.Status = models.StatusPending
	
	// 使用数据库层的方法创建任务
	if err := a.db.CreateDownloadTask(&task); err != nil {
		return fmt.Errorf("创建下载任务失败: %v", err)
	}

	// 启动下载
	if err := a.downloadService.StartDownload(task.ID); err != nil {
		a.logger.Errorf("启动下载任务失败: %v", err)
		// 不返回错误，因为任务已经创建成功，下载失败可以稍后重试
	}

	return nil
}

// PauseDownloadTask 暂停下载任务
func (a *App) PauseDownloadTask(taskID uint) error {
	return a.downloadService.PauseDownload(taskID)
}

// ResumeDownloadTask 恢复下载任务
func (a *App) ResumeDownloadTask(taskID uint) error {
	return a.downloadService.StartDownload(taskID)
}

// CancelDownloadTask 取消下载任务
func (a *App) CancelDownloadTask(taskID uint) error {
	return a.downloadService.CancelDownload(taskID)
}

// GetActiveDownloads 获取活跃的下载任务
func (a *App) GetActiveDownloads() []models.DownloadTask {
	tasks, err := a.downloadService.GetAllTasks()
	if err != nil {
		return []models.DownloadTask{}
	}

	var activeTasks []models.DownloadTask
	for _, task := range tasks {
		if task.Status == models.StatusDownloading || task.Status == models.StatusPending {
			activeTasks = append(activeTasks, task)
		}
	}

	return activeTasks
}

// ====================
// 配置管理 API
// ====================

// GetStatistics 获取统计数据
func (a *App) GetStatistics(days int) ([]models.DownloadStatistics, error) {
	return a.db.GetStatistics(days)
}

// GetConfig 获取应用配置
func (a *App) GetConfig() (models.AppConfig, error) {
	return a.db.GetConfig()
}

// CreateConfig 创建配置
func (a *App) CreateConfig(config models.AppConfig) error {
	return a.db.CreateConfig(config)
}

// UpdateConfig 更新应用配置
func (a *App) UpdateConfig(config models.AppConfig) error {
	oldConfig, err := a.GetConfig()
	if err != nil {
		return err
	}

	// 更新配置
	if err := a.db.UpdateConfig(&config); err != nil {
		return err
	}

	// 处理配置变更
	a.handleConfigChange(&oldConfig, &config)
	
	return nil
}

// handleConfigChange 处理配置变更
func (a *App) handleConfigChange(oldConfig, newConfig *models.AppConfig) {
	// 更新下载服务的最大并发数
	if oldConfig.MaxConcurrent != newConfig.MaxConcurrent {
		a.downloadService.SetMaxConcurrent(newConfig.MaxConcurrent)
	}

	// 更新邮件检查间隔
	if oldConfig.CheckInterval != newConfig.CheckInterval {
		a.emailService.SetCheckInterval(time.Duration(newConfig.CheckInterval) * time.Second)
	}

	// 处理自动检查状态变更
	if oldConfig.AutoCheck != newConfig.AutoCheck {
		if newConfig.AutoCheck {
			if err := a.emailService.Start(); err != nil {
				a.logger.Errorf("启动邮件监控失败: %v", err)
			}
		} else {
			a.emailService.Stop()
		}
	}

	// 处理托盘状态变更
	if oldConfig.MinimizeToTray != newConfig.MinimizeToTray {
		if newConfig.MinimizeToTray {
			if err := a.trayService.Start(); err != nil {
				a.logger.Errorf("启动系统托盘失败: %v", err)
			}
		} else {
			a.trayService.Stop()
		}
	}
}

// ====================
// 统计和文件管理 API
// ====================

// OpenDownloadFolder 打开下载文件夹
func (a *App) OpenDownloadFolder() error {
	config, err := a.GetConfig()
	if err != nil {
		return err
	}

	// 检查目录是否存在
	if _, err := os.Stat(config.DownloadPath); os.IsNotExist(err) {
		// 创建目录
		if err := os.MkdirAll(config.DownloadPath, 0755); err != nil {
			return fmt.Errorf("创建下载目录失败: %v", err)
		}
	}

	// 使用系统默认程序打开文件夹
	return open.Run(config.DownloadPath)
}

// OpenFile 打开文件
func (a *App) OpenFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}

	// 使用系统默认程序打开文件
	return open.Run(filePath)
}

// SelectDownloadFolder 选择下载文件夹
func (a *App) SelectDownloadFolder() (string, error) {
	options := runtime.OpenDialogOptions{
		Title: "选择下载文件夹",
	}

	selectedPath, err := runtime.OpenDirectoryDialog(a.ctx, options)
	if err != nil {
		return "", err
	}

	return selectedPath, nil
}

// ====================
// 窗口和通知管理 API
// ====================

// MinimizeToTray 最小化到托盘
func (a *App) MinimizeToTray() {
	runtime.WindowHide(a.ctx)
}

// RestoreFromTray 从托盘恢复
func (a *App) RestoreFromTray() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// QuitApp 退出应用
func (a *App) QuitApp() {
	runtime.Quit(a.ctx)
}

// ShowNotification 显示通知
func (a *App) ShowNotification(title, message string) {
	config, err := a.GetConfig()
	if err != nil || !config.EnableNotification {
		return
	}

	// 使用系统托盘显示通知
	if a.trayService != nil {
		a.trayService.ShowNotification(title, message)
	}
}

// ====================
// 系统信息和状态 API
// ====================

// GetAppInfo 获取应用信息
func (a *App) GetAppInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":    "邮件附件下载器",
		"version": "1.0.0",
		"author":  "Assistant",
	}
}

// IsEmailServiceRunning 检查邮件服务是否运行
func (a *App) IsEmailServiceRunning() bool {
	return a.emailService != nil && a.emailService.IsRunning()
}

// GetActiveDownloadsCount 获取活跃下载数量
func (a *App) GetActiveDownloadsCount() int {
	return a.downloadService.GetActiveDownloads()
}

// GetServiceStatus 获取服务状态
func (a *App) GetServiceStatus() map[string]bool {
	return map[string]bool{
		"email":    a.IsEmailServiceRunning(),
		"download": a.downloadService != nil,
		"tray":     a.trayService != nil,
	}
}

// GetEmailMessages 获取邮件消息列表
func (a *App) GetEmailMessages(page, pageSize int) ([]models.EmailMessage, error) {
	offset := (page - 1) * pageSize
	return a.emailService.GetEmailMessages(pageSize, offset)
}

// initializeServices 初始化所有服务
func (a *App) initializeServices() error {
	a.initMutex.Lock()
	defer a.initMutex.Unlock()
	
	if a.isInitialized {
		return nil
	}
	
	a.logger.Info("开始初始化应用服务")
	
	// 初始化数据库
	db, err := database.NewDatabase()
	if err != nil {
		return fmt.Errorf("初始化数据库失败: %v", err)
	}
	a.db = db
	a.logger.Info("数据库初始化完成")
	
	// 初始化下载服务
	a.downloadService = services.NewDownloadService(db)
	a.logger.Info("下载服务初始化完成")
	
	// 初始化邮件服务
	a.emailService = services.NewEmailService(db, a.downloadService, a.logger)
	a.logger.Info("邮件服务初始化完成")
	
	// 初始化托盘服务
	a.trayService = services.NewTrayService(db, a.logger)
	a.logger.Info("托盘服务初始化完成")
	
	// 设置托盘回调
	a.setupTrayCallbacks()
	
	// 启动托盘服务
	if err := a.trayService.Start(); err != nil {
		a.logger.Errorf("启动托盘服务失败: %v", err)
		// 托盘服务失败不应该阻止应用启动
	}
	
	a.isInitialized = true
	a.logger.Info("所有服务初始化完成")
	
	return nil
}

// setupTrayCallbacks 设置托盘回调函数
func (a *App) setupTrayCallbacks() {
	a.trayService.SetCallbacks(
		func() { // onShow
			a.logger.Info("显示主窗口")
			a.RestoreFromTray()
		},
		func() { // onHide
			a.logger.Info("隐藏主窗口")
			a.MinimizeToTray()
		},
		func() { // onCheck
			a.logger.Info("用户触发邮件检查")
			go func() {
				results, err := a.CheckAllEmails()
				if err != nil {
					a.logger.Errorf("手动邮件检查失败: %v", err)
					a.ShowNotification("邮件检查失败", err.Error())
				} else {
					totalEmails := 0
					totalPDFs := 0
					for _, result := range results {
						if result.Success {
							totalEmails += result.NewEmails
							totalPDFs += result.PDFsFound
						}
					}
					a.ShowNotification("邮件检查完成", fmt.Sprintf("发现 %d 封新邮件，%d 个PDF文件", totalEmails, totalPDFs))
				}
			}()
		},
		func() { // onSettings
			a.logger.Info("打开设置页面")
			a.RestoreFromTray()
			// 前端需要实现路由跳转到设置页面
		},
		func() { // onQuit
			a.logger.Info("用户请求退出应用")
			go func() {
				a.shutdown()
				runtime.Quit(a.ctx)
			}()
		},
	)
}

// shutdown 优雅关闭应用
func (a *App) shutdown() {
	a.shutdownOnce.Do(func() {
		a.logger.Info("开始关闭应用")
		
		// 设置关闭状态
		a.shutdownMutex.Lock()
		a.isShuttingDown = true
		a.shutdownMutex.Unlock()
		
		// 停止所有服务
		var wg sync.WaitGroup
		
		// 停止邮件服务
		if a.emailService != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.emailService.StopEmailMonitoring()
				a.logger.Info("邮件服务已停止")
			}()
		}
		
		// 停止下载服务
		if a.downloadService != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.downloadService.Stop()
				a.logger.Info("下载服务已停止")
			}()
		}
		
		// 停止托盘服务
		if a.trayService != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a.trayService.Stop()
				a.logger.Info("托盘服务已停止")
			}()
		}
		
		// 等待所有服务停止（带超时）
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		
		select {
		case <-done:
			a.logger.Info("所有服务已正常停止")
		case <-time.After(30 * time.Second):
			a.logger.Warn("等待服务停止超时，强制退出")
		}
		
		// 关闭数据库连接
		if a.db != nil && a.db.DB != nil {
			if err := a.db.DB.Close(); err != nil {
				a.logger.Errorf("关闭数据库连接失败: %v", err)
			} else {
				a.logger.Info("数据库连接已关闭")
			}
		}
		
		// 取消上下文
		a.cancel()
		
		a.logger.Info("应用关闭完成")
	})
}

// showErrorDialog 显示错误对话框
func (a *App) showErrorDialog(title, message string) {
	// 这里应该调用Wails的对话框API，但为了保持兼容性，先记录日志
	a.logger.Errorf("错误对话框 - %s: %s", title, message)
	// TODO: 集成Wails对话框API
}

// 检查服务是否正在关闭的辅助方法
func (a *App) isServiceShuttingDown() bool {
	a.shutdownMutex.RLock()
	defer a.shutdownMutex.RUnlock()
	return a.isShuttingDown
}

// 等待服务初始化完成的辅助方法
func (a *App) waitForInitialization() error {
	// 最多等待30秒
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-timeout:
			return fmt.Errorf("等待服务初始化超时")
		case <-ticker.C:
			a.initMutex.RLock()
			initialized := a.isInitialized
			a.initMutex.RUnlock()
			
			if initialized {
				return nil
			}
		case <-a.ctx.Done():
			return fmt.Errorf("应用正在关闭")
		}
	}
}

// ensureServicesReady 确保服务已准备就绪的统一检查方法
func (a *App) ensureServicesReady() error {
	if err := a.waitForInitialization(); err != nil {
		return err
	}
	
	if a.isServiceShuttingDown() {
		return fmt.Errorf("服务正在关闭")
	}
	
	return nil
} 