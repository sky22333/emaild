package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"email-pdf-downloader/backend/models"
	"go.etcd.io/bbolt"
)

var DB *bbolt.DB

// 数据库桶名称
const (
	EmailAccountsBucket    = "email_accounts"
	DownloadTasksBucket    = "download_tasks"
	EmailMessagesBucket    = "email_messages"
	AppConfigBucket        = "app_config"
	StatisticsBucket       = "statistics"
)

// InitDatabase 初始化数据库连接
func InitDatabase() error {
	// 确保数据库目录存在
	dbDir := "./data"
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("创建数据库目录失败: %v", err)
	}

	dbPath := filepath.Join(dbDir, "email_pdf_downloader.db")
	
	var err error
	// 打开BoltDB数据库
	DB, err = bbolt.Open(dbPath, 0600, &bbolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %v", err)
	}

	// 创建数据库桶
	if err := createBuckets(); err != nil {
		return fmt.Errorf("创建数据库桶失败: %v", err)
	}

	// 初始化默认配置
	if err := initDefaultConfig(); err != nil {
		return fmt.Errorf("初始化默认配置失败: %v", err)
	}

	return nil
}

// createBuckets 创建数据库桶
func createBuckets() error {
	return DB.Update(func(tx *bbolt.Tx) error {
		buckets := []string{
			EmailAccountsBucket,
			DownloadTasksBucket,
			EmailMessagesBucket,
			AppConfigBucket,
			StatisticsBucket,
		}
		
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return fmt.Errorf("创建桶 %s 失败: %v", bucket, err)
			}
		}
		
		return nil
	})
}

// initDefaultConfig 初始化默认配置
func initDefaultConfig() error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(AppConfigBucket))
		
		// 检查是否已有配置
		if v := bucket.Get([]byte("config")); v != nil {
			return nil // 已有配置，跳过
		}
		
		// 创建默认配置
		defaultConfig := &models.AppConfig{
			ID:                 1,
			DownloadPath:       "./downloads",
			MaxConcurrent:      3,
			CheckInterval:      300,
			AutoCheck:          false,
			MinimizeToTray:     true,
			StartMinimized:     false,
			EnableNotification: true,
			Theme:              "auto",
			Language:           "zh-CN",
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		
		data, err := json.Marshal(defaultConfig)
		if err != nil {
			return err
		}
		
		return bucket.Put([]byte("config"), data)
	})
}

// GetConfig 获取应用配置
func GetConfig() (*models.AppConfig, error) {
	var config models.AppConfig
	
	err := DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(AppConfigBucket))
		v := bucket.Get([]byte("config"))
		if v == nil {
			return fmt.Errorf("配置不存在")
		}
		
		return json.Unmarshal(v, &config)
	})
	
	if err != nil {
		return nil, err
	}
	
	return &config, nil
}

// UpdateConfig 更新应用配置
func UpdateConfig(config *models.AppConfig) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(AppConfigBucket))
		
		config.UpdatedAt = time.Now()
		data, err := json.Marshal(config)
		if err != nil {
			return err
		}
		
		return bucket.Put([]byte("config"), data)
	})
}

// CreateEmailAccount 创建邮箱账户
func CreateEmailAccount(account *models.EmailAccount) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailAccountsBucket))
		
		// 生成ID
		id, _ := bucket.NextSequence()
		account.ID = uint(id)
		account.CreatedAt = time.Now()
		account.UpdatedAt = time.Now()
		account.IsActive = true
		
		data, err := json.Marshal(account)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%d", account.ID)
		return bucket.Put([]byte(key), data)
	})
}

// GetEmailAccounts 获取所有邮箱账户
func GetEmailAccounts() ([]models.EmailAccount, error) {
	var accounts []models.EmailAccount
	
	err := DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailAccountsBucket))
		
		return bucket.ForEach(func(k, v []byte) error {
			var account models.EmailAccount
			if err := json.Unmarshal(v, &account); err != nil {
				return err
			}
			
			if account.IsActive {
				accounts = append(accounts, account)
			}
			
			return nil
		})
	})
	
	return accounts, err
}

// GetEmailAccountByID 根据ID获取邮箱账户
func GetEmailAccountByID(id uint) (*models.EmailAccount, error) {
	var account models.EmailAccount
	
	err := DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailAccountsBucket))
		key := fmt.Sprintf("%d", id)
		v := bucket.Get([]byte(key))
		if v == nil {
			return fmt.Errorf("邮箱账户不存在")
		}
		
		return json.Unmarshal(v, &account)
	})
	
	if err != nil {
		return nil, err
	}
	
	return &account, nil
}

// UpdateEmailAccount 更新邮箱账户
func UpdateEmailAccount(account *models.EmailAccount) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailAccountsBucket))
		
		account.UpdatedAt = time.Now()
		data, err := json.Marshal(account)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%d", account.ID)
		return bucket.Put([]byte(key), data)
	})
}

