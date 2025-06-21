package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"emaild/backend/database"
	"emaild/backend/models"
	"emaild/backend/utils"

	"github.com/sirupsen/logrus"
)

// DownloadService 下载服务结构体
type DownloadService struct {
	logger        *logrus.Logger
	workers       int                           // 工作线程数
	workerPool    chan chan *models.DownloadTask // 工作线程池
	taskQueue     chan *models.DownloadTask     // 任务队列
	activeWorkers map[uint]*DownloadWorker      // 活跃的工作线程
	mutex         sync.RWMutex                  // 读写锁
	stopChan      chan bool                     // 停止信号
	isRunning     bool                          // 是否正在运行
	client        *http.Client                  // HTTP客户端
}

// DownloadWorker 下载工作线程
type DownloadWorker struct {
	ID         uint
	TaskChan   chan *models.DownloadTask
	service    *DownloadService
	ctx        context.Context
	cancel     context.CancelFunc
	isActive   bool
	currentTask *models.DownloadTask
}

// NewDownloadService 创建新的下载服务实例
func NewDownloadService(logger *logrus.Logger, workers int) *DownloadService {
	// 创建HTTP客户端，配置超时和重试
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:       100,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: false,
		},
	}

	return &DownloadService{
		logger:        logger,
		workers:       workers,
		workerPool:    make(chan chan *models.DownloadTask, workers),
		taskQueue:     make(chan *models.DownloadTask, 1000),
		activeWorkers: make(map[uint]*DownloadWorker),
		stopChan:      make(chan bool),
		client:        client,
	}
}

// Start 启动下载服务
func (ds *DownloadService) Start() error {
	if ds.isRunning {
		return fmt.Errorf("下载服务已经在运行中")
	}

	ds.isRunning = true
	ds.logger.Infof("启动下载服务，工作线程数: %d", ds.workers)

	// 启动工作线程
	for i := 0; i < ds.workers; i++ {
		worker := ds.createWorker(uint(i + 1))
		go worker.start()
	}

	// 启动任务分发器
	go ds.dispatcher()

	// 恢复未完成的下载任务
	go ds.resumePendingTasks()

	return nil
}

// Stop 停止下载服务
func (ds *DownloadService) Stop() {
	if !ds.isRunning {
		return
	}

	ds.logger.Info("正在停止下载服务...")
	ds.isRunning = false
	
	// 发送停止信号
	close(ds.stopChan)

	// 停止所有工作线程
	ds.mutex.Lock()
	for _, worker := range ds.activeWorkers {
		worker.stop()
	}
	ds.activeWorkers = make(map[uint]*DownloadWorker)
	ds.mutex.Unlock()

	// 关闭通道
	close(ds.taskQueue)
	close(ds.workerPool)

	ds.logger.Info("下载服务已停止")
}

// createWorker 创建工作线程
func (ds *DownloadService) createWorker(id uint) *DownloadWorker {
	ctx, cancel := context.WithCancel(context.Background())
	
	worker := &DownloadWorker{
		ID:       id,
		TaskChan: make(chan *models.DownloadTask),
		service:  ds,
		ctx:      ctx,
		cancel:   cancel,
		isActive: false,
	}

	ds.mutex.Lock()
	ds.activeWorkers[id] = worker
	ds.mutex.Unlock()

	return worker
}

// dispatcher 任务分发器
func (ds *DownloadService) dispatcher() {
	for {
		select {
		case task := <-ds.taskQueue:
			if task != nil {
				// 获取可用的工作线程
				go func() {
					workerTaskChan := <-ds.workerPool
					workerTaskChan <- task
				}()
			}
		case <-ds.stopChan:
			ds.logger.Debug("任务分发器收到停止信号")
			return
		}
	}
}

