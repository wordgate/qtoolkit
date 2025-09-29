package mail

import (
	"testing"

	"github.com/spf13/viper"
)

func TestNewMailService(t *testing.T) {
	config := &MailConfig{
		SendFrom: "test@example.com",
		Username: "test@example.com",
		Password: "password",
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
	}

	service := NewMailService(config)
	if service == nil {
		t.Fatal("NewMailService returned nil")
	}

	if service.config.SendFrom != "test@example.com" {
		t.Errorf("Expected SendFrom to be 'test@example.com', got '%s'", service.config.SendFrom)
	}
}

func TestNewMailServiceFromViper(t *testing.T) {
	// Set up viper configuration for testing
	viper.Set("mail.send_from", "viper@example.com")
	viper.Set("mail.username", "viper@example.com")
	viper.Set("mail.password", "viperpass")
	viper.Set("mail.smtp_host", "smtp.viper.com")
	viper.Set("mail.smtp_port", 465)

	service := NewMailServiceFromViper()
	if service == nil {
		t.Fatal("NewMailServiceFromViper returned nil")
	}

	if service.config.SendFrom != "viper@example.com" {
		t.Errorf("Expected SendFrom to be 'viper@example.com', got '%s'", service.config.SendFrom)
	}

	if service.config.SMTPPort != 465 {
		t.Errorf("Expected SMTPPort to be 465, got %d", service.config.SMTPPort)
	}
}

func TestGetMailer(t *testing.T) {
	config := &MailConfig{
		SendFrom: "test@example.com",
		Username: "test@example.com",
		Password: "password",
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
	}

	service := NewMailService(config)
	mailer := service.GetMailer()

	if mailer == nil {
		t.Fatal("GetMailer returned nil")
	}
}

func TestSendSms(t *testing.T) {
	config := &MailConfig{
		SendFrom: "test@example.com",
		Username: "test@example.com",
		Password: "password",
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
	}

	service := NewMailService(config)
	err := service.SendSms("us", "1234567890", "test message")

	// Currently returns nil as it's a placeholder
	if err != nil {
		t.Errorf("Expected SendSms to return nil, got %v", err)
	}
}

func TestLegacyFunctions(t *testing.T) {
	// Set up viper configuration for testing
	viper.Set("mail.send_from", "legacy@example.com")
	viper.Set("mail.username", "legacy@example.com")
	viper.Set("mail.password", "legacypass")
	viper.Set("mail.smtp_host", "smtp.legacy.com")
	viper.Set("mail.smtp_port", 465)

	// Reset defaultService to test initialization
	defaultService = nil

	mailer := GetMailer()
	if mailer == nil {
		t.Fatal("Legacy GetMailer returned nil")
	}

	// Test that default service was initialized
	if defaultService == nil {
		t.Fatal("Default service was not initialized")
	}

	if defaultService.config.SendFrom != "legacy@example.com" {
		t.Errorf("Expected default service SendFrom to be 'legacy@example.com', got '%s'", defaultService.config.SendFrom)
	}

	// Test legacy SMS function
	err := SendSms("us", "1234567890", "test message")
	if err != nil {
		t.Errorf("Expected legacy SendSms to return nil, got %v", err)
	}
}