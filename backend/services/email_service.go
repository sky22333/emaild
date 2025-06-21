package services

import (
	"crypto/tls"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"strings"
	"time"

	"emaild/backend/database"
	"emaild/backend/models"
	"emaild/backend/utils"

	"github.com/PuerkitoBio/goquery"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

// EmailService 邮件服务结构体
type EmailService struct {
	logger    *logrus.Logger
	clients   map[uint]*client.Client // 邮箱ID -> IMAP客户端的映射
	stopChan  chan bool              // 停止信号通道
	isRunning bool                   // 是否正在运行
}

// NewEmailService 创建新的邮件服务实例
func NewEmailService(logger *logrus.Logger) *EmailService {
	return &EmailService{
		logger:   logger,
		clients:  make(map[uint]*client.Client),
		stopChan: make(chan bool),
	}
}

// EmailCheckResult 邮件检查结果
type EmailCheckResult struct {
	Account     *models.EmailAccount `json:"account"`
	NewEmails   int                  `json:"new_emails"`
	PDFsFound   int                  `json:"pdfs_found"`
	Error       string               `json:"error,omitempty"`
	Success     bool                 `json:"success"`
}

// StartEmailMonitoring 启动邮件监控
func (es *EmailService) StartEmailMonitoring() error {
	if es.isRunning {
		return fmt.Errorf("邮件监控已经在运行中")
	}

	es.isRunning = true
	es.logger.Info("启动邮件监控服务")

	go es.monitoringLoop()
	return nil
}

// StopEmailMonitoring 停止邮件监控
func (es *EmailService) StopEmailMonitoring() {
	if !es.isRunning {
		return
	}

	es.logger.Info("停止邮件监控服务")
	es.stopChan <- true
	es.isRunning = false

	// 关闭所有IMAP连接
	for _, client := range es.clients {
		if client != nil {
			client.Close()
		}
	}
	es.clients = make(map[uint]*client.Client)
}

// monitoringLoop 监控循环
func (es *EmailService) monitoringLoop() {
	config, err := database.GetConfig()
	if err != nil {
		es.logger.Errorf("获取配置失败: %v", err)
		return
	}

	if !config.AutoCheck {
		es.logger.Info("自动检查已禁用")
		return
	}

	ticker := time.NewTicker(time.Duration(config.CheckInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-es.stopChan:
			es.logger.Info("收到停止信号，退出监控循环")
			return
		case <-ticker.C:
			es.logger.Debug("定时检查邮件...")
			results := es.CheckAllAccounts()
			
			// 记录检查结果
			totalPDFs := 0
			for _, result := range results {
				if result.Success {
					totalPDFs += result.PDFsFound
				}
			}
			
			if totalPDFs > 0 {
				es.logger.Infof("本次检查发现 %d 个PDF文件", totalPDFs)
			}
		}
	}
}

// CheckAllAccounts 检查所有邮箱账户
func (es *EmailService) CheckAllAccounts() []EmailCheckResult {
	accounts, err := database.GetEmailAccounts()
	if err != nil {
		es.logger.Errorf("获取邮箱账户失败: %v", err)
		return nil
	}

	var results []EmailCheckResult
	
	for _, account := range accounts {
		result := es.CheckAccount(&account)
		results = append(results, result)
	}

	return results
}

// CheckAccount 检查单个邮箱账户
func (es *EmailService) CheckAccount(account *models.EmailAccount) EmailCheckResult {
	result := EmailCheckResult{
		Account: account,
		Success: false,
	}

	es.logger.Infof("检查邮箱: %s", account.Email)

	// 连接到IMAP服务器
	imapClient, err := es.connectToIMAP(account)
	if err != nil {
		result.Error = fmt.Sprintf("连接IMAP失败: %v", err)
		es.logger.Error(result.Error)
		return result
	}

	// 选择收件箱
	_, err = imapClient.Select("INBOX", false)
	if err != nil {
		result.Error = fmt.Sprintf("选择收件箱失败: %v", err)
		es.logger.Error(result.Error)
		imapClient.Close()
		return result
	}

	// 搜索未读邮件
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	
	uids, err := imapClient.Search(criteria)
	if err != nil {
		result.Error = fmt.Sprintf("搜索邮件失败: %v", err)
		es.logger.Error(result.Error)
		imapClient.Close()
		return result
	}

	result.NewEmails = len(uids)
	es.logger.Infof("发现 %d 封未读邮件", result.NewEmails)

	if result.NewEmails == 0 {
		result.Success = true
		imapClient.Close()
		return result
	}

	// 处理邮件
	pdfsFound := 0
	for _, uid := range uids {
		seqSet := new(imap.SeqSet)
		seqSet.AddNum(uid)

		// 获取邮件内容
		messages := make(chan *imap.Message, 1)
		done := make(chan error, 1)
		
		go func() {
			done <- imapClient.Fetch(seqSet, []imap.FetchItem{
				imap.FetchEnvelope,
				imap.FetchBody,
				imap.FetchBodyStructure,
			}, messages)
		}()

		select {
		case msg := <-messages:
			if msg != nil {
				count, err := es.processMessage(msg, account)
				if err != nil {
					es.logger.Errorf("处理邮件失败: %v", err)
				} else {
					pdfsFound += count
				}

				// 标记为已读
				es.markAsRead(imapClient, uid)
			}
		case err := <-done:
			if err != nil {
				es.logger.Errorf("获取邮件失败: %v", err)
			}
		case <-time.After(30 * time.Second):
			es.logger.Warn("获取邮件超时")
		}
	}

	result.PDFsFound = pdfsFound
	result.Success = true
	
	imapClient.Close()
	return result
}

// connectToIMAP 连接到IMAP服务器
func (es *EmailService) connectToIMAP(account *models.EmailAccount) (*client.Client, error) {
	// 检查是否已有连接
	if existingClient, exists := es.clients[account.ID]; exists {
		// 测试连接是否仍然有效
		if err := existingClient.Noop(); err == nil {
			return existingClient, nil
		} else {
			// 连接已断开，清理并重新连接
			es.logger.Warnf("IMAP连接已失效，重新连接: %s", account.Email)
			existingClient.Close()
			delete(es.clients, account.ID)
		}
	}

	// 建立新连接，支持重试机制
	addr := fmt.Sprintf("%s:%d", account.IMAPServer, account.IMAPPort)
	
	var imapClient *client.Client
	var err error
	
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if account.UseSSL {
			// 使用SSL/TLS连接
			tlsConfig := &tls.Config{
				ServerName:         account.IMAPServer,
				InsecureSkipVerify: false,
			}
			imapClient, err = client.DialTLS(addr, tlsConfig)
		} else {
			// 使用普通连接
			imapClient, err = client.Dial(addr)
		}

		if err == nil {
			// 连接成功，尝试登录
			if loginErr := imapClient.Login(account.Email, account.Password); loginErr != nil {
				imapClient.Close()
				err = fmt.Errorf("IMAP登录失败: %v", loginErr)
			} else {
				// 登录成功，跳出重试循环
				break
			}
		}
		
		// 如果不是最后一次尝试，等待后重试
		if attempt < maxRetries {
			es.logger.Warnf("IMAP连接失败（尝试 %d/%d）: %v", attempt, maxRetries, err)
			time.Sleep(time.Duration(attempt*2) * time.Second) // 指数退避
		}
	}

	if err != nil {
		return nil, fmt.Errorf("连接IMAP服务器失败（已重试%d次）: %v", maxRetries, err)
	}

	// 缓存连接
	es.clients[account.ID] = imapClient
	
	es.logger.Infof("成功连接到IMAP服务器: %s", account.Email)
	return imapClient, nil
}