// AddTask 添加下载任务
func (ds *DownloadService) AddTask(task *models.DownloadTask) error {
	if !ds.isRunning {
		return fmt.Errorf("下载服务未运行")
	}

	// 验证任务
	if task.Source == "" {
		return fmt.Errorf("下载源不能为空")
	}

	// 设置任务状态
	task.Status = models.StatusPending
	task.Progress = 0.0
	task.Speed = "等待中"

	// 保存到数据库
	if err := database.CreateDownloadTask(task); err != nil {
		return fmt.Errorf("保存下载任务失败: %v", err)
	}

	// 添加到队列
	select {
	case ds.taskQueue <- task:
		ds.logger.Infof("添加下载任务: %s", task.FileName)
		return nil
	default:
		return fmt.Errorf("任务队列已满")
	}
}

// PauseTask 暂停下载任务
func (ds *DownloadService) PauseTask(taskID uint) error {
	// 更新数据库状态
	task, err := ds.getTaskByID(taskID)
	if err != nil {
		return err
	}

	if task.Status == models.StatusDownloading {
		task.Status = models.StatusPaused
		task.Speed = "已暂停"
		
		if err := database.UpdateDownloadTask(task); err != nil {
			return fmt.Errorf("更新任务状态失败: %v", err)
		}

		// 通知工作线程暂停
		ds.mutex.RLock()
		for _, worker := range ds.activeWorkers {
			if worker.currentTask != nil && worker.currentTask.ID == taskID {
				worker.cancel()
				break
			}
		}
		ds.mutex.RUnlock()

		ds.logger.Infof("暂停下载任务: %s", task.FileName)
		return nil
	}

	return fmt.Errorf("任务状态不允许暂停: %s", task.Status)
}

// ResumeTask 恢复下载任务
func (ds *DownloadService) ResumeTask(taskID uint) error {
	task, err := ds.getTaskByID(taskID)
	if err != nil {
		return err
	}

	if task.Status == models.StatusPaused || task.Status == models.StatusFailed {
		task.Status = models.StatusPending
		task.Speed = "等待中"
		task.Error = ""
		
		if err := database.UpdateDownloadTask(task); err != nil {
			return fmt.Errorf("更新任务状态失败: %v", err)
		}

		// 重新添加到队列
		select {
		case ds.taskQueue <- task:
			ds.logger.Infof("恢复下载任务: %s", task.FileName)
			return nil
		default:
			return fmt.Errorf("任务队列已满")
		}
	}

	return fmt.Errorf("任务状态不允许恢复: %s", task.Status)
}

// CancelTask 取消下载任务
func (ds *DownloadService) CancelTask(taskID uint) error {
	task, err := ds.getTaskByID(taskID)
	if err != nil {
		return err
	}

	// 更新状态
	task.Status = models.StatusCancelled
	task.Error = "用户取消"
	task.Speed = "已取消"
	
	if err := database.UpdateDownloadTask(task); err != nil {
		return fmt.Errorf("更新任务状态失败: %v", err)
	}

	// 通知工作线程取消
	ds.mutex.RLock()
	for _, worker := range ds.activeWorkers {
		if worker.currentTask != nil && worker.currentTask.ID == taskID {
			worker.cancel()
			break
		}
	}
	ds.mutex.RUnlock()

	// 删除未完成的文件
	if task.LocalPath != "" && task.Status != models.StatusCompleted {
		os.Remove(task.LocalPath)
		os.Remove(task.LocalPath + ".tmp")
	}

	ds.logger.Infof("取消下载任务: %s", task.FileName)
	return nil
}

// getTaskByID 根据ID获取任务
func (ds *DownloadService) getTaskByID(taskID uint) (*models.DownloadTask, error) {
	tasks, _, err := database.GetDownloadTasks(1, 0)
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if task.ID == taskID {
			return &task, nil
		}
	}

	return nil, fmt.Errorf("找不到任务ID: %d", taskID)
}

