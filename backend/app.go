package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"emaild/backend/database"
	"emaild/backend/models"
	"emaild/backend/services"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 主应用结构体
type App struct {
	ctx             context.Context
	logger          *logrus.Logger
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
	if err := database.InitDatabase(); err != nil {
		a.logger.Fatalf("数据库初始化失败: %v", err)
	}

	// 获取配置
	config, err := database.GetConfig()
	if err != nil {
		a.logger.Errorf("获取配置失败: %v", err)
		return
	}

	// 初始化服务
	a.emailService = services.NewEmailService(a.logger)
	a.downloadService = services.NewDownloadService(a.logger, config.MaxConcurrent)
	a.trayService = services.NewTrayService(a.logger, a)

	// 启动下载服务
	if err := a.downloadService.Start(); err != nil {
		a.logger.Errorf("启动下载服务失败: %v", err)
	}

	// 启动系统托盘（如果启用）
	if config.MinimizeToTray {
		if err := a.trayService.Start(); err != nil {
			a.logger.Errorf("启动系统托盘失败: %v", err)
		}
	}

	// 如果启用自动检查，启动邮件监控
	if config.AutoCheck {
		if err := a.emailService.StartEmailMonitoring(); err != nil {
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
		a.emailService.StopEmailMonitoring()
	}

	if a.downloadService != nil {
		a.downloadService.Stop()
	}

	if a.trayService != nil {
		a.trayService.Stop()
	}

	// 关闭数据库连接
	if err := database.CloseDatabase(); err != nil {
		a.logger.Errorf("关闭数据库失败: %v", err)
	}

	a.logger.Info("应用已关闭")
}

// OnDomReady 前端DOM准备完成时的回调
func (a *App) OnDomReady(ctx context.Context) {
	// 检查是否需要在启动时最小化
	config, err := database.GetConfig()
	if err == nil && config.StartMinimized {
		runtime.WindowMinimise(ctx)
	}
}

// ====================
// 邮箱账户管理 API
// ====================

// GetEmailAccounts 获取所有邮箱账户
func (a *App) GetEmailAccounts() ([]models.EmailAccount, error) {
	return database.GetEmailAccounts()
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

	return database.CreateEmailAccount(&account)
}

// UpdateEmailAccount 更新邮箱账户
func (a *App) UpdateEmailAccount(account models.EmailAccount) error {
	// 测试连接
	if err := a.emailService.TestConnection(&account); err != nil {
		return fmt.Errorf("邮箱连接测试失败: %v", err)
	}

	return database.UpdateEmailAccount(&account)
}

// DeleteEmailAccount 删除邮箱账户
func (a *App) DeleteEmailAccount(id uint) error {
	return database.DeleteEmailAccount(id)
}

// TestEmailConnection 测试邮箱连接
func (a *App) TestEmailConnection(account models.EmailAccount) error {
	return a.emailService.TestConnection(&account)
}

// ====================
// 邮件检查 API
// ====================

// CheckAllEmails 检查所有邮箱
func (a *App) CheckAllEmails() ([]services.EmailCheckResult, error) {
	results := a.emailService.CheckAllAccounts()
	return results, nil
}

// CheckSingleEmail 检查单个邮箱
func (a *App) CheckSingleEmail(accountID uint) (services.EmailCheckResult, error) {
	account, err := database.GetEmailAccountByID(accountID)
	if err != nil {
		return services.EmailCheckResult{}, err
	}

	result := a.emailService.CheckAccount(account)
	return result, nil
}

// StartEmailMonitoring 启动邮件监控
func (a *App) StartEmailMonitoring() error {
	return a.emailService.StartEmailMonitoring()
}

// StopEmailMonitoring 停止邮件监控
func (a *App) StopEmailMonitoring() {
	a.emailService.StopEmailMonitoring()
}

// ====================
// 下载任务管理 API
// ====================

// GetDownloadTasksResponse 下载任务列表响应
type GetDownloadTasksResponse struct {
	Tasks []models.DownloadTask `json:"tasks"`
	Total int64                 `json:"total"`
}

// GetDownloadTasks 获取下载任务列表
func (a *App) GetDownloadTasks(page, pageSize int) (GetDownloadTasksResponse, error) {
	offset := (page - 1) * pageSize
	tasks, total, err := database.GetDownloadTasks(pageSize, offset)
	if err != nil {
		return GetDownloadTasksResponse{}, err
	}
	
	return GetDownloadTasksResponse{
		Tasks: tasks,
		Total: total,
	}, nil
}

// GetDownloadTasksByStatus 根据状态获取下载任务
func (a *App) GetDownloadTasksByStatus(status models.DownloadStatus) ([]models.DownloadTask, error) {
	return database.GetDownloadTasksByStatus(status)
}

// CreateDownloadTask 创建下载任务
func (a *App) CreateDownloadTask(task models.DownloadTask) error {
	return a.downloadService.AddTask(&task)
}

// PauseDownloadTask 暂停下载任务
func (a *App) PauseDownloadTask(taskID uint) error {
	return a.downloadService.PauseTask(taskID)
}

// ResumeDownloadTask 恢复下载任务
func (a *App) ResumeDownloadTask(taskID uint) error {
	return a.downloadService.ResumeTask(taskID)
}

// CancelDownloadTask 取消下载任务
func (a *App) CancelDownloadTask(taskID uint) error {
	return a.downloadService.CancelTask(taskID)
}

// GetActiveDownloads 获取活跃的下载任务
func (a *App) GetActiveDownloads() []models.DownloadTask {
	activeTasks := a.downloadService.GetActiveDownloads()
	result := make([]models.DownloadTask, len(activeTasks))
	for i, task := range activeTasks {
		if task != nil {
			result[i] = *task
		}
	}
	return result
}

// ====================
// 配置管理 API
// ====================

// GetConfig 获取应用配置
func (a *App) GetConfig() (models.AppConfig, error) {
	config, err := database.GetConfig()
	if err != nil {
		return models.AppConfig{}, err
	}
	return *config, nil
}

// UpdateConfig 更新应用配置
func (a *App) UpdateConfig(config models.AppConfig) error {
	oldConfig, err := database.GetConfig()
	if err != nil {
		return err
	}

	// 更新数据库
	if err := database.UpdateConfig(&config); err != nil {
		return err
	}

	// 处理配置变化
	a.handleConfigChange(oldConfig, &config)

	return nil
}

// handleConfigChange 处理配置变化
func (a *App) handleConfigChange(oldConfig, newConfig *models.AppConfig) {
	// 处理并发下载数变化
	if oldConfig.MaxConcurrent != newConfig.MaxConcurrent {
		a.downloadService.Stop()
		a.downloadService = services.NewDownloadService(a.logger, newConfig.MaxConcurrent)
		a.downloadService.Start()
	}

	// 处理托盘设置变化
	if oldConfig.MinimizeToTray != newConfig.MinimizeToTray {
		if newConfig.MinimizeToTray {
			a.trayService.Start()
		} else {
			a.trayService.Stop()
		}
	}

	// 处理邮件监控设置变化
	if oldConfig.AutoCheck != newConfig.AutoCheck {
		if newConfig.AutoCheck {
			a.emailService.StartEmailMonitoring()
		} else {
			a.emailService.StopEmailMonitoring()
		}
	}
}

// ====================
// 统计数据 API
// ====================

// GetStatistics 获取统计数据
func (a *App) GetStatistics(days int) ([]models.DownloadStatistics, error) {
	return database.GetStatistics(days)
}

// ====================
// 文件管理 API
// ====================

// OpenDownloadFolder 打开下载文件夹
func (a *App) OpenDownloadFolder() error {
	config, err := database.GetConfig()
	if err != nil {
		return err
	}

	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(config.DownloadPath))
	return nil
}

