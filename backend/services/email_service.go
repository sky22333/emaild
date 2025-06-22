package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"emaild/backend/database"
	"emaild/backend/models"
	"emaild/backend/utils"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/sirupsen/logrus"
)

// EmailService 邮件服务结构体
type EmailService struct {
	db               *database.Database
	connections      map[uint]*IMAPConnection    // 按邮箱ID管理连接
	connectionsMutex sync.RWMutex               // 保护连接映射的读写锁
	downloadService  *DownloadService           // 下载服务
	ctx              context.Context            // 服务上下文
	cancel           context.CancelFunc         // 取消函数
	checkInterval    time.Duration              // 检查间隔
	isRunning        bool                       // 是否正在运行
	runningMutex     sync.RWMutex               // 保护运行状态的锁
	logger           *logrus.Logger
}

// IMAPConnection IMAP连接管理
type IMAPConnection struct {
	ID          uint
	Account     *models.EmailAccount
	Client      *client.Client
	LastUsed    time.Time
	IsConnected bool
	Mutex       sync.Mutex // 连接级别的锁
	ctx         context.Context
	cancel      context.CancelFunc
}

// EmailCheckResult 邮件检查结果
type EmailCheckResult struct {
	Account     *models.EmailAccount `json:"account"`
	NewEmails   int                  `json:"new_emails"`
	PDFsFound   int                  `json:"pdfs_found"`
	Error       string               `json:"error,omitempty"`
	Success     bool                 `json:"success"`
}

// NewEmailService 创建新的邮件服务实例
func NewEmailService(db *database.Database, downloadService *DownloadService, logger *logrus.Logger) *EmailService {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &EmailService{
		db:               db,
		connections:      make(map[uint]*IMAPConnection),
		downloadService:  downloadService,
		ctx:              ctx,
		cancel:           cancel,
		checkInterval:    5 * time.Minute, // 默认5分钟检查一次
		isRunning:        false,
		logger:           logger,
	}
}

// StartEmailMonitoring 启动邮件监控
func (es *EmailService) StartEmailMonitoring() error {
	es.runningMutex.Lock()
	defer es.runningMutex.Unlock()
	
	if es.isRunning {
		return fmt.Errorf("邮件监控已经在运行中")
	}
	
	es.isRunning = true
	es.logger.Info("启动邮件监控服务")
	
	// 启动邮件检查器
	go es.emailChecker()
	
	// 启动连接清理器
	go es.connectionCleaner()
	
	return nil
}

// StopEmailMonitoring 停止邮件监控
func (es *EmailService) StopEmailMonitoring() {
	es.runningMutex.Lock()
	defer es.runningMutex.Unlock()
	
	if !es.isRunning {
		return
	}
	
	es.isRunning = false
	es.cancel()
	
	// 关闭所有连接
	es.connectionsMutex.Lock()
	for _, conn := range es.connections {
		conn.close()
	}
	es.connections = make(map[uint]*IMAPConnection)
	es.connectionsMutex.Unlock()
}

// emailChecker 邮件检查器
func (es *EmailService) emailChecker() {
	ticker := time.NewTicker(es.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-es.ctx.Done():
			return
		case <-ticker.C:
			es.checkAllAccounts()
		}
	}
}

// connectionCleaner 连接清理器，清理长时间未使用的连接
func (es *EmailService) connectionCleaner() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-es.ctx.Done():
			return
		case <-ticker.C:
			es.cleanupIdleConnections()
		}
	}
}

// cleanupIdleConnections 清理空闲连接
func (es *EmailService) cleanupIdleConnections() {
	es.connectionsMutex.Lock()
	defer es.connectionsMutex.Unlock()
	
	cutoff := time.Now().Add(-30 * time.Minute) // 30分钟未使用则清理
	
	for accountID, conn := range es.connections {
		if conn.LastUsed.Before(cutoff) {
			conn.close()
			delete(es.connections, accountID)
		}
	}
}

// checkAllAccounts 检查所有邮箱账户
func (es *EmailService) checkAllAccounts() {
	accounts, err := es.getActiveAccounts()
	if err != nil {
		return
	}
	
	for _, account := range accounts {
		go es.checkAccount(&account)
	}
}

