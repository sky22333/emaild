package services

import (
	"fmt"

	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"
)

// TrayService 系统托盘服务
type TrayService struct {
	logger    *logrus.Logger
	app       TrayApp // 应用接口
	isRunning bool
}

// TrayApp 应用接口，用于托盘服务调用应用方法
type TrayApp interface {
	RestoreFromTray()
	QuitApp()
	GetServiceStatus() map[string]bool
}

// NewTrayService 创建系统托盘服务
func NewTrayService(logger *logrus.Logger, app TrayApp) *TrayService {
	return &TrayService{
		logger: logger,
		app:    app,
	}
}

// Start 启动系统托盘
func (ts *TrayService) Start() error {
	if ts.isRunning {
		return fmt.Errorf("系统托盘已经在运行中")
	}

	ts.logger.Info("启动系统托盘")
	
	// 在独立的goroutine中运行systray
	go func() {
		systray.Run(ts.onReady, ts.onExit)
	}()

	ts.isRunning = true
	return nil
}

// Stop 停止系统托盘
func (ts *TrayService) Stop() {
	if !ts.isRunning {
		return
	}

	ts.logger.Info("停止系统托盘")
	systray.Quit()
	ts.isRunning = false
}

// onReady 托盘准备就绪时的回调
func (ts *TrayService) onReady() {
	// 设置托盘图标和标题
	systray.SetTitle("邮件附件下载器")
	systray.SetTooltip("邮件附件下载器 - 自动下载邮件中的PDF文件")
	
	// 设置托盘图标，添加错误处理
	iconData := getTrayIcon()
	if len(iconData) > 0 {
		systray.SetIcon(iconData)
		ts.logger.Info("托盘图标设置成功")
	} else {
		ts.logger.Warn("托盘图标数据为空，跳过图标设置")
	}

	// 创建托盘菜单
	ts.createTrayMenu()
}

// onExit 托盘退出时的回调
func (ts *TrayService) onExit() {
	ts.logger.Info("系统托盘已退出")
}

// createTrayMenu 创建托盘菜单
func (ts *TrayService) createTrayMenu() {
	// 显示主窗口
	mShow := systray.AddMenuItem("显示主窗口", "显示主窗口")
	go func() {
		for range mShow.ClickedCh {
			ts.app.RestoreFromTray()
		}
	}()

	// 分隔符
	systray.AddSeparator()

	// 服务状态
	mStatus := systray.AddMenuItem("服务状态", "查看服务运行状态")
	mStatus.Disable() // 状态菜单项不可点击

	// 定期更新服务状态
	go ts.updateServiceStatus(mStatus)

	// 分隔符
	systray.AddSeparator()

	// 快速操作菜单
	mQuickCheck := systray.AddMenuItem("立即检查邮件", "立即检查所有邮箱的新邮件")
	go func() {
		for range mQuickCheck.ClickedCh {
			ts.logger.Info("用户通过托盘触发邮件检查")
			// 这里可以调用邮件检查功能
			ts.ShowNotification("邮件检查", "正在检查新邮件...")
		}
	}()

	mOpenDownloadFolder := systray.AddMenuItem("打开下载文件夹", "打开PDF文件下载目录")
	go func() {
		for range mOpenDownloadFolder.ClickedCh {
			ts.logger.Info("用户通过托盘打开下载文件夹")
			// 这里可以调用打开文件夹功能
		}
	}()

	// 分隔符
	systray.AddSeparator()

	// 设置菜单
	mSettings := systray.AddMenuItem("设置", "打开应用设置")
	go func() {
		for range mSettings.ClickedCh {
			ts.app.RestoreFromTray()
			// 这里可以直接跳转到设置页面
		}
	}()

	// 关于菜单
	mAbout := systray.AddMenuItem("关于", "关于邮件附件下载器")
	go func() {
		for range mAbout.ClickedCh {
			ts.app.RestoreFromTray()
			// 这里可以显示关于对话框
		}
	}()

	// 分隔符
	systray.AddSeparator()

	// 退出菜单
	mQuit := systray.AddMenuItem("退出", "退出邮件附件下载器")
	go func() {
		for range mQuit.ClickedCh {
			ts.logger.Info("用户通过托盘退出应用")
			ts.app.QuitApp()
		}
	}()
}

// updateServiceStatus 更新服务状态显示
func (ts *TrayService) updateServiceStatus(statusMenuItem *systray.MenuItem) {
	// 这个功能可以定期更新服务状态显示
	// 由于systray的限制，这里简化处理
	status := ts.app.GetServiceStatus()
	
	statusText := "服务状态: "
	if status["email"] {
		statusText += "邮件✓ "
	} else {
		statusText += "邮件✗ "
	}
	
	if status["download"] {
		statusText += "下载✓"
	} else {
		statusText += "下载✗"
	}
	
	statusMenuItem.SetTitle(statusText)
}

