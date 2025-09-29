package mail

import (
	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

// MailService provides email functionality
type MailService struct {
	config *MailConfig
}

// MailConfig holds mail configuration
type MailConfig struct {
	SendFrom string `mapstructure:"send_from" yaml:"send_from"`
	Username string `mapstructure:"username" yaml:"username"`
	Password string `mapstructure:"password" yaml:"password"`
	SMTPHost string `mapstructure:"smtp_host" yaml:"smtp_host"`
	SMTPPort int    `mapstructure:"smtp_port" yaml:"smtp_port"`
}

// NewMailService creates a new mail service instance
func NewMailService(config *MailConfig) *MailService {
	return &MailService{
		config: config,
	}
}

// NewMailServiceFromViper creates a mail service from viper configuration
func NewMailServiceFromViper() *MailService {
	config := &MailConfig{
		SendFrom: viper.GetString("mail.send_from"),
		Username: viper.GetString("mail.username"),
		Password: viper.GetString("mail.password"),
		SMTPHost: viper.GetString("mail.smtp_host"),
		SMTPPort: viper.GetInt("mail.smtp_port"),
	}
	return NewMailService(config)
}

// GetMailer returns a configured gomail dialer
func (m *MailService) GetMailer() *gomail.Dialer {
	return gomail.NewDialer(m.config.SMTPHost, m.config.SMTPPort, m.config.Username, m.config.Password)
}

// SendMail sends a plain text email
func (m *MailService) SendMail(to, subject, content string) error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.config.SendFrom)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", content)
	return m.GetMailer().DialAndSend(msg)
}

// SendRichMail sends an HTML email
func (m *MailService) SendRichMail(to, subject, html string) error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.config.SendFrom)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", html)
	return m.GetMailer().DialAndSend(msg)
}

// SendSms is a placeholder for SMS functionality
func (m *MailService) SendSms(region, mobile string, content string) error {
	return nil
}

// Legacy functions for backward compatibility
var defaultService *MailService

// InitDefaultService initializes the default mail service
func InitDefaultService() {
	defaultService = NewMailServiceFromViper()
}

// GetMailer returns a gomail dialer using viper configuration (legacy)
func GetMailer() *gomail.Dialer {
	if defaultService == nil {
		InitDefaultService()
	}
	return defaultService.GetMailer()
}

// SendMail sends a plain text email using viper configuration (legacy)
func SendMail(to, subject, content string) error {
	if defaultService == nil {
		InitDefaultService()
	}
	return defaultService.SendMail(to, subject, content)
}

// SendRichMail sends an HTML email using viper configuration (legacy)
func SendRichMail(to, subject, html string) error {
	if defaultService == nil {
		InitDefaultService()
	}
	return defaultService.SendRichMail(to, subject, html)
}

// SendSms is a placeholder for SMS functionality (legacy)
func SendSms(region, mobile string, content string) error {
	if defaultService == nil {
		InitDefaultService()
	}
	return defaultService.SendSms(region, mobile, content)
}