// DeleteEmailAccount 删除邮箱账户（软删除）
func DeleteEmailAccount(id uint) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailAccountsBucket))
		key := fmt.Sprintf("%d", id)
		v := bucket.Get([]byte(key))
		if v == nil {
			return fmt.Errorf("邮箱账户不存在")
		}
		
		var account models.EmailAccount
		if err := json.Unmarshal(v, &account); err != nil {
			return err
		}
		
		account.IsActive = false
		account.UpdatedAt = time.Now()
		
		data, err := json.Marshal(account)
		if err != nil {
			return err
		}
		
		return bucket.Put([]byte(key), data)
	})
}

// CreateDownloadTask 创建下载任务
func CreateDownloadTask(task *models.DownloadTask) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(DownloadTasksBucket))
		
		// 生成ID
		id, _ := bucket.NextSequence()
		task.ID = uint(id)
		task.CreatedAt = time.Now()
		task.UpdatedAt = time.Now()
		
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%d", task.ID)
		return bucket.Put([]byte(key), data)
	})
}

// GetDownloadTasksResponse 下载任务响应结构
type GetDownloadTasksResponse struct {
	Tasks []models.DownloadTask `json:"tasks"`
	Total int64                 `json:"total"`
}

// GetDownloadTasks 获取下载任务列表
func GetDownloadTasks(limit, offset int) ([]models.DownloadTask, int64, error) {
	var tasks []models.DownloadTask
	var total int64
	
	err := DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(DownloadTasksBucket))
		accountBucket := tx.Bucket([]byte(EmailAccountsBucket))
		
		// 统计总数
		bucket.ForEach(func(k, v []byte) error {
			total++
			return nil
		})
		
		// 获取分页数据
		count := 0
		return bucket.ForEach(func(k, v []byte) error {
			if count < offset {
				count++
				return nil
			}
			
			if len(tasks) >= limit {
				return nil
			}
			
			var task models.DownloadTask
			if err := json.Unmarshal(v, &task); err != nil {
				return err
			}
			
			// 加载关联的邮箱账户信息
			if task.EmailID > 0 {
				accountKey := fmt.Sprintf("%d", task.EmailID)
				if accountData := accountBucket.Get([]byte(accountKey)); accountData != nil {
					var account models.EmailAccount
					if err := json.Unmarshal(accountData, &account); err == nil {
						task.EmailAccount = account
					}
				}
			}
			
			tasks = append(tasks, task)
			count++
			return nil
		})
	})
	
	return tasks, total, err
}

// GetDownloadTasksByStatus 根据状态获取下载任务
func GetDownloadTasksByStatus(status models.DownloadStatus) ([]models.DownloadTask, error) {
	var tasks []models.DownloadTask
	
	err := DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(DownloadTasksBucket))
		accountBucket := tx.Bucket([]byte(EmailAccountsBucket))
		
		return bucket.ForEach(func(k, v []byte) error {
			var task models.DownloadTask
			if err := json.Unmarshal(v, &task); err != nil {
				return err
			}
			
			if task.Status == status {
				// 加载关联的邮箱账户信息
				if task.EmailID > 0 {
					accountKey := fmt.Sprintf("%d", task.EmailID)
					if accountData := accountBucket.Get([]byte(accountKey)); accountData != nil {
						var account models.EmailAccount
						if err := json.Unmarshal(accountData, &account); err == nil {
							task.EmailAccount = account
						}
					}
				}
				
				tasks = append(tasks, task)
			}
			
			return nil
		})
	})
	
	return tasks, err
}

// UpdateDownloadTask 更新下载任务
func UpdateDownloadTask(task *models.DownloadTask) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(DownloadTasksBucket))
		
		task.UpdatedAt = time.Now()
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%d", task.ID)
		return bucket.Put([]byte(key), data)
	})
}

// UpdateDownloadProgress 更新下载进度
func UpdateDownloadProgress(taskID uint, downloadedSize int64, progress float64, speed string) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(DownloadTasksBucket))
		key := fmt.Sprintf("%d", taskID)
		v := bucket.Get([]byte(key))
		if v == nil {
			return fmt.Errorf("下载任务不存在")
		}
		
		var task models.DownloadTask
		if err := json.Unmarshal(v, &task); err != nil {
			return err
		}
		
		task.DownloadedSize = downloadedSize
		task.Progress = progress
		task.Speed = speed
		task.UpdatedAt = time.Now()
		
		data, err := json.Marshal(task)
		if err != nil {
			return err
		}
		
		return bucket.Put([]byte(key), data)
	})
}

// CreateEmailMessage 创建邮件记录
func CreateEmailMessage(message *models.EmailMessage) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailMessagesBucket))
		
		// 生成ID
		id, _ := bucket.NextSequence()
		message.ID = uint(id)
		message.CreatedAt = time.Now()
		message.UpdatedAt = time.Now()
		
		data, err := json.Marshal(message)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%d", message.ID)
		return bucket.Put([]byte(key), data)
	})
}