// getActiveAccounts 获取活跃的邮箱账户
func (es *EmailService) getActiveAccounts() ([]models.EmailAccount, error) {
	query := `SELECT id, name, email, password, imap_server, imap_port, use_ssl, is_active, created_at, updated_at 
			  FROM email_accounts WHERE is_active = 1`
	
	rows, err := es.db.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var accounts []models.EmailAccount
	for rows.Next() {
		var account models.EmailAccount
		err := rows.Scan(
			&account.ID, &account.Name, &account.Email, &account.Password,
			&account.IMAPServer, &account.IMAPPort, &account.UseSSL, &account.IsActive,
			&account.CreatedAt, &account.UpdatedAt,
		)
		if err != nil {
			continue
		}
		accounts = append(accounts, account)
	}
	
	return accounts, nil
}

// checkAccount 检查指定邮箱账户
func (es *EmailService) checkAccount(account *models.EmailAccount) {
	conn, err := es.getConnection(account.ID)
	if err != nil {
		return
	}
	defer es.releaseConnection(account.ID)
	
	// 选择收件箱
	if err := conn.selectInbox(); err != nil {
		return
	}
	
	// 搜索未读邮件
	messages, err := conn.searchUnreadMessages()
	if err != nil {
		return
	}
	
	// 处理每封邮件
	for _, msg := range messages {
		es.processMessage(account, msg)
	}
}

// getConnection 获取连接（支持连接复用和重连）
func (es *EmailService) getConnection(accountID uint) (*IMAPConnection, error) {
	es.connectionsMutex.Lock()
	defer es.connectionsMutex.Unlock()
	
	// 检查是否已有连接
	if conn, exists := es.connections[accountID]; exists {
		conn.Mutex.Lock()
		defer conn.Mutex.Unlock()
		
		// 检查连接是否仍然有效
		if conn.IsConnected && conn.isAlive() {
			conn.LastUsed = time.Now()
			return conn, nil
		}
		
		// 连接失效，关闭并重新创建
		conn.close()
		delete(es.connections, accountID)
	}
	
	// 创建新连接
	account, err := es.getAccountByID(accountID)
	if err != nil {
		return nil, err
	}
	
	conn, err := es.createConnection(account)
	if err != nil {
		return nil, err
	}
	
	es.connections[accountID] = conn
	return conn, nil
}

// releaseConnection 释放连接（不实际关闭，只是标记为可用）
func (es *EmailService) releaseConnection(accountID uint) {
	// 连接复用，不在这里关闭连接
	// 连接将由连接清理器定期清理
}

// getAccountByID 根据ID获取邮箱账户
func (es *EmailService) getAccountByID(accountID uint) (*models.EmailAccount, error) {
	return es.db.GetEmailAccountByID(accountID)
}

// createConnection 创建IMAP连接
func (es *EmailService) createConnection(account *models.EmailAccount) (*IMAPConnection, error) {
	// 连接到IMAP服务器
	var c *client.Client
	var err error
	
	if account.UseSSL {
		// SSL连接
		c, err = client.DialTLS(fmt.Sprintf("%s:%d", account.IMAPServer, account.IMAPPort), &tls.Config{
			InsecureSkipVerify: false,
		})
	} else {
		// 普通连接
		c, err = client.Dial(fmt.Sprintf("%s:%d", account.IMAPServer, account.IMAPPort))
	}
	
	if err != nil {
		return nil, fmt.Errorf("连接IMAP服务器失败: %v", err)
	}
	
	// 登录
	if err := c.Login(account.Email, account.Password); err != nil {
		c.Close()
		return nil, fmt.Errorf("IMAP登录失败: %v", err)
	}
	
	ctx, cancel := context.WithCancel(es.ctx)
	
	conn := &IMAPConnection{
		ID:          account.ID,
		Account:     account,
		Client:      c,
		LastUsed:    time.Now(),
		IsConnected: true,
		ctx:         ctx,
		cancel:      cancel,
	}
	
	return conn, nil
}

