package mail

import (
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

var (
	dialer *gomail.Dialer
	from   string
	once   sync.Once
)

// initConfig initializes mail configuration from viper (lazy load)
func initConfig() {
	once.Do(func() {
		from = viper.GetString("mail.send_from")
		username := viper.GetString("mail.username")
		password := viper.GetString("mail.password")
		smtpHost := viper.GetString("mail.smtp_host")
		smtpPort := viper.GetInt("mail.smtp_port")

		dialer = gomail.NewDialer(smtpHost, smtpPort, username, password)
	})
}

// SendMail sends a plain text email
// Configuration is automatically loaded from viper on first use
func SendMail(to, subject, content string) error {
	initConfig()

	msg := gomail.NewMessage()
	msg.SetHeader("From", from)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", content)

	return dialer.DialAndSend(msg)
}

// SendRichMail sends an HTML email
// Configuration is automatically loaded from viper on first use
func SendRichMail(to, subject, html string) error {
	initConfig()

	msg := gomail.NewMessage()
	msg.SetHeader("From", from)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", html)

	return dialer.DialAndSend(msg)
}

// GetMailer returns the configured gomail dialer
// Useful for advanced use cases that need direct access to the dialer
func GetMailer() *gomail.Dialer {
	initConfig()
	return dialer
}