// resumePendingTasks 恢复未完成的下载任务
func (ds *DownloadService) resumePendingTasks() {
	ds.logger.Info("检查未完成的下载任务...")

	// 获取未完成的任务
	pendingTasks, err := database.GetDownloadTasksByStatus(models.StatusDownloading)
	if err != nil {
		ds.logger.Errorf("获取未完成任务失败: %v", err)
		return
	}

	pausedTasks, err := database.GetDownloadTasksByStatus(models.StatusPending)
	if err != nil {
		ds.logger.Errorf("获取待处理任务失败: %v", err)
		return
	}

	allTasks := append(pendingTasks, pausedTasks...)

	if len(allTasks) > 0 {
		ds.logger.Infof("发现 %d 个未完成的任务，准备恢复", len(allTasks))
		
		for _, task := range allTasks {
			// 重置状态
			task.Status = models.StatusPending
			task.Speed = "等待中"
			database.UpdateDownloadTask(&task)

			// 添加到队列
			select {
			case ds.taskQueue <- &task:
				ds.logger.Debugf("恢复任务: %s", task.FileName)
			default:
				ds.logger.Warn("任务队列已满，无法恢复更多任务")
				break
			}
		}
	}
}

// GetActiveDownloads 获取活跃的下载任务
func (ds *DownloadService) GetActiveDownloads() []*models.DownloadTask {
	var activeTasks []*models.DownloadTask
	
	ds.mutex.RLock()
	for _, worker := range ds.activeWorkers {
		if worker.currentTask != nil && worker.isActive {
			activeTasks = append(activeTasks, worker.currentTask)
		}
	}
	ds.mutex.RUnlock()

	return activeTasks
}

// 工作线程方法
// start 启动工作线程
func (dw *DownloadWorker) start() {
	dw.service.logger.Debugf("工作线程 %d 启动", dw.ID)

	for {
		// 将工作线程通道放入池中
		dw.service.workerPool <- dw.TaskChan

		select {
		case task := <-dw.TaskChan:
			if task != nil {
				dw.processTask(task)
			}
		case <-dw.ctx.Done():
			dw.service.logger.Debugf("工作线程 %d 收到停止信号", dw.ID)
			return
		}
	}
}

// stop 停止工作线程
func (dw *DownloadWorker) stop() {
	dw.cancel()
}

// processTask 处理下载任务
func (dw *DownloadWorker) processTask(task *models.DownloadTask) {
	dw.currentTask = task
	dw.isActive = true
	
	defer func() {
		dw.currentTask = nil
		dw.isActive = false
	}()

	dw.service.logger.Infof("工作线程 %d 开始下载: %s", dw.ID, task.FileName)

	// 更新任务状态
	task.Status = models.StatusDownloading
	task.Speed = "正在下载"
	database.UpdateDownloadTask(task)

	// 执行下载
	err := dw.downloadFile(task)
	if err != nil {
		// 下载失败
		task.Status = models.StatusFailed
		task.Error = err.Error()
		task.Speed = "下载失败"
		dw.service.logger.Errorf("下载失败 [%s]: %v", task.FileName, err)
	} else {
		// 下载成功
		task.Status = models.StatusCompleted
		task.Progress = 100.0
		task.Speed = "完成"
		dw.service.logger.Infof("下载完成: %s", task.FileName)
		
		// 更新统计数据
		go func() {
			today := time.Now().Truncate(24 * time.Hour)
			database.CreateOrUpdateStatistics(today, 1, 1, 0, task.FileSize)
		}()
	}

	// 更新任务状态
	database.UpdateDownloadTask(task)
}

