package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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

// EmailCheckResult 邮件检查结果 - 应该和backend包中的定义保持一致
type EmailCheckResult struct {
	Account   *models.EmailAccount `json:"account"`
	NewEmails int                  `json:"new_emails"`
	PDFsFound int                  `json:"pdfs_found"`
	Error     string               `json:"error,omitempty"`
	Success   bool                 `json:"success"`
}

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
	
	// 优雅关闭相关
	wg              sync.WaitGroup    // 等待所有goroutine完成
	shutdownOnce    sync.Once         // 确保只关闭一次
	isShuttingDown  bool              // 关闭状态标记
	shutdownMutex   sync.RWMutex      // 保护关闭状态的锁
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
	closeOnce   sync.Once  // 确保连接只关闭一次
}

// 使用backend包中的EmailCheckResult定义

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
		isShuttingDown:   false,
	}
}

// SetCheckInterval 设置检查间隔
func (es *EmailService) SetCheckInterval(interval time.Duration) {
	es.runningMutex.Lock()
	defer es.runningMutex.Unlock()
	
	es.checkInterval = interval
	es.logger.Infof("邮件检查间隔已设置为: %v", interval)
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
	es.wg.Add(1)
	go es.emailChecker()
	
	// 启动连接清理器
	es.wg.Add(1)
	go es.connectionCleaner()
	
	return nil
}

// StopEmailMonitoring 停止邮件监控
func (es *EmailService) StopEmailMonitoring() {
	es.shutdownOnce.Do(func() {
		es.logger.Info("开始停止邮件监控服务")
		
		es.runningMutex.Lock()
		if !es.isRunning {
			es.runningMutex.Unlock()
			return
		}
		es.isRunning = false
		es.runningMutex.Unlock()
		
		// 设置关闭状态
		es.shutdownMutex.Lock()
		es.isShuttingDown = true
		es.shutdownMutex.Unlock()
		
		// 取消上下文
		es.cancel()
		
		// 等待所有goroutine完成（带超时）
		done := make(chan struct{})
		go func() {
			es.wg.Wait()
			close(done)
		}()
		
		select {
		case <-done:
			es.logger.Info("所有邮件服务goroutine已正常退出")
		case <-time.After(30 * time.Second):
			es.logger.Warn("等待邮件服务goroutine退出超时，强制退出")
		}
		
		// 关闭所有连接
		es.connectionsMutex.Lock()
		for accountID, conn := range es.connections {
			conn.close()
			delete(es.connections, accountID)
		}
		es.connectionsMutex.Unlock()
		
		es.logger.Info("邮件监控服务已停止")
	})
}

// emailChecker 邮件检查器
func (es *EmailService) emailChecker() {
	defer es.wg.Done()
	
	ticker := time.NewTicker(es.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-es.ctx.Done():
			es.logger.Info("邮件检查器收到关闭信号")
			return
		case <-ticker.C:
			// 检查是否正在关闭
			es.shutdownMutex.RLock()
			if es.isShuttingDown {
				es.shutdownMutex.RUnlock()
				return
			}
			es.shutdownMutex.RUnlock()
			
			es.checkAllAccounts()
		}
	}
}

// connectionCleaner 连接清理器，清理长时间未使用的连接
func (es *EmailService) connectionCleaner() {
	defer es.wg.Done()
	
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-es.ctx.Done():
			es.logger.Info("连接清理器收到关闭信号")
			return
		case <-ticker.C:
			// 检查是否正在关闭
			es.shutdownMutex.RLock()
			if es.isShuttingDown {
				es.shutdownMutex.RUnlock()
				return
			}
			es.shutdownMutex.RUnlock()
			
			es.cleanupIdleConnections()
		}
	}
}

