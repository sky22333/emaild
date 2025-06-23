package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"mime/quotedprintable"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/sirupsen/logrus"

	"emaild/backend/database"
	"emaild/backend/models"
	"emaild/backend/utils"
)

// DownloadService 下载服务
type DownloadService struct {
	db                *database.Database
	workers           map[uint]*DownloadWorker // 按任务ID管理的工作者
	workerMutex       sync.RWMutex             // 保护workers map的读写锁
	maxConcurrent     int                      // 最大并发数
	activeWorkers     int                      // 当前活跃工作者数
	activeWorkerMutex sync.RWMutex             // 保护activeWorkers的读写锁
	ctx               context.Context          // 服务上下文
	cancel            context.CancelFunc       // 取消函数
	taskQueue         chan *models.DownloadTask // 任务队列
	logger            *logrus.Logger           // 日志记录器
	
	// 优雅关闭相关
	wg              sync.WaitGroup    // 等待所有goroutine完成
	shutdownOnce    sync.Once         // 确保只关闭一次
	isShuttingDown  bool              // 关闭状态标记
	shutdownMutex   sync.RWMutex      // 保护关闭状态的锁
}

// DownloadWorker 下载工作者
type DownloadWorker struct {
	ID           uint
	Task         *models.DownloadTask
	Client       *http.Client
	Context      context.Context
	Cancel       context.CancelFunc
	Progress     chan ProgressUpdate
	progressOnce sync.Once  // 确保progress channel只关闭一次
}

// ProgressUpdate 进度更新
type ProgressUpdate struct {
	TaskID           uint
	DownloadedSize   int64
	Progress         float64
	Speed            string
	Status           models.DownloadStatus
	Error            string
}

// PDFPartInfo PDF部分信息
type PDFPartInfo struct {
	Section  string // IMAP部分标识符，如 "2" 或 "2.1"
	FileName string
	Encoding string
	Size     uint32
}

// NewDownloadService 创建下载服务
func NewDownloadService(db *database.Database) *DownloadService {
	ctx, cancel := context.WithCancel(context.Background())
	
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel) // 修复：使用配置化的日志级别
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	
	service := &DownloadService{
		db:              db,
		workers:         make(map[uint]*DownloadWorker),
		maxConcurrent:   3, // 默认最大并发数，后续可配置
		ctx:             ctx,
		cancel:          cancel,
		taskQueue:       make(chan *models.DownloadTask, 100), // 缓冲队列
		logger:          logger,
		isShuttingDown:  false,
	}
	
	// 启动服务组件
	service.startServiceComponents()
	
	return service
}

// startServiceComponents 启动服务组件
func (ds *DownloadService) startServiceComponents() {
	// 恢复未完成的任务
	ds.wg.Add(1)
	go ds.recoverUnfinishedTasks()
	
	// 启动任务调度器
	ds.wg.Add(1)
	go ds.taskScheduler()
}

// recoverUnfinishedTasks 恢复未完成的任务
func (ds *DownloadService) recoverUnfinishedTasks() {
	defer ds.wg.Done()
	
	// 等待服务完全初始化
	select {
	case <-time.After(2 * time.Second):
	case <-ds.ctx.Done():
		return
	}
	
	// 查找所有未完成的任务
	query := `
		SELECT 
			dt.id, dt.email_id, dt.subject, dt.sender, dt.file_name, 
			dt.file_size, dt.downloaded_size, dt.status, dt.type, 
			dt.source, dt.local_path, dt.error, dt.progress, dt.speed,
			dt.created_at, dt.updated_at,
			ea.id, ea.name, ea.email, ea.password, ea.imap_server, 
			ea.imap_port, ea.use_ssl, ea.is_active, ea.created_at, ea.updated_at
		FROM download_tasks dt
		LEFT JOIN email_accounts ea ON dt.email_id = ea.id
		WHERE dt.status IN ('downloading', 'pending')
		ORDER BY dt.created_at ASC
	`
	
	rows, err := ds.db.DB.Query(query)
	if err != nil {
		ds.logger.Errorf("查询未完成任务失败: %v", err)
		return
	}
	defer rows.Close()
	
	var recoveredTasks []*models.DownloadTask
	
	for rows.Next() {
		task := &models.DownloadTask{}
		account := &models.EmailAccount{}
		
		err := rows.Scan(
			&task.ID, &task.EmailID, &task.Subject, &task.Sender, &task.FileName,
			&task.FileSize, &task.DownloadedSize, &task.Status, &task.Type,
			&task.Source, &task.LocalPath, &task.Error, &task.Progress, &task.Speed,
			&task.CreatedAt, &task.UpdatedAt,
			&account.ID, &account.Name, &account.Email, &account.Password, &account.IMAPServer,
			&account.IMAPPort, &account.UseSSL, &account.IsActive, &account.CreatedAt, &account.UpdatedAt,
		)
		
		if err != nil {
			ds.logger.Errorf("扫描任务数据失败: %v", err)
			continue
		}
		
		task.EmailAccount = *account
		
		// 检查任务是否应该恢复
		if ds.shouldRecoverTask(task) {
			recoveredTasks = append(recoveredTasks, task)
		} else {
			// 任务过期或有问题，标记为失败
			ds.updateTaskStatus(task.ID, models.StatusFailed, "任务恢复时发现异常", 0, 0, "")
		}
	}
	
	// 重新将恢复的任务放入队列
	for _, task := range recoveredTasks {
		// 重置任务状态为pending
		ds.updateTaskStatus(task.ID, models.StatusPending, "", task.DownloadedSize, 0, "")
		
		// 放入任务队列（带超时保护）
		select {
		case ds.taskQueue <- task:
			ds.logger.Infof("任务 %d 已恢复到队列", task.ID)
		case <-time.After(5 * time.Second):
			ds.logger.Errorf("任务 %d 恢复超时", task.ID)
			ds.updateTaskStatus(task.ID, models.StatusFailed, "恢复任务时队列超时", 0, 0, "")
		case <-ds.ctx.Done():
			return
		}
	}
	
	ds.logger.Infof("成功恢复 %d 个未完成任务", len(recoveredTasks))
}

// shouldRecoverTask 判断是否应当恢复任务
func (ds *DownloadService) shouldRecoverTask(task *models.DownloadTask) bool {
	// 检查任务创建时间（超过24小时的任务不恢复）
	if createdAt, err := time.Parse("2006-01-02 15:04:05", task.CreatedAt); err == nil {
		if time.Since(createdAt) > 24*time.Hour {
			ds.logger.Infof("任务 %d 创建时间过久，不恢复", task.ID)
			return false
		}
	}
	
	// 检查账户是否仍然有效
	if !task.EmailAccount.IsActive {
		ds.logger.Infof("任务 %d 对应的邮箱账户已禁用，不恢复", task.ID)
		return false
	}
	
	// 检查本地路径是否已经存在完整文件
	if task.LocalPath != "" {
		if info, err := os.Stat(task.LocalPath); err == nil {
			// 文件已存在，检查大小是否匹配
			if task.FileSize > 0 && info.Size() == task.FileSize {
				ds.updateTaskStatus(task.ID, models.StatusCompleted, "", task.FileSize, 100, "")
				ds.logger.Infof("任务 %d 文件已存在且完整，标记为完成", task.ID)
				return false
			}
		}
	}
	
	return true
}

