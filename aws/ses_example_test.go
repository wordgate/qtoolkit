package aws_test

import (
	"fmt"

	"github.com/wordgate/qtoolkit/aws"
)

// ExampleSendSimpleEmail demonstrates sending a simple text email
func ExampleSendSimpleEmail() {
	// Configure AWS (optional on EC2 with IAM Role)
	config := &aws.Config{
		Region: "us-east-1",
		SES: aws.SESConfig{
			Region: "us-east-1",
		},
	}
	aws.SetConfig(config)

	// Send a simple email
	resp, err := aws.SendSimpleEmail(
		"sender@example.com",
		"recipient@example.com",
		"Test Email",
		"This is a test email from AWS SES!",
	)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Email sent! MessageID: %s\n", resp.MessageID)
}

// ExampleSendHTMLEmail demonstrates sending an HTML email
func ExampleSendHTMLEmail() {
	// Send HTML email
	resp, err := aws.SendHTMLEmail(
		"noreply@example.com",
		"user@example.com",
		"Welcome!",
		"<h1>Welcome to our service!</h1><p>Thank you for signing up.</p>",
	)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("HTML email sent! MessageID: %s\n", resp.MessageID)
}

// ExampleSendEmail demonstrates sending an email with all options
func ExampleSendEmail() {
	// Send email with CC, BCC, and Reply-To
	resp, err := aws.SendEmail(&aws.EmailRequest{
		From:     "noreply@example.com",
		To:       []string{"user1@example.com", "user2@example.com"},
		CC:       []string{"manager@example.com"},
		BCC:      []string{"archive@example.com"},
		ReplyTo:  []string{"support@example.com"},
		Subject:  "Important Update",
		BodyText: "This is the plain text version.",
		BodyHTML: "<h1>This is the HTML version</h1>",
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Email sent with options! MessageID: %s\n", resp.MessageID)
}

// ExampleSendEmail_ec2IAMRole demonstrates sending email on EC2 without explicit credentials
func ExampleSendEmail_ec2IAMRole() {
	// On EC2 with IAM Role, no configuration needed!
	// The SDK will automatically use the instance's IAM Role

	resp, err := aws.SendSimpleEmail(
		"noreply@example.com",
		"user@example.com",
		"Test from EC2",
		"This email was sent from an EC2 instance using IAM Role!",
	)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Email sent from EC2! MessageID: %s\n", resp.MessageID)
}
