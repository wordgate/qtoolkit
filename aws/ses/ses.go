package ses

import (
	"context"
	"fmt"
	"sync"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/spf13/viper"
)

// Config represents SES configuration
type Config struct {
	AccessKey   string `yaml:"access_key" json:"access_key"`
	SecretKey   string `yaml:"secret_key" json:"secret_key"`
	UseIMDS     bool   `yaml:"use_imds" json:"use_imds"`
	Region      string `yaml:"region" json:"region"`
	DefaultFrom string `yaml:"default_from" json:"default_from"`
}

// EmailRequest represents a simplified email sending request
type EmailRequest struct {
	From     string   // Sender email (must be verified in SES)
	To       []string // Recipient email addresses
	Subject  string   // Email subject
	BodyText string   // Plain text body (optional if BodyHTML is provided)
	BodyHTML string   // HTML body (optional if BodyText is provided)
	ReplyTo  []string // Reply-to addresses (optional)
	CC       []string // CC addresses (optional)
	BCC      []string // BCC addresses (optional)
}

// EmailResponse contains the result of sending an email
type EmailResponse struct {
	MessageID string // AWS SES message ID
	Success   bool   // Whether the email was sent successfully
	Error     error  // Error if sending failed
}

var (
	globalConfig *Config
	globalClient *sesv2.Client
	clientOnce   sync.Once
	initErr      error
	configMux    sync.RWMutex
)

// loadConfigFromViper loads SES configuration from viper
// Configuration path priority (cascading fallback):
// 1. aws.ses - SES service config
// 2. aws - Global AWS config
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	// Load SES-specific config
	cfg.Region = viper.GetString("aws.ses.region")
	cfg.AccessKey = viper.GetString("aws.ses.access_key")
	cfg.SecretKey = viper.GetString("aws.ses.secret_key")
	cfg.UseIMDS = viper.GetBool("aws.ses.use_imds")
	cfg.DefaultFrom = viper.GetString("aws.ses.default_from")

	// Fall back to global AWS config for missing credentials/region
	if cfg.Region == "" {
		cfg.Region = viper.GetString("aws.region")
	}
	if cfg.AccessKey == "" {
		cfg.AccessKey = viper.GetString("aws.access_key")
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = viper.GetString("aws.secret_key")
	}
	if !viper.IsSet("aws.ses.use_imds") && viper.IsSet("aws.use_imds") {
		cfg.UseIMDS = viper.GetBool("aws.use_imds")
	}

	// Validate: default to us-east-1 if no region configured
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	return cfg, nil
}

// initialize performs the actual SES client initialization
func initialize() {
	cfg, err := loadConfigFromViper()
	if err != nil {
		initErr = fmt.Errorf("failed to load SES config: %v", err)
		return
	}

	// Store config for later use
	configMux.Lock()
	globalConfig = cfg
	configMux.Unlock()

	// Default SES region
	region := "us-east-1"
	if cfg.Region != "" {
		region = cfg.Region
	}

	ctx := context.Background()
	var awsCfg awsv2.Config

	// If UseIMDS is explicitly set to false, use static credentials
	if !cfg.UseIMDS {
		if cfg.AccessKey != "" && cfg.SecretKey != "" {
			awsCfg, err = config.LoadDefaultConfig(ctx,
				config.WithRegion(region),
				config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
					cfg.AccessKey,
					cfg.SecretKey,
					"",
				)),
			)
		} else {
			initErr = fmt.Errorf("UseIMDS is false but AccessKey/SecretKey are not configured")
			return
		}
	} else {
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}

	if err != nil {
		initErr = fmt.Errorf("failed to load AWS config: %v", err)
		return
	}

	globalClient = sesv2.NewFromConfig(awsCfg)
	initErr = nil
}

// getClient returns the SES client with lazy initialization
func getClient() (*sesv2.Client, error) {
	clientOnce.Do(initialize)
	if initErr != nil {
		return nil, initErr
	}
	return globalClient, nil
}