// taskScheduler 任务调度器
func (ds *DownloadService) taskScheduler() {
	defer ds.wg.Done()
	
	retryTicker := time.NewTicker(5 * time.Second) // 每5秒检查一次待处理任务
	defer retryTicker.Stop()
	
	var pendingTasks []*models.DownloadTask // 待处理任务队列
	
	for {
		select {
		case <-ds.ctx.Done():
			ds.logger.Info("任务调度器收到关闭信号")
			return
			
		case task := <-ds.taskQueue:
			// 检查是否正在关闭
			ds.shutdownMutex.RLock()
			if ds.isShuttingDown {
				ds.shutdownMutex.RUnlock()
				ds.logger.Info("服务正在关闭，不接受新任务")
				return
			}
			ds.shutdownMutex.RUnlock()
			
			// 检查是否可以启动新任务
			ds.activeWorkerMutex.RLock()
			canStart := ds.activeWorkers < ds.maxConcurrent
			ds.activeWorkerMutex.RUnlock()
			
			if canStart {
				ds.wg.Add(1)
				go ds.startDownload(task)
			} else {
				// 加入待处理队列
				pendingTasks = append(pendingTasks, task)
				ds.logger.Debugf("任务 %d 加入待处理队列，当前队列长度: %d", task.ID, len(pendingTasks))
			}
			
		case <-retryTicker.C:
			// 定期检查待处理任务
			if len(pendingTasks) == 0 {
				continue
			}
			
			ds.activeWorkerMutex.RLock()
			availableSlots := ds.maxConcurrent - ds.activeWorkers
			ds.activeWorkerMutex.RUnlock()
			
			if availableSlots > 0 {
				// 启动尽可能多的任务
				toStart := availableSlots
				if len(pendingTasks) < toStart {
					toStart = len(pendingTasks)
				}
				
				for i := 0; i < toStart; i++ {
					ds.wg.Add(1)
					go ds.startDownload(pendingTasks[i])
				}
				
				// 移除已启动的任务
				pendingTasks = pendingTasks[toStart:]
				ds.logger.Debugf("启动了 %d 个待处理任务，剩余队列长度: %d", toStart, len(pendingTasks))
			}
			
			// 清理过期的待处理任务（超过10分钟）
			now := time.Now()
			var validTasks []*models.DownloadTask
			for _, task := range pendingTasks {
				if createdAt, err := time.Parse("2006-01-02 15:04:05", task.CreatedAt); err == nil {
					if now.Sub(createdAt) < 10*time.Minute {
						validTasks = append(validTasks, task)
					} else {
						// 任务过期，标记为失败
						ds.updateTaskStatus(task.ID, models.StatusFailed, "任务排队超时", 0, 0, "")
						ds.logger.Warnf("任务 %d 排队超时，已标记为失败", task.ID)
					}
				} else {
					validTasks = append(validTasks, task) // 保留无法解析时间的任务
				}
			}
			pendingTasks = validTasks
		}
	}
}

// StartDownload 开始下载任务
func (ds *DownloadService) StartDownload(taskID uint) error {
	// 检查服务是否正在关闭
	ds.shutdownMutex.RLock()
	if ds.isShuttingDown {
		ds.shutdownMutex.RUnlock()
		return fmt.Errorf("服务正在关闭，无法启动新任务")
	}
	ds.shutdownMutex.RUnlock()
	
	// 使用索引优化的查询
	task, err := ds.getTaskByIDOptimized(taskID)
	if err != nil {
		return fmt.Errorf("获取任务失败: %v", err)
	}
	
	if task.Status != models.StatusPending {
		return fmt.Errorf("任务状态不正确: %s", task.Status)
	}
	
	// 将任务放入队列（带超时保护）
	select {
	case ds.taskQueue <- task:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("任务队列超时")
	case <-ds.ctx.Done():
		return fmt.Errorf("服务已关闭")
	}
}

// getTaskByIDOptimized 优化的任务查询
func (ds *DownloadService) getTaskByIDOptimized(taskID uint) (*models.DownloadTask, error) {
	query := `
		SELECT 
			dt.id, dt.email_id, dt.subject, dt.sender, dt.file_name, 
			dt.file_size, dt.downloaded_size, dt.status, dt.type, 
			dt.source, dt.local_path, dt.error, dt.progress, dt.speed,
			dt.created_at, dt.updated_at,
			ea.id, ea.name, ea.email, ea.password, ea.imap_server, 
			ea.imap_port, ea.use_ssl, ea.is_active, ea.created_at, ea.updated_at
		FROM download_tasks dt
		LEFT JOIN email_accounts ea ON dt.email_id = ea.id
		WHERE dt.id = ?
	`
	
	row := ds.db.DB.QueryRow(query, taskID)
	
	task := &models.DownloadTask{}
	account := &models.EmailAccount{}
	
	err := row.Scan(
		&task.ID, &task.EmailID, &task.Subject, &task.Sender, &task.FileName,
		&task.FileSize, &task.DownloadedSize, &task.Status, &task.Type,
		&task.Source, &task.LocalPath, &task.Error, &task.Progress, &task.Speed,
		&task.CreatedAt, &task.UpdatedAt,
		&account.ID, &account.Name, &account.Email, &account.Password, &account.IMAPServer,
		&account.IMAPPort, &account.UseSSL, &account.IsActive, &account.CreatedAt, &account.UpdatedAt,
	)
	
	if err != nil {
		return nil, err
	}
	
	task.EmailAccount = *account
	return task, nil
}

// startDownload 启动下载
func (ds *DownloadService) startDownload(task *models.DownloadTask) {
	defer ds.wg.Done()
	
	// 增加活跃工作者计数
	ds.activeWorkerMutex.Lock()
	ds.activeWorkers++
	ds.activeWorkerMutex.Unlock()
	
	// 全面的清理和错误恢复机制
	defer func() {
		// panic恢复
		if r := recover(); r != nil {
			// 记录panic信息并更新任务状态
			errorMsg := fmt.Sprintf("下载过程中发生严重错误: %v", r)
			ds.logger.Errorf("任务 %d panic: %v", task.ID, r)
			ds.updateTaskStatus(task.ID, models.StatusFailed, errorMsg, 0, 0, "")
		}
		
		// 减少活跃工作者计数
		ds.activeWorkerMutex.Lock()
		ds.activeWorkers--
		ds.activeWorkerMutex.Unlock()
	}()
	
	// 创建工作者上下文
	workerCtx, workerCancel := context.WithCancel(ds.ctx)
	defer workerCancel()
	
	// 创建工作者
	worker := &DownloadWorker{
		ID:       task.ID,
		Task:     task,
		Client:   &http.Client{Timeout: 30 * time.Second},
		Context:  workerCtx,
		Cancel:   workerCancel,
		Progress: make(chan ProgressUpdate, 10),
	}
	
	// 注册工作者
	ds.workerMutex.Lock()
	ds.workers[task.ID] = worker
	ds.workerMutex.Unlock()
	
	// 确保完成时清理工作者
	defer func() {
		ds.workerMutex.Lock()
		delete(ds.workers, task.ID)
		ds.workerMutex.Unlock()
		
		// 安全关闭progress channel
		worker.progressOnce.Do(func() {
			close(worker.Progress)
		})
	}()
	
	// 启动进度监控（带恢复机制）
	monitorWg := sync.WaitGroup{}
	monitorWg.Add(1)
	go func() {
		defer func() {
			monitorWg.Done()
			if r := recover(); r != nil {
				// 进度监控goroutine panic恢复
				ds.logger.Errorf("任务 %d 进度监控panic: %v", task.ID, r)
				ds.updateTaskStatus(task.ID, models.StatusFailed, 
					fmt.Sprintf("进度监控出错: %v", r), 0, 0, "")
			}
		}()
		ds.monitorProgress(worker)
	}()
	
	// 执行下载（带恢复机制）
	func() {
		defer func() {
			if r := recover(); r != nil {
				// 下载执行panic恢复
				ds.logger.Errorf("任务 %d 下载执行panic: %v", task.ID, r)
				select {
				case worker.Progress <- ProgressUpdate{
					TaskID: task.ID,
					Status: models.StatusFailed,
					Error:  fmt.Sprintf("下载执行出错: %v", r),
				}:
				default:
					// 如果progress channel已满或已关闭，直接更新数据库
					ds.updateTaskStatus(task.ID, models.StatusFailed, 
						fmt.Sprintf("下载执行出错: %v", r), 0, 0, "")
				}
			}
		}()
		ds.performDownload(worker)
	}()
	
	// 等待进度监控完成
	monitorWg.Wait()
}