// cleanupIdleConnections 清理空闲连接
func (es *EmailService) cleanupIdleConnections() {
	es.connectionsMutex.Lock()
	defer es.connectionsMutex.Unlock()
	
	cutoff := time.Now().Add(-30 * time.Minute) // 30分钟未使用则清理
	var toDelete []uint
	
	for accountID, conn := range es.connections {
		if conn.LastUsed.Before(cutoff) || !conn.isAlive() {
			conn.close()
			toDelete = append(toDelete, accountID)
		}
	}
	
	// 删除已关闭的连接
	for _, accountID := range toDelete {
		delete(es.connections, accountID)
		es.logger.Debugf("清理了账户 %d 的空闲连接", accountID)
	}
	
	if len(toDelete) > 0 {
		es.logger.Infof("清理了 %d 个空闲连接", len(toDelete))
	}
}

// checkAllAccounts 检查所有邮箱账户
func (es *EmailService) checkAllAccounts() {
	accounts, err := es.getActiveAccounts()
	if err != nil {
		es.logger.Errorf("获取活跃账户失败: %v", err)
		return
	}
	
	es.logger.Debugf("开始检查 %d 个活跃邮箱账户", len(accounts))
	
	// 使用WaitGroup等待所有检查完成
	var checkWg sync.WaitGroup
	for _, account := range accounts {
		// 检查是否正在关闭
		es.shutdownMutex.RLock()
		if es.isShuttingDown {
			es.shutdownMutex.RUnlock()
			break
		}
		es.shutdownMutex.RUnlock()
		
		checkWg.Add(1)
		go func(acc models.EmailAccount) {
			defer checkWg.Done()
			es.checkAccount(&acc)
		}(account)
	}
	
	// 等待所有检查完成或超时
	done := make(chan struct{})
	go func() {
		checkWg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		es.logger.Debug("所有邮箱账户检查完成")
	case <-time.After(5 * time.Minute):
		es.logger.Warn("邮箱账户检查超时")
	case <-es.ctx.Done():
		es.logger.Info("邮箱检查被中断")
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

// CheckAccountWithResult 检查指定账户并返回详细结果
func (es *EmailService) CheckAccountWithResult(account *models.EmailAccount) EmailCheckResult {
	result := EmailCheckResult{
		Account:   account,
		NewEmails: 0,
		PDFsFound: 0,
		Success:   false,
	}

	conn, err := es.getConnection(account.ID)
	if err != nil {
		result.Error = fmt.Sprintf("获取连接失败: %v", err)
		es.logger.Errorf("账户%d连接失败: %v", account.ID, err)
		return result
	}
	defer es.releaseConnection(account.ID)

	// 选择收件箱
	if err := conn.selectInbox(); err != nil {
		result.Error = fmt.Sprintf("选择收件箱失败: %v", err)
		es.logger.Errorf("账户%d选择收件箱失败: %v", account.ID, err)
		return result
	}

	// 搜索未读邮件
	messages, err := conn.searchUnreadMessages()
	if err != nil {
		result.Error = fmt.Sprintf("搜索邮件失败: %v", err)
		es.logger.Errorf("账户%d搜索邮件失败: %v", account.ID, err)
		return result
	}

	result.NewEmails = len(messages)
	es.logger.Infof("账户%d发现%d封未读邮件", account.ID, len(messages))

	// 处理每封邮件并统计PDF数量
	pdfCount := 0
	for _, msg := range messages {
		pdfSources := es.analyzePDFSources(account, msg)
		if len(pdfSources) > 0 {
			pdfCount += len(pdfSources)
			// 处理邮件（保存记录和创建下载任务）
			es.processMessage(account, msg)
		}
	}

	result.PDFsFound = pdfCount
	result.Success = true
	es.logger.Infof("账户%d检查完成: %d封邮件, %d个PDF", account.ID, result.NewEmails, result.PDFsFound)
	
	return result
}

func (es *EmailService) checkAccount(account *models.EmailAccount) {
	// 使用新的CheckAccountWithResult方法
	result := es.CheckAccountWithResult(account)
	if !result.Success {
		es.logger.Errorf("账户%d检查失败: %s", account.ID, result.Error)
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
	return es.createConnectionWithTimeout(es.ctx, account)
}

// createConnectionWithTimeout 创建带超时的IMAP连接
func (es *EmailService) createConnectionWithTimeout(ctx context.Context, account *models.EmailAccount) (*IMAPConnection, error) {
	// 连接到IMAP服务器
	var c *client.Client
	var err error
	
	serverAddr := fmt.Sprintf("%s:%d", account.IMAPServer, account.IMAPPort)
	es.logger.Infof("正在连接到 %s (SSL: %v)", serverAddr, account.UseSSL)
	
	if account.UseSSL {
		// SSL连接 - 添加更灵活的TLS配置
		tlsConfig := &tls.Config{
			ServerName:         account.IMAPServer,
			InsecureSkipVerify: false,
		}
		
		c, err = client.DialTLS(serverAddr, tlsConfig)
		if err != nil {
			// 如果严格验证失败，尝试宽松模式
			es.logger.Warnf("严格SSL验证失败，尝试跳过证书验证: %v", err)
			tlsConfig.InsecureSkipVerify = true
			c, err = client.DialTLS(serverAddr, tlsConfig)
		}
	} else {
		// 普通连接
		c, err = client.Dial(serverAddr)
	}
	
	if err != nil {
		return nil, fmt.Errorf("连接IMAP服务器失败 %s: %v", serverAddr, err)
	}
	
	// 登录
	es.logger.Infof("正在登录账户 %s", account.Email)
	if err := c.Login(account.Email, account.Password); err != nil {
		c.Close()
		return nil, fmt.Errorf("IMAP登录失败 %s: %v", account.Email, err)
	}
	
	connCtx, cancel := context.WithCancel(ctx)
	
	conn := &IMAPConnection{
		ID:          account.ID,
		Account:     account,
		Client:      c,
		LastUsed:    time.Now(),
		IsConnected: true,
		ctx:         connCtx,
		cancel:      cancel,
	}
	
	es.logger.Infof("成功创建连接 %s", account.Email)
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
	
	// 使用统一的搜索策略
	uids, err := conn.searchWithFallback()
	if err != nil {
		return nil, err
	}
	
	if len(uids) == 0 {
		return nil, nil
	}
	
	// 获取邮件详情并过滤未读邮件
	return conn.fetchAndFilterMessages(uids)
}

// searchWithFallback 统一的搜索策略（重用逻辑）
func (conn *IMAPConnection) searchWithFallback() ([]uint32, error) {
	// 策略1: 搜索未读邮件（标准方式）
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{"\\Seen"}
	
	uids, err := conn.Client.Search(criteria)
	if err == nil && len(uids) > 0 {
		return uids, nil
	}
	
	// 策略2: 使用UNSEEN标志
	criteria = imap.NewSearchCriteria()
	criteria.WithFlags = []string{"\\Recent"}
	uids, err = conn.Client.Search(criteria)
	if err == nil && len(uids) > 0 {
		return uids, nil
	}
	
	// 策略3: 搜索最近的邮件（最后的备选方案）
	criteria = imap.NewSearchCriteria()
	since := time.Now().AddDate(0, 0, -7) // 最近7天
	criteria.Since = since
	uids, err = conn.Client.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("所有搜索策略均失败: %v", err)
	}
	
	return uids, nil
}

// fetchAndFilterMessages 获取邮件详情并过滤（重用逻辑）
func (conn *IMAPConnection) fetchAndFilterMessages(uids []uint32) ([]*imap.Message, error) {
	// 限制批量获取的邮件数量，避免超时
	maxMessages := 50
	if len(uids) > maxMessages {
		uids = uids[:maxMessages]
	}
	
	// 获取邮件详情
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)
	
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	
	go func() {
		done <- conn.Client.Fetch(seqset, []imap.FetchItem{
			imap.FetchUid,          // 关键修复：确保获取UID
			imap.FetchEnvelope, 
			imap.FetchBodyStructure,
			imap.FetchFlags,
			"BODY[TEXT]", // 获取邮件正文内容
			"BODY[1]",    // 获取第一个body部分
		}, messages)
	}()
	
	var msgs []*imap.Message
	for msg := range messages {
		// 验证UID是否正确获取
		if msg.Uid == 0 {
			// UID为0说明获取失败，记录警告但继续处理
			continue
		}
		
		// 验证邮件确实是未读的
		if conn.isMessageUnread(msg) {
			msgs = append(msgs, msg)
		}
	}
	
	if err := <-done; err != nil {
		return nil, fmt.Errorf("获取邮件详情失败: %v", err)
	}
	
	return msgs, nil
}

// isMessageUnread 检查邮件是否为未读状态
func (conn *IMAPConnection) isMessageUnread(msg *imap.Message) bool {
	if msg.Flags == nil {
		return true // 如果没有标志信息，假定为未读
	}
	
	for _, flag := range msg.Flags {
		if flag == "\\Seen" {
			return false // 已读
		}
	}
	return true // 未读
}

func (conn *IMAPConnection) isAlive() bool {
	conn.Mutex.Lock()
	defer conn.Mutex.Unlock()
	
	if !conn.IsConnected || conn.Client == nil {
		return false
	}
	
	// 使用带超时的上下文检测连接状态
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("NOOP操作panic: %v", r)
			}
		}()
		done <- conn.Client.Noop()
	}()
	
	select {
	case err := <-done:
		if err != nil {
			conn.IsConnected = false
			return false
		}
		return true
	case <-ctx.Done():
		// 超时认为连接失效
		conn.IsConnected = false
		return false
	}
}