// processMessage 处理单封邮件
func (es *EmailService) processMessage(msg *imap.Message, account *models.EmailAccount) (int, error) {
	pdfsFound := 0

	// 解析邮件头
	subject := ""
	sender := ""
	if msg.Envelope != nil {
		subject = utils.DecodeMimeHeader(msg.Envelope.Subject)
		if len(msg.Envelope.From) > 0 {
			sender = msg.Envelope.From[0].Address()
		}
	}

	es.logger.Debugf("处理邮件: %s (来自: %s)", subject, sender)

	// 创建邮件记录
	emailMessage := &models.EmailMessage{
		EmailID:     account.ID,
		MessageID:   msg.Envelope.MessageId,
		Subject:     subject,
		Sender:      sender,
		Date:        msg.Envelope.Date,
		HasPDF:      false,
		IsProcessed: false,
	}

	// 检查是否已处理过此邮件
	if existingMsg, err := database.GetEmailMessageByMessageID(msg.Envelope.MessageId); err == nil {
		if existingMsg.IsProcessed {
			es.logger.Debugf("邮件已处理过，跳过: %s", msg.Envelope.MessageId)
			return 0, nil
		}
	}

	// 处理邮件内容
	for _, bodyPart := range msg.Body {
		if bodyPart != nil {
			reader := bodyPart
			entity, err := message.Read(reader)
			if err != nil {
				es.logger.Errorf("读取邮件实体失败: %v", err)
				continue
			}

			count, err := es.processEntity(entity, account, subject, sender)
			if err != nil {
				es.logger.Errorf("处理邮件实体失败: %v", err)
			} else {
				pdfsFound += count
			}
		}
	}

	// 更新邮件记录
	emailMessage.HasPDF = pdfsFound > 0
	emailMessage.IsProcessed = true
	
	if err := database.CreateEmailMessage(emailMessage); err != nil {
		es.logger.Errorf("创建邮件记录失败: %v", err)
	}

	return pdfsFound, nil
}