// OpenFile 打开文件
func (a *App) OpenFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}

	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(filePath))
	return nil
}

// SelectDownloadFolder 选择下载文件夹
func (a *App) SelectDownloadFolder() (string, error) {
	options := runtime.OpenDialogOptions{
		Title: "选择下载文件夹",
	}

	path, err := runtime.OpenDirectoryDialog(a.ctx, options)
	if err != nil {
		return "", err
	}

	return path, nil
}

// ====================
// 窗口管理 API
// ====================

// MinimizeToTray 最小化到系统托盘
func (a *App) MinimizeToTray() {
	runtime.WindowHide(a.ctx)
}

// RestoreFromTray 从系统托盘恢复
func (a *App) RestoreFromTray() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// QuitApp 退出应用
func (a *App) QuitApp() {
	runtime.Quit(a.ctx)
}

// ====================
// 通知 API
// ====================

// ShowNotification 显示系统通知
func (a *App) ShowNotification(title, message string) {
	if a.trayService != nil {
		a.trayService.ShowNotification(title, message)
	}
}

// ====================
// 日志 API
// ====================

// GetLogs 获取应用日志
func (a *App) GetLogs(lines int) ([]string, error) {
	// 这里可以实现日志读取逻辑
	// 暂时返回空列表
	return []string{}, nil
}

// ====================
// 工具方法
// ====================

// GetAppInfo 获取应用信息
func (a *App) GetAppInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":    "邮件附件下载器",
		"version": "1.0.0",
		"author":  "Developer",
	}
}

// IsEmailServiceRunning 检查邮件服务是否运行
func (a *App) IsEmailServiceRunning() bool {
	return a.emailService != nil
}

// IsDownloadServiceRunning 检查下载服务是否运行
func (a *App) IsDownloadServiceRunning() bool {
	return a.downloadService != nil
}

// GetServiceStatus 获取服务状态
func (a *App) GetServiceStatus() map[string]bool {
	return map[string]bool{
		"email":    a.IsEmailServiceRunning(),
		"download": a.IsDownloadServiceRunning(),
	}
} 