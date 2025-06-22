package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/sirupsen/logrus"
	"github.com/skratchdot/open-golang/open"
)

// TrayService 系统托盘服务
type TrayService struct {
	logger    *logrus.Logger
	app       TrayApp
	isRunning bool
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	
	// 防抖机制
	lastClickTime time.Time
	clickMutex    sync.Mutex
}

// TrayApp 托盘应用接口
type TrayApp interface {
	RestoreFromTray()
	QuitApp()
	ShowNotification(title, message string)
}

// NewTrayService 创建托盘服务
func NewTrayService(logger *logrus.Logger, app TrayApp) *TrayService {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &TrayService{
		logger: logger,
		app:    app,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 启动托盘服务
func (ts *TrayService) Start() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	if ts.isRunning {
		return fmt.Errorf("托盘服务已在运行")
	}

	ts.isRunning = true
	
	// 使用RunWithExternalLoop避免阻塞
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ts.logger.Errorf("托盘服务崩溃: %v", r)
				ts.mu.Lock()
				ts.isRunning = false
				ts.mu.Unlock()
			}
		}()
		
		systray.Run(ts.onReady, ts.onExit)
	}()
	
	ts.logger.Info("系统托盘服务已启动")
	return nil
}

// Stop 停止托盘服务
func (ts *TrayService) Stop() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	if !ts.isRunning {
		return
	}

	ts.isRunning = false
	ts.cancel()
	
	// 安全退出托盘
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ts.logger.Errorf("停止托盘服务时出错: %v", r)
			}
		}()
		systray.Quit()
	}()
	
	ts.logger.Info("系统托盘服务已停止")
}

// IsRunning 检查服务是否运行中
func (ts *TrayService) IsRunning() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.isRunning
}

// onReady 托盘准备就绪回调
func (ts *TrayService) onReady() {
	defer func() {
		if r := recover(); r != nil {
			ts.logger.Errorf("托盘初始化失败: %v", r)
		}
	}()
	
	// 设置托盘图标
	ts.setTrayIcon()
	
	// 设置托盘提示
	systray.SetTooltip("邮件附件下载器")
	
	// 创建菜单项
	ts.createMenuItems()
	
	ts.logger.Info("系统托盘已就绪")
}

// onExit 托盘退出回调
func (ts *TrayService) onExit() {
	ts.logger.Info("系统托盘退出")
	ts.mu.Lock()
	ts.isRunning = false
	ts.mu.Unlock()
}

// setTrayIcon 设置托盘图标
func (ts *TrayService) setTrayIcon() {
	iconData := ts.loadIcon()
	if iconData != nil {
		systray.SetIcon(iconData)
	} else {
		// 使用默认图标
		systray.SetIcon(ts.getDefaultIcon())
		ts.logger.Warn("使用默认托盘图标")
	}
}

// loadIcon 加载图标
func (ts *TrayService) loadIcon() []byte {
	// 尝试从多个位置加载图标
	iconPaths := []string{
		"assets/icon.ico",
		"assets/icon.png",
		"./assets/icon.ico", 
		"./assets/icon.png",
		"icon.ico",
		"icon.png",
		"build/icon.ico",
		"build/icon.png",
	}
	
	// 获取可执行文件目录
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		iconPaths = append(iconPaths, 
			filepath.Join(execDir, "assets", "icon.ico"),
			filepath.Join(execDir, "assets", "icon.png"),
			filepath.Join(execDir, "icon.ico"),
			filepath.Join(execDir, "icon.png"),
		)
	}
	
	for _, iconPath := range iconPaths {
		if iconData, err := os.ReadFile(iconPath); err == nil {
			ts.logger.Infof("成功加载托盘图标: %s", iconPath)
			return iconData
		} else {
			ts.logger.Debugf("图标路径不存在: %s, 错误: %v", iconPath, err)
		}
	}
	
	ts.logger.Warn("未找到自定义图标，将使用默认图标")
	return nil
}