// IMAP连接方法
func (conn *IMAPConnection) selectInbox() error {
	conn.Mutex.Lock()
	defer conn.Mutex.Unlock()
	
	if !conn.IsConnected {
		return fmt.Errorf("连接已断开")
	}
	
	_, err := conn.Client.Select("INBOX", false)
	return err
}

func (conn *IMAPConnection) searchUnreadMessages() ([]*imap.Message, error) {
	conn.Mutex.Lock()
	defer conn.Mutex.Unlock()
	
	if !conn.IsConnected {
		return nil, fmt.Errorf("连接已断开")
	}
	
	// 搜索未读邮件
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{"\\Seen"}
	
	uids, err := conn.Client.Search(criteria)
	if err != nil {
		return nil, err
	}
	
	if len(uids) == 0 {
		return nil, nil
	}
	
	// 获取邮件详情
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)
	
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	
	go func() {
		done <- conn.Client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchBodyStructure}, messages)
	}()
	
	var msgs []*imap.Message
	for msg := range messages {
		msgs = append(msgs, msg)
	}
	
	if err := <-done; err != nil {
		return nil, err
	}
	
	return msgs, nil
}

func (conn *IMAPConnection) isAlive() bool {
	if !conn.IsConnected {
		return false
	}
	
	// 发送NOOP命令检测连接是否存活
	err := conn.Client.Noop()
	return err == nil
}

func (conn *IMAPConnection) close() {
	conn.Mutex.Lock()
	defer conn.Mutex.Unlock()
	
	if conn.IsConnected {
		conn.Client.Close()
		conn.IsConnected = false
	}
	
	if conn.cancel != nil {
		conn.cancel()
	}
}

// processMessage 处理邮件消息
func (es *EmailService) processMessage(account *models.EmailAccount, msg *imap.Message) {
	// 检查是否已处理过
	messageID := ""
	if msg.Envelope != nil && len(msg.Envelope.MessageId) > 0 {
		messageID = msg.Envelope.MessageId
		if es.isMessageProcessed(messageID) {
			return
		}
	}
	
	now := time.Now()
	// 保存邮件记录
	emailMsg := &models.EmailMessage{
		EmailID:     account.ID,
		MessageID:   messageID,
		Subject:     "",
		Sender:      "",
		Recipients:  "",
		Date:        models.TimeToString(now),
		HasPDF:      false,
		IsProcessed: false,
		CreatedAt:   models.TimeToString(now),
		UpdatedAt:   models.TimeToString(now),
	}
	
	if msg.Envelope != nil {
		emailMsg.Subject = msg.Envelope.Subject
		if len(msg.Envelope.From) > 0 {
			emailMsg.Sender = msg.Envelope.From[0].Address()
		}
		if len(msg.Envelope.To) > 0 {
			var recipients []string
			for _, to := range msg.Envelope.To {
				recipients = append(recipients, to.Address())
			}
			emailMsg.Recipients = strings.Join(recipients, ";")
		}
		if !msg.Envelope.Date.IsZero() {
			emailMsg.Date = models.TimeToString(msg.Envelope.Date)
		}
	}
	
	// 分析邮件内容，查找PDF附件和链接
	pdfSources := es.analyzePDFSources(account, msg)
	if len(pdfSources) > 0 {
		emailMsg.HasPDF = true
	}
	
	// 保存邮件记录
	if err := es.saveEmailMessage(emailMsg); err != nil {
		return
	}
	
	// 创建下载任务
	for _, source := range pdfSources {
		now := time.Now()
		task := &models.DownloadTask{
			EmailID:        account.ID,
			Subject:        emailMsg.Subject,
			Sender:         emailMsg.Sender,
			FileName:       source.FileName,
			FileSize:       source.FileSize,
			DownloadedSize: 0,
			Status:         models.StatusPending,
			Type:           source.Type,
			Source:         source.Source,
			LocalPath:      source.LocalPath,
			Progress:       0,
			Speed:          "",
			CreatedAt:      models.TimeToString(now),
			UpdatedAt:      models.TimeToString(now),
		}
		
		if err := es.createDownloadTask(task); err != nil {
			continue
		}
		
		// 启动下载
		es.downloadService.StartDownload(task.ID)
	}
	
	// 标记邮件为已处理
	emailMsg.IsProcessed = true
	es.updateEmailMessage(emailMsg)
}