// downloadFile 下载文件（支持断点续传）
func (dw *DownloadWorker) downloadFile(task *models.DownloadTask) error {
	// 获取配置
	config, err := database.GetConfig()
	if err != nil {
		return fmt.Errorf("获取配置失败: %v", err)
	}

	// 确保下载目录存在
	if err := os.MkdirAll(config.DownloadPath, 0755); err != nil {
		return fmt.Errorf("创建下载目录失败: %v", err)
	}

	// 生成文件路径
	if task.FileName == "" {
		task.FileName = utils.ExtractFilenameFromURL(task.Source)
	}
	task.FileName = utils.CleanFilename(task.FileName)
	
	filePath := filepath.Join(config.DownloadPath, task.FileName)
	tempPath := filePath + ".tmp"

	// 检查文件是否已存在且完整
	if info, err := os.Stat(filePath); err == nil {
		if task.FileSize > 0 && info.Size() == task.FileSize {
			// 文件已存在且大小匹配，跳过下载
			task.LocalPath = filePath
			task.DownloadedSize = task.FileSize
			return nil
		}
	}

	// 检查是否有未完成的下载
	resumePos := int64(0)
	if info, err := os.Stat(tempPath); err == nil {
		resumePos = info.Size()
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(dw.ctx, "GET", task.Source, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置User-Agent和其他头部
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/pdf,*/*")
	
	// 如果支持断点续传，设置Range头
	if resumePos > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumePos))
		dw.service.logger.Debugf("断点续传，从位置 %d 开始", resumePos)
	}

	// 发送请求
	resp, err := dw.service.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP错误: %d %s", resp.StatusCode, resp.Status)
	}

	// 获取文件大小
	contentLength := resp.ContentLength
	if task.FileSize == 0 && contentLength > 0 {
		if resumePos > 0 {
			task.FileSize = resumePos + contentLength
		} else {
			task.FileSize = contentLength
		}
		database.UpdateDownloadTask(task)
	}

	// 验证内容类型
	if !dw.isValidPDFResponse(resp) {
		return fmt.Errorf("响应内容不是PDF文件")
	}

	// 打开文件进行写入
	var file *os.File
	if resumePos > 0 {
		// 断点续传，以追加模式打开
		file, err = os.OpenFile(tempPath, os.O_WRONLY|os.O_APPEND, 0644)
	} else {
		// 新下载，创建新文件
		file, err = os.Create(tempPath)
	}
	
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	// 开始下载
	startTime := time.Now()
	lastUpdate := startTime
	downloadedBytes := resumePos

	buffer := make([]byte, 32*1024) // 32KB缓冲区
	
	for {
		select {
		case <-dw.ctx.Done():
			return fmt.Errorf("下载被取消")
		default:
			// 读取数据
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				// 写入文件
				if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
					return fmt.Errorf("写入文件失败: %v", writeErr)
				}
				
				downloadedBytes += int64(n)
				
				// 定期更新进度（每秒更新一次）
				now := time.Now()
				if now.Sub(lastUpdate) >= time.Second {
					progress := float64(downloadedBytes) / float64(task.FileSize) * 100
					if task.FileSize == 0 {
						progress = 0
					}
					
					// 计算下载速度
					elapsed := now.Sub(startTime).Seconds()
					speed := ""
					if elapsed > 0 {
						bytesPerSecond := float64(downloadedBytes-resumePos) / elapsed
						speed = utils.FormatBytes(int64(bytesPerSecond)) + "/s"
					}
					
					// 更新数据库
					database.UpdateDownloadProgress(task.ID, downloadedBytes, progress, speed)
					task.DownloadedSize = downloadedBytes
					task.Progress = progress
					task.Speed = speed
					
					lastUpdate = now
				}
			}
			
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("读取响应失败: %v", err)
			}
		}
	}

	// 确保文件同步到磁盘
	file.Sync()
	file.Close()

	// 验证下载的文件
	if err := dw.validateDownloadedFile(tempPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("文件验证失败: %v", err)
	}

	// 重命名临时文件
	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("重命名文件失败: %v", err)
	}

	task.LocalPath = filePath
	return nil
}

// isValidPDFResponse 验证HTTP响应是否为有效的PDF
func (dw *DownloadWorker) isValidPDFResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	
	// 检查Content-Type
	if strings.Contains(strings.ToLower(contentType), "pdf") {
		return true
	}
	
	// 如果Content-Type不明确，暂时允许通过，后续在文件验证中检查
	return true
}

// validateDownloadedFile 验证下载的文件
func (dw *DownloadWorker) validateDownloadedFile(filePath string) error {
	// 使用工具函数验证PDF文件
	return utils.ValidatePDFFile(filePath)
} 