// performDownload 执行下载
func (ds *DownloadService) performDownload(worker *DownloadWorker) {
	task := worker.Task
	
	ds.logger.Infof("开始下载任务 %d: %s", task.ID, task.FileName)
	
	// 更新状态为下载中
	ds.updateTaskStatus(task.ID, models.StatusDownloading, "", 0, 0, "")
	
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(task.LocalPath), 0755); err != nil {
		worker.Progress <- ProgressUpdate{
			TaskID: task.ID,
			Status: models.StatusFailed,
			Error:  fmt.Sprintf("创建目录失败: %v", err),
		}
		return
	}
	
	// 根据类型执行不同的下载逻辑
	var err error
	switch task.Type {
	case models.TypeAttachment:
		err = ds.downloadAttachment(worker)
	case models.TypeLink:
		err = ds.downloadFromURL(worker)
	default:
		err = fmt.Errorf("不支持的下载类型: %s", task.Type)
	}
	
	if err != nil {
		ds.logger.Errorf("任务 %d 下载失败: %v", task.ID, err)
		worker.Progress <- ProgressUpdate{
			TaskID: task.ID,
			Status: models.StatusFailed,
			Error:  err.Error(),
		}
	} else {
		ds.logger.Infof("任务 %d 下载成功: %s", task.ID, task.FileName)
	}
}

