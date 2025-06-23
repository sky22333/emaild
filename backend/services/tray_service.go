package services

import (
	"fmt"
	"sync"
	_ "embed"
	"context"
	"time"

	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"

	"emaild/backend/database"
)

//go:embed icon.ico
var iconData []byte

// TrayService 系统托盘服务
type TrayService struct {
	db     *database.Database
	logger *logrus.Logger
	
	// 菜单项
	mShow     *systray.MenuItem
	mHide     *systray.MenuItem
	mCheck    *systray.MenuItem
	mSettings *systray.MenuItem
	mQuit     *systray.MenuItem
	
	// 状态
	isVisible bool
	mutex     sync.RWMutex
	
	// 回调函数
	onShow     func()
	onHide     func()
	onCheck    func()
	onSettings func()
	onQuit     func()
	
	// 优雅关闭相关
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	shutdownOnce   sync.Once
	isShuttingDown bool
	shutdownMutex  sync.RWMutex
}

// NewTrayService 创建系统托盘服务
func NewTrayService(db *database.Database, logger *logrus.Logger) *TrayService {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &TrayService{
		db:             db,
		logger:         logger,
		isVisible:      true,
		ctx:            ctx,
		cancel:         cancel,
		isShuttingDown: false,
	}
}

// Start 启动系统托盘
func (ts *TrayService) Start() error {
	ts.logger.Info("启动系统托盘服务")
	
	ts.wg.Add(1)
	go func() {
		defer ts.wg.Done()
		systray.Run(ts.onReady, ts.onExit)
	}()
	
	return nil
}

// Stop 停止系统托盘
func (ts *TrayService) Stop() {
	ts.shutdownOnce.Do(func() {
		ts.logger.Info("开始停止系统托盘服务")
		
		// 设置关闭状态
		ts.shutdownMutex.Lock()
		ts.isShuttingDown = true
		ts.shutdownMutex.Unlock()
		
		// 取消上下文
		ts.cancel()
		
		// 退出系统托盘
		systray.Quit()
		
		// 等待goroutine完成（带超时）
		done := make(chan struct{})
		go func() {
			ts.wg.Wait()
			close(done)
		}()
		
		select {
		case <-done:
			ts.logger.Info("系统托盘服务已正常停止")
		case <-time.After(10 * time.Second):
			ts.logger.Warn("等待系统托盘服务停止超时")
		}
	})
}

// onReady 托盘就绪回调
func (ts *TrayService) onReady() {
	ts.logger.Info("系统托盘就绪")
	
	// 设置内嵌的图标
	if len(iconData) > 0 {
		systray.SetIcon(iconData)
		ts.logger.Info("已设置内嵌托盘图标")
	} else {
		ts.logger.Warn("内嵌图标数据为空")
	}
	
	// 设置标题和提示
	systray.SetTitle("邮件附件下载器")
	systray.SetTooltip("邮件附件下载器 - 自动下载邮件中的PDF附件")
	
	// 创建菜单项
	ts.createMenuItems()
	
	// 启动菜单事件处理
	ts.wg.Add(1)
	go ts.handleMenuEvents()
}

// onExit 托盘退出回调
func (ts *TrayService) onExit() {
	ts.logger.Info("系统托盘退出")
}

// createMenuItems 创建菜单项
func (ts *TrayService) createMenuItems() {
	ts.mShow = systray.AddMenuItem("显示主窗口", "显示主窗口")
	ts.mHide = systray.AddMenuItem("隐藏主窗口", "隐藏主窗口")
	systray.AddSeparator()
	ts.mCheck = systray.AddMenuItem("立即检查邮件", "立即检查所有邮箱的新邮件")
	systray.AddSeparator()
	ts.mSettings = systray.AddMenuItem("设置", "打开设置页面")
	systray.AddSeparator()
	ts.mQuit = systray.AddMenuItem("退出", "退出程序")
	
	// 初始状态
	ts.updateMenuState()
}

// handleMenuEvents 处理菜单事件
func (ts *TrayService) handleMenuEvents() {
	defer ts.wg.Done()
	
	for {
		select {
		case <-ts.ctx.Done():
			ts.logger.Info("托盘菜单事件处理器收到关闭信号")
			return
			
		case <-ts.mShow.ClickedCh:
			// 检查是否正在关闭
			ts.shutdownMutex.RLock()
			if ts.isShuttingDown {
				ts.shutdownMutex.RUnlock()
				return
			}
			ts.shutdownMutex.RUnlock()
			
			if ts.onShow != nil {
				ts.onShow()
			}
			ts.setVisible(true)
			
		case <-ts.mHide.ClickedCh:
			ts.shutdownMutex.RLock()
			if ts.isShuttingDown {
				ts.shutdownMutex.RUnlock()
				return
			}
			ts.shutdownMutex.RUnlock()
			
			if ts.onHide != nil {
				ts.onHide()
			}
			ts.setVisible(false)
			
		case <-ts.mCheck.ClickedCh:
			ts.shutdownMutex.RLock()
			if ts.isShuttingDown {
				ts.shutdownMutex.RUnlock()
				return
			}
			ts.shutdownMutex.RUnlock()
			
			if ts.onCheck != nil {
				ts.onCheck()
			}
			
		case <-ts.mSettings.ClickedCh:
			ts.shutdownMutex.RLock()
			if ts.isShuttingDown {
				ts.shutdownMutex.RUnlock()
				return
			}
			ts.shutdownMutex.RUnlock()
			
			if ts.onSettings != nil {
				ts.onSettings()
			}
			
		case <-ts.mQuit.ClickedCh:
			ts.logger.Info("用户点击退出菜单")
			if ts.onQuit != nil {
				ts.onQuit()
			}
			return
		}
	}
}

// updateMenuState 更新菜单状态
func (ts *TrayService) updateMenuState() {
	ts.mutex.RLock()
	isVisible := ts.isVisible
	ts.mutex.RUnlock()
	
	if isVisible {
		ts.mShow.Hide()
		ts.mHide.Show()
	} else {
		ts.mShow.Show()
		ts.mHide.Hide()
	}
}

// setVisible 设置窗口可见性
func (ts *TrayService) setVisible(visible bool) {
	ts.mutex.Lock()
	ts.isVisible = visible
	ts.mutex.Unlock()
	
	ts.updateMenuState()
}

// SetCallbacks 设置回调函数
func (ts *TrayService) SetCallbacks(onShow, onHide, onCheck, onSettings, onQuit func()) {
	ts.onShow = onShow
	ts.onHide = onHide
	ts.onCheck = onCheck
	ts.onSettings = onSettings
	ts.onQuit = onQuit
}

// ShowNotification 显示通知
func (ts *TrayService) ShowNotification(title, message string) {
	ts.logger.Infof("托盘通知: %s - %s", title, message)
	// systray包本身不支持通知，这里只记录日志
}

// UpdateStatus 更新状态
func (ts *TrayService) UpdateStatus(status string) {
	systray.SetTooltip(fmt.Sprintf("邮件附件下载器 - %s", status))
} 