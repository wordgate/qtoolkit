package ses_test

import (
	"fmt"

	"github.com/wordgate/qtoolkit/aws/ses"
)

// ExampleSendMail demonstrates the ultra-simple API using default sender
func ExampleSendMail() {
	// Configure SES once
	ses.SetConfig(&ses.Config{
		Region:      "us-east-1",
		DefaultFrom: "noreply@yourdomain.com",
		UseIMDS:     true,
	})

	// Then just use 3 parameters! (automatically initialized on first call)
	err := ses.SendMail("user@example.com", "Welcome", "Thank you for signing up!")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Email sent successfully!")
}

// ExampleSendRichMail demonstrates sending HTML email using default sender
func ExampleSendRichMail() {
	// Configure SES once
	ses.SetConfig(&ses.Config{
		Region:      "us-east-1",
		DefaultFrom: "noreply@yourdomain.com",
		UseIMDS:     true,
	})

	// Send HTML email with 3 parameters
	htmlContent := "<h1>Newsletter</h1><p>Check out our latest updates!</p>"
	err := ses.SendRichMail("user@example.com", "Monthly Newsletter", htmlContent)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("HTML email sent successfully!")
}

// ExampleSendSimpleEmail demonstrates sending a simple text email
func ExampleSendSimpleEmail() {
	// Configure SES (optional on EC2 with IAM Role)
	ses.SetConfig(&ses.Config{
		Region:  "us-east-1",
		UseIMDS: true,
	})

	// Send a simple email
	resp, err := ses.SendSimpleEmail(
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
	ses.SetConfig(&ses.Config{
		Region:  "us-east-1",
		UseIMDS: true,
	})

	// Send HTML email
	resp, err := ses.SendHTMLEmail(
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
	ses.SetConfig(&ses.Config{
		Region:  "us-east-1",
		UseIMDS: true,
	})

	// Send email with CC, BCC, and Reply-To
	resp, err := ses.SendEmail(&ses.EmailRequest{
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
	// On EC2 with IAM Role, minimal configuration needed!
	// The SDK will automatically use the instance's IAM Role
	ses.SetConfig(&ses.Config{
		Region:  "us-east-1",
		UseIMDS: true, // Use EC2 IAM role
	})

	resp, err := ses.SendSimpleEmail(
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