// downloadFromURL 从URL下载文件（增强版，支持各种邮件服务商）
func (ds *DownloadService) downloadFromURL(worker *DownloadWorker) error {
	task := worker.Task
	
	// 创建请求
	req, err := http.NewRequestWithContext(worker.Context, "GET", task.Source, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 设置通用的请求头，模拟浏览器行为
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "application/pdf,application/octet-stream,*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	
	// 特殊处理不同邮件服务商的请求头
	ds.setServiceSpecificHeaders(req, task.Source)
	
	ds.logger.Infof("开始下载URL: %s", task.Source)
	
	// 发送请求
	resp, err := worker.Client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	
	ds.logger.Infof("服务器响应状态: %d, Content-Type: %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	
	// 处理重定向和特殊状态码
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		location := resp.Header.Get("Location")
		if location != "" {
			ds.logger.Infof("处理重定向到: %s", location)
			// 递归处理重定向（最多3次）
			return ds.handleRedirect(worker, location, 0)
		}
	}
	
	if resp.StatusCode != http.StatusOK {
		// 读取错误响应内容
		body, _ := io.ReadAll(resp.Body)
		ds.logger.Errorf("服务器响应错误: %d, 内容: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
		return fmt.Errorf("服务器响应错误: %d", resp.StatusCode)
	}
	
	// 验证内容类型
	contentType := resp.Header.Get("Content-Type")
	if !ds.isValidPDFContentType(contentType) {
		ds.logger.Warnf("可疑的内容类型: %s，继续尝试下载", contentType)
	}
	
	// 获取文件大小
	contentLength := resp.ContentLength
	if contentLength > 0 {
		task.FileSize = contentLength
		ds.logger.Infof("文件大小: %s", utils.FormatBytes(contentLength))
	}
	
	// 创建目录
	if err := os.MkdirAll(filepath.Dir(task.LocalPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}
	
	// 创建临时文件
	tempPath := task.LocalPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer file.Close()
	
	// 下载文件并监控进度
	err = ds.downloadWithProgress(worker, resp.Body, file)
	if err != nil {
		os.Remove(tempPath) // 清理临时文件
		return err
	}
	
	// 验证下载的文件是否为有效PDF
	if err := utils.ValidatePDFFile(tempPath); err != nil {
		os.Remove(tempPath) // 删除无效文件
		return fmt.Errorf("下载的文件不是有效的PDF: %v", err)
	}
	
	// 原子性重命名文件
	if err := os.Rename(tempPath, task.LocalPath); err != nil {
		os.Remove(tempPath) // 清理临时文件
		return fmt.Errorf("完成文件写入失败: %v", err)
	}
	
	ds.logger.Infof("成功下载文件: %s", task.LocalPath)
	return nil
}

// setServiceSpecificHeaders 为不同邮件服务商设置特定的请求头
func (ds *DownloadService) setServiceSpecificHeaders(req *http.Request, url string) {
	urlLower := strings.ToLower(url)
	
	if strings.Contains(urlLower, "qq.com") {
		// QQ邮箱特殊请求头
		req.Header.Set("Referer", "https://mail.qq.com/")
		req.Header.Set("Origin", "https://mail.qq.com")
	} else if strings.Contains(urlLower, "163.com") || strings.Contains(urlLower, "126.com") {
		// 网易邮箱特殊请求头
		req.Header.Set("Referer", "https://mail.163.com/")
		req.Header.Set("Origin", "https://mail.163.com")
	} else if strings.Contains(urlLower, "gmail.com") || strings.Contains(urlLower, "google.com") {
		// Gmail特殊请求头
		req.Header.Set("Referer", "https://mail.google.com/")
		req.Header.Set("Origin", "https://mail.google.com")
	} else if strings.Contains(urlLower, "outlook.com") || strings.Contains(urlLower, "hotmail.com") {
		// Outlook特殊请求头
		req.Header.Set("Referer", "https://outlook.live.com/")
		req.Header.Set("Origin", "https://outlook.live.com")
	}
}

// handleRedirect 处理重定向
func (ds *DownloadService) handleRedirect(worker *DownloadWorker, location string, depth int) error {
	if depth >= 3 {
		return fmt.Errorf("重定向次数过多")
	}
	
	// 更新任务源地址
	originalSource := worker.Task.Source
	worker.Task.Source = location
	
	// 递归下载
	err := ds.downloadFromURL(worker)
	
	// 恢复原始源地址
	worker.Task.Source = originalSource
	
	return err
}

// isValidPDFContentType 检查内容类型是否可能是PDF
func (ds *DownloadService) isValidPDFContentType(contentType string) bool {
	if contentType == "" {
		return true // 允许空的内容类型
	}
	
	contentTypeLower := strings.ToLower(contentType)
	validTypes := []string{
		"application/pdf",
		"application/octet-stream",
		"application/binary",
		"application/force-download",
		"application/download",
		"binary/octet-stream",
	}
	
	for _, validType := range validTypes {
		if strings.Contains(contentTypeLower, validType) {
			return true
		}
	}
	
	return false
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// downloadAttachment 下载邮件附件
func (ds *DownloadService) downloadAttachment(worker *DownloadWorker) error {
	task := worker.Task
	
	// 获取邮箱账户信息
	account := &task.EmailAccount
	if account.ID == 0 {
		return fmt.Errorf("无效的邮箱账户信息")
	}
	
	// 创建安全的邮件服务来获取附件
	emailService := ds.createEmailServiceForDownload(worker.Context)
	
	// 连接到邮箱
	conn, err := emailService.createConnectionWithTimeout(worker.Context, account)
	if err != nil {
		return fmt.Errorf("连接邮箱失败: %v", err)
	}
	defer func() {
		// 安全关闭连接
		defer func() {
			if r := recover(); r != nil {
				// 忽略关闭连接时的panic
			}
		}()
		conn.close()
	}()
	
	// 选择收件箱
	if err := conn.selectInbox(); err != nil {
		return fmt.Errorf("选择收件箱失败: %v", err)
	}
	
	// 搜索包含指定附件的邮件
	attachmentData, err := ds.findAndDownloadAttachment(conn, task)
	if err != nil {
		return fmt.Errorf("下载附件失败: %v", err)
	}
	
	if len(attachmentData) == 0 {
		return fmt.Errorf("未找到指定的附件")
	}
	
	// 验证是否为有效的PDF文件
	if !utils.IsPDFContent(attachmentData) {
		return fmt.Errorf("附件不是有效的PDF文件")
	}
	
	// 创建目录
	if err := os.MkdirAll(filepath.Dir(task.LocalPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}
	
	// 原子性写入文件
	tempPath := task.LocalPath + ".tmp"
	if err := os.WriteFile(tempPath, attachmentData, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %v", err)
	}
	
	// 验证写入的文件
	if err := utils.ValidatePDFFile(tempPath); err != nil {
		os.Remove(tempPath) // 删除无效文件
		return fmt.Errorf("PDF文件验证失败: %v", err)
	}
	
	// 原子性重命名文件
	if err := os.Rename(tempPath, task.LocalPath); err != nil {
		os.Remove(tempPath) // 清理临时文件
		return fmt.Errorf("完成文件写入失败: %v", err)
	}
	
	// 发送完成进度
	worker.Progress <- ProgressUpdate{
		TaskID:         task.ID,
		DownloadedSize: int64(len(attachmentData)),
		Progress:       100,
		Status:         models.StatusCompleted,
	}
	
	return nil
}

// createEmailServiceForDownload 创建用于下载的安全EmailService实例
func (ds *DownloadService) createEmailServiceForDownload(ctx context.Context) *EmailService {
	// 创建专用的logger
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // 下载时使用较低的日志级别
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	
	// 创建带超时的上下文
	downloadCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	
	return &EmailService{
		db:               ds.db,
		connections:      make(map[uint]*IMAPConnection),
		connectionsMutex: sync.RWMutex{},
		downloadService:  nil, // 避免循环引用
		ctx:              downloadCtx,
		cancel:           cancel,
		checkInterval:    time.Hour, // 不需要定期检查
		isRunning:        false,
		runningMutex:     sync.RWMutex{},
		logger:           logger,
	}
}

// validateUID 验证并记录UID信息，用于调试UID问题（改进版）
func (ds *DownloadService) validateUID(expectedUID, actualUID uint32, operation string) {
	if actualUID == 0 {
		ds.logger.Errorf("UID验证失败 - %s: UID为0，可能是Fetch操作缺少imap.FetchUid", operation)
	} else if expectedUID != actualUID {
		ds.logger.Warnf("UID不匹配 - %s: 期望=%d, 实际=%d", operation, expectedUID, actualUID)
		// 注意：UID不匹配在某些IMAP服务器中是正常的，特别是在搜索和获取操作之间
		// 这可能是由于：
		// 1. 邮箱状态在搜索和获取之间发生了变化
		// 2. IMAP服务器实现差异
		// 3. 搜索使用的是序列号而不是UID
		// 我们记录警告但允许下载继续进行，使用实际获取到的UID
		ds.logger.Infof("UID不匹配被容忍，继续使用实际UID: %d", actualUID)
	} else {
		ds.logger.Debugf("UID验证成功 - %s: UID=%d", operation, actualUID)
	}
}

// findAndDownloadAttachment 查找并下载指定的附件（重构版，支持PDF链接和传统附件）
func (ds *DownloadService) findAndDownloadAttachment(conn *IMAPConnection, task *models.DownloadTask) ([]byte, error) {
	ds.logger.Infof("开始查找附件 - 主题: '%s', 发件人: '%s', 文件名: '%s'", task.Subject, task.Sender, task.FileName)
	
	// 搜索匹配的邮件
	uids, err := ds.searchEmailsSafely(conn, task.Subject, task.Sender)
	if err != nil {
		return nil, fmt.Errorf("搜索邮件失败: %v", err)
	}
	
	ds.logger.Infof("找到 %d 封匹配的邮件", len(uids))
	
	if len(uids) == 0 {
		return nil, fmt.Errorf("未找到匹配的邮件")
	}

	// 遍历找到的邮件，提取PDF
	for i, uid := range uids {
		ds.logger.Infof("处理邮件 %d/%d (搜索UID: %d)", i+1, len(uids), uid)
		
		// 首先尝试从邮件内容中提取PDF链接
		pdfData, err := ds.extractPDFFromEmail(conn, uid, task.FileName)
		if err == nil && len(pdfData) > 0 {
			ds.logger.Infof("成功从邮件 UID %d 提取PDF (大小: %d bytes)", uid, len(pdfData))
			return pdfData, nil
		}
		ds.logger.Debugf("邮件UID %d 未找到匹配的PDF: %v", uid, err)
	}
	
	return nil, fmt.Errorf("在匹配的邮件中未找到指定的附件: %s", task.FileName)
}

// extractPDFFromEmail 从邮件中提取PDF（支持附件和链接）
func (ds *DownloadService) extractPDFFromEmail(conn *IMAPConnection, uid uint32, targetFileName string) ([]byte, error) {
	// 获取完整的邮件内容
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	
	messages := make(chan *imap.Message, 1)
	
	conn.Mutex.Lock()
	// 关键修复：使用UidFetch而不是Fetch，确保UID一致性
	err := conn.Client.UidFetch(seqset, []imap.FetchItem{
		imap.FetchUid,          
		imap.FetchBodyStructure,
		imap.FetchEnvelope,
		"BODY[TEXT]",  // 获取邮件正文
		"BODY[1]",     // 获取第一个body部分
		"BODY[]",      // 获取完整邮件内容
	}, messages)
	conn.Mutex.Unlock()
	
	if err != nil {
		return nil, fmt.Errorf("获取邮件内容失败: %v", err)
	}
	
	var msg *imap.Message
	select {
	case msg = <-messages:
		if msg == nil {
			return nil, fmt.Errorf("邮件为空")
		}
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("获取邮件内容超时")
	}
	
	// 验证UID是否正确获取
	ds.validateUID(uid, msg.Uid, "邮件内容获取")
	
	ds.logger.Infof("成功获取邮件内容 (UID: %d)", msg.Uid)
	
	// 方法1: 尝试从邮件内容中提取PDF链接
	if pdfData, err := ds.extractPDFFromEmailContent(msg, targetFileName); err == nil && len(pdfData) > 0 {
		return pdfData, nil
	}
	
	// 方法2: 尝试从传统附件中提取PDF
	if msg.BodyStructure != nil {
		if pdfData, err := ds.extractPDFFromAttachment(conn, msg.Uid, msg.BodyStructure, targetFileName); err == nil && len(pdfData) > 0 {
			return pdfData, nil
		}
	}
	
	return nil, fmt.Errorf("未找到PDF内容")
}

// extractPDFFromEmailContent 从邮件内容中提取PDF（支持PDF链接）
func (ds *DownloadService) extractPDFFromEmailContent(msg *imap.Message, targetFileName string) ([]byte, error) {
	// 获取邮件正文内容
	var bodyContent string
	
	// 尝试从不同的body部分获取内容
	for section, body := range msg.Body {
		ds.logger.Debugf("处理邮件部分: %s", section)
		
		if body != nil {
			content, err := ioutil.ReadAll(body)
			if err == nil {
				bodyContent += string(content) + "\n"
			}
		}
	}
	
	if bodyContent == "" {
		return nil, fmt.Errorf("邮件内容为空")
	}
	
	ds.logger.Debugf("邮件内容长度: %d", len(bodyContent))
	
	// 从邮件内容中提取PDF链接
	pdfLinks := ds.extractPDFLinksFromContent(bodyContent)
	ds.logger.Infof("从邮件内容中提取到 %d 个PDF链接", len(pdfLinks))
	
	// 尝试下载每个PDF链接
	for i, link := range pdfLinks {
		ds.logger.Infof("尝试下载PDF链接 %d/%d: %s", i+1, len(pdfLinks), link)
		
		pdfData, err := ds.downloadPDFFromURL(link, targetFileName)
		if err == nil && len(pdfData) > 0 {
			ds.logger.Infof("成功从链接下载PDF (大小: %d bytes)", len(pdfData))
			return pdfData, nil
		}
		ds.logger.Debugf("链接下载失败: %v", err)
	}
	
	// 尝试直接从邮件内容中提取PDF数据
	if pdfData := ds.extractDirectPDFContent(bodyContent, targetFileName); len(pdfData) > 0 {
		ds.logger.Infof("成功从邮件内容直接提取PDF (大小: %d bytes)", len(pdfData))
		return pdfData, nil
	}
	
	return nil, fmt.Errorf("未找到PDF内容")
}

// extractPDFLinksFromContent 从邮件内容中提取PDF链接
func (ds *DownloadService) extractPDFLinksFromContent(content string) []string {
	var pdfLinks []string
	
	// 多种PDF链接模式
	patterns := []string{
		// QQ邮箱下载链接
		`https://[^/]*\.mail\.qq\.com/[^\s"'>]+`,
		`https://[^/]*\.mail\.ftn\.qq\.com/[^\s"'>]+`,
		// 网易邮箱链接
		`https://[^/]*\.mail\.163\.com/[^\s"'>]+`,
		`https://[^/]*\.mail\.126\.com/[^\s"'>]+`,
		// Gmail链接
		`https://[^/]*\.googleusercontent\.com/[^\s"'>]+`,
		// 通用PDF链接
		`https?://[^\s"'>]*\.pdf[^\s"'>]*`,
		`https?://[^\s"'>]*[?&].*\.pdf[^\s"'>]*`,
		// 通用下载链接（可能是PDF）
		`https?://[^\s"'>]*download[^\s"'>]*`,
		`https?://[^\s"'>]*attachment[^\s"'>]*`,
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(content, -1)
		for _, match := range matches {
			// 清理链接
			link := strings.TrimSpace(match)
			link = strings.Trim(link, `"'>`)
			if link != "" && !contains(pdfLinks, link) {
				pdfLinks = append(pdfLinks, link)
			}
		}
	}
	
	return pdfLinks
}

// downloadPDFFromURL 从URL下载PDF
func (ds *DownloadService) downloadPDFFromURL(url, targetFileName string) ([]byte, error) {
	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	
	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 设置请求头（模拟浏览器）
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "application/pdf,*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	
	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP状态错误: %d", resp.StatusCode)
	}
	
	// 检查Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "application/pdf") && 
	   !strings.Contains(contentType, "application/octet-stream") {
		ds.logger.Debugf("内容类型可能不是PDF: %s", contentType)
	}
	
	// 检查文件大小，避免内存溢出
	contentLength := resp.ContentLength
	const maxFileSize = 100 * 1024 * 1024 // 100MB限制
	if contentLength > maxFileSize {
		return nil, fmt.Errorf("文件过大: %d bytes，超过限制 %d bytes", contentLength, maxFileSize)
	}
	
	// 使用缓冲读取，避免一次性加载大文件到内存
	var buf bytes.Buffer
	bufSize := 32 * 1024 // 32KB缓冲区
	buffer := make([]byte, bufSize)
	totalRead := int64(0)
	
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			totalRead += int64(n)
			// 检查总大小限制
			if totalRead > maxFileSize {
				return nil, fmt.Errorf("文件读取超过大小限制: %d bytes", maxFileSize)
			}
			buf.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %v", err)
		}
	}
	
	data := buf.Bytes()
	
	// 验证PDF文件
	if !ds.isPDFData(data) {
		return nil, fmt.Errorf("下载的文件不是有效的PDF")
	}
	
	return data, nil
}

// extractDirectPDFContent 直接从邮件内容中提取PDF数据
func (ds *DownloadService) extractDirectPDFContent(content, targetFileName string) []byte {
	// 查找PDF文件的开始和结束标记
	pdfStart := "%PDF-"
	pdfEnd := "%%EOF"
	
	startIndex := strings.Index(content, pdfStart)
	if startIndex == -1 {
		return nil
	}
	
	endIndex := strings.LastIndex(content, pdfEnd)
	if endIndex == -1 || endIndex <= startIndex {
		return nil
	}
	
	// 提取PDF内容
	pdfContent := content[startIndex:endIndex+len(pdfEnd)]
	
	// 如果内容看起来是Base64编码的，尝试解码
	if ds.isBase64Content(pdfContent) {
		if decoded, err := base64.StdEncoding.DecodeString(pdfContent); err == nil {
			if ds.isPDFData(decoded) {
				return decoded
			}
		}
	}
	
	// 直接返回原始内容
	pdfData := []byte(pdfContent)
	if ds.isPDFData(pdfData) {
		return pdfData
	}
	
	return nil
}

// extractPDFFromAttachment 从传统附件中提取PDF
func (ds *DownloadService) extractPDFFromAttachment(conn *IMAPConnection, uid uint32, bs *imap.BodyStructure, targetFileName string) ([]byte, error) {
	// 查找PDF附件
	pdfPart := ds.findPDFPartInStructure(bs, targetFileName)
	if pdfPart == nil {
		return nil, fmt.Errorf("未找到PDF附件")
	}
	
	// 获取附件内容
	return ds.fetchPDFPartContent(conn, uid, pdfPart)
}

// 辅助函数
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (ds *DownloadService) isBase64Content(content string) bool {
	// 简单检查是否可能是Base64编码
	if len(content) < 100 {
		return false
	}
	
	// Base64字符集检查
	base64Chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	validChars := 0
	for _, char := range content[:100] { // 检查前100个字符
		if strings.ContainsRune(base64Chars, char) || char == '\n' || char == '\r' {
			validChars++
		}
	}
	
	return float64(validChars)/100.0 > 0.8 // 80%以上是有效字符
}

func (ds *DownloadService) isPDFData(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	
	// 检查PDF文件头
	return bytes.HasPrefix(data, []byte("%PDF-"))
}

// monitorProgress 监控下载进度
func (ds *DownloadService) monitorProgress(worker *DownloadWorker) {
	for update := range worker.Progress {
		ds.updateTaskStatus(
			update.TaskID,
			update.Status,
			update.Error,
			update.DownloadedSize,
			update.Progress,
			update.Speed,
		)
	}
}

// updateTaskStatus 更新任务状态（使用统一事务处理）
func (ds *DownloadService) updateTaskStatus(taskID uint, status models.DownloadStatus, errorMsg string, downloadedSize int64, progress float64, speed string) error {
	return ds.db.WithRetry(func() error {
		return ds.db.WithTransaction(func(tx *sql.Tx) error {
			query := `
				UPDATE download_tasks 
				SET status = ?, error = ?, downloaded_size = ?, progress = ?, speed = ?, updated_at = ?
				WHERE id = ?
			`
			
			_, err := tx.Exec(query, status, errorMsg, downloadedSize, progress, speed, time.Now(), taskID)
			if err != nil {
				return fmt.Errorf("更新任务状态失败: %v", err)
			}
			
			return nil
		})
	}, 3) // 最多重试3次
}

// PauseDownload 暂停下载
func (ds *DownloadService) PauseDownload(taskID uint) error {
	ds.workerMutex.RLock()
	worker, exists := ds.workers[taskID]
	ds.workerMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("任务不存在或未在下载中")
	}
	
	worker.Cancel()
	return ds.updateTaskStatus(taskID, models.StatusPaused, "", 0, 0, "")
}

// CancelDownload 取消下载
func (ds *DownloadService) CancelDownload(taskID uint) error {
	ds.workerMutex.RLock()
	worker, exists := ds.workers[taskID]
	ds.workerMutex.RUnlock()
	
	if exists {
		worker.Cancel()
	}
	
	// 删除未完成的文件
	task, err := ds.getTaskByIDOptimized(taskID)
	if err == nil && task.LocalPath != "" {
		if _, err := os.Stat(task.LocalPath); err == nil {
			os.Remove(task.LocalPath)
		}
	}
	
	return ds.updateTaskStatus(taskID, models.StatusCancelled, "", 0, 0, "")
}

// GetDownloadStatus 获取下载状态
func (ds *DownloadService) GetDownloadStatus(taskID uint) (*models.DownloadTask, error) {
	return ds.getTaskByIDOptimized(taskID)
}

// GetAllTasks 获取所有任务
func (ds *DownloadService) GetAllTasks() ([]models.DownloadTask, error) {
	query := `
		SELECT 
			dt.id, dt.email_id, dt.subject, dt.sender, dt.file_name, 
			dt.file_size, dt.downloaded_size, dt.status, dt.type, 
			dt.source, dt.local_path, dt.error, dt.progress, dt.speed,
			dt.created_at, dt.updated_at,
			ea.id, ea.name, ea.email, ea.password, ea.imap_server, 
			ea.imap_port, ea.use_ssl, ea.is_active, ea.created_at, ea.updated_at
		FROM download_tasks dt
		LEFT JOIN email_accounts ea ON dt.email_id = ea.id
		ORDER BY dt.created_at DESC
	`
	
	rows, err := ds.db.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var tasks []models.DownloadTask
	for rows.Next() {
		task := models.DownloadTask{}
		account := models.EmailAccount{}
		
		err := rows.Scan(
			&task.ID, &task.EmailID, &task.Subject, &task.Sender, &task.FileName,
			&task.FileSize, &task.DownloadedSize, &task.Status, &task.Type,
			&task.Source, &task.LocalPath, &task.Error, &task.Progress, &task.Speed,
			&task.CreatedAt, &task.UpdatedAt,
			&account.ID, &account.Name, &account.Email, &account.Password, &account.IMAPServer,
			&account.IMAPPort, &account.UseSSL, &account.IsActive, &account.CreatedAt, &account.UpdatedAt,
		)
		
		if err != nil {
			return nil, err
		}
		
		task.EmailAccount = account
		tasks = append(tasks, task)
	}
	
	return tasks, nil
}

// SetMaxConcurrent 设置最大并发数
func (ds *DownloadService) SetMaxConcurrent(max int) {
	ds.activeWorkerMutex.Lock()
	defer ds.activeWorkerMutex.Unlock()
	ds.maxConcurrent = max
}

// GetActiveDownloads 获取活跃下载数
func (ds *DownloadService) GetActiveDownloads() int {
	ds.activeWorkerMutex.RLock()
	defer ds.activeWorkerMutex.RUnlock()
	return ds.activeWorkers
}

// Stop 停止下载服务
func (ds *DownloadService) Stop() {
	ds.shutdownOnce.Do(func() {
		ds.logger.Info("开始停止下载服务")
		
		// 设置关闭状态
		ds.shutdownMutex.Lock()
		ds.isShuttingDown = true
		ds.shutdownMutex.Unlock()
		
		// 取消所有任务
		ds.cancel()
		
		// 取消所有活跃的工作者
		ds.workerMutex.RLock()
		for _, worker := range ds.workers {
			worker.Cancel()
		}
		ds.workerMutex.RUnlock()
		
		// 等待所有goroutine完成（带超时）
		done := make(chan struct{})
		go func() {
			ds.wg.Wait()
			close(done)
		}()
		
		select {
		case <-done:
			ds.logger.Info("所有goroutine已正常退出")
		case <-time.After(30 * time.Second):
			ds.logger.Warn("等待goroutine退出超时，强制退出")
		}
		
		// 清理资源
		ds.workerMutex.Lock()
		for taskID, worker := range ds.workers {
			worker.progressOnce.Do(func() {
				close(worker.Progress)
			})
			delete(ds.workers, taskID)
		}
		ds.workerMutex.Unlock()
		
		ds.logger.Info("下载服务已停止")
	})
}

// findPDFPartInStructure 在邮件结构中查找PDF附件部分
func (ds *DownloadService) findPDFPartInStructure(bs *imap.BodyStructure, targetFileName string) *PDFPartInfo {
	// 首先尝试精确匹配
	if pdfPart := ds.findPDFPartRecursive(bs, targetFileName, ""); pdfPart != nil {
		return pdfPart
	}
	
	// 如果精确匹配失败，尝试找任何PDF附件
	ds.logger.Infof("精确匹配失败，尝试查找任何PDF附件")
	return ds.findPDFPartRecursive(bs, "", "")
}

// findPDFPartRecursive 递归查找PDF部分
func (ds *DownloadService) findPDFPartRecursive(bs *imap.BodyStructure, targetFileName, section string) *PDFPartInfo {
	if bs == nil {
		return nil
	}
	
	// 检查当前部分是否为PDF
	if ds.isPDFPart(bs) {
		fileName := ds.extractFileName(bs)
		ds.logger.Infof("找到PDF部分 - 节点: %s, 文件名: '%s', 目标: '%s', MIME: %s/%s", 
			section, fileName, targetFileName, bs.MIMEType, bs.MIMESubType)
		
		// 宽松匹配策略：如果目标文件名为空或者文件名匹配
		if targetFileName == "" || ds.isFileNameMatch(fileName, targetFileName) {
			encoding := "base64" // 默认编码
			if bs.Encoding != "" {
				encoding = strings.ToLower(bs.Encoding)
			}
			
			ds.logger.Infof("匹配成功 - 文件: '%s', 编码: %s, 大小: %d", fileName, encoding, bs.Size)
			return &PDFPartInfo{
				Section:  section,
				FileName: fileName,
				Encoding: encoding,
				Size:     bs.Size,
			}
		} else {
			ds.logger.Infof("文件名不匹配 - 实际: '%s', 目标: '%s'", fileName, targetFileName)
		}
	}
	
	// 递归搜索子部分
	for i, part := range bs.Parts {
		childSection := section
		if childSection == "" {
			childSection = fmt.Sprintf("%d", i+1)
		} else {
			childSection = fmt.Sprintf("%s.%d", childSection, i+1)
		}
		
		if pdfPart := ds.findPDFPartRecursive(part, targetFileName, childSection); pdfPart != nil {
			return pdfPart
		}
	}
	
	return nil
}

// isPDFPart 检查是否为PDF部分
func (ds *DownloadService) isPDFPart(bs *imap.BodyStructure) bool {
	if bs == nil {
		return false
	}
	
	// 检查MIME类型
	mimeType := strings.ToLower(bs.MIMEType)
	mimeSubType := strings.ToLower(bs.MIMESubType)
	
	// 更宽松的PDF检测
	isPDF := (mimeType == "application" && mimeSubType == "pdf") ||
			 (mimeType == "application" && mimeSubType == "octet-stream") ||
			 (mimeType == "application" && mimeSubType == "binary")
	
	// 如果MIME类型不明确，检查文件名
	if !isPDF {
		fileName := ds.extractFileName(bs)
		if fileName != "" && strings.HasSuffix(strings.ToLower(fileName), ".pdf") {
			isPDF = true
		}
	}
	
	return isPDF
}

// extractFileName 从BodyStructure提取文件名
func (ds *DownloadService) extractFileName(bs *imap.BodyStructure) string {
	if bs == nil {
		return ""
	}
	
	var fileName string
	
	// 优先从Content-Disposition参数获取
	if bs.DispositionParams != nil {
		if filename, exists := bs.DispositionParams["filename"]; exists {
			fileName = utils.DecodeMimeHeader(filename)
			if fileName != "" {
				return fileName
			}
		}
	}
	
	// 从Content-Type参数获取
	if bs.Params != nil {
		if name, exists := bs.Params["name"]; exists {
			fileName = utils.DecodeMimeHeader(name)
			if fileName != "" {
				return fileName
			}
		}
	}
	
	return ""
}

// fetchPDFPartContent 获取PDF部分的实际内容
func (ds *DownloadService) fetchPDFPartContent(conn *IMAPConnection, uid uint32, pdfPart *PDFPartInfo) ([]byte, error) {
	// 构建IMAP FETCH命令获取指定部分
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	
	// 构建部分标识符
	var fetchItem imap.FetchItem
	if pdfPart.Section == "" {
		fetchItem = "BODY[]"
	} else {
		fetchItem = imap.FetchItem(fmt.Sprintf("BODY[%s]", pdfPart.Section))
	}
	
	messages := make(chan *imap.Message, 1)
	
	conn.Mutex.Lock()
	// 关键修复：使用UidFetch确保UID一致性
	err := conn.Client.UidFetch(seqset, []imap.FetchItem{
		imap.FetchUid, 
		fetchItem,
	}, messages)
	conn.Mutex.Unlock()
	
	if err != nil {
		return nil, fmt.Errorf("获取PDF部分内容失败: %v", err)
	}
	
	var msg *imap.Message
	select {
	case msg = <-messages:
		if msg == nil {
			return nil, fmt.Errorf("获取的邮件为空")
		}
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("获取PDF内容超时")
	}
	
	// 验证UID匹配
	ds.validateUID(uid, msg.Uid, "PDF部分内容获取")
	
	// 从Body中提取内容
	var rawContent []byte
	for _, body := range msg.Body {
		if body == nil {
			continue
		}
		
		content, err := io.ReadAll(body)
		if err != nil {
			continue
		}
		
		rawContent = content
		break
	}
	
	if len(rawContent) == 0 {
		return nil, fmt.Errorf("PDF部分内容为空")
	}
	
	// 根据编码解码内容
	return ds.decodeContent(rawContent, pdfPart.Encoding)
}

// decodeContent 根据编码类型解码内容
func (ds *DownloadService) decodeContent(content []byte, encoding string) ([]byte, error) {
	encoding = strings.ToLower(strings.TrimSpace(encoding))
	
	switch encoding {
	case "base64":
		// 清理Base64内容（移除换行符和空格）
		cleanContent := regexp.MustCompile(`\s`).ReplaceAll(content, []byte(""))
		decoded, err := base64.StdEncoding.DecodeString(string(cleanContent))
		if err != nil {
			return nil, fmt.Errorf("Base64解码失败: %v", err)
		}
		return decoded, nil
		
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(content))
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("Quoted-Printable解码失败: %v", err)
		}
		return decoded, nil
		
	case "7bit", "8bit", "binary", "":
		// 无需解码
		return content, nil
		
	default:
		ds.logger.Warnf("未知的编码类型: %s，尝试直接使用", encoding)
		return content, nil
	}
}