// GetEmailMessageByMessageID 根据MessageID获取邮件记录
func GetEmailMessageByMessageID(messageID string) (*models.EmailMessage, error) {
	var message models.EmailMessage
	found := false
	
	err := DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailMessagesBucket))
		
		return bucket.ForEach(func(k, v []byte) error {
			var msg models.EmailMessage
			if err := json.Unmarshal(v, &msg); err != nil {
				return err
			}
			
			if msg.MessageID == messageID {
				message = msg
				found = true
				return fmt.Errorf("found") // 用于跳出循环
			}
			
			return nil
		})
	})
	
	if err != nil && err.Error() != "found" {
		return nil, err
	}
	
	if !found {
		return nil, fmt.Errorf("邮件记录不存在")
	}
	
	return &message, nil
}

// UpdateEmailMessage 更新邮件记录
func UpdateEmailMessage(message *models.EmailMessage) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(EmailMessagesBucket))
		
		message.UpdatedAt = time.Now()
		data, err := json.Marshal(message)
		if err != nil {
			return err
		}
		
		key := fmt.Sprintf("%d", message.ID)
		return bucket.Put([]byte(key), data)
	})
}

// CreateOrUpdateStatistics 创建或更新统计数据
func CreateOrUpdateStatistics(date time.Time, totalDownloads, successDownloads, failedDownloads int, totalSize int64) error {
	return DB.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(StatisticsBucket))
		
		dateKey := date.Format("2006-01-02")
		
		var stats models.DownloadStatistics
		if v := bucket.Get([]byte(dateKey)); v != nil {
			if err := json.Unmarshal(v, &stats); err != nil {
				return err
			}
		} else {
			// 创建新的统计记录
			id, _ := bucket.NextSequence()
			stats.ID = uint(id)
			stats.Date = date
			stats.CreatedAt = time.Now()
		}
		
		stats.TotalDownloads = totalDownloads
		stats.SuccessDownloads = successDownloads
		stats.FailedDownloads = failedDownloads
		stats.TotalSize = totalSize
		stats.UpdatedAt = time.Now()
		
		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		
		return bucket.Put([]byte(dateKey), data)
	})
}

// GetStatistics 获取统计数据
func GetStatistics(days int) ([]models.DownloadStatistics, error) {
	var statistics []models.DownloadStatistics
	
	err := DB.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(StatisticsBucket))
		
		return bucket.ForEach(func(k, v []byte) error {
			var stats models.DownloadStatistics
			if err := json.Unmarshal(v, &stats); err != nil {
				return err
			}
			
			// 过滤指定天数内的数据
			if days > 0 {
				cutoff := time.Now().AddDate(0, 0, -days)
				if stats.Date.Before(cutoff) {
					return nil
				}
			}
			
			statistics = append(statistics, stats)
			return nil
		})
	})
	
	return statistics, err
}

// CleanOldData 清理旧数据
func CleanOldData(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	
	return DB.Update(func(tx *bbolt.Tx) error {
		// 清理旧的下载任务
		taskBucket := tx.Bucket([]byte(DownloadTasksBucket))
		var keysToDelete [][]byte
		
		taskBucket.ForEach(func(k, v []byte) error {
			var task models.DownloadTask
			if err := json.Unmarshal(v, &task); err != nil {
				return err
			}
			
			if task.CreatedAt.Before(cutoff) {
				keysToDelete = append(keysToDelete, k)
			}
			
			return nil
		})
		
		for _, key := range keysToDelete {
			taskBucket.Delete(key)
		}
		
		// 清理旧的邮件记录
		messageBucket := tx.Bucket([]byte(EmailMessagesBucket))
		keysToDelete = nil
		
		messageBucket.ForEach(func(k, v []byte) error {
			var message models.EmailMessage
			if err := json.Unmarshal(v, &message); err != nil {
				return err
			}
			
			if message.CreatedAt.Before(cutoff) {
				keysToDelete = append(keysToDelete, k)
			}
			
			return nil
		})
		
		for _, key := range keysToDelete {
			messageBucket.Delete(key)
		}
		
		// 清理旧的统计数据
		statsBucket := tx.Bucket([]byte(StatisticsBucket))
		keysToDelete = nil
		
		statsBucket.ForEach(func(k, v []byte) error {
			var stats models.DownloadStatistics
			if err := json.Unmarshal(v, &stats); err != nil {
				return err
			}
			
			if stats.Date.Before(cutoff) {
				keysToDelete = append(keysToDelete, k)
			}
			
			return nil
		})
		
		for _, key := range keysToDelete {
			statsBucket.Delete(key)
		}
		
		return nil
	})
}

// CloseDatabase 关闭数据库连接
func CloseDatabase() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
} 