// processEntity 处理邮件实体（递归处理多部分邮件）
func (es *EmailService) processEntity(entity *message.Entity, account *models.EmailAccount, subject, sender string) (int, error) {
	pdfsFound := 0

	// 获取内容类型
	contentType, params, err := entity.Header.ContentType()
	if err != nil {
		contentType = "text/plain"
	}

	// 处理多部分消息
	if strings.HasPrefix(contentType, "multipart/") {
		mr := multipart.NewReader(entity.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				es.logger.Errorf("读取多部分消息失败: %v", err)
				break
			}

			// 将multipart.Part转换为message.Entity
			partEntity, err := message.Read(part)
			if err != nil {
				es.logger.Errorf("读取多部分实体失败: %v", err)
				continue
			}

			count, err := es.processEntity(partEntity, account, subject, sender)
			if err != nil {
				es.logger.Errorf("处理多部分实体失败: %v", err)
			} else {
				pdfsFound += count
			}
		}
		return pdfsFound, nil
	}

	// 处理附件
	if contentType == "application/pdf" || 
	   contentType == "application/octet-stream" ||
	   strings.Contains(contentType, "pdf") {
		
		filename := es.getAttachmentFilename(entity)
		if filename != "" || contentType == "application/pdf" {
			if err := es.saveAttachment(entity, account, subject, sender, filename); err != nil {
				es.logger.Errorf("保存附件失败: %v", err)
			} else {
				pdfsFound++
			}
		}
	}

	// 处理HTML内容中的PDF链接
	if contentType == "text/html" {
		links, err := es.extractPDFLinks(entity)
		if err != nil {
			es.logger.Errorf("提取PDF链接失败: %v", err)
		} else {
			for _, link := range links {
				if err := es.createDownloadTask(link, account, subject, sender, models.TypeLink); err != nil {
					es.logger.Errorf("创建下载任务失败: %v", err)
				} else {
					pdfsFound++
				}
			}
		}
	}

	return pdfsFound, nil
}

// getAttachmentFilename 获取附件文件名
func (es *EmailService) getAttachmentFilename(entity *message.Entity) string {
	// 检查 Content-Disposition 头
	disp, params, err := entity.Header.ContentDisposition()
	if err == nil && disp == "attachment" {
		if filename, ok := params["filename"]; ok {
			return utils.DecodeMimeHeader(filename)
		}
	}

	// 检查 Content-Type 头中的 name 参数
	_, params, err = entity.Header.ContentType()
	if err == nil {
		if name, ok := params["name"]; ok {
			return utils.DecodeMimeHeader(name)
		}
	}

	return ""
}

// saveAttachment 保存附件
func (es *EmailService) saveAttachment(entity *message.Entity, account *models.EmailAccount, subject, sender, filename string) error {
	// 读取附件内容
	body, err := io.ReadAll(entity.Body)
	if err != nil {
		return fmt.Errorf("读取附件内容失败: %v", err)
	}

	// 验证是否为PDF文件
	if !utils.IsPDFContent(body) {
		return fmt.Errorf("文件不是有效的PDF格式")
	}

	// 生成文件名
	if filename == "" {
		filename = fmt.Sprintf("pdf_%d.pdf", time.Now().Unix())
	}
	filename = utils.CleanFilename(filename)

	// 创建下载任务
	task := &models.DownloadTask{
		EmailID:        account.ID,
		Subject:        subject,
		Sender:         sender,
		FileName:       filename,
		FileSize:       int64(len(body)),
		DownloadedSize: int64(len(body)),
		Status:         models.StatusCompleted,
		Type:           models.TypeAttachment,
		Source:         filename,
		Progress:       100.0,
		Speed:          "完成",
	}

	// 保存到数据库
	if err := database.CreateDownloadTask(task); err != nil {
		return fmt.Errorf("创建下载任务失败: %v", err)
	}

	// 保存文件
	config, err := database.GetConfig()
	if err != nil {
		return fmt.Errorf("获取配置失败: %v", err)
	}

	filepath, err := utils.SaveFile(body, filename, config.DownloadPath)
	if err != nil {
		return fmt.Errorf("保存文件失败: %v", err)
	}

	// 更新任务的本地路径
	task.LocalPath = filepath
	database.UpdateDownloadTask(task)

	es.logger.Infof("成功保存PDF附件: %s", filename)
	return nil
}