// searchEmailsSafely 安全地搜索邮件（使用UID搜索修复版本）
func (ds *DownloadService) searchEmailsSafely(conn *IMAPConnection, subject, sender string) ([]uint32, error) {
	conn.Mutex.Lock()
	defer conn.Mutex.Unlock()
	
	if !conn.IsConnected {
		return nil, fmt.Errorf("连接已断开")
	}
	
	ds.logger.Infof("开始UID搜索邮件 - 主题: '%s', 发件人: '%s'", subject, sender)
	
	// 策略1: 如果没有搜索条件，搜索最近的邮件
	if subject == "" && sender == "" {
		criteria := imap.NewSearchCriteria()
		since := time.Now().AddDate(0, 0, -7) // 最近7天
		criteria.Since = since
		// 关键修复：使用UidSearch而不是Search
		uids, err := conn.Client.UidSearch(criteria)
		if err != nil {
			return nil, err
		}
		ds.logger.Infof("无条件UID搜索完成 - 找到 %d 封邮件", len(uids))
		return uids, nil
	}
	
	// 策略2: 只使用ASCII字符的搜索条件
	criteria := imap.NewSearchCriteria()
	hasValidCriteria := false
	
	// 检查发件人是否包含非ASCII字符
	if sender != "" && ds.isASCII(sender) {
		criteria.Header.Set("From", sender)
		hasValidCriteria = true
		ds.logger.Debugf("添加发件人搜索条件: %s", sender)
	}
	
	// 对于主题，如果包含非ASCII字符，则不使用Header搜索
	// 而是搜索最近的邮件，然后在客户端过滤
	if subject != "" && ds.isASCII(subject) {
		criteria.Header.Set("Subject", subject)
		hasValidCriteria = true
		ds.logger.Debugf("添加主题搜索条件: %s", subject)
	} else if subject != "" {
		// 包含非ASCII字符的主题，搜索最近的邮件
		since := time.Now().AddDate(0, 0, -7) // 最近7天
		criteria.Since = since
		hasValidCriteria = true
		ds.logger.Debugf("主题包含非ASCII字符，使用时间范围搜索")
	}
	
	// 如果没有任何有效的搜索条件，搜索最近的邮件
	if !hasValidCriteria {
		since := time.Now().AddDate(0, 0, -7)
		criteria.Since = since
		ds.logger.Debugf("使用默认时间范围搜索")
	}
	
	// 关键修复：使用UidSearch而不是Search
	uids, err := conn.Client.UidSearch(criteria)
	if err != nil {
		// 如果搜索失败，尝试最基本的搜索
		ds.logger.Warnf("UID搜索失败，尝试基本搜索: %v", err)
		criteria = imap.NewSearchCriteria()
		since := time.Now().AddDate(0, 0, -7)
		criteria.Since = since
		uids, err = conn.Client.UidSearch(criteria)
		if err != nil {
			return nil, fmt.Errorf("所有UID搜索策略均失败: %v", err)
		}
	}
	
	ds.logger.Infof("初始UID搜索完成 - 找到 %d 封邮件", len(uids))
	
	// 如果主题包含非ASCII字符，需要在客户端进行过滤
	if subject != "" && !ds.isASCII(subject) {
		ds.logger.Infof("开始客户端主题过滤 - 目标主题: '%s'", subject)
		filteredUIDs, err := ds.filterEmailsBySubjectUID(conn, uids, subject)
		if err != nil {
			return nil, err
		}
		ds.logger.Infof("主题过滤完成 - 过滤后: %d 封邮件", len(filteredUIDs))
		return filteredUIDs, nil
	}
	
	return uids, nil
}