// SendEmail sends an email using AWS SES with simplified configuration
func SendEmail(req *EmailRequest) (*EmailResponse, error) {
	// Validate required fields
	if err := validateEmailRequest(req); err != nil {
		return &EmailResponse{Success: false, Error: err}, err
	}

	client, err := getClient()
	if err != nil {
		return &EmailResponse{Success: false, Error: err}, err
	}

	// Build email input
	input := buildSESv2Input(req)

	// Send email
	ctx := context.Background()
	result, err := client.SendEmail(ctx, input)
	if err != nil {
		return &EmailResponse{Success: false, Error: err}, err
	}

	return &EmailResponse{
		MessageID: *result.MessageId,
		Success:   true,
		Error:     nil,
	}, nil
}

// SendSimpleEmail is a convenience function for sending basic text emails
func SendSimpleEmail(from, to, subject, body string) (*EmailResponse, error) {
	return SendEmail(&EmailRequest{
		From:     from,
		To:       []string{to},
		Subject:  subject,
		BodyText: body,
	})
}

// SendHTMLEmail is a convenience function for sending HTML emails
func SendHTMLEmail(from, to, subject, htmlBody string) (*EmailResponse, error) {
	return SendEmail(&EmailRequest{
		From:     from,
		To:       []string{to},
		Subject:  subject,
		BodyHTML: htmlBody,
	})
}

// SendMail sends a plain text email using the default sender from config
func SendMail(to, subject, content string) error {
	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil || cfg.DefaultFrom == "" {
		return fmt.Errorf("default sender (DefaultFrom) not configured, use SendSimpleEmail() instead")
	}

	_, err := SendSimpleEmail(cfg.DefaultFrom, to, subject, content)
	return err
}

// SendRichMail sends an HTML email using the default sender from config
func SendRichMail(to, subject, htmlContent string) error {
	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil || cfg.DefaultFrom == "" {
		return fmt.Errorf("default sender (DefaultFrom) not configured, use SendHTMLEmail() instead")
	}

	_, err := SendHTMLEmail(cfg.DefaultFrom, to, subject, htmlContent)
	return err
}

// validateEmailRequest validates the email request
func validateEmailRequest(req *EmailRequest) error {
	if req.From == "" {
		return fmt.Errorf("sender email (From) is required")
	}
	if len(req.To) == 0 {
		return fmt.Errorf("at least one recipient (To) is required")
	}
	if req.Subject == "" {
		return fmt.Errorf("email subject is required")
	}
	if req.BodyText == "" && req.BodyHTML == "" {
		return fmt.Errorf("email body (BodyText or BodyHTML) is required")
	}
	return nil
}

// buildSESv2Input builds the SES v2 SendEmail input from EmailRequest
func buildSESv2Input(req *EmailRequest) *sesv2.SendEmailInput {
	input := &sesv2.SendEmailInput{
		FromEmailAddress: &req.From,
		Destination: &types.Destination{
			ToAddresses: req.To,
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    &req.Subject,
					Charset: strPtr("UTF-8"),
				},
				Body: &types.Body{},
			},
		},
	}

	// Add CC if provided
	if len(req.CC) > 0 {
		input.Destination.CcAddresses = req.CC
	}

	// Add BCC if provided
	if len(req.BCC) > 0 {
		input.Destination.BccAddresses = req.BCC
	}

	// Add ReplyTo if provided
	if len(req.ReplyTo) > 0 {
		input.ReplyToAddresses = req.ReplyTo
	}

	// Add text body if provided
	if req.BodyText != "" {
		input.Content.Simple.Body.Text = &types.Content{
			Data:    &req.BodyText,
			Charset: strPtr("UTF-8"),
		}
	}

	// Add HTML body if provided
	if req.BodyHTML != "" {
		input.Content.Simple.Body.Html = &types.Content{
			Data:    &req.BodyHTML,
			Charset: strPtr("UTF-8"),
		}
	}

	return input
}

// strPtr is a helper function to get a pointer to a string
func strPtr(s string) *string {
	return &s
}

// Reset resets the SES client and configuration
// This is mainly useful for testing
func Reset() {
	configMux.Lock()
	defer configMux.Unlock()

	globalConfig = nil
	globalClient = nil
	initErr = nil
	clientOnce = sync.Once{}
}