// getDefaultIcon 获取默认图标数据
func (ts *TrayService) getDefaultIcon() []byte {
	// 简单的默认图标数据（16x16 PNG）
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x91, 0x68,
		0x36, 0x00, 0x00, 0x00, 0x3C, 0x49, 0x44, 0x41,
		0x54, 0x28, 0x15, 0x63, 0xF8, 0x0F, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x18, 0xDD, 0x8D, 0xB4, 0x1C,
		0x20, 0x02, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}
}

// createMenuItems 创建菜单项
func (ts *TrayService) createMenuItems() {
	// 显示主窗口
	mShow := systray.AddMenuItem("显示主窗口", "显示应用主窗口")
	go ts.handleMenuClick(mShow.ClickedCh, "show", func() {
		ts.app.RestoreFromTray()
	})
	
	// 分隔符
	systray.AddSeparator()
	
	// 打开下载文件夹
	mOpenFolder := systray.AddMenuItem("打开下载文件夹", "打开下载文件夹")
	go ts.handleMenuClick(mOpenFolder.ClickedCh, "folder", func() {
		ts.openDownloadFolder()
	})
	
	// 分隔符
	systray.AddSeparator()
	
	// 退出应用
	mQuit := systray.AddMenuItem("退出", "退出应用")
	go ts.handleMenuClick(mQuit.ClickedCh, "quit", func() {
		ts.app.QuitApp()
	})
}

// handleMenuClick 处理菜单点击事件，带防抖机制
func (ts *TrayService) handleMenuClick(clickedCh chan struct{}, action string, handler func()) {
	for {
		select {
		case <-clickedCh:
			// 防抖机制：防止快速重复点击
			ts.clickMutex.Lock()
			now := time.Now()
			if now.Sub(ts.lastClickTime) < 300*time.Millisecond {
				ts.clickMutex.Unlock()
				continue
			}
			ts.lastClickTime = now
			ts.clickMutex.Unlock()
			
			// 安全执行处理函数
			go func() {
				defer func() {
					if r := recover(); r != nil {
						ts.logger.Errorf("处理托盘菜单点击(%s)时出错: %v", action, r)
					}
				}()
				handler()
			}()
			
		case <-ts.ctx.Done():
			return
		}
	}
}

// openDownloadFolder 打开下载文件夹
func (ts *TrayService) openDownloadFolder() {
	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		ts.logger.Errorf("获取用户主目录失败: %v", err)
		return
	}
	
	// 默认下载路径
	downloadPath := filepath.Join(homeDir, "Downloads", "EmailPDFs")
	
	// 确保目录存在
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		ts.logger.Errorf("创建下载目录失败: %v", err)
		return
	}
	
	// 打开目录
	if err := ts.openPath(downloadPath); err != nil {
		ts.logger.Errorf("打开下载目录失败: %v", err)
	}
}

// openPath 跨平台打开路径
func (ts *TrayService) openPath(path string) error {
	switch runtime.GOOS {
	case "windows":
		return open.Run(path)
	case "darwin":
		return open.Run(path)
	case "linux":
		return open.Run(path)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// ShowNotification 显示通知
func (ts *TrayService) ShowNotification(title, message string) {
	ts.logger.Infof("通知: %s - %s", title, message)
	
	// 根据操作系统显示通知
	switch runtime.GOOS {
	case "windows":
		ts.showWindowsNotification(title, message)
	case "darwin":
		ts.showMacNotification(title, message)
	case "linux":
		ts.showLinuxNotification(title, message)
	default:
		ts.logger.Warnf("不支持的操作系统通知: %s", runtime.GOOS)
	}
}

// showWindowsNotification Windows通知
func (ts *TrayService) showWindowsNotification(title, message string) {
	ts.logger.Infof("Windows通知: %s - %s", title, message)
	// 这里可以使用Windows API显示通知
}

// showMacNotification macOS通知
func (ts *TrayService) showMacNotification(title, message string) {
	ts.logger.Infof("macOS通知: %s - %s", title, message)
	// 这里可以使用macOS API显示通知
}

// showLinuxNotification Linux通知
func (ts *TrayService) showLinuxNotification(title, message string) {
	ts.logger.Infof("Linux通知: %s - %s", title, message)
	// 这里可以使用Linux API显示通知
} 