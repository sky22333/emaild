package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"emaild/backend/database"
	"emaild/backend/models"
	"emaild/backend/services"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/skratchdot/open-golang/open"
)

// EmailCheckResult 邮件检查结果
type EmailCheckResult struct {
	Account   *models.EmailAccount `json:"account"`
	NewEmails int                  `json:"new_emails"`
	PDFsFound int                  `json:"pdfs_found"`
	Error     string               `json:"error,omitempty"`
	Success   bool                 `json:"success"`
}

// App 主应用结构体
type App struct {
	ctx             context.Context
	logger          *logrus.Logger
	db              *database.Database
	emailService    *services.EmailService
	downloadService *services.DownloadService
	trayService     *services.TrayService
}

// NewApp 创建应用实例
func NewApp() *App {
	// 初始化日志
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	return &App{
		logger: logger,
	}
}

// OnStartup 应用启动时的回调
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	a.logger.Info("应用启动中...")

	// 初始化数据库
	db, err := database.NewDatabase()
	if err != nil {
		a.logger.Errorf("数据库初始化失败: %v", err)
		// 显示错误对话框给用户
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "数据库初始化失败",
			Message: fmt.Sprintf("无法初始化数据库，请检查文件权限和磁盘空间。\n错误信息: %v", err),
		})
		return
	}
	a.db = db

	// 初始化服务（按正确的依赖顺序）
	a.downloadService = services.NewDownloadService(a.db)
	a.emailService = services.NewEmailService(a.db, a.downloadService, a.logger)
	a.trayService = services.NewTrayService(a.logger, a)

	// 获取配置并启动服务
	config, err := a.getOrCreateDefaultConfig()
	if err != nil {
		a.logger.Errorf("获取配置失败: %v", err)
		return
	}

	// 设置下载服务的最大并发数
	a.downloadService.SetMaxConcurrent(config.MaxConcurrent)

	// 启动系统托盘（如果启用）
	if config.MinimizeToTray {
		if err := a.trayService.Start(); err != nil {
			a.logger.Errorf("启动系统托盘失败: %v", err)
		}
	}

	// 如果启用自动检查，启动邮件监控
	if config.AutoCheck {
		a.emailService.SetCheckInterval(time.Duration(config.CheckInterval) * time.Second)
		if err := a.emailService.Start(); err != nil {
			a.logger.Errorf("启动邮件监控失败: %v", err)
		}
	}

	a.logger.Info("应用启动完成")
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
	return a.db.CreateEmailAccount(&account)
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

// ====================
// 邮件检查 API
// ====================

// CheckAllEmails 检查所有邮箱
func (a *App) CheckAllEmails() ([]EmailCheckResult, error) {
	if a.emailService == nil {
		return nil, fmt.Errorf("邮件服务未初始化")
	}

	accounts, err := a.db.GetEmailAccounts()
	if err != nil {
		return nil, fmt.Errorf("获取邮箱账户失败: %v", err)
	}

	results := make([]EmailCheckResult, 0, len(accounts))
	
	for _, account := range accounts {
		if !account.IsActive {
			continue
		}
		
		// 调用实际的邮件检查逻辑
		serviceResult := a.emailService.CheckAccountWithResult(&account)
		// 转换为App包的EmailCheckResult
		result := EmailCheckResult{
			Account:   serviceResult.Account,
			NewEmails: serviceResult.NewEmails,
			PDFsFound: serviceResult.PDFsFound,
			Error:     serviceResult.Error,
			Success:   serviceResult.Success,
		}
		results = append(results, result)
	}
	
	return results, nil
}

// CheckSingleEmail 检查单个邮箱
func (a *App) CheckSingleEmail(accountID uint) (EmailCheckResult, error) {
	account, err := a.db.GetEmailAccountByID(accountID)
	if err != nil {
		return EmailCheckResult{
			Error:   fmt.Sprintf("获取邮箱账户失败: %v", err),
			Success: false,
		}, err
	}

	if a.emailService == nil {
		return EmailCheckResult{
			Account: account,
			Error:   "邮件服务未初始化",
			Success: false,
		}, fmt.Errorf("邮件服务未初始化")
	}

	// 调用实际的邮件检查逻辑
	serviceResult := a.emailService.CheckAccountWithResult(account)
	// 转换为App包的EmailCheckResult
	result := EmailCheckResult{
		Account:   serviceResult.Account,
		NewEmails: serviceResult.NewEmails,
		PDFsFound: serviceResult.PDFsFound,
		Error:     serviceResult.Error,
		Success:   serviceResult.Success,
	}
	return result, nil
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
	// 设置任务时间
	now := time.Now()
	task.CreatedAt = models.TimeToString(now)
	task.UpdatedAt = models.TimeToString(now)
	task.Status = models.StatusPending

	// 保存到数据库
	tx, err := a.db.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO download_tasks (email_id, subject, sender, file_name, file_size, downloaded_size, status, type, source, local_path, progress, speed, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	result, err := tx.Exec(query,
		task.EmailID, task.Subject, task.Sender, task.FileName, task.FileSize,
		task.DownloadedSize, task.Status, task.Type, task.Source, task.LocalPath,
		task.Progress, task.Speed, now, now,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// 启动下载
	return a.downloadService.StartDownload(uint(id))
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