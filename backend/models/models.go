package models

import (
	"time"
)

// EmailAccount 邮箱账户配置
type EmailAccount struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`        // 账户名称（用户自定义）
	Email       string    `json:"email" gorm:"not null;unique"` // 邮箱地址
	Password    string    `json:"password" gorm:"not null"`     // 邮箱密码或授权码
	IMAPServer  string    `json:"imap_server" gorm:"not null"`  // IMAP服务器地址
	IMAPPort    int       `json:"imap_port" gorm:"default:993"` // IMAP端口
	UseSSL      bool      `json:"use_ssl" gorm:"default:true"`  // 是否使用SSL
	IsActive    bool      `json:"is_active" gorm:"default:true"` // 是否启用
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DownloadTask 下载任务
type DownloadTask struct {
	ID            uint                `json:"id" gorm:"primaryKey"`
	EmailID       uint                `json:"email_id"`                                // 关联的邮箱ID
	EmailAccount  EmailAccount        `json:"email_account" gorm:"foreignKey:EmailID"` // 关联的邮箱账户
	Subject       string              `json:"subject"`                                 // 邮件主题
	Sender        string              `json:"sender"`                                  // 发件人
	FileName      string              `json:"file_name"`                               // 文件名
	FileSize      int64               `json:"file_size"`                               // 文件大小（字节）
	DownloadedSize int64              `json:"downloaded_size"`                         // 已下载大小
	Status        DownloadStatus      `json:"status" gorm:"default:pending"`           // 下载状态
	Type          DownloadType        `json:"type"`                                    // 下载类型（附件/链接）
	Source        string              `json:"source"`                                  // 源（附件名称或URL）
	LocalPath     string              `json:"local_path"`                              // 本地保存路径
	Error         string              `json:"error"`                                   // 错误信息
	Progress      float64             `json:"progress"`                                // 下载进度（0-100）
	Speed         string              `json:"speed"`                                   // 下载速度
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// DownloadStatus 下载状态枚举
type DownloadStatus string

const (
	StatusPending     DownloadStatus = "pending"     // 等待中
	StatusDownloading DownloadStatus = "downloading" // 下载中
	StatusCompleted   DownloadStatus = "completed"   // 已完成
	StatusFailed      DownloadStatus = "failed"      // 失败
	StatusPaused      DownloadStatus = "paused"      // 暂停
	StatusCancelled   DownloadStatus = "cancelled"   // 已取消
)

// DownloadType 下载类型枚举
type DownloadType string

const (
	TypeAttachment DownloadType = "attachment" // 附件
	TypeLink       DownloadType = "link"       // 链接
)

// EmailMessage 邮件信息
type EmailMessage struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	EmailID     uint      `json:"email_id"`                                // 关联的邮箱ID
	EmailAccount EmailAccount `json:"email_account" gorm:"foreignKey:EmailID"` // 关联的邮箱账户
	MessageID   string    `json:"message_id" gorm:"uniqueIndex"`           // 邮件唯一ID
	Subject     string    `json:"subject"`                                 // 主题
	Sender      string    `json:"sender"`                                  // 发件人
	Recipients  string    `json:"recipients"`                              // 收件人
	Date        time.Time `json:"date"`                                    // 邮件日期
	HasPDF      bool      `json:"has_pdf" gorm:"default:false"`            // 是否包含PDF
	IsProcessed bool      `json:"is_processed" gorm:"default:false"`       // 是否已处理
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AppConfig 应用配置
type AppConfig struct {
	ID                uint   `json:"id" gorm:"primaryKey"`
	DownloadPath      string `json:"download_path" gorm:"default:./downloads"` // 默认下载路径
	MaxConcurrent     int    `json:"max_concurrent" gorm:"default:3"`           // 最大并发下载数
	CheckInterval     int    `json:"check_interval" gorm:"default:300"`         // 检查邮件间隔（秒）
	AutoCheck         bool   `json:"auto_check" gorm:"default:false"`           // 自动检查邮件
	MinimizeToTray    bool   `json:"minimize_to_tray" gorm:"default:true"`      // 最小化到托盘
	StartMinimized    bool   `json:"start_minimized" gorm:"default:false"`      // 启动时最小化
	EnableNotification bool  `json:"enable_notification" gorm:"default:true"`   // 启用通知
	Theme             string `json:"theme" gorm:"default:auto"`                 // 主题（light/dark/auto）
	Language          string `json:"language" gorm:"default:zh-CN"`             // 语言
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// DownloadStatistics 下载统计
type DownloadStatistics struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	Date             time.Time `json:"date" gorm:"uniqueIndex"`               // 统计日期
	TotalDownloads   int       `json:"total_downloads" gorm:"default:0"`      // 总下载数
	SuccessDownloads int       `json:"success_downloads" gorm:"default:0"`    // 成功下载数
	FailedDownloads  int       `json:"failed_downloads" gorm:"default:0"`     // 失败下载数
	TotalSize        int64     `json:"total_size" gorm:"default:0"`           // 总下载大小
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName 方法用于指定表名
func (EmailAccount) TableName() string {
	return "email_accounts"
}

func (DownloadTask) TableName() string {
	return "download_tasks"
}

func (EmailMessage) TableName() string {
	return "email_messages"
}

func (AppConfig) TableName() string {
	return "app_configs"
}

func (DownloadStatistics) TableName() string {
	return "download_statistics"
} 