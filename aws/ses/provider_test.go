package ses

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/mail"
)

func resetSESState() {
	Reset()
}

func TestNewProviderCompiles(t *testing.T) {
	p := NewProvider()
	var _ mail.Sender = p
}

func TestProviderRequiresDefaultFrom(t *testing.T) {
	resetSESState()
	viper.Reset()

	viper.Set("aws.ses.access_key", "AKIATEST")
	viper.Set("aws.ses.secret_key", "testkey")
	viper.Set("aws.ses.region", "us-east-1")
	// No default_from set

	p := NewProvider()
	err := p.Send(&mail.Message{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Hello",
	})

	if err == nil {
		t.Fatal("expected error when default_from is not set")
	}
	if err.Error() != "ses provider: aws.ses.default_from is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildProviderInput(t *testing.T) {
	msg := &mail.Message{
		To:      "user@example.com",
		Subject: "Test Subject",
		Body:    "<h1>Hello</h1>",
		IsHTML:  true,
		ReplyTo: "reply@example.com",
		Cc:      []string{"cc1@example.com", "cc2@example.com"},
		Attachments: []mail.Attachment{
			{Filename: "report.pdf", Data: []byte("fake-pdf-data")},
		},
	}

	input := buildProviderInput("sender@example.com", msg)

	// From
	if *input.FromEmailAddress != "sender@example.com" {
		t.Errorf("From: expected 'sender@example.com', got '%s'", *input.FromEmailAddress)
	}

	// To
	if len(input.Destination.ToAddresses) != 1 || input.Destination.ToAddresses[0] != "user@example.com" {
		t.Errorf("To: expected ['user@example.com'], got %v", input.Destination.ToAddresses)
	}

	// Subject
	if *input.Content.Simple.Subject.Data != "Test Subject" {
		t.Errorf("Subject: expected 'Test Subject', got '%s'", *input.Content.Simple.Subject.Data)
	}

	// HTML body
	if input.Content.Simple.Body.Html == nil {
		t.Fatal("HTML body should be set")
	}
	if *input.Content.Simple.Body.Html.Data != "<h1>Hello</h1>" {
		t.Errorf("Body HTML: expected '<h1>Hello</h1>', got '%s'", *input.Content.Simple.Body.Html.Data)
	}
	if input.Content.Simple.Body.Text != nil {
		t.Error("Text body should be nil when IsHTML=true")
	}

	// ReplyTo
	if len(input.ReplyToAddresses) != 1 || input.ReplyToAddresses[0] != "reply@example.com" {
		t.Errorf("ReplyTo: expected ['reply@example.com'], got %v", input.ReplyToAddresses)
	}

	// CC
	if len(input.Destination.CcAddresses) != 2 {
		t.Errorf("CC: expected 2 addresses, got %d", len(input.Destination.CcAddresses))
	}

	// Attachments
	if len(input.Content.Simple.Attachments) != 1 {
		t.Fatalf("Attachments: expected 1, got %d", len(input.Content.Simple.Attachments))
	}
	att := input.Content.Simple.Attachments[0]
	if *att.FileName != "report.pdf" {
		t.Errorf("Attachment filename: expected 'report.pdf', got '%s'", *att.FileName)
	}
	if string(att.RawContent) != "fake-pdf-data" {
		t.Errorf("Attachment data mismatch")
	}
}

func TestBuildProviderInputTextBody(t *testing.T) {
	msg := &mail.Message{
		To:      "user@example.com",
		Subject: "Plain",
		Body:    "Hello plain",
		IsHTML:  false,
	}

	input := buildProviderInput("sender@example.com", msg)

	if input.Content.Simple.Body.Text == nil {
		t.Fatal("Text body should be set")
	}
	if *input.Content.Simple.Body.Text.Data != "Hello plain" {
		t.Errorf("Body Text: expected 'Hello plain', got '%s'", *input.Content.Simple.Body.Text.Data)
	}
	if input.Content.Simple.Body.Html != nil {
		t.Error("HTML body should be nil when IsHTML=false")
	}
}

func TestBuildProviderInputMinimal(t *testing.T) {
	msg := &mail.Message{
		To:      "user@example.com",
		Subject: "Min",
		Body:    "body",
	}

	input := buildProviderInput("sender@example.com", msg)

	if len(input.Destination.CcAddresses) != 0 {
		t.Error("CC should be empty")
	}
	if len(input.ReplyToAddresses) != 0 {
		t.Error("ReplyTo should be empty")
	}
	if len(input.Content.Simple.Attachments) != 0 {
		t.Error("Attachments should be empty")
	}
}
