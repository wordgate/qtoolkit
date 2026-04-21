package mail

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
	// Loopback + reserved port: connect refuses immediately (ECONNREFUSED), no DNS,
	// no hang on hostile/captive networks. Send's dial will fail fast; we only
	// assert that the registry was populated before the dial was attempted.
	viper.Set("mail.smtp_host", "127.0.0.1")
	viper.Set("mail.smtp_port", 1)

	// Send must trigger resolveSender("mail"). The dial error is expected.
	_ = Send(&Message{
		To:      "u@example.com",
		Subject: "s",
		Body:    "b",
	})

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

func TestConfig_SESMultiIdentity(t *testing.T) {
	resetMailer()

	viper.Set("ses_a.provider", "ses")
	viper.Set("ses_a.send_from", "a@example.com")
	viper.Set("ses_a.region", "us-east-1")
	viper.Set("ses_a.access_key", "AKIA_A")
	viper.Set("ses_a.secret_key", "secret_a")

	viper.Set("ses_b.provider", "ses")
	viper.Set("ses_b.send_from", "b@example.com")
	viper.Set("ses_b.region", "eu-west-1")
	viper.Set("ses_b.access_key", "AKIA_B")
	viper.Set("ses_b.secret_key", "secret_b")

	a, err := resolveSender("ses_a")
	if err != nil {
		t.Fatalf("resolve ses_a: %v", err)
	}
	b, err := resolveSender("ses_b")
	if err != nil {
		t.Fatalf("resolve ses_b: %v", err)
	}

	if a.ses == nil || b.ses == nil {
		t.Fatal("both SES senders must have non-nil clients")
	}
	if a.ses == b.ses {
		t.Error("each SES prefix must own its own *sesv2.Client")
	}
	if a.ses.Options().Region != "us-east-1" {
		t.Errorf("ses_a region = %q, want us-east-1", a.ses.Options().Region)
	}
	if b.ses.Options().Region != "eu-west-1" {
		t.Errorf("ses_b region = %q, want eu-west-1", b.ses.Options().Region)
	}
}

// captureSMTP starts a minimal in-process SMTP server on 127.0.0.1:<random>.
// It advertises no extensions (no STARTTLS, no AUTH), so gomail sends plain,
// unauthenticated mail. It accepts exactly one connection, captures the DATA
// bytes, and delivers them on the returned channel when the session ends.
func captureSMTP(t *testing.T) (host string, port int, body <-chan string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ch := make(chan string, 1)

	go func() {
		var raw strings.Builder
		defer func() { ch <- raw.String() }()

		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = fmt.Fprint(conn, s) }
		write("220 localhost test ready\r\n")

		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if strings.TrimRight(line, "\r\n") == "." {
					write("250 2.0.0 OK\r\n")
					inData = false
					continue
				}
				raw.WriteString(line)
				continue
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				write("250 localhost\r\n")
			case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
				write("250 2.1.0 OK\r\n")
			case cmd == "DATA":
				write("354 End data with <CRLF>.<CRLF>\r\n")
				inData = true
			case cmd == "QUIT":
				write("221 2.0.0 bye\r\n")
				return
			case cmd == "RSET", cmd == "NOOP":
				write("250 2.0.0 OK\r\n")
			default:
				write("502 5.5.1 not implemented\r\n")
			}
		}
	}()

	addrHost, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	return addrHost, p, ch
}

// TestSMTP_SubjectRFC2047Encoding proves gomail emits an RFC 2047 encoded
// Subject header (=?UTF-8?...?=) when the Message.Subject contains non-ASCII
// characters. This is the root-cause fix for the k2app Chinese-subject
// mojibake incident: gomail applies Q-encoding via mime.QEncoding in its
// default configuration, while hand-rolled net/smtp did not.
func TestSMTP_SubjectRFC2047Encoding(t *testing.T) {
	host, port, bodyCh := captureSMTP(t)

	resetMailer()
	viper.Set("mail.provider", "")
	viper.Set("mail.send_from", "from@example.com")
	viper.Set("mail.username", "u")
	viper.Set("mail.password", "p")
	viper.Set("mail.smtp_host", host)
	viper.Set("mail.smtp_port", port)

	if err := Send(&Message{
		To:      "rcpt@example.com",
		Subject: "五月中文主题测试",
		Body:    "hello",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	body := <-bodyCh
	if !strings.Contains(body, "Subject: =?UTF-8?") {
		t.Errorf("Subject must be RFC 2047 encoded (=?UTF-8?...?=); raw DATA was:\n%s", body)
	}
	if strings.Contains(body, "五月中文主题测试") {
		t.Errorf("raw UTF-8 Chinese must NOT appear in Subject (would mojibake on GBK clients); raw DATA was:\n%s", body)
	}
}
