package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"emaild/backend/models"
	
	_ "modernc.org/sqlite"
)

// Database 数据库连接管理器
type Database struct {
	DB *sql.DB
	mu sync.RWMutex // 保护数据库操作的读写锁
}

// WithTransaction 执行事务的通用方法（增强版）
func (d *Database) WithTransaction(fn func(*sql.Tx) error) error {
	return d.WithTransactionTimeout(fn, 30*time.Second)
}

// WithTransactionTimeout 带超时的事务执行
func (d *Database) WithTransactionTimeout(fn func(*sql.Tx) error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %v", err)
	}
	
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				// 记录回滚错误，但不覆盖原始错误
				fmt.Printf("事务回滚失败: %v\n", rollbackErr)
			}
		}
	}()

	err = fn(tx)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}

	return nil
}

// WithRetry 带重试的数据库操作
func (d *Database) WithRetry(operation func() error, maxRetries int) error {
	var lastErr error
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避
			backoff := time.Duration(attempt) * time.Second
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			time.Sleep(backoff)
		}
		
		lastErr = operation()
		if lastErr == nil {
			return nil
		}
		
		// 检查是否是可重试的错误
		if !isRetryableError(lastErr) {
			break
		}
	}
	
	return fmt.Errorf("操作失败，已重试 %d 次: %v", maxRetries, lastErr)
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"database is locked",
		"database is busy",
		"connection reset",
		"timeout",
		"temporary failure",
	}
	
	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}
	
	return false
}

// NewDatabase 创建新的数据库连接
func NewDatabase() (*Database, error) {
	// 获取用户目录
	userDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户目录失败: %v", err)
	}

	// 创建应用数据目录
	appDataDir := filepath.Join(userDir, ".emaild")
	if err := os.MkdirAll(appDataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %v", err)
	}

	dbPath := filepath.Join(appDataDir, "emaild.db")
	
	// 打开SQLite数据库
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %v", err)
	}

	// 优化连接池参数
	db.SetMaxOpenConns(10)        // 减少最大连接数，避免资源竞争
	db.SetMaxIdleConns(5)         // 设置合理的空闲连接数
	db.SetConnMaxLifetime(15 * time.Minute) // 延长连接生命周期

	// 启用关键的SQLite配置
	pragmas := []string{
		"PRAGMA foreign_keys = ON",           // 启用外键约束
		"PRAGMA journal_mode = WAL",          // 启用WAL模式
		"PRAGMA synchronous = NORMAL",        // 平衡性能和安全性
		"PRAGMA cache_size = 10000",          // 增加缓存大小
		"PRAGMA temp_store = memory",         // 临时表存储在内存中
		"PRAGMA busy_timeout = 30000",        // 设置忙碌超时为30秒
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("执行PRAGMA失败 (%s): %v", pragma, err)
		}
	}

	database := &Database{DB: db}

	// 创建表结构
	if err := database.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("创建表结构失败: %v", err)
	}

	// 初始化默认配置
	if err := database.initDefaultConfig(); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化默认配置失败: %v", err)
	}

	return database, nil
}

// Close 关闭数据库连接
func (d *Database) Close() error {
	if d.DB != nil {
		return d.DB.Close()
	}
	return nil
}