// isASCII 检查字符串是否只包含ASCII字符
func (ds *DownloadService) isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// filterEmailsBySubjectUID 在客户端过滤邮件主题（使用UID版本）
func (ds *DownloadService) filterEmailsBySubjectUID(conn *IMAPConnection, uids []uint32, targetSubject string) ([]uint32, error) {
	if len(uids) == 0 {
		return uids, nil
	}
	
	// 限制检查的邮件数量
	maxCheck := 50
	if len(uids) > maxCheck {
		uids = uids[:maxCheck]
	}
	
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)
	
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	
	go func() {
		// 关键修复：使用UidFetch而不是Fetch
		done <- conn.Client.UidFetch(seqset, []imap.FetchItem{
			imap.FetchUid,        
			imap.FetchEnvelope,
		}, messages)
	}()
	
	var matchedUIDs []uint32
	for msg := range messages {
		if msg.Envelope != nil && msg.Envelope.Subject != "" {
			// 比较主题（忽略大小写）
			if strings.Contains(strings.ToLower(msg.Envelope.Subject), strings.ToLower(targetSubject)) {
				matchedUIDs = append(matchedUIDs, msg.Uid)
				ds.logger.Debugf("主题匹配成功 - UID: %d, 主题: %s", msg.Uid, msg.Envelope.Subject)
			}
		}
	}
	
	if err := <-done; err != nil {
		return nil, fmt.Errorf("获取邮件信息失败: %v", err)
	}
	
	ds.logger.Infof("主题过滤完成 - 输入: %d 封邮件, 匹配: %d 封邮件", len(uids), len(matchedUIDs))
	return matchedUIDs, nil
}

