package mods

import (
	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

func GetMailer() *gomail.Dialer {
	username := viper.GetString("mail.username")
	password := viper.GetString("mail.password")
	smtpHost := viper.GetString("mail.smtp_host")
	smtpPort := viper.GetInt("mail.smtp_port")
	return gomail.NewDialer(smtpHost, smtpPort, username, password)
}

func SendMail(to, subject, content string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", viper.GetString("mail.send_from"))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", content)
	return GetMailer().DialAndSend(m)
}

func SendRichMail(to, subject string, html string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", viper.GetString("mail.send_from"))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", html)
	return GetMailer().DialAndSend(m)
}

func SendSms(region, mobile string, content string) error {
	return nil
}