// createTables 创建数据库表
func (d *Database) createTables() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS email_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			imap_server TEXT NOT NULL,
			imap_port INTEGER DEFAULT 993,
			use_ssl BOOLEAN DEFAULT TRUE,
			is_active BOOLEAN DEFAULT TRUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		
		`CREATE TABLE IF NOT EXISTS download_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email_id INTEGER NOT NULL,
			subject TEXT NOT NULL,
			sender TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size INTEGER DEFAULT 0,
			downloaded_size INTEGER DEFAULT 0,
			status TEXT DEFAULT 'pending',
			type TEXT NOT NULL,
			source TEXT NOT NULL,
			local_path TEXT,
			error TEXT,
			progress REAL DEFAULT 0.0,
			speed TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (email_id) REFERENCES email_accounts(id) ON DELETE CASCADE
		)`,
		
		`CREATE TABLE IF NOT EXISTS email_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email_id INTEGER NOT NULL,
			message_id TEXT NOT NULL UNIQUE,
			subject TEXT NOT NULL,
			sender TEXT NOT NULL,
			recipients TEXT,
			date DATETIME NOT NULL,
			has_pdf BOOLEAN DEFAULT FALSE,
			is_processed BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (email_id) REFERENCES email_accounts(id) ON DELETE CASCADE
		)`,
		
		`CREATE TABLE IF NOT EXISTS app_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			download_path TEXT DEFAULT '',
			max_concurrent INTEGER DEFAULT 3,
			check_interval INTEGER DEFAULT 60,
			auto_check BOOLEAN DEFAULT FALSE,
			minimize_to_tray BOOLEAN DEFAULT TRUE,
			start_minimized BOOLEAN DEFAULT FALSE,
			enable_notification BOOLEAN DEFAULT TRUE,
			theme TEXT DEFAULT 'auto',
			language TEXT DEFAULT 'zh-CN',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		
		`CREATE TABLE IF NOT EXISTS download_statistics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date DATE NOT NULL UNIQUE,
			total_downloads INTEGER DEFAULT 0,
			success_downloads INTEGER DEFAULT 0,
			failed_downloads INTEGER DEFAULT 0,
			total_size INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, table := range tables {
		if _, err := d.DB.Exec(table); err != nil {
			return fmt.Errorf("创建表失败: %v", err)
		}
	}

	// 创建索引
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_download_tasks_status ON download_tasks(status)",
		"CREATE INDEX IF NOT EXISTS idx_download_tasks_email_id ON download_tasks(email_id)",
		"CREATE INDEX IF NOT EXISTS idx_email_messages_message_id ON email_messages(message_id)",
		"CREATE INDEX IF NOT EXISTS idx_email_messages_email_id ON email_messages(email_id)",
		"CREATE INDEX IF NOT EXISTS idx_download_statistics_date ON download_statistics(date)",
	}

	for _, index := range indexes {
		if _, err := d.DB.Exec(index); err != nil {
			return fmt.Errorf("创建索引失败: %v", err)
		}
	}

	return nil
}

// initDefaultConfig 初始化默认配置
func (d *Database) initDefaultConfig() error {
	var count int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM app_configs").Scan(&count); err != nil {
		return err
	}

	if count == 0 {
		// 获取用户下载目录
		userDir, _ := os.UserHomeDir()
		defaultDownloadPath := filepath.Join(userDir, "Downloads", "EmailPDFs")
		
		_, err := d.DB.Exec(`
			INSERT INTO app_configs (download_path, max_concurrent, check_interval, auto_check, 
			minimize_to_tray, start_minimized, enable_notification, theme, language) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			defaultDownloadPath, 3, 60, false, true, false, true, "auto", "zh-CN")
		return err
	}

	return nil
}

// 移除重复的全局函数，统一使用Database结构体方法

// CreateEmailAccount 创建邮箱账户
func (d *Database) CreateEmailAccount(account *models.EmailAccount) error {
	now := time.Now()
	
	return d.WithTransaction(func(tx *sql.Tx) error {
		query := `
			INSERT INTO email_accounts (name, email, password, imap_server, imap_port, use_ssl, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		
		result, err := tx.Exec(query,
			account.Name, account.Email, account.Password, account.IMAPServer,
			account.IMAPPort, account.UseSSL, account.IsActive, now, now,
		)
		if err != nil {
			return err
		}

		id, err := result.LastInsertId()
		if err != nil {
			return err
		}

		account.ID = uint(id)
		account.CreatedAt = models.TimeToString(now)
		account.UpdatedAt = models.TimeToString(now)
		
		return nil
	})
}