// 保持原有方法的兼容性
func (ds *DownloadService) filterEmailsBySubject(conn *IMAPConnection, uids []uint32, targetSubject string) ([]uint32, error) {
	return ds.filterEmailsBySubjectUID(conn, uids, targetSubject)
}

// isFileNameMatch 检查文件名是否匹配（宽松匹配）
func (ds *DownloadService) isFileNameMatch(actualName, targetName string) bool {
	if actualName == "" {
		return false
	}
	
	if targetName == "" {
		// 如果目标文件名为空，只要是PDF文件就匹配
		return strings.HasSuffix(strings.ToLower(actualName), ".pdf")
	}
	
	// 清理文件名
	cleanActual := strings.ToLower(utils.CleanFilename(actualName))
	cleanTarget := strings.ToLower(utils.CleanFilename(targetName))
	
	// 解码文件名
	decodedActual := strings.ToLower(utils.DecodeMimeHeader(actualName))
	decodedTarget := strings.ToLower(utils.DecodeMimeHeader(targetName))
	
	// 记录匹配过程
	ds.logger.Debugf("文件名匹配检查 - 实际: '%s' -> '%s' -> '%s', 目标: '%s' -> '%s' -> '%s'", 
		actualName, cleanActual, decodedActual, targetName, cleanTarget, decodedTarget)
	
	// 多种匹配策略（都转为小写比较）
	match := cleanActual == cleanTarget ||
			 strings.ToLower(actualName) == strings.ToLower(targetName) ||
			 decodedActual == decodedTarget ||
			 strings.Contains(cleanActual, cleanTarget) ||
			 strings.Contains(cleanTarget, cleanActual) ||
			 strings.Contains(decodedActual, decodedTarget) ||
			 strings.Contains(decodedTarget, decodedActual)
	
	ds.logger.Debugf("文件名匹配结果: %v", match)
	return match
}

