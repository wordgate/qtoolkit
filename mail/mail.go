package mail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/aws/ses"
	"gopkg.in/gomail.v2"
)

// Sentinel errors.
var (
	ErrEmptyPrefix   = errors.New("mail: empty config prefix")
	ErrMissingConfig = errors.New("mail: required config field missing")
)

// Message is the outbound email payload.
type Message struct {
	To          string       // Recipient
	Subject     string       // Subject line
	Body        string       // Plain text or HTML body
	IsHTML      bool         // Body is HTML when true
	ReplyTo     string       // Optional Reply-To header
	Cc          []string     // Optional CC recipients
	Attachments []Attachment // Optional attachments
}

// Attachment is an in-memory file attached to a Message.
type Attachment struct {
	Filename string
	Data     []byte
}

// Sender is a handle bound to a viper config prefix.
// Returned by Config(prefix). Safe for concurrent use.
type Sender struct {
	prefix string
}

type config struct {
	Provider  string
	SendFrom  string
	SMTPHost  string
	SMTPPort  int
	Username  string
	Password  string
	Region    string
	AccessKey string
	SecretKey string
	UseIMDS   bool
}

type sender struct {
	prefix   string
	cfg      *config
	smtp     *gomail.Dialer
	ses      *sesv2.Client
	initOnce sync.Once
	initErr  error
}

var registry sync.Map // string -> *sender

// Config returns a Sender bound to the given viper key prefix.
//
// The underlying dialer / SES client is lazy-loaded on first Send.
// Passing an empty prefix is legal; it fails at Send() with ErrEmptyPrefix.
//
// Example:
//
//	err := mail.Config("edm").Send(&mail.Message{...})
func Config(prefix string) *Sender {
	return &Sender{prefix: prefix}
}

// Send dispatches msg using the sender identity bound to s.prefix.
func (s *Sender) Send(msg *Message) error {
	if s.prefix == "" {
		return ErrEmptyPrefix
	}
	if err := validateMessage(msg); err != nil {
		return err
	}
	snd, err := resolveSender(s.prefix)
	if err != nil {
		return err
	}
	if snd.cfg.Provider == "ses" {
		return sendViaSES(snd, msg)
	}
	return sendViaSMTP(snd, msg)
}

// Send is the package-level shortcut for Config("mail").Send(msg).
//
// Example:
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
	return Config("mail").Send(msg)
}

// ResetForTest clears the sender registry. Intended for tests only.
func ResetForTest() {
	registry = sync.Map{}
}

// resolveSender returns (and lazy-initializes) the *sender for prefix.
func resolveSender(prefix string) (*sender, error) {
	v, _ := registry.LoadOrStore(prefix, &sender{prefix: prefix})
	snd := v.(*sender)
	snd.initOnce.Do(func() {
		cfg, err := loadConfig(prefix)
		if err != nil {
			snd.initErr = err
			return
		}
		snd.cfg = cfg
		switch cfg.Provider {
		case "smtp":
			snd.smtp = gomail.NewDialer(cfg.SMTPHost, cfg.SMTPPort, cfg.Username, cfg.Password)
		case "ses":
			client, err := ses.NewClient(&ses.Config{
				AccessKey:   cfg.AccessKey,
				SecretKey:   cfg.SecretKey,
				UseIMDS:     cfg.UseIMDS,
				Region:      cfg.Region,
				DefaultFrom: cfg.SendFrom,
			})
			if err != nil {
				snd.initErr = err
				return
			}
			snd.ses = client
		}
	})
	if snd.initErr != nil {
		return nil, snd.initErr
	}
	return snd, nil
}

// senderFor is a private accessor used by tests.
func senderFor(prefix string) *sender {
	v, _ := registry.Load(prefix)
	if v == nil {
		return nil
	}
	return v.(*sender)
}

// loadConfig reads and validates <prefix>.* from viper.
func loadConfig(prefix string) (*config, error) {
	cfg := &config{
		Provider:  viper.GetString(prefix + ".provider"),
		SendFrom:  viper.GetString(prefix + ".send_from"),
		SMTPHost:  viper.GetString(prefix + ".smtp_host"),
		SMTPPort:  viper.GetInt(prefix + ".smtp_port"),
		Username:  viper.GetString(prefix + ".username"),
		Password:  viper.GetString(prefix + ".password"),
		Region:    viper.GetString(prefix + ".region"),
		AccessKey: viper.GetString(prefix + ".access_key"),
		SecretKey: viper.GetString(prefix + ".secret_key"),
		UseIMDS:   viper.GetBool(prefix + ".use_imds"),
	}
	if cfg.Provider == "" {
		cfg.Provider = "smtp"
	}
	if cfg.SendFrom == "" {
		return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "send_from")
	}
	switch cfg.Provider {
	case "smtp":
		for _, pair := range []struct {
			name, val string
		}{
			{"smtp_host", cfg.SMTPHost},
			{"username", cfg.Username},
			{"password", cfg.Password},
		} {
			if pair.val == "" {
				return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, pair.name)
			}
		}
		if cfg.SMTPPort == 0 {
			return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "smtp_port")
		}
	case "ses":
		if cfg.Region == "" {
			return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "region")
		}
		if !cfg.UseIMDS {
			if cfg.AccessKey == "" {
				return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "access_key")
			}
			if cfg.SecretKey == "" {
				return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "secret_key")
			}
		}
	default:
		return nil, fmt.Errorf("%w: prefix=%q unknown provider=%q", ErrMissingConfig, prefix, cfg.Provider)
	}
	return cfg, nil
}

func validateMessage(msg *Message) error {
	if msg.To == "" {
		return fmt.Errorf("recipient (To) is required")
	}
	if msg.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	for _, att := range msg.Attachments {
		if att.Filename == "" {
			return fmt.Errorf("attachment filename cannot be empty")
		}
		if len(att.Data) == 0 {
			return fmt.Errorf("attachment data cannot be empty")
		}
	}
	return nil
}

func sendViaSMTP(snd *sender, msg *Message) error {
	m := gomail.NewMessage()
	m.SetHeader("From", snd.cfg.SendFrom)
	m.SetHeader("To", msg.To)
	m.SetHeader("Subject", msg.Subject)

	contentType := "text/plain"
	if msg.IsHTML {
		contentType = "text/html"
	}
	m.SetBody(contentType, msg.Body)

	if msg.ReplyTo != "" {
		m.SetHeader("Reply-To", msg.ReplyTo)
	}
	if len(msg.Cc) > 0 {
		m.SetHeader("Cc", msg.Cc...)
	}

	for _, att := range msg.Attachments {
		if err := attachBytes(m, att.Filename, att.Data); err != nil {
			return err
		}
	}
	return snd.smtp.DialAndSend(m)
}

func sendViaSES(snd *sender, msg *Message) error {
	req := &ses.EmailRequest{
		From:    snd.cfg.SendFrom,
		To:      []string{msg.To},
		Subject: msg.Subject,
	}
	if msg.IsHTML {
		req.BodyHTML = msg.Body
	} else {
		req.BodyText = msg.Body
	}
	if len(msg.Cc) > 0 {
		req.CC = msg.Cc
	}
	if msg.ReplyTo != "" {
		req.ReplyTo = []string{msg.ReplyTo}
	}
	for _, att := range msg.Attachments {
		req.Attachments = append(req.Attachments, ses.EmailAttachment{
			Filename: att.Filename,
			Data:     att.Data,
		})
	}
	_, err := ses.SendEmailWith(context.Background(), snd.ses, req)
	return err
}

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