// PDFSource PDF源信息
type PDFSource struct {
	Type      models.DownloadType
	Source    string // 附件名称或URL
	FileName  string
	FileSize  int64
	LocalPath string
}

// analyzePDFSources 分析PDF源（附件和链接）
func (es *EmailService) analyzePDFSources(account *models.EmailAccount, msg *imap.Message) []PDFSource {
	var sources []PDFSource
	
	// 获取下载路径配置
	config, err := es.getDownloadConfig()
	if err != nil {
		return sources
	}
	
	// 分析PDF附件
	if msg.BodyStructure != nil {
		attachments := es.findPDFAttachments(msg.BodyStructure)
		for _, att := range attachments {
			fileName := utils.CleanFilename(att.FileName)
			localPath := filepath.Join(config.DownloadPath, fileName)
			
			sources = append(sources, PDFSource{
				Type:      models.TypeAttachment,
				Source:    att.FileName, // 附件名称
				FileName:  fileName,
				FileSize:  att.Size,
				LocalPath: localPath,
			})
		}
	}
	
	// 分析邮件内容中的PDF链接
	if msg.Envelope != nil {
		pdfLinks := es.extractPDFLinks(msg.Envelope.Subject)
		for _, link := range pdfLinks {
			fileName := utils.ExtractFilenameFromURL(link)
			if fileName == "" {
				fileName = fmt.Sprintf("pdf_%d.pdf", time.Now().Unix())
			}
			fileName = utils.CleanFilename(fileName)
			localPath := filepath.Join(config.DownloadPath, fileName)
			
			sources = append(sources, PDFSource{
				Type:      models.TypeLink,
				Source:    link,
				FileName:  fileName,
				FileSize:  0, // 链接大小未知
				LocalPath: localPath,
			})
		}
	}
	
	return sources
}

// AttachmentInfo 附件信息
type AttachmentInfo struct {
	FileName string
	Size     int64
}

// findPDFAttachments 查找PDF附件
func (es *EmailService) findPDFAttachments(bodyStructure *imap.BodyStructure) []AttachmentInfo {
	var attachments []AttachmentInfo
	
	var searchParts func(*imap.BodyStructure)
	searchParts = func(bs *imap.BodyStructure) {
		// 检查当前部分是否为PDF附件
		if strings.ToLower(bs.MIMEType) == "application" && 
		   strings.ToLower(bs.MIMESubType) == "pdf" {
			
			// 获取文件名
			fileName := ""
			// 修复：检查Disposition是否为attachment类型
			if bs.Disposition == "attachment" && bs.Params != nil {
				if name, exists := bs.Params["filename"]; exists {
					fileName = name
				}
			}
			if fileName == "" && bs.Params != nil {
				if name, exists := bs.Params["name"]; exists {
					fileName = name
				}
			}
			
			if fileName != "" {
				attachments = append(attachments, AttachmentInfo{
					FileName: fileName,
					Size:     int64(bs.Size),
				})
			}
		}
		
		// 递归搜索子部分
		for _, part := range bs.Parts {
			searchParts(part)
		}
	}
	
	searchParts(bodyStructure)
	return attachments
}

// extractPDFLinks 从文本中提取PDF链接
func (es *EmailService) extractPDFLinks(text string) []string {
	// 匹配PDF链接的正则表达式
	pdfRegex := regexp.MustCompile(`https?://[^\s]+\.pdf(?:\?[^\s]*)?`)
	matches := pdfRegex.FindAllString(text, -1)
	
	var validLinks []string
	for _, match := range matches {
		// 验证URL格式
		if _, err := url.Parse(match); err == nil {
			validLinks = append(validLinks, match)
		}
	}
	
	return validLinks
}

// isMessageProcessed 检查消息是否已处理
func (es *EmailService) isMessageProcessed(messageID string) bool {
	_, err := es.db.GetEmailMessageByMessageID(messageID)
	return err == nil
}

// saveEmailMessage 保存邮件消息
func (es *EmailService) saveEmailMessage(msg *models.EmailMessage) error {
	return es.db.CreateEmailMessage(msg)
}