// downloadWithProgress 带进度的下载
func (ds *DownloadService) downloadWithProgress(worker *DownloadWorker, src io.Reader, dst io.Writer) error {
	task := worker.Task
	
	// 动态调整缓冲区大小
	bufferSize := ds.calculateOptimalBufferSize(task.FileSize)
	buffer := make([]byte, bufferSize)
	
	var downloaded int64
	startTime := time.Now()
	lastProgressUpdate := time.Now()
	
	for {
		select {
		case <-worker.Context.Done():
			return fmt.Errorf("下载被取消")
		default:
			n, err := src.Read(buffer)
			if n > 0 {
				if _, writeErr := dst.Write(buffer[:n]); writeErr != nil {
					return fmt.Errorf("写入文件失败: %v", writeErr)
				}
				
				downloaded += int64(n)
				
				// 限制进度更新频率，避免过多的数据库写入
				now := time.Now()
				if now.Sub(lastProgressUpdate) >= 500*time.Millisecond || err == io.EOF {
					lastProgressUpdate = now
					
					// 计算进度和速度
					var progress float64
					if task.FileSize > 0 {
						progress = float64(downloaded) / float64(task.FileSize) * 100
					} else {
						// 文件大小未知时，显示已下载的字节数
						progress = 0
					}
					
					elapsed := now.Sub(startTime).Seconds()
					speed := ""
					if elapsed > 0 {
						bytesPerSecond := float64(downloaded) / elapsed
						speed = utils.FormatBytes(int64(bytesPerSecond)) + "/s"
					}
					
					// 发送进度更新
					select {
					case worker.Progress <- ProgressUpdate{
						TaskID:         task.ID,
						DownloadedSize: downloaded,
						Progress:       progress,
						Speed:          speed,
						Status:         models.StatusDownloading,
					}:
					default:
						// 如果progress channel已满，跳过这次更新
					}
				}
			}
			
			if err == io.EOF {
				// 下载完成
				select {
				case worker.Progress <- ProgressUpdate{
					TaskID:   task.ID,
					Status:   models.StatusCompleted,
					Progress: 100,
				}:
				default:
					// 如果channel已关闭，直接更新数据库
					ds.updateTaskStatus(task.ID, models.StatusCompleted, "", downloaded, 100, "")
				}
				return nil
			}
			
			if err != nil {
				return fmt.Errorf("读取数据失败: %v", err)
			}
		}
	}
}

// calculateOptimalBufferSize 计算最优缓冲区大小
func (ds *DownloadService) calculateOptimalBufferSize(fileSize int64) int {
	const minBufferSize = 8 * 1024   // 8KB
	const maxBufferSize = 1024 * 1024 // 1MB
	
	if fileSize <= 0 {
		return 64 * 1024 // 默认64KB
	}
	
	// 根据文件大小动态调整缓冲区
	var bufferSize int
	if fileSize < 1024*1024 { // 小于1MB
		bufferSize = minBufferSize
	} else if fileSize < 10*1024*1024 { // 小于10MB
		bufferSize = 64 * 1024 // 64KB
	} else if fileSize < 100*1024*1024 { // 小于100MB
		bufferSize = 256 * 1024 // 256KB
	} else {
		bufferSize = maxBufferSize // 1MB
	}
	
	return bufferSize
} 