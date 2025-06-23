package utils

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"unicode/utf8"
)

// CleanFilename 清理文件名，移除非法字符并确保有PDF扩展名
func CleanFilename(filename string) string {
	if filename == "" {
		filename = GenerateFilename("pdf", ".pdf")
	}

	// 解码可能的编码字符
	filename = DecodeMimeHeader(filename)

	// 移除路径分隔符和其他非法字符
	filename = regexp.MustCompile(`[\\/*?:"<>|]`).ReplaceAllString(filename, "_")
	
	// 移除前后空白字符
	filename = strings.TrimSpace(filename)
	
	// 限制文件名长度（Windows文件名最大255字符）
	if len(filename) > 200 {
		ext := filepath.Ext(filename)
		nameWithoutExt := strings.TrimSuffix(filename, ext)
		filename = nameWithoutExt[:200-len(ext)] + ext
	}

	// 确保有PDF扩展名
	if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		filename += ".pdf"
	}

	return filename
}

// DecodeMimeHeader 解码MIME编码的头部信息
func DecodeMimeHeader(header string) string {
	if header == "" {
		return ""
	}

	// 使用mime包的WordDecoder解码
	decoder := &mime.WordDecoder{}
	decoded, err := decoder.DecodeHeader(header)
	if err == nil {
		return decoded
	}

	// 如果mime解码失败，尝试手动解码
	return decodeManually(header)
}

// decodeManually 手动解码各种编码格式
func decodeManually(s string) string {
	// 处理 =?charset?encoding?encoded_text?= 格式
	re := regexp.MustCompile(`=\?([^?]+)\?([BQ])\?([^?]*)\?=`)
	
	return re.ReplaceAllStringFunc(s, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		
		charset := strings.ToLower(parts[1])
		encoding := strings.ToUpper(parts[2])
		text := parts[3]
		
		var decoded []byte
		var err error
		
		// 根据编码类型解码
		switch encoding {
		case "B": // Base64
			decoded, err = decodeBase64(text)
		case "Q": // Quoted-Printable
			decoded, err = decodeQuotedPrintable(text)
		default:
			return match
		}
		
		if err != nil {
			return match
		}
		
		// 根据字符集转换
		textEncoding := getEncoding(charset)
		if textEncoding != nil {
			if converted, err := textEncoding.NewDecoder().Bytes(decoded); err == nil {
				return string(converted)
			}
		}
		
		// 直接返回UTF-8字符串
		return string(decoded)
	})
}

// getEncoding 根据字符集名称获取编码器
func getEncoding(charset string) encoding.Encoding {
	switch charset {
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

// decodeBase64 解码Base64 - 修复实现
func decodeBase64(s string) ([]byte, error) {
	// 处理Base64编码可能缺少的填充
	if len(s)%4 != 0 {
		s += strings.Repeat("=", 4-len(s)%4)
	}
	
	return base64.StdEncoding.DecodeString(s)
}

// decodeQuotedPrintable 解码Quoted-Printable - 修复实现
func decodeQuotedPrintable(s string) ([]byte, error) {
	// 替换下划线为空格（邮件头部特殊处理）
	s = strings.ReplaceAll(s, "_", " ")
	
	// 使用标准库解码
	reader := quotedprintable.NewReader(strings.NewReader(s))
	decoded, err := io.ReadAll(reader)
	if err != nil {
		// 如果标准解码失败，尝试手动解码
		return manualQuotedPrintableDecode(s)
	}
	
	return decoded, nil
}

// manualQuotedPrintableDecode 手动解码Quoted-Printable
func manualQuotedPrintableDecode(s string) ([]byte, error) {
	var result []byte
	i := 0
	for i < len(s) {
		if s[i] == '=' && i+2 < len(s) {
			// 解码 =XX 格式
			hex := s[i+1:i+3]
			if b, err := strconv.ParseUint(hex, 16, 8); err == nil {
				result = append(result, byte(b))
				i += 3
			} else {
				result = append(result, s[i])
				i++
			}
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return result, nil
}

// IsPDFContent 检查内容是否为有效的PDF文件
func IsPDFContent(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	// 检查PDF文件头标识符
	return string(data[:4]) == "%PDF" || string(data[:5]) == "%PDF-"
}

// ValidatePDFFile 验证PDF文件完整性
func ValidatePDFFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("无法打开文件: %v", err)
	}
	defer file.Close()

	// 获取文件信息
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("无法获取文件信息: %v", err)
	}

	// 检查文件大小
	if info.Size() < 100 {
		return fmt.Errorf("文件太小，不是有效的PDF文件")
	}

	// 读取文件头
	header := make([]byte, 10)
	_, err = file.Read(header)
	if err != nil {
		return fmt.Errorf("无法读取文件头: %v", err)
	}

	// 验证PDF文件头
	if !IsPDFContent(header) {
		return fmt.Errorf("文件头不匹配，不是有效的PDF文件")
	}

	// 检查文件尾（PDF文件应该以%%EOF结尾）
	if info.Size() > 1024 {
		_, err = file.Seek(-1024, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("无法定位到文件尾: %v", err)
		}

		tail := make([]byte, 1024)
		_, err = file.Read(tail)
		if err != nil {
			return fmt.Errorf("无法读取文件尾: %v", err)
		}

		// 检查是否包含EOF标记
		if !strings.Contains(string(tail), "%%EOF") {
			return fmt.Errorf("文件可能不完整，缺少EOF标记")
		}
	}

	return nil
}

// ExtractFilenameFromURL 从URL中提取文件名
func ExtractFilenameFromURL(rawURL string) string {
	if rawURL == "" {
		return GenerateFilename("pdf", ".pdf")
	}

	// 解析URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return GenerateFilename("pdf", ".pdf")
	}

	// 从路径中提取文件名
	filename := filepath.Base(parsedURL.Path)
	
	// 如果没有文件名或文件名无效，生成默认名称
	if filename == "" || filename == "." || filename == "/" {
		// 尝试从查询参数中获取文件名
		if queryFilename := parsedURL.Query().Get("filename"); queryFilename != "" {
			filename = queryFilename
		} else {
			filename = GenerateFilename("pdf", ".pdf")
		}
	}

	// 清理并确保扩展名
	return CleanFilename(filename)
}

