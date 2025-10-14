package mail

import (
	"sync"
	"testing"

	"github.com/spf13/viper"
)

func TestSendMailLazyLoad(t *testing.T) {
	// Set up viper configuration for testing
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)

	// Reset once to ensure fresh initialization
	once = sync.Once{}
	dialer = nil
	from = ""

	// Test that GetMailer triggers lazy initialization
	mailer := GetMailer()
	if mailer == nil {
		t.Fatal("GetMailer returned nil")
	}

	if from != "test@example.com" {
		t.Errorf("Expected from to be 'test@example.com', got '%s'", from)
	}

	if dialer == nil {
		t.Fatal("Dialer was not initialized")
	}
}

func TestGetMailer(t *testing.T) {
	// Set up viper configuration
	viper.Set("mail.send_from", "mailer@example.com")
	viper.Set("mail.username", "mailer@example.com")
	viper.Set("mail.password", "mailerpass")
	viper.Set("mail.smtp_host", "smtp.mailer.com")
	viper.Set("mail.smtp_port", 465)

	// Reset for this test
	once = sync.Once{}
	dialer = nil
	from = ""

	mailer := GetMailer()
	if mailer == nil {
		t.Fatal("GetMailer returned nil")
	}

	// Verify that calling GetMailer again returns the same instance
	mailer2 := GetMailer()
	if mailer != mailer2 {
		t.Error("GetMailer should return the same instance (singleton pattern)")
	}
}

func TestViperConfiguration(t *testing.T) {
	// Test configuration loading from viper
	viper.Set("mail.send_from", "config@example.com")
	viper.Set("mail.username", "config@example.com")
	viper.Set("mail.password", "configpass")
	viper.Set("mail.smtp_host", "smtp.config.com")
	viper.Set("mail.smtp_port", 25)

	// Reset
	once = sync.Once{}
	dialer = nil
	from = ""

	// Trigger initialization
	GetMailer()

	// Verify configuration was loaded correctly
	if from != "config@example.com" {
		t.Errorf("Expected from to be 'config@example.com', got '%s'", from)
	}
}
