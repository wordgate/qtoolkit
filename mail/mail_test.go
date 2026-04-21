package mail

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// resetMailer clears the sender registry. Kept as a local alias so existing
// tests compile with minimal churn.
func resetMailer() {
	ResetForTest()
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
	resetMailer()
	viper.Set("mail.provider", "") // defensive reset; no dependency on prior tests
	viper.Set("mail.send_from", "init@example.com")
	viper.Set("mail.username", "init@example.com")
	viper.Set("mail.password", "initpass")
	viper.Set("mail.smtp_host", "smtp.init.com")
	viper.Set("mail.smtp_port", 465)

	snd1, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd1.smtp == nil {
		t.Fatal("SMTP dialer should be initialized")
	}
	if snd1.cfg.SendFrom != "init@example.com" {
		t.Errorf("expected from 'init@example.com', got %q", snd1.cfg.SendFrom)
	}

	// Second resolve must return the cached *sender.
	snd2, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd2 != snd1 {
		t.Error("resolveSender should return cached sender (one *sender per prefix)")
	}
}

func TestSendProviderConfig(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "ses")
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.region", "us-east-1")
	viper.Set("mail.access_key", "AKIA_TEST")
	viper.Set("mail.secret_key", "secret_test")

	snd, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd.cfg.Provider != "ses" {
		t.Errorf("expected provider 'ses', got %q", snd.cfg.Provider)
	}
	if snd.cfg.SendFrom != "test@example.com" {
		t.Errorf("expected from 'test@example.com', got %q", snd.cfg.SendFrom)
	}
	if snd.smtp != nil {
		t.Error("SMTP dialer should be nil when provider is 'ses'")
	}
	if snd.ses == nil {
		t.Error("SES client should be non-nil when provider is 'ses'")
	}
}

func TestSendProviderDefaultSMTP(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "")
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)

	snd, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd.cfg.Provider != "smtp" {
		t.Errorf("expected provider to default to 'smtp', got %q", snd.cfg.Provider)
	}
	if snd.smtp == nil {
		t.Error("SMTP dialer should be initialized for SMTP provider")
	}
	if snd.ses != nil {
		t.Error("SES client should be nil for SMTP provider")
	}
}

func TestConfig_PrefixIsolation(t *testing.T) {
	resetMailer()
	// Explicit provider resets — viper keys persist across tests, and a prior
	// test may have set mail.provider to "ses". Set both to empty so loadConfig
	// defaults to "smtp".
	viper.Set("mail.provider", "")
	viper.Set("edm.provider", "")

	viper.Set("mail.send_from", "tx@example.com")
	viper.Set("mail.username", "tx_user")
	viper.Set("mail.password", "tx_pass")
	viper.Set("mail.smtp_host", "smtp.tx.com")
	viper.Set("mail.smtp_port", 465)

	viper.Set("edm.send_from", "edm@example.com")
	viper.Set("edm.username", "edm_user")
	viper.Set("edm.password", "edm_pass")
	viper.Set("edm.smtp_host", "smtp.edm.com")
	viper.Set("edm.smtp_port", 587)

	txSnd, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolve mail: %v", err)
	}
	edmSnd, err := resolveSender("edm")
	if err != nil {
		t.Fatalf("resolve edm: %v", err)
	}

	if txSnd == edmSnd {
		t.Fatal("mail and edm must resolve to distinct *sender instances")
	}
	if txSnd.cfg.SMTPHost != "smtp.tx.com" {
		t.Errorf("mail host = %q, want smtp.tx.com", txSnd.cfg.SMTPHost)
	}
	if edmSnd.cfg.SMTPHost != "smtp.edm.com" {
		t.Errorf("edm host = %q, want smtp.edm.com", edmSnd.cfg.SMTPHost)
	}
	if txSnd.smtp == edmSnd.smtp {
		t.Error("mail and edm must have distinct *gomail.Dialer instances")
	}
}

func TestConfig_EmptyPrefix(t *testing.T) {
	resetMailer()
	err := Config("").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrEmptyPrefix) {
		t.Errorf("expected ErrEmptyPrefix, got %v", err)
	}
}

func TestConfig_MissingSendFrom(t *testing.T) {
	resetMailer()
	// ghost prefix has no config at all
	err := Config("ghost").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Errorf("expected ErrMissingConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "send_from") {
		t.Errorf("error should name missing field send_from, got %v", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should name prefix 'ghost', got %v", err)
	}
}

func TestConfig_MissingSMTPHost(t *testing.T) {
	resetMailer()
	viper.Set("partial.send_from", "p@example.com")
	// intentionally omit smtp_host/username/password/smtp_port
	err := Config("partial").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Errorf("expected ErrMissingConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "smtp_host") {
		t.Errorf("error should name missing field smtp_host, got %v", err)
	}
}

func TestConfig_SESMissingRegion(t *testing.T) {
	resetMailer()
	viper.Set("ses_bad.provider", "ses")
	viper.Set("ses_bad.send_from", "x@example.com")
	// intentionally omit region / access_key / secret_key
	err := Config("ses_bad").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("expected ErrMissingConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "region") {
		t.Errorf("error should name missing field region, got %v", err)
	}
}

func TestConfig_UnknownProvider(t *testing.T) {
	resetMailer()
	viper.Set("weird.provider", "pigeon")
	viper.Set("weird.send_from", "p@example.com")
	err := Config("weird").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Errorf("expected ErrMissingConfig for unknown provider, got %v", err)
	}
	if !strings.Contains(err.Error(), "pigeon") {
		t.Errorf("error should name the unknown provider, got %v", err)
	}
}

func TestConfig_LazyInit(t *testing.T) {
	resetMailer()
	// Create a Sender handle without triggering resolveSender.
	_ = Config("edm")
	// Registry must still be empty.
	if senderFor("edm") != nil {
		t.Error("Config(prefix) must not populate the registry before Send()")
	}
}

func TestSend_EquivalentToConfigMail(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "") // defensive: prior tests may have set "ses"
	viper.Set("mail.send_from", "a@example.com")
	viper.Set("mail.username", "a")
	viper.Set("mail.password", "b")
	viper.Set("mail.smtp_host", "smtp.a.com")
	viper.Set("mail.smtp_port", 25)

	// Force init via package-level Send's validation path (we do not
	// actually dial — we only care that both paths hit the same *sender).
	if err := Send(&Message{
		To:      "u@example.com",
		Subject: "s",
		Body:    "b",
	}); err == nil {
		t.Log("Send attempted dial (expected SMTP failure or success); ignoring error")
	}

	direct := senderFor("mail")
	if direct == nil {
		t.Fatal("package-level Send should have resolved the 'mail' sender")
	}

	// Config("mail") must produce a handle whose Send targets the same *sender.
	viaConfig, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender: %v", err)
	}
	if viaConfig != direct {
		t.Error("Send(msg) and Config(\"mail\").Send(msg) must share the same *sender")
	}
}