// extractPDFLinks 从HTML内容中提取PDF链接
func (es *EmailService) extractPDFLinks(entity *message.Entity) ([]string, error) {
	// 读取HTML内容
	body, err := io.ReadAll(entity.Body)
	if err != nil {
		return nil, err
	}

	// 解码HTML内容
	htmlContent := string(body)
	
	// 尝试检测编码并转换
	if encoding := es.detectEncoding(entity); encoding != nil {
		if decoded, err := encoding.NewDecoder().Bytes(body); err == nil {
			htmlContent = string(decoded)
		}
	}

	// 使用goquery解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	var links []string
	
	// 查找包含PDF的链接
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		// 检查链接是否指向PDF
		if es.isPDFLink(href) || es.isPDFText(s.Text()) {
			// 转换相对链接为绝对链接（如果需要）
			if absoluteURL := es.makeAbsoluteURL(href); absoluteURL != "" {
				links = append(links, absoluteURL)
			}
		}
	})

	// 去重
	return utils.RemoveDuplicates(links), nil
}

// isPDFLink 检查链接是否指向PDF
func (es *EmailService) isPDFLink(href string) bool {
	lower := strings.ToLower(href)
	return strings.Contains(lower, ".pdf") || 
		   strings.Contains(lower, "pdf") ||
		   strings.Contains(lower, "download")
}

// isPDFText 检查链接文本是否表示PDF
func (es *EmailService) isPDFText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(lower, "pdf") ||
		   strings.Contains(lower, "下载") ||
		   strings.Contains(lower, "download")
}

// makeAbsoluteURL 将相对URL转换为绝对URL
func (es *EmailService) makeAbsoluteURL(href string) string {
	// 如果已经是绝对URL，直接返回
	if u, err := url.Parse(href); err == nil && u.IsAbs() {
		return href
	}
	
	// 对于相对URL，暂时返回空字符串
	// 在实际应用中，需要根据邮件上下文确定基础URL
	return ""
}



// detectEncoding 检测文本编码
func (es *EmailService) detectEncoding(entity *message.Entity) encoding.Encoding {
	_, params, err := entity.Header.ContentType()
	if err != nil {
		return nil
	}

	charset, ok := params["charset"]
	if !ok {
		return nil
	}

	switch strings.ToLower(charset) {
	case "gb2312", "gbk", "gb18030":
		return simplifiedchinese.GBK
	case "utf-8":
		return unicode.UTF8
	case "utf-16":
		return unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
	default:
		return nil
	}
}

// createDownloadTask 创建下载任务
func (es *EmailService) createDownloadTask(url string, account *models.EmailAccount, subject, sender string, taskType models.DownloadType) error {
	task := &models.DownloadTask{
		EmailID:        account.ID,
		Subject:        subject,
		Sender:         sender,
		FileName:       utils.ExtractFilenameFromURL(url),
		Status:         models.StatusPending,
		Type:           taskType,
		Source:         url,
		Progress:       0.0,
		Speed:          "等待中",
	}

	return database.CreateDownloadTask(task)
}

// markAsRead 标记邮件为已读
func (es *EmailService) markAsRead(imapClient *client.Client, uid uint32) error {
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)
	
	flags := []interface{}{imap.SeenFlag}
	return imapClient.Store(seqSet, "+FLAGS", flags, nil)
}

// TestConnection 测试邮箱连接
func (es *EmailService) TestConnection(account *models.EmailAccount) error {
	imapClient, err := es.connectToIMAP(account)
	if err != nil {
		return err
	}
	
	defer imapClient.Close()
	
	// 尝试选择收件箱来验证连接
	_, err = imapClient.Select("INBOX", true) // 只读模式
	return err
} 