// GetEmailAccounts 获取所有邮箱账户
func (d *Database) GetEmailAccounts() ([]models.EmailAccount, error) {
	query := `SELECT id, name, email, password, imap_server, imap_port, use_ssl, is_active, created_at, updated_at FROM email_accounts ORDER BY created_at DESC`
	
	rows, err := d.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var accounts []models.EmailAccount
	for rows.Next() {
		var account models.EmailAccount
		var createdAt, updatedAt time.Time
		
		err := rows.Scan(
			&account.ID, &account.Name, &account.Email, &account.Password,
			&account.IMAPServer, &account.IMAPPort, &account.UseSSL, &account.IsActive,
			&createdAt, &updatedAt,
		)
		if err != nil {
			continue
		}
		
		account.CreatedAt = models.TimeToString(createdAt)
		account.UpdatedAt = models.TimeToString(updatedAt)
		accounts = append(accounts, account)
	}
	
	return accounts, nil
}

// GetEmailAccountByID 根据ID获取邮箱账户
func (d *Database) GetEmailAccountByID(id uint) (*models.EmailAccount, error) {
	query := `SELECT id, name, email, password, imap_server, imap_port, use_ssl, is_active, created_at, updated_at FROM email_accounts WHERE id = ?`
	
	row := d.DB.QueryRow(query, id)
	
	var account models.EmailAccount
	var createdAt, updatedAt time.Time
	err := row.Scan(
		&account.ID, &account.Name, &account.Email, &account.Password,
		&account.IMAPServer, &account.IMAPPort, &account.UseSSL, &account.IsActive,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	account.CreatedAt = models.TimeToString(createdAt)
	account.UpdatedAt = models.TimeToString(updatedAt)
	
	return &account, nil
}

// UpdateEmailAccount 更新邮箱账户
func (d *Database) UpdateEmailAccount(account *models.EmailAccount) error {
	now := time.Now()
	
	return d.WithTransaction(func(tx *sql.Tx) error {
		query := `
			UPDATE email_accounts 
			SET name = ?, email = ?, password = ?, imap_server = ?, imap_port = ?, 
				use_ssl = ?, is_active = ?, updated_at = ?
			WHERE id = ?
		`
		
		_, err := tx.Exec(query,
			account.Name, account.Email, account.Password, account.IMAPServer,
			account.IMAPPort, account.UseSSL, account.IsActive, now, account.ID,
		)
		if err != nil {
			return err
		}

		account.UpdatedAt = models.TimeToString(now)
		return nil
	})
}

// DeleteEmailAccount 删除邮箱账户
func (d *Database) DeleteEmailAccount(id uint) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// 删除相关的下载任务
	_, err = tx.Exec("DELETE FROM download_tasks WHERE email_id = ?", id)
	if err != nil {
		return err
	}

	// 删除相关的邮件消息
	_, err = tx.Exec("DELETE FROM email_messages WHERE email_id = ?", id)
	if err != nil {
		return err
	}

	// 删除邮箱账户
	_, err = tx.Exec("DELETE FROM email_accounts WHERE id = ?", id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// 数据库桶名称
const (
	EmailAccountsBucket    = "email_accounts"
	DownloadTasksBucket    = "download_tasks"
	EmailMessagesBucket    = "email_messages"
	AppConfigBucket        = "app_config"
	StatisticsBucket       = "statistics"
)

// CreateDownloadTask 创建下载任务
func (d *Database) CreateDownloadTask(task *models.DownloadTask) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	query := `
		INSERT INTO download_tasks (
			email_id, subject, sender, file_name, file_size, downloaded_size,
			status, type, source, local_path, error, progress, speed, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	result, err := tx.Exec(query,
		task.EmailID, task.Subject, task.Sender, task.FileName,
		task.FileSize, task.DownloadedSize, task.Status, task.Type,
		task.Source, task.LocalPath, task.Error, task.Progress,
		task.Speed, now, now,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	task.ID = uint(id)
	task.CreatedAt = models.TimeToString(now)
	task.UpdatedAt = models.TimeToString(now)

	return tx.Commit()
}

// GetDownloadTasksResponse 下载任务列表响应
type GetDownloadTasksResponse struct {
	Tasks []models.DownloadTask `json:"tasks"`
	Total int64                 `json:"total"`
}

// GetDownloadTasks 获取下载任务列表
func (d *Database) GetDownloadTasks(limit, offset int) ([]models.DownloadTask, int64, error) {
	// 获取总数
	var total int64
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM download_tasks").Scan(&total); err != nil {
		return nil, 0, err
	}

	// 获取任务列表，统一查询逻辑
	tasks, err := d.queryDownloadTasksWithJoin(`
		SELECT dt.id, dt.email_id, dt.subject, dt.sender, dt.file_name, dt.file_size,
		dt.downloaded_size, dt.status, dt.type, dt.source, dt.local_path, dt.error,
		dt.progress, dt.speed, dt.created_at, dt.updated_at,
		ea.id, ea.name, ea.email, ea.password, ea.imap_server, ea.imap_port, 
		ea.use_ssl, ea.is_active, ea.created_at, ea.updated_at
		FROM download_tasks dt
		LEFT JOIN email_accounts ea ON dt.email_id = ea.id
		ORDER BY dt.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	
	return tasks, total, err
}

// GetDownloadTasksByStatus 根据状态获取下载任务
func (d *Database) GetDownloadTasksByStatus(status models.DownloadStatus) ([]models.DownloadTask, error) {
	return d.queryDownloadTasksWithJoin(`
		SELECT dt.id, dt.email_id, dt.subject, dt.sender, dt.file_name, dt.file_size,
		dt.downloaded_size, dt.status, dt.type, dt.source, dt.local_path, dt.error,
		dt.progress, dt.speed, dt.created_at, dt.updated_at,
		ea.id, ea.name, ea.email, ea.password, ea.imap_server, ea.imap_port, 
		ea.use_ssl, ea.is_active, ea.created_at, ea.updated_at
		FROM download_tasks dt
		LEFT JOIN email_accounts ea ON dt.email_id = ea.id
		WHERE dt.status = ? ORDER BY dt.created_at DESC`, status)
}

// queryDownloadTasksWithJoin 统一的下载任务查询方法，消除重复代码
func (d *Database) queryDownloadTasksWithJoin(query string, args ...interface{}) ([]models.DownloadTask, error) {
	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var tasks []models.DownloadTask
	for rows.Next() {
		var task models.DownloadTask
		var account models.EmailAccount
		var taskCreatedAt, taskUpdatedAt sql.NullTime
		var accountCreatedAt, accountUpdatedAt sql.NullTime
		var accountID sql.NullInt64
		var accountName, accountEmail, accountPassword, accountIMAPServer sql.NullString
		var accountIMAPPort sql.NullInt64
		var accountUseSSL, accountIsActive sql.NullBool
		
		if err := rows.Scan(&task.ID, &task.EmailID, &task.Subject, &task.Sender,
			&task.FileName, &task.FileSize, &task.DownloadedSize, &task.Status,
			&task.Type, &task.Source, &task.LocalPath, &task.Error,
			&task.Progress, &task.Speed, &taskCreatedAt, &taskUpdatedAt,
			&accountID, &accountName, &accountEmail, &accountPassword, &accountIMAPServer,
			&accountIMAPPort, &accountUseSSL, &accountIsActive, &accountCreatedAt, &accountUpdatedAt); err != nil {
			return nil, err
		}
		
		// 转换时间 - 处理NULL值
		if taskCreatedAt.Valid {
			task.CreatedAt = models.TimeToString(taskCreatedAt.Time)
		} else {
			task.CreatedAt = models.TimeToString(time.Now())
		}
		
		if taskUpdatedAt.Valid {
			task.UpdatedAt = models.TimeToString(taskUpdatedAt.Time)
		} else {
			task.UpdatedAt = models.TimeToString(time.Now())
		}
		
		// 设置邮箱账户信息
		if accountID.Valid {
			account.ID = uint(accountID.Int64)
			if accountName.Valid {
				account.Name = accountName.String
			}
			if accountEmail.Valid {
				account.Email = accountEmail.String
			}
			if accountPassword.Valid {
				account.Password = accountPassword.String
			}
			if accountIMAPServer.Valid {
				account.IMAPServer = accountIMAPServer.String
			}
			if accountIMAPPort.Valid {
				account.IMAPPort = int(accountIMAPPort.Int64)
			}
			if accountUseSSL.Valid {
				account.UseSSL = accountUseSSL.Bool
			}
			if accountIsActive.Valid {
				account.IsActive = accountIsActive.Bool
			}
			
			// 处理账户时间字段的NULL值
			if accountCreatedAt.Valid {
				account.CreatedAt = models.TimeToString(accountCreatedAt.Time)
			} else {
				account.CreatedAt = models.TimeToString(time.Now())
			}
			
			if accountUpdatedAt.Valid {
				account.UpdatedAt = models.TimeToString(accountUpdatedAt.Time)
			} else {
				account.UpdatedAt = models.TimeToString(time.Now())
			}
			
			task.EmailAccount = account
		}
		
		tasks = append(tasks, task)
	}
	
	return tasks, rows.Err()
}

// CreateEmailMessage 创建邮件记录
func (d *Database) CreateEmailMessage(message *models.EmailMessage) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	query := `
		INSERT INTO email_messages (
			email_id, message_id, subject, sender, recipients, date,
			has_pdf, is_processed, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	result, err := tx.Exec(query,
		message.EmailID, message.MessageID, message.Subject, message.Sender,
		message.Recipients, message.Date, message.HasPDF, message.IsProcessed,
		now, now,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	message.ID = uint(id)
	message.CreatedAt = models.TimeToString(now)
	message.UpdatedAt = models.TimeToString(now)

	return tx.Commit()
}

// GetEmailMessageByMessageID 根据消息ID获取邮件记录
func (d *Database) GetEmailMessageByMessageID(messageID string) (*models.EmailMessage, error) {
	message := &models.EmailMessage{}
	var createdAt, updatedAt time.Time

	err := d.DB.QueryRow(`
		SELECT id, email_id, message_id, subject, sender, recipients, date,
		has_pdf, is_processed, created_at, updated_at 
		FROM email_messages WHERE message_id = ?`, messageID).Scan(
		&message.ID, &message.EmailID, &message.MessageID, &message.Subject,
		&message.Sender, &message.Recipients, &message.Date, &message.HasPDF,
		&message.IsProcessed, &createdAt, &updatedAt)
	
	if err != nil {
		return nil, err
	}
	
	message.CreatedAt = models.TimeToString(createdAt)
	message.UpdatedAt = models.TimeToString(updatedAt)
	
	return message, nil
}

// UpdateEmailMessage 更新邮件记录
func (d *Database) UpdateEmailMessage(message *models.EmailMessage) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	query := `
		UPDATE email_messages 
		SET subject = ?, sender = ?, recipients = ?, date = ?, 
			has_pdf = ?, is_processed = ?, updated_at = ?
		WHERE id = ?
	`
	
	_, err = tx.Exec(query,
		message.Subject, message.Sender, message.Recipients, message.Date,
		message.HasPDF, message.IsProcessed, now, message.ID,
	)
	if err != nil {
		return err
	}

	message.UpdatedAt = models.TimeToString(now)
	return tx.Commit()
}

// CreateOrUpdateStatistics 创建或更新统计数据
func (d *Database) CreateOrUpdateStatistics(date time.Time, totalDownloads, successDownloads, failedDownloads int, totalSize int64) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	dateStr := date.Format("2006-01-02")
	now := time.Now()
	
	query := `
		INSERT OR REPLACE INTO download_statistics 
		(date, total_downloads, success_downloads, failed_downloads, total_size, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	
	_, err = tx.Exec(query, dateStr, totalDownloads, successDownloads, failedDownloads, totalSize, now, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetStatistics 获取统计数据
func (d *Database) GetStatistics(days int) ([]models.DownloadStatistics, error) {
	rows, err := d.DB.Query(`
		SELECT id, date, total_downloads, success_downloads, failed_downloads, total_size,
		created_at, updated_at FROM download_statistics 
		WHERE date >= DATE('now', '-' || ? || ' days')
		ORDER BY date DESC`, days)
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var stats []models.DownloadStatistics
	for rows.Next() {
		var stat models.DownloadStatistics
		var dateStr string
		var createdAt, updatedAt time.Time
		
		if err := rows.Scan(&stat.ID, &dateStr, &stat.TotalDownloads,
			&stat.SuccessDownloads, &stat.FailedDownloads, &stat.TotalSize,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		
		// 将time.Time转换为string
		stat.Date = dateStr
		stat.CreatedAt = models.TimeToString(createdAt)
		stat.UpdatedAt = models.TimeToString(updatedAt)
		
		stats = append(stats, stat)
	}
	
	return stats, rows.Err()
}

// CleanOldData 清理旧数据
func (d *Database) CleanOldData(days int) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 清理旧的下载任务
	if _, err := tx.Exec(`
		DELETE FROM download_tasks 
		WHERE status IN ('completed', 'failed', 'cancelled') 
		AND created_at < DATE('now', '-' || ? || ' days')`, days); err != nil {
		return err
	}

	// 清理旧的邮件记录
	if _, err := tx.Exec(`
		DELETE FROM email_messages 
		WHERE created_at < DATE('now', '-' || ? || ' days')`, days); err != nil {
		return err
	}

	// 清理旧的统计数据
	if _, err := tx.Exec(`
		DELETE FROM download_statistics 
		WHERE date < DATE('now', '-' || ? || ' days')`, days); err != nil {
		return err
	}

	return tx.Commit()
}

// GetConfig 获取应用配置
func (d *Database) GetConfig() (models.AppConfig, error) {
	query := `SELECT id, download_path, max_concurrent, check_interval, auto_check, minimize_to_tray, start_minimized, enable_notification, theme, language, created_at, updated_at FROM app_configs LIMIT 1`
	
	row := d.DB.QueryRow(query)
	
	var config models.AppConfig
	var createdAt, updatedAt time.Time
	err := row.Scan(
		&config.ID, &config.DownloadPath, &config.MaxConcurrent, &config.CheckInterval,
		&config.AutoCheck, &config.MinimizeToTray, &config.StartMinimized,
		&config.EnableNotification, &config.Theme, &config.Language,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return config, err
	}
	
	config.CreatedAt = models.TimeToString(createdAt)
	config.UpdatedAt = models.TimeToString(updatedAt)
	
	return config, nil
}

// CreateConfig 创建配置
func (d *Database) CreateConfig(config models.AppConfig) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	query := `
		INSERT INTO app_configs (
			download_path, max_concurrent, check_interval, auto_check,
			minimize_to_tray, start_minimized, enable_notification,
			theme, language, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	_, err = tx.Exec(query,
		config.DownloadPath, config.MaxConcurrent, config.CheckInterval,
		config.AutoCheck, config.MinimizeToTray, config.StartMinimized,
		config.EnableNotification, config.Theme, config.Language, now, now,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateConfig 更新应用配置
func (d *Database) UpdateConfig(config *models.AppConfig) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	now := time.Now()
	query := `
		UPDATE app_configs 
		SET download_path = ?, max_concurrent = ?, check_interval = ?, auto_check = ?, 
			minimize_to_tray = ?, start_minimized = ?, enable_notification = ?, 
			theme = ?, language = ?, updated_at = ?
		WHERE id = ?
	`
	
	_, err = tx.Exec(query,
		config.DownloadPath, config.MaxConcurrent, config.CheckInterval,
		config.AutoCheck, config.MinimizeToTray, config.StartMinimized,
		config.EnableNotification, config.Theme, config.Language, now, config.ID,
	)
	if err != nil {
		return err
	}

	config.UpdatedAt = models.TimeToString(now)
	return tx.Commit()
} 
