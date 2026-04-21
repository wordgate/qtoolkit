package ses

import (
	"context"
	"strings"
	"testing"
)

func TestBuildSESv2InputWithAttachments(t *testing.T) {
	req := &EmailRequest{
		From:     "sender@example.com",
		To:       []string{"user@example.com"},
		Subject:  "Test",
		BodyText: "Hello",
		Attachments: []EmailAttachment{
			{Filename: "report.pdf", Data: []byte("fake-pdf")},
			{Filename: "data.csv", Data: []byte("a,b\n1,2")},
		},
	}

	input := buildSESv2Input(req)

	if len(input.Content.Simple.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(input.Content.Simple.Attachments))
	}
	if *input.Content.Simple.Attachments[0].FileName != "report.pdf" {
		t.Errorf("first attachment filename: expected 'report.pdf', got '%s'", *input.Content.Simple.Attachments[0].FileName)
	}
	if string(input.Content.Simple.Attachments[1].RawContent) != "a,b\n1,2" {
		t.Errorf("second attachment data mismatch")
	}
}

func TestBuildSESv2InputNoAttachments(t *testing.T) {
	req := &EmailRequest{
		From:     "sender@example.com",
		To:       []string{"user@example.com"},
		Subject:  "Test",
		BodyText: "Hello",
	}

	input := buildSESv2Input(req)

	if len(input.Content.Simple.Attachments) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(input.Content.Simple.Attachments))
	}
}

func TestValidateEmailRequestAttachments(t *testing.T) {
	// Empty filename
	req := &EmailRequest{
		From:     "sender@example.com",
		To:       []string{"user@example.com"},
		Subject:  "Test",
		BodyText: "Hello",
		Attachments: []EmailAttachment{
			{Filename: "", Data: []byte("data")},
		},
	}
	if err := validateEmailRequest(req); err == nil {
		t.Error("expected error for empty attachment filename")
	}

	// Empty data
	req.Attachments = []EmailAttachment{
		{Filename: "test.txt", Data: []byte{}},
	}
	if err := validateEmailRequest(req); err == nil {
		t.Error("expected error for empty attachment data")
	}

	// Valid attachment
	req.Attachments = []EmailAttachment{
		{Filename: "test.txt", Data: []byte("content")},
	}
	if err := validateEmailRequest(req); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewClient_NoGlobalMutation(t *testing.T) {
	Reset()

	cfg := &Config{
		AccessKey: "AKIA_TEST_KEY",
		SecretKey: "test_secret",
		Region:    "eu-west-1",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned nil client")
	}
	if client.Options().Region != "eu-west-1" {
		t.Errorf("expected region 'eu-west-1', got %q", client.Options().Region)
	}

	// Global must remain untouched.
	configMux.RLock()
	defer configMux.RUnlock()
	if globalConfig != nil {
		t.Error("NewClient must not mutate globalConfig")
	}
	if globalClient != nil {
		t.Error("NewClient must not mutate globalClient")
	}
}

func TestNewClient_RejectsMissingCreds(t *testing.T) {
	Reset()

	cfg := &Config{Region: "us-east-1"} // no creds, UseIMDS=false
	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("NewClient must reject missing creds when UseIMDS=false")
	}
}

func TestSendEmailWith_UsesProvidedClient(t *testing.T) {
	Reset()

	cfg := &Config{
		AccessKey: "AKIA_A",
		SecretKey: "secret_a",
		Region:    "ap-southeast-1",
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Valid in all respects except missing From — targets the sender-validation branch.
	_, err = SendEmailWith(context.Background(), client, &EmailRequest{
		To:       []string{"rcpt@example.com"},
		Subject:  "hi",
		BodyText: "body",
	})
	if err == nil {
		t.Fatal("SendEmailWith must reject request with empty From")
	}
	if !strings.Contains(err.Error(), "sender email") {
		t.Errorf("expected validation error about sender, got: %v", err)
	}
}
