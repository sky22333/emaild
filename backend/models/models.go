package models

import (
	"time"
)

// EmailAccount 邮箱账户配置
type EmailAccount struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`        // 账户名称（用户自定义）
	Email       string `json:"email"`       // 邮箱地址
	Password    string `json:"password"`    // 邮箱密码或授权码
	IMAPServer  string `json:"imap_server"` // IMAP服务器地址
	IMAPPort    int    `json:"imap_port"`   // IMAP端口
	UseSSL      bool   `json:"use_ssl"`     // 是否使用SSL
	IsActive    bool   `json:"is_active"`   // 是否启用
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// DownloadTask 下载任务
type DownloadTask struct {
	ID             uint          `json:"id"`
	EmailID        uint          `json:"email_id"`        // 关联的邮箱ID
	EmailAccount   EmailAccount  `json:"email_account"`   // 关联的邮箱账户
	Subject        string        `json:"subject"`         // 邮件主题
	Sender         string        `json:"sender"`          // 发件人
	FileName       string        `json:"file_name"`       // 文件名
	FileSize       int64         `json:"file_size"`       // 文件大小（字节）
	DownloadedSize int64         `json:"downloaded_size"` // 已下载大小
	Status         DownloadStatus `json:"status"`         // 下载状态
	Type           DownloadType  `json:"type"`            // 下载类型（附件/链接）
	Source         string        `json:"source"`          // 源（附件名称或URL）
	LocalPath      string        `json:"local_path"`      // 本地保存路径
	Error          string        `json:"error"`           // 错误信息
	Progress       float64       `json:"progress"`        // 下载进度（0-100）
	Speed          string        `json:"speed"`           // 下载速度
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
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
	ID           uint         `json:"id"`
	EmailID      uint         `json:"email_id"`      // 关联的邮箱ID
	EmailAccount EmailAccount `json:"email_account"` // 关联的邮箱账户
	MessageID    string       `json:"message_id"`    // 邮件唯一ID
	Subject      string       `json:"subject"`       // 主题
	Sender       string       `json:"sender"`        // 发件人
	Recipients   string       `json:"recipients"`    // 收件人
	Date         string       `json:"date"`          // 邮件日期
	HasPDF       bool         `json:"has_pdf"`       // 是否包含PDF
	IsProcessed  bool         `json:"is_processed"`  // 是否已处理
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
}

// AppConfig 应用配置
type AppConfig struct {
	ID                 uint   `json:"id"`
	DownloadPath       string `json:"download_path"`       // 默认下载路径
	MaxConcurrent      int    `json:"max_concurrent"`      // 最大并发下载数
	CheckInterval      int    `json:"check_interval"`      // 检查邮件间隔（秒）
	AutoCheck          bool   `json:"auto_check"`          // 自动检查邮件
	MinimizeToTray     bool   `json:"minimize_to_tray"`    // 最小化到托盘
	StartMinimized     bool   `json:"start_minimized"`     // 启动时最小化
	EnableNotification bool   `json:"enable_notification"` // 启用通知
	Theme              string `json:"theme"`               // 主题（light/dark/auto）
	Language           string `json:"language"`            // 语言
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// DownloadStatistics 下载统计
type DownloadStatistics struct {
	ID               uint   `json:"id"`
	Date             string `json:"date"`             // 统计日期
	TotalDownloads   int    `json:"total_downloads"`  // 总下载数
	SuccessDownloads int    `json:"success_downloads"` // 成功下载数
	FailedDownloads  int    `json:"failed_downloads"` // 失败下载数
	TotalSize        int64  `json:"total_size"`       // 总下载大小
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// 辅助函数：time.Time 到 string 的转换
func TimeToString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// EmailCheckResult 邮件检查结果
type EmailCheckResult struct {
	Account   *EmailAccount `json:"account"`
	NewEmails int           `json:"new_emails"`
	PDFsFound int           `json:"pdfs_found"`
	Error     string        `json:"error,omitempty"`
	Success   bool          `json:"success"`
}

// 辅助函数：string 到 time.Time 的转换
func StringToTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
} 