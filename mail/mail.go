package mail

import (
	"fmt"
	"io"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

var (
	dialer *gomail.Dialer
	from   string
	once   sync.Once
)

// Message 邮件消息
type Message struct {
	To          string       // 收件人
	Subject     string       // 主题
	Body        string       // 正文
	IsHTML      bool         // 是否 HTML 格式
	ReplyTo     string       // 回复地址（可选）
	Cc          []string     // 抄送（可选）
	Attachments []Attachment // 附件（可选）
}

// Attachment 附件
type Attachment struct {
	Filename string // 文件名
	Data     []byte // 文件数据
}

// Send 发送邮件（唯一的公共 API）
//
// 示例：
//
//	// 纯文本邮件
//	mail.Send(&mail.Message{
//	    To:      "user@example.com",
//	    Subject: "Hello",
//	    Body:    "Hello World",
//	})
//
//	// HTML 邮件带附件
//	mail.Send(&mail.Message{
//	    To:      "user@example.com",
//	    Subject: "Report",
//	    Body:    "<h1>Monthly Report</h1>",
//	    IsHTML:  true,
//	    ReplyTo: "noreply@example.com",
//	    Cc:      []string{"boss@example.com"},
//	    Attachments: []mail.Attachment{
//	        {Filename: "report.csv", Data: csvData},
//	    },
//	})
func Send(msg *Message) error {
	// 验证必需字段
	if msg.To == "" {
		return fmt.Errorf("recipient (To) is required")
	}
	if msg.Subject == "" {
		return fmt.Errorf("subject is required")
	}

	// 确保配置已加载
	initMailer()

	// 创建 gomail 消息
	m := gomail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", msg.To)
	m.SetHeader("Subject", msg.Subject)

	// 设置正文
	contentType := "text/plain"
	if msg.IsHTML {
		contentType = "text/html"
	}
	m.SetBody(contentType, msg.Body)

	// 设置可选 Header
	if msg.ReplyTo != "" {
		m.SetHeader("Reply-To", msg.ReplyTo)
	}
	if len(msg.Cc) > 0 {
		m.SetHeader("Cc", msg.Cc...)
	}

	// 添加附件
	for _, att := range msg.Attachments {
		if err := attachBytes(m, att.Filename, att.Data); err != nil {
			return err
		}
	}

	// 发送邮件
	return dialer.DialAndSend(m)
}

// initMailer 初始化邮件发送器（懒加载）
func initMailer() {
	once.Do(func() {
		from = viper.GetString("mail.send_from")
		username := viper.GetString("mail.username")
		password := viper.GetString("mail.password")
		smtpHost := viper.GetString("mail.smtp_host")
		smtpPort := viper.GetInt("mail.smtp_port")

		dialer = gomail.NewDialer(smtpHost, smtpPort, username, password)
	})
}

// attachBytes 从内存添加附件
func attachBytes(m *gomail.Message, filename string, data []byte) error {
	if filename == "" {
		return fmt.Errorf("attachment filename cannot be empty")
	}
	if len(data) == 0 {
		return fmt.Errorf("attachment data cannot be empty")
	}

	m.Attach(filename, gomail.SetCopyFunc(func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}))
	return nil
}
