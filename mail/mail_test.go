package mail

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

// 重置测试环境
func resetMailer() {
	once = sync.Once{}
	dialer = nil
	from = ""
}

func TestSendTextEmail(t *testing.T) {
	// 配置
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	// 测试纯文本邮件（不实际发送）
	msg := &Message{
		To:      "recipient@example.com",
		Subject: "Test Subject",
		Body:    "Test body content",
	}

	// 验证消息结构
	if msg.To == "" {
		t.Error("To field should not be empty")
	}
	if msg.Subject == "" {
		t.Error("Subject field should not be empty")
	}
	if msg.IsHTML {
		t.Error("IsHTML should be false for text email")
	}
}

func TestSendHtmlEmail(t *testing.T) {
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	msg := &Message{
		To:      "recipient@example.com",
		Subject: "HTML Email",
		Body:    "<h1>Hello</h1><p>This is HTML</p>",
		IsHTML:  true,
	}

	if !msg.IsHTML {
		t.Error("IsHTML should be true for HTML email")
	}
}

func TestSendWithReplyTo(t *testing.T) {
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	msg := &Message{
		To:      "recipient@example.com",
		Subject: "Test",
		Body:    "Test",
		ReplyTo: "noreply@example.com",
	}

	if msg.ReplyTo != "noreply@example.com" {
		t.Errorf("Expected ReplyTo to be 'noreply@example.com', got '%s'", msg.ReplyTo)
	}
}

func TestSendWithCc(t *testing.T) {
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	msg := &Message{
		To:      "recipient@example.com",
		Subject: "Test",
		Body:    "Test",
		Cc:      []string{"cc1@example.com", "cc2@example.com"},
	}

	if len(msg.Cc) != 2 {
		t.Errorf("Expected 2 Cc recipients, got %d", len(msg.Cc))
	}
}

func TestSendWithAttachments(t *testing.T) {
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	// 创建测试数据
	csvData := []byte("Name,Age\nJohn,30\nJane,25")
	pdfData := []byte("Fake PDF content")

	msg := &Message{
		To:      "recipient@example.com",
		Subject: "Report with Attachments",
		Body:    "<h1>Monthly Report</h1>",
		IsHTML:  true,
		Attachments: []Attachment{
			{Filename: "data.csv", Data: csvData},
			{Filename: "report.pdf", Data: pdfData},
		},
	}

	if len(msg.Attachments) != 2 {
		t.Errorf("Expected 2 attachments, got %d", len(msg.Attachments))
	}

	if msg.Attachments[0].Filename != "data.csv" {
		t.Errorf("Expected first attachment to be 'data.csv', got '%s'", msg.Attachments[0].Filename)
	}
}

func TestSendValidation(t *testing.T) {
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	// 测试缺少 To
	err := Send(&Message{
		Subject: "Test",
		Body:    "Test",
	})
	if err == nil {
		t.Error("Send should return error when To is missing")
	}

	// 测试缺少 Subject
	err = Send(&Message{
		To:   "recipient@example.com",
		Body: "Test",
	})
	if err == nil {
		t.Error("Send should return error when Subject is missing")
	}
}

func TestAttachBytesValidation(t *testing.T) {
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	// 测试空文件名
	msg := &Message{
		To:      "recipient@example.com",
		Subject: "Test",
		Body:    "Test",
		Attachments: []Attachment{
			{Filename: "", Data: []byte("data")},
		},
	}

	err := Send(msg)
	if err == nil {
		t.Error("Send should return error for empty attachment filename")
	}

	// 测试空数据
	msg = &Message{
		To:      "recipient@example.com",
		Subject: "Test",
		Body:    "Test",
		Attachments: []Attachment{
			{Filename: "test.txt", Data: []byte{}},
		},
	}

	err = Send(msg)
	if err == nil {
		t.Error("Send should return error for empty attachment data")
	}
}

func TestCompleteEmailWithAllFeatures(t *testing.T) {
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)
	resetMailer()

	// 创建临时文件数据
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	fileData := []byte("Test file content")
	if err := os.WriteFile(testFile, fileData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// 读取文件数据
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// 完整功能测试
	msg := &Message{
		To:      "recipient@example.com",
		Subject: "Complete Test Email",
		Body:    "<h1>Test</h1><p>This email has all features</p>",
		IsHTML:  true,
		ReplyTo: "noreply@example.com",
		Cc:      []string{"cc@example.com"},
		Attachments: []Attachment{
			{Filename: "test.txt", Data: data},
			{Filename: "inline.csv", Data: []byte("a,b\n1,2")},
		},
	}

	// 验证所有字段
	if msg.To != "recipient@example.com" {
		t.Error("To field mismatch")
	}
	if !msg.IsHTML {
		t.Error("IsHTML should be true")
	}
	if msg.ReplyTo != "noreply@example.com" {
		t.Error("ReplyTo field mismatch")
	}
	if len(msg.Cc) != 1 {
		t.Error("Cc count mismatch")
	}
	if len(msg.Attachments) != 2 {
		t.Error("Attachments count mismatch")
	}
}

func TestMailerInitialization(t *testing.T) {
	viper.Set("mail.send_from", "init@example.com")
	viper.Set("mail.username", "init@example.com")
	viper.Set("mail.password", "initpass")
	viper.Set("mail.smtp_host", "smtp.init.com")
	viper.Set("mail.smtp_port", 465)

	// 重置
	resetMailer()

	// 触发初始化
	initMailer()

	if dialer == nil {
		t.Fatal("Dialer should be initialized")
	}

	if from != "init@example.com" {
		t.Errorf("Expected from to be 'init@example.com', got '%s'", from)
	}

	// 再次初始化应该保持同一实例
	firstDialer := dialer
	initMailer()

	if dialer != firstDialer {
		t.Error("initMailer should return the same instance (singleton)")
	}
}