func (conn *IMAPConnection) close() {
	conn.closeOnce.Do(func() {
		conn.Mutex.Lock()
		defer conn.Mutex.Unlock()
		
		if conn.IsConnected && conn.Client != nil {
			// 设置较短的超时来关闭连接
			go func() {
				defer func() {
					if r := recover(); r != nil {
						// 忽略关闭时的panic
					}
				}()
				conn.Client.Close()
			}()
			conn.IsConnected = false
		}
		
		if conn.cancel != nil {
			conn.cancel()
		}
	})
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

// analyzePDFSources 分析PDF源（附件和链接）- 业界最佳实践版本
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
	
	// 分析邮件内容中的PDF链接（完整内容解析）
	pdfLinks := es.extractPDFLinksFromMessage(msg)
	for _, link := range pdfLinks {
		fileName := utils.ExtractFilenameFromURL(link)
		if fileName == "" {
			// 如果无法从URL提取文件名，使用默认命名
			fileName = fmt.Sprintf("download_%d.pdf", time.Now().Unix())
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
	
	return sources
}

// extractPDFLinksFromMessage 从邮件消息中提取PDF链接（完整解析）
func (es *EmailService) extractPDFLinksFromMessage(msg *imap.Message) []string {
	var allLinks []string
	
	// 1. 从主题中提取链接
	if msg.Envelope != nil && msg.Envelope.Subject != "" {
		subjectLinks := es.extractPDFLinks(msg.Envelope.Subject)
		allLinks = append(allLinks, subjectLinks...)
	}
	
	// 2. 从邮件正文中提取链接
	bodyLinks := es.extractPDFLinksFromBody(msg)
	allLinks = append(allLinks, bodyLinks...)
	
	// 去重
	linkMap := make(map[string]bool)
	var uniqueLinks []string
	for _, link := range allLinks {
		if !linkMap[link] {
			linkMap[link] = true
			uniqueLinks = append(uniqueLinks, link)
		}
	}
	
	return uniqueLinks
}

// extractPDFLinksFromBody 从邮件正文中提取PDF链接
func (es *EmailService) extractPDFLinksFromBody(msg *imap.Message) []string {
	var links []string
	
	if msg.Body == nil {
		es.logger.Debug("邮件Body为空，无法提取链接")
		return links
	}
	
	es.logger.Debugf("开始从邮件正文提取PDF链接，Body部分数量: %d", len(msg.Body))
	
	// 遍历所有Body部分
	for i, body := range msg.Body {
		if body == nil {
			es.logger.Debugf("Body部分 %d 为空", i)
			continue
		}
		
		// 读取正文内容
		content, err := io.ReadAll(body)
		if err != nil {
			es.logger.Debugf("读取Body部分 %d 失败: %v", i, err)
			continue
		}
		
		es.logger.Debugf("Body部分 %d 内容长度: %d 字节", i, len(content))
		
		// 尝试不同的编码解析
		textContent := es.decodeBodyContent(content)
		
		// 记录解码后的内容（仅前500字符用于调试）
		if len(textContent) > 0 {
			preview := textContent
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			es.logger.Debugf("Body部分 %d 解码后内容预览: %s", i, preview)
		}
		
		// 从文本内容中提取PDF链接
		bodyLinks := es.extractPDFLinks(textContent)
		if len(bodyLinks) > 0 {
			es.logger.Infof("从Body部分 %d 提取到PDF链接: %v", i, bodyLinks)
		}
		links = append(links, bodyLinks...)
		
		// 特殊处理：查找QQ邮箱等服务商的下载链接
		specialLinks := es.extractSpecialDownloadLinks(textContent)
		if len(specialLinks) > 0 {
			es.logger.Infof("从Body部分 %d 提取到特殊下载链接: %v", i, specialLinks)
		}
		links = append(links, specialLinks...)
	}
	
	es.logger.Infof("总共从邮件正文提取到 %d 个链接", len(links))
	return links
}

// decodeBodyContent 解码邮件正文内容
func (es *EmailService) decodeBodyContent(content []byte) string {
	// 尝试多种编码方式
	encodings := []string{"utf-8", "gbk", "gb2312", "iso-8859-1"}
	
	for _, encoding := range encodings {
		if decoded := utils.DecodeText(content, encoding); decoded != "" {
			return decoded
		}
	}
	
	// 如果都失败，返回原始字符串
	return string(content)
}

// extractSpecialDownloadLinks 提取特殊的下载链接（如QQ邮箱、网易邮箱等）
func (es *EmailService) extractSpecialDownloadLinks(text string) []string {
	var links []string
	
	// 定义各种邮件服务商的下载链接模式
	patterns := []string{
		// QQ邮箱下载链接
		`https?://[^/]*\.mail\.qq\.com/[^\s"'<>]+`,
		`https?://[^/]*dfsdown\.mail\.ftn\.qq\.com/[^\s"'<>]+`,
		
		// 网易邮箱下载链接
		`https?://[^/]*\.mail\.163\.com/[^\s"'<>]+`,
		`https?://[^/]*\.mail\.126\.com/[^\s"'<>]+`,
		
		// Gmail下载链接
		`https?://mail\.google\.com/mail/[^\s"'<>]+`,
		
		// Outlook下载链接
		`https?://[^/]*\.outlook\.com/[^\s"'<>]+`,
		
		// 通用下载链接（包含download、attachment等关键词）
		`https?://[^\s"'<>]*(?:download|attachment|file)[^\s"'<>]*`,
		
		// 通用PDF直链
		`https?://[^\s"'<>]+\.pdf(?:\?[^\s"'<>]*)?`,
	}
	
	for _, pattern := range patterns {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		
		matches := regex.FindAllString(text, -1)
		for _, match := range matches {
			// 验证URL格式
			if _, err := url.Parse(match); err == nil {
				// 进一步验证是否可能是PDF相关链接
				if es.isPotentialPDFLink(match) {
					links = append(links, match)
				}
			}
		}
	}
	
	return links
}

// isPotentialPDFLink 判断是否是潜在的PDF链接
func (es *EmailService) isPotentialPDFLink(link string) bool {
	linkLower := strings.ToLower(link)
	
	// 直接包含.pdf的链接
	if strings.Contains(linkLower, ".pdf") {
		return true
	}
	
	// 包含下载相关关键词的链接
	downloadKeywords := []string{
		"download", "attachment", "file", "doc", "document",
		"dfsdown", "mailattach", "attach", "getfile",
	}
	
	for _, keyword := range downloadKeywords {
		if strings.Contains(linkLower, keyword) {
			return true
		}
	}
	
	// 邮件服务商的特殊域名
	mailDomains := []string{
		"mail.qq.com", "mail.163.com", "mail.126.com",
		"mail.google.com", "outlook.com", "hotmail.com",
		"ftn.qq.com", "dfsdown",
	}
	
	for _, domain := range mailDomains {
		if strings.Contains(linkLower, domain) {
			return true
		}
	}
	
	return false
}

// AttachmentInfo 附件信息
type AttachmentInfo struct {
	FileName string
	Size     int64
}

// findPDFAttachments 查找PDF附件（使用统一的逻辑）
func (es *EmailService) findPDFAttachments(bodyStructure *imap.BodyStructure) []AttachmentInfo {
	var attachments []AttachmentInfo
	
	// 使用统一的PDF搜索逻辑
	es.searchPDFPartsRecursively(bodyStructure, func(fileName string, size int64) {
		if fileName != "" {
			attachments = append(attachments, AttachmentInfo{
				FileName: fileName,
				Size:     size,
			})
		}
	}, 0)
	
	return attachments
}

// searchPDFPartsRecursively 递归搜索PDF部分（统一逻辑，避免重复代码）
func (es *EmailService) searchPDFPartsRecursively(bs *imap.BodyStructure, callback func(string, int64), depth int) {
	// 防止无限递归
	if depth > 10 || bs == nil {
		return
	}
	
	// 检查当前部分是否为PDF附件（与下载服务保持一致的逻辑）
	mimeType := strings.ToLower(bs.MIMEType)
	mimeSubType := strings.ToLower(bs.MIMESubType)
	
	isPDF := (mimeType == "application" && mimeSubType == "pdf") ||
			 (mimeType == "application" && mimeSubType == "octet-stream") ||
			 (mimeType == "application" && mimeSubType == "binary")
	
	// 如果MIME类型不明确，检查文件名
	if !isPDF {
		fileName := es.extractFileNameFromBodyStructure(bs)
		if fileName != "" && strings.HasSuffix(strings.ToLower(fileName), ".pdf") {
			isPDF = true
		}
	}
	
	if isPDF {
		fileName := es.extractFileNameFromBodyStructure(bs)
		es.logger.Infof("邮件服务发现PDF附件 - 文件名: '%s', MIME: %s/%s, 大小: %d", 
			fileName, bs.MIMEType, bs.MIMESubType, bs.Size)
		callback(fileName, int64(bs.Size))
	}
	
	// 递归搜索子部分
	for i, part := range bs.Parts {
		if i > 20 { // 限制搜索数量
			break
		}
		es.searchPDFPartsRecursively(part, callback, depth+1)
	}
}

// extractFileNameFromBodyStructure 从BodyStructure提取文件名（统一逻辑）
func (es *EmailService) extractFileNameFromBodyStructure(bs *imap.BodyStructure) string {
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
	es.logger.Infof("开始测试账户%s的连接", account.Email)
	
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	conn, err := es.createConnectionWithTimeout(ctx, account)
	if err != nil {
		es.logger.Errorf("创建连接失败 %s: %v", account.Email, err)
		return fmt.Errorf("连接失败: %v", err)
	}
	defer conn.close()
	
	// 尝试选择收件箱来验证连接
	if err := conn.selectInbox(); err != nil {
		es.logger.Errorf("选择收件箱失败 %s: %v", account.Email, err)
		return fmt.Errorf("无法访问收件箱: %v", err)
	}
	
	// 尝试获取邮箱状态确认连接正常
	if status, err := conn.Client.Status("INBOX", []imap.StatusItem{imap.StatusMessages}); err != nil {
		es.logger.Errorf("获取邮箱状态失败 %s: %v", account.Email, err)
		return fmt.Errorf("无法获取邮箱状态: %v", err)
	} else {
		es.logger.Infof("连接测试成功 %s: 邮箱中有%d封邮件", account.Email, status.Messages)
	}
	
	return nil
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