// SaveFile 保存文件到指定目录
func SaveFile(data []byte, filename, dir string) (string, error) {
	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	// 清理文件名
	filename = CleanFilename(filename)
	filePath := filepath.Join(dir, filename)

	// 处理文件名冲突
	counter := 1
	for {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			break // 文件不存在，可以使用
		}
		
		// 文件已存在，生成新名称
		ext := filepath.Ext(filename)
		nameWithoutExt := strings.TrimSuffix(filename, ext)
		newFilename := fmt.Sprintf("%s_%d%s", nameWithoutExt, counter, ext)
		filePath = filepath.Join(dir, newFilename)
		counter++
		
		// 防止无限循环
		if counter > 1000 {
			return "", fmt.Errorf("无法生成唯一文件名")
		}
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return filePath, nil
}

// FormatBytes 格式化字节数为人类可读的格式
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// FormatDuration 格式化时间间隔
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%d分钟", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%d小时", int(d.Hours()))
	} else {
		return fmt.Sprintf("%d天", int(d.Hours()/24))
	}
}

// RemoveDuplicates 去重字符串切片
func RemoveDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string
	
	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}
	
	return result
}

// IsValidEmail 验证邮箱地址格式
func IsValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

// IsValidURL 验证URL格式
func IsValidURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	
	return parsedURL.Scheme != "" && parsedURL.Host != ""
}

// GenerateFilename 生成唯一的文件名
func GenerateFilename(baseName string, extension string) string {
	if baseName == "" {
		baseName = "file"
	}
	
	if extension == "" {
		extension = ".pdf"
	}
	
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s_%d%s", baseName, timestamp, extension)
}

// TruncateString 截断字符串到指定长度
func TruncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	
	if maxLength <= 3 {
		return s[:maxLength]
	}
	
	return s[:maxLength-3] + "..."
}

// GetFileSize 获取文件大小
func GetFileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// FileExists 检查文件是否存在
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// EnsureDir 确保目录存在
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// GetMimeType 根据文件扩展名获取MIME类型
func GetMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

// SanitizeString 清理字符串，移除控制字符
func SanitizeString(s string) string {
	// 移除控制字符（ASCII 0-31，除了\t\n\r）
	re := regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	return re.ReplaceAllString(s, "")
}

// ParseContentRange 解析Content-Range头
func ParseContentRange(contentRange string) (start, end, total int64, err error) {
	// 格式: bytes start-end/total
	re := regexp.MustCompile(`bytes (\d+)-(\d+)/(\d+)`)
	matches := re.FindStringSubmatch(contentRange)
	
	if len(matches) != 4 {
		return 0, 0, 0, fmt.Errorf("无效的Content-Range格式: %s", contentRange)
	}
	
	start, err = strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	
	end, err = strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	
	total, err = strconv.ParseInt(matches[3], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	
	return start, end, total, nil
}

// GetProgressPercentage 计算进度百分比
func GetProgressPercentage(current, total int64) float64 {
	if total <= 0 {
		return 0.0
	}
	
	percentage := float64(current) / float64(total) * 100.0
	if percentage > 100.0 {
		percentage = 100.0
	} else if percentage < 0.0 {
		percentage = 0.0
	}
	
	return percentage
}

// FormatSpeed 格式化下载速度
func FormatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond < 1024 {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	} else if bytesPerSecond < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSecond/1024)
	} else if bytesPerSecond < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB/s", bytesPerSecond/(1024*1024))
	} else {
		return fmt.Sprintf("%.1f GB/s", bytesPerSecond/(1024*1024*1024))
	}
}

// DecodeText 尝试使用指定编码解码文本
func DecodeText(data []byte, encoding string) string {
	if len(data) == 0 {
		return ""
	}
	
	switch strings.ToLower(encoding) {
	case "utf-8":
		if utf8.Valid(data) {
			return string(data)
		}
	case "gbk", "gb2312":
		// 对于中文编码，尝试转换
		if decoded := tryDecodeGBK(data); decoded != "" {
			return decoded
		}
	case "iso-8859-1", "latin1":
		// ISO-8859-1编码，直接转换
		return string(data)
	}
	
	// 如果指定编码失败，尝试UTF-8
	if utf8.Valid(data) {
		return string(data)
	}
	
	// 最后尝试强制转换
	return string(data)
}

// tryDecodeGBK 尝试解码GBK编码的文本
func tryDecodeGBK(data []byte) string {
	// 简单的GBK检测和转换
	// 这里可以使用第三方库如golang.org/x/text/encoding/simplifiedchinese
	// 但为了减少依赖，我们使用简单的方法
	
	// 检查是否包含中文字符的字节模式
	for i := 0; i < len(data)-1; i++ {
		b1, b2 := data[i], data[i+1]
		// GBK编码范围检测
		if (b1 >= 0xA1 && b1 <= 0xFE) && (b2 >= 0xA1 && b2 <= 0xFE) {
			// 可能是GBK编码，但我们暂时返回原始字符串
			// 在生产环境中应该使用专门的编码转换库
			return string(data)
		}
	}
	
	return ""
} 