// updateEmailMessage 更新邮件消息
func (es *EmailService) updateEmailMessage(msg *models.EmailMessage) error {
	return es.db.UpdateEmailMessage(msg)
}

// createDownloadTask 创建下载任务
func (es *EmailService) createDownloadTask(task *models.DownloadTask) error {
	return es.db.CreateDownloadTask(task)
}

func (es *EmailService) getDownloadConfig() (*models.AppConfig, error) {
	query := `SELECT id, download_path, max_concurrent, check_interval, auto_check, minimize_to_tray, start_minimized, enable_notification, theme, language, created_at, updated_at FROM app_configs LIMIT 1`
	
	row := es.db.DB.QueryRow(query)
	
	var config models.AppConfig
	err := row.Scan(
		&config.ID, &config.DownloadPath, &config.MaxConcurrent, &config.CheckInterval,
		&config.AutoCheck, &config.MinimizeToTray, &config.StartMinimized,
		&config.EnableNotification, &config.Theme, &config.Language,
		&config.CreatedAt, &config.UpdatedAt,
	)
	
	if err != nil {
		// 返回默认配置
		homeDir, _ := os.UserHomeDir()
		return &models.AppConfig{
			DownloadPath:  filepath.Join(homeDir, "Downloads", "EmailPDFs"),
			MaxConcurrent: 3,
		}, nil
	}
	
	return &config, nil
}

// SetCheckInterval 设置检查间隔
func (es *EmailService) SetCheckInterval(interval time.Duration) {
	es.checkInterval = interval
}

// CheckAccountNow 立即检查指定账户
func (es *EmailService) CheckAccountNow(accountID uint) error {
	account, err := es.getAccountByID(accountID)
	if err != nil {
		return err
	}
	
	go es.checkAccount(account)
	return nil
}

// GetEmailMessages 获取邮件消息列表
func (es *EmailService) GetEmailMessages(limit, offset int) ([]models.EmailMessage, error) {
	query := `
		SELECT em.id, em.email_id, em.message_id, em.subject, em.sender, em.recipients, em.date, em.has_pdf, em.is_processed, em.created_at, em.updated_at,
			   ea.id, ea.name, ea.email, ea.password, ea.imap_server, ea.imap_port, ea.use_ssl, ea.is_active, ea.created_at, ea.updated_at
		FROM email_messages em
		LEFT JOIN email_accounts ea ON em.email_id = ea.id
		ORDER BY em.created_at DESC
		LIMIT ? OFFSET ?
	`
	
	rows, err := es.db.DB.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []models.EmailMessage
	for rows.Next() {
		var msg models.EmailMessage
		var account models.EmailAccount
		
		err := rows.Scan(
			&msg.ID, &msg.EmailID, &msg.MessageID, &msg.Subject, &msg.Sender, &msg.Recipients,
			&msg.Date, &msg.HasPDF, &msg.IsProcessed, &msg.CreatedAt, &msg.UpdatedAt,
			&account.ID, &account.Name, &account.Email, &account.Password, &account.IMAPServer,
			&account.IMAPPort, &account.UseSSL, &account.IsActive, &account.CreatedAt, &account.UpdatedAt,
		)
		if err != nil {
			continue
		}
		
		msg.EmailAccount = account
		messages = append(messages, msg)
	}
	
	return messages, nil
}

// TestConnection 测试邮箱连接
func (es *EmailService) TestConnection(account *models.EmailAccount) error {
	conn, err := es.createConnection(account)
	if err != nil {
		return err
	}
	defer conn.close()
	
	// 尝试选择收件箱来验证连接
	err = conn.selectInbox()
	return err
}

// Start 启动邮件服务
func (es *EmailService) Start() error {
	return es.StartEmailMonitoring()
}

// Stop 停止邮件服务
func (es *EmailService) Stop() {
	es.StopEmailMonitoring()
}

// IsRunning 检查邮件服务是否运行中
func (es *EmailService) IsRunning() bool {
	es.runningMutex.RLock()
	defer es.runningMutex.RUnlock()
	return es.isRunning
} 