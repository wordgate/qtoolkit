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

// getMailer returns the configured gomail dialer (lazy initialization)
func getMailer() *gomail.Dialer {
	once.Do(func() {
		from = viper.GetString("mail.send_from")
		username := viper.GetString("mail.username")
		password := viper.GetString("mail.password")
		smtpHost := viper.GetString("mail.smtp_host")
		smtpPort := viper.GetInt("mail.smtp_port")

		dialer = gomail.NewDialer(smtpHost, smtpPort, username, password)
	})
	return dialer
}

// NewTextMessage creates a plain text email message with From, To, Subject headers set
// Returns the message for further customization (e.g., adding Reply-To header)
func NewTextMessage(to, subject, text string) *gomail.Message {
	getMailer() // ensure config is loaded

	msg := gomail.NewMessage()
	msg.SetHeader("From", from)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", text)

	return msg
}

// NewHtmlMessage creates an HTML email message with From, To, Subject headers set
// Returns the message for further customization (e.g., adding Reply-To header)
func NewHtmlMessage(to, subject, html string) *gomail.Message {
	getMailer() // ensure config is loaded

	msg := gomail.NewMessage()
	msg.SetHeader("From", from)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", html)

	return msg
}

// SendMessage sends a pre-built gomail.Message
// Use NewTextMessage or NewHtmlMessage to create the message,
// then customize headers (e.g., msg.SetHeader("Reply-To", "...")) before sending
func SendMessage(msg *gomail.Message) error {
	return getMailer().DialAndSend(msg)
}

// SendText sends a plain text email
// Configuration is automatically loaded from viper on first use
func SendText(to, subject, content string) error {
	return SendMessage(NewTextMessage(to, subject, content))
}

// SendHtml sends an HTML email
// Configuration is automatically loaded from viper on first use
func SendHtml(to, subject, html string) error {
	return SendMessage(NewHtmlMessage(to, subject, html))
}
