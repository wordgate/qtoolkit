package ses

import "testing"

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
