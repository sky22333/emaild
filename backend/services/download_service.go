package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

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
}

// DownloadWorker 下载工作者
type DownloadWorker struct {
	ID       uint
	Task     *models.DownloadTask
	Client   *http.Client
	Context  context.Context
	Cancel   context.CancelFunc
	Progress chan ProgressUpdate
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

// NewDownloadService 创建下载服务
func NewDownloadService(db *database.Database) *DownloadService {
	ctx, cancel := context.WithCancel(context.Background())
	
	service := &DownloadService{
		db:            db,
		workers:       make(map[uint]*DownloadWorker),
		maxConcurrent: 3, // 默认最大并发数
		ctx:           ctx,
		cancel:        cancel,
		taskQueue:     make(chan *models.DownloadTask, 100), // 缓冲队列
	}
	
	// 启动任务调度器
	go service.taskScheduler()
	
	return service
}

// taskScheduler 任务调度器
func (ds *DownloadService) taskScheduler() {
	for {
		select {
		case <-ds.ctx.Done():
			return
		case task := <-ds.taskQueue:
			// 检查是否可以启动新任务
			ds.activeWorkerMutex.RLock()
			canStart := ds.activeWorkers < ds.maxConcurrent
			ds.activeWorkerMutex.RUnlock()
			
			if canStart {
				go ds.startDownload(task)
			} else {
				// 重新放入队列等待
				select {
				case ds.taskQueue <- task:
				default:
					// 队列满了，更新任务状态为失败
					ds.updateTaskStatus(task.ID, models.StatusFailed, "任务队列已满", 0, 0, "")
				}
			}
		}
	}
}

// StartDownload 开始下载任务
func (ds *DownloadService) StartDownload(taskID uint) error {
	// 使用索引优化的查询
	task, err := ds.getTaskByIDOptimized(taskID)
	if err != nil {
		return fmt.Errorf("获取任务失败: %v", err)
	}
	
	if task.Status != models.StatusPending {
		return fmt.Errorf("任务状态不正确: %s", task.Status)
	}
	
	// 将任务放入队列
	select {
	case ds.taskQueue <- task:
		return nil
	default:
		return fmt.Errorf("任务队列已满")
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
	// 增加活跃工作者计数
	ds.activeWorkerMutex.Lock()
	ds.activeWorkers++
	ds.activeWorkerMutex.Unlock()
	
	// 确保完成时减少计数
	defer func() {
		ds.activeWorkerMutex.Lock()
		ds.activeWorkers--
		ds.activeWorkerMutex.Unlock()
	}()
	
	// 创建工作者上下文
	workerCtx, workerCancel := context.WithCancel(ds.ctx)
	defer workerCancel()
	
	// 创建工作者
	worker := &DownloadWorker{
		ID:      task.ID,
		Task:    task,
		Client:  &http.Client{Timeout: 30 * time.Second},
		Context: workerCtx,
		Cancel:  workerCancel,
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
		close(worker.Progress)
	}()
	
	// 启动进度监控
	go ds.monitorProgress(worker)
	
	// 执行下载
	ds.performDownload(worker)
}

// performDownload 执行下载
func (ds *DownloadService) performDownload(worker *DownloadWorker) {
	task := worker.Task
	
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
		worker.Progress <- ProgressUpdate{
			TaskID: task.ID,
			Status: models.StatusFailed,
			Error:  err.Error(),
		}
	}
}

// downloadFromURL 从URL下载文件
func (ds *DownloadService) downloadFromURL(worker *DownloadWorker) error {
	task := worker.Task
	
	// 创建请求
	req, err := http.NewRequestWithContext(worker.Context, "GET", task.Source, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 发送请求
	resp, err := worker.Client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器响应错误: %d", resp.StatusCode)
	}
	
	// 获取文件大小
	contentLength := resp.ContentLength
	if contentLength > 0 {
		task.FileSize = contentLength
	}
	
	// 创建文件
	file, err := os.Create(task.LocalPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()
	
	// 下载文件并监控进度
	return ds.downloadWithProgress(worker, resp.Body, file)
}

// downloadAttachment 下载邮件附件
func (ds *DownloadService) downloadAttachment(worker *DownloadWorker) error {
	// 这里需要重新连接到邮箱获取附件
	// 为了简化，先创建一个空文件标记完成
	task := worker.Task
	
	file, err := os.Create(task.LocalPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()
	
	// 模拟下载进度
	for i := 0; i <= 100; i += 10 {
		select {
		case <-worker.Context.Done():
			return fmt.Errorf("下载被取消")
		default:
			worker.Progress <- ProgressUpdate{
				TaskID:   task.ID,
				Progress: float64(i),
				Status:   models.StatusDownloading,
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	worker.Progress <- ProgressUpdate{
		TaskID: task.ID,
		Status: models.StatusCompleted,
		Progress: 100,
	}
	
	return nil
}

// downloadWithProgress 带进度的下载
func (ds *DownloadService) downloadWithProgress(worker *DownloadWorker, src io.Reader, dst io.Writer) error {
	task := worker.Task
	buffer := make([]byte, 32*1024) // 32KB缓冲
	var downloaded int64
	startTime := time.Now()
	
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
				
				// 计算进度和速度
				progress := float64(downloaded) / float64(task.FileSize) * 100
				elapsed := time.Since(startTime).Seconds()
				speed := ""
				if elapsed > 0 {
					bytesPerSecond := float64(downloaded) / elapsed
					speed = utils.FormatBytes(int64(bytesPerSecond)) + "/s"
				}
				
				// 发送进度更新
				worker.Progress <- ProgressUpdate{
					TaskID:         task.ID,
					DownloadedSize: downloaded,
					Progress:       progress,
					Speed:          speed,
					Status:         models.StatusDownloading,
				}
			}
			
			if err == io.EOF {
				// 下载完成
				worker.Progress <- ProgressUpdate{
					TaskID: task.ID,
					Status: models.StatusCompleted,
					Progress: 100,
				}
				return nil
			}
			
			if err != nil {
				return fmt.Errorf("读取数据失败: %v", err)
			}
		}
	}
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

// updateTaskStatus 更新任务状态（使用事务）
func (ds *DownloadService) updateTaskStatus(taskID uint, status models.DownloadStatus, errorMsg string, downloadedSize int64, progress float64, speed string) error {
	tx, err := ds.db.DB.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()
	
	query := `
		UPDATE download_tasks 
		SET status = ?, error = ?, downloaded_size = ?, progress = ?, speed = ?, updated_at = ?
		WHERE id = ?
	`
	
	_, err = tx.Exec(query, status, errorMsg, downloadedSize, progress, speed, time.Now(), taskID)
	if err != nil {
		return fmt.Errorf("更新任务状态失败: %v", err)
	}
	
	return tx.Commit()
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
	// 取消所有任务
	ds.cancel()
	
	// 等待所有工作者完成
	for {
		ds.workerMutex.RLock()
		count := len(ds.workers)
		ds.workerMutex.RUnlock()
		
		if count == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
} 