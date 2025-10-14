package aws

import (
	"context"
	"fmt"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// EmailRequest represents a simplified email sending request
type EmailRequest struct {
	From        string   // Sender email (must be verified in SES)
	To          []string // Recipient email addresses
	Subject     string   // Email subject
	BodyText    string   // Plain text body (optional if BodyHTML is provided)
	BodyHTML    string   // HTML body (optional if BodyText is provided)
	ReplyTo     []string // Reply-to addresses (optional)
	CC          []string // CC addresses (optional)
	BCC         []string // BCC addresses (optional)
}

// EmailResponse contains the result of sending an email
type EmailResponse struct {
	MessageID string // AWS SES message ID
	Success   bool   // Whether the email was sent successfully
	Error     error  // Error if sending failed
}

// SendEmail sends an email using AWS SES with simplified configuration
// It supports multiple authentication methods:
// 1. Explicit credentials via SetConfig() - for development/external servers
// 2. EC2 IAM Role - automatic when running on EC2 (no config needed)
// 3. Environment variables - AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
func SendEmail(req *EmailRequest) (*EmailResponse, error) {
	// Validate required fields
	if err := validateEmailRequest(req); err != nil {
		return &EmailResponse{Success: false, Error: err}, err
	}

	// Create AWS config
	cfg, err := createSESConfig()
	if err != nil {
		return &EmailResponse{Success: false, Error: err}, err
	}

	// Create SES v2 service client
	client := sesv2.NewFromConfig(cfg)

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
// Usage: SendSimpleEmail("from@example.com", "to@example.com", "Subject", "Body text")
func SendSimpleEmail(from, to, subject, body string) (*EmailResponse, error) {
	return SendEmail(&EmailRequest{
		From:     from,
		To:       []string{to},
		Subject:  subject,
		BodyText: body,
	})
}

// SendHTMLEmail is a convenience function for sending HTML emails
// Usage: SendHTMLEmail("from@example.com", "to@example.com", "Subject", "<h1>HTML Body</h1>")
func SendHTMLEmail(from, to, subject, htmlBody string) (*EmailResponse, error) {
	return SendEmail(&EmailRequest{
		From:     from,
		To:       []string{to},
		Subject:  subject,
		BodyHTML: htmlBody,
	})
}

// SendMail sends a plain text email using the default sender from config
// This is a simplified function that uses the configured default_from address
// Usage: SendMail("recipient@example.com", "Subject", "Body text")
// Note: Requires SES.DefaultFrom to be configured in SetConfig(), or will return an error
func SendMail(to, subject, content string) error {
	from := getDefaultFrom()
	if from == "" {
		return fmt.Errorf("default sender (SES.DefaultFrom) not configured, use SendSimpleEmail() instead or configure SetConfig()")
	}

	_, err := SendSimpleEmail(from, to, subject, content)
	return err
}

// SendRichMail sends an HTML email using the default sender from config
// This is a simplified function that uses the configured default_from address
// Usage: SendRichMail("recipient@example.com", "Subject", "<h1>HTML Body</h1>")
// Note: Requires SES.DefaultFrom to be configured in SetConfig(), or will return an error
func SendRichMail(to, subject, htmlContent string) error {
	from := getDefaultFrom()
	if from == "" {
		return fmt.Errorf("default sender (SES.DefaultFrom) not configured, use SendHTMLEmail() instead or configure SetConfig()")
	}

	_, err := SendHTMLEmail(from, to, subject, htmlContent)
	return err
}

// getDefaultFrom returns the default sender email from config
func getDefaultFrom() string {
	if globalConfig != nil && globalConfig.SES.DefaultFrom != "" {
		return globalConfig.SES.DefaultFrom
	}
	return ""
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

// createSESConfig creates an AWS config for SES v2
func createSESConfig() (awsv2.Config, error) {
	// Determine the region to use
	region := "us-east-1" // Default SES region

	if globalConfig != nil {
		// Use configured region if available
		if globalConfig.SES.Region != "" {
			region = globalConfig.SES.Region
		} else if globalConfig.Region != "" {
			region = globalConfig.Region
		}
	}

	return loadConfig(region)
}

// strPtr is a helper function to get a pointer to a string
func strPtr(s string) *string {
	return &s
}