// ShowNotification 显示系统通知
func (ts *TrayService) ShowNotification(title, message string) {
	ts.logger.Infof("显示通知: %s - %s", title, message)
	
	// 在Windows上，可以使用系统通知
	// 这里提供一个基本实现
	if ts.isRunning {
		// systray本身不直接支持通知，但可以通过其他方式实现
		// 例如在Windows上使用toast通知
		ts.showSystemNotification(title, message)
	}
}

// showSystemNotification 显示系统级通知
func (ts *TrayService) showSystemNotification(title, message string) {
	// 这里可以实现特定平台的通知功能
	// 例如Windows的Toast通知、macOS的NSUserNotification等
	ts.logger.Infof("系统通知: %s - %s", title, message)
}

// getTrayIcon 获取托盘图标数据
func getTrayIcon() []byte {
	// 返回一个简单的16x16像素ICO图标数据
	// 这是一个最小化的有效ICO文件，显示为一个简单的方块
	return []byte{
		0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10, 0x00, 0x00, 0x01, 0x00, 0x04, 0x00, 0x28, 0x01,
		0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x20, 0x00,
		0x00, 0x00, 0x01, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x80, 0x80, 0x00, 0x80, 0x00,
		0x00, 0x00, 0x80, 0x00, 0x80, 0x00, 0x80, 0x80, 0x00, 0x00, 0xC0, 0xC0, 0xC0, 0x00, 0x80, 0x80,
		0x80, 0x00, 0x00, 0x00, 0xFF, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0x00, 0xFF, 0x00,
		0x00, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0x00, 0x77, 0x77,
		0x77, 0x77, 0x77, 0x77, 0x77, 0x77, 0x70, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x70, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x70, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x70, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x70, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x70, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x70, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x77, 0x77,
		0x77, 0x77, 0x77, 0x77, 0x77, 0x77, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0x80, 0x00, 0x00, 0x01, 0x80, 0x00,
		0x00, 0x01, 0x80, 0x00, 0x00, 0x01, 0x80, 0x00, 0x00, 0x01, 0x80, 0x00, 0x00, 0x01, 0x80, 0x00,
		0x00, 0x01, 0x80, 0x00, 0x00, 0x01, 0xFF, 0xFF, 0xFF, 0xFF,
	}
}

// IsRunning 检查托盘服务是否正在运行
func (ts *TrayService) IsRunning() bool {
	return ts.isRunning
}

// UpdateIcon 更新托盘图标（例如根据状态变化）
func (ts *TrayService) UpdateIcon(hasNewEmails bool) {
	if !ts.isRunning {
		return
	}

	var iconData []byte
	var tooltip string
	
	if hasNewEmails {
		// 设置有新邮件时的图标
		iconData = getTrayIconWithNotification()
		tooltip = "邮件附件下载器 - 有新的PDF文件"
	} else {
		// 设置正常图标
		iconData = getTrayIcon()
		tooltip = "邮件附件下载器 - 运行中"
	}
	
	if len(iconData) > 0 {
		systray.SetIcon(iconData)
		systray.SetTooltip(tooltip)
		ts.logger.Debug("托盘图标已更新")
	} else {
		ts.logger.Warn("图标数据为空，跳过图标更新")
	}
}

// getTrayIconWithNotification 获取带通知标识的托盘图标
func getTrayIconWithNotification() []byte {
	// 返回带有通知标识的图标数据
	// 可以是原图标上加上小红点或其他标识
	return getTrayIcon() // 简化实现
}

// SetTooltip 设置托盘提示文本
func (ts *TrayService) SetTooltip(text string) {
	if ts.isRunning {
		systray.SetTooltip(text)
	}
}

// HandleEmailCheckResult 处理邮件检查结果
func (ts *TrayService) HandleEmailCheckResult(totalPDFs int, errors []string) {
	if !ts.isRunning {
		return
	}

	if totalPDFs > 0 {
		title := "发现新的PDF文件"
		message := fmt.Sprintf("找到 %d 个PDF文件正在下载", totalPDFs)
		ts.ShowNotification(title, message)
		ts.UpdateIcon(true)
	}

	if len(errors) > 0 {
		title := "邮件检查出现错误"
		message := fmt.Sprintf("有 %d 个邮箱检查失败", len(errors))
		ts.ShowNotification(title, message)
	}
}

// HandleDownloadComplete 处理下载完成事件
func (ts *TrayService) HandleDownloadComplete(fileName string, success bool) {
	if !ts.isRunning {
		return
	}

	if success {
		title := "文件下载完成"
		message := fmt.Sprintf("已下载: %s", fileName)
		ts.ShowNotification(title, message)
	} else {
		title := "文件下载失败"
		message := fmt.Sprintf("下载失败: %s", fileName)
		ts.ShowNotification(title, message)
	}
} 