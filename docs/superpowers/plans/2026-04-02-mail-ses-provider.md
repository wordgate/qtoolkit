# mail: SES Provider Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `Sender` interface to `mail/` so callers can inject `aws/ses` as an alternative email backend via `mail.SetProvider()`, with zero changes to the existing SMTP path.

**Architecture:** `mail/` defines `Sender` interface + keeps SMTP as default. `aws/ses/` implements `mail.Sender` as `Provider`, mapping `mail.Message` fields to SES API calls including native attachment support via SESv2 `types.Attachment`. Dependency direction: `aws/ses` -> `mail` (Message type only). No circular deps.

**Tech Stack:** Go 1.24, AWS SDK v2 SESv2, gomail, viper, sync.RWMutex

**Spec:** `docs/superpowers/specs/2026-04-02-mail-ses-provider-design.md`

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `mail/mail.go` | Add `Sender` interface, `SetProvider()`, refactor `Send()` to delegate |
| Modify | `mail/mail_test.go` | Add provider delegation tests, update `resetMailer()` |
| Modify | `aws/ses/ses.go` | Add `Provider` struct implementing `mail.Sender`, attachment mapping |
| Create | `aws/ses/provider_test.go` | Tests for `Provider.Send()` field mapping and attachments |
| Modify | `aws/ses/go.mod` | Add dependency on `github.com/wordgate/qtoolkit/mail` |
| Modify | `mail/mail_config.yml` | Add doc note about provider support |

---

### Task 1: Add Sender Interface and SetProvider to mail/

**Files:**
- Modify: `mail/mail.go:1-16` (package vars), `mail/mail.go:58-100` (Send function)
- Modify: `mail/mail_test.go:13-17` (resetMailer)

- [ ] **Step 1: Write failing test — provider delegation**

Add to `mail/mail_test.go`:

```go
func TestSendWithProvider(t *testing.T) {
	resetMailer()

	var called bool
	var received *Message
	mock := &mockSender{fn: func(msg *Message) error {
		called = true
		received = msg
		return nil
	}}

	SetProvider(mock)

	err := Send(&Message{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("provider Send was not called")
	}
	if received.To != "user@example.com" {
		t.Errorf("expected To 'user@example.com', got '%s'", received.To)
	}
}
```

Also add the `mockSender` type at the top of the test file (after imports):

```go
type mockSender struct {
	fn func(msg *Message) error
}

func (m *mockSender) Send(msg *Message) error {
	return m.fn(msg)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/david/projects/wordgate/qtoolkit/mail && go test -run TestSendWithProvider -v`
Expected: FAIL — `SetProvider` undefined, `Sender` type undefined

- [ ] **Step 3: Implement Sender interface, SetProvider, refactor Send**

In `mail/mail.go`, replace the package-level var block (lines 12-16) with:

```go
var (
	dialer      *gomail.Dialer
	from        string
	once        sync.Once
	provider    Sender
	providerMux sync.RWMutex
)

// Sender defines the interface for sending emails.
// Implement this interface to use a custom email backend (e.g., AWS SES).
type Sender interface {
	Send(msg *Message) error
}

// SetProvider sets a custom email sender. When set, Send() delegates to it
// instead of using the built-in SMTP sender. Pass nil to revert to SMTP.
// Must be called before Send() (typically at application startup).
func SetProvider(s Sender) {
	providerMux.Lock()
	defer providerMux.Unlock()
	provider = s
}
```

Replace the `Send` function (lines 58-100) with:

```go
func Send(msg *Message) error {
	if msg.To == "" {
		return fmt.Errorf("recipient (To) is required")
	}
	if msg.Subject == "" {
		return fmt.Errorf("subject is required")
	}

	providerMux.RLock()
	p := provider
	providerMux.RUnlock()
	if p != nil {
		return p.Send(msg)
	}
	return sendSMTP(msg)
}

// sendSMTP sends email via SMTP using gomail (the built-in default).
func sendSMTP(msg *Message) error {
	initMailer()

	m := gomail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", msg.To)
	m.SetHeader("Subject", msg.Subject)

	contentType := "text/plain"
	if msg.IsHTML {
		contentType = "text/html"
	}
	m.SetBody(contentType, msg.Body)

	if msg.ReplyTo != "" {
		m.SetHeader("Reply-To", msg.ReplyTo)
	}
	if len(msg.Cc) > 0 {
		m.SetHeader("Cc", msg.Cc...)
	}

	for _, att := range msg.Attachments {
		if err := attachBytes(m, att.Filename, att.Data); err != nil {
			return err
		}
	}

	return dialer.DialAndSend(m)
}
```

- [ ] **Step 4: Update resetMailer to also reset provider**

In `mail/mail_test.go`, update `resetMailer()` (lines 13-17):

```go
func resetMailer() {
	once = sync.Once{}
	dialer = nil
	from = ""
	providerMux.Lock()
	provider = nil
	providerMux.Unlock()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /Users/david/projects/wordgate/qtoolkit/mail && go test -run TestSendWithProvider -v`
Expected: PASS

- [ ] **Step 6: Write test — provider error propagation**

Add to `mail/mail_test.go`:

```go
func TestSendWithProviderError(t *testing.T) {
	resetMailer()

	mock := &mockSender{fn: func(msg *Message) error {
		return fmt.Errorf("ses: delivery failed")
	}}
	SetProvider(mock)

	err := Send(&Message{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Hello",
	})
	if err == nil {
		t.Fatal("expected error from provider")
	}
	if err.Error() != "ses: delivery failed" {
		t.Errorf("expected 'ses: delivery failed', got '%s'", err.Error())
	}
}
```

- [ ] **Step 7: Run test to verify it passes**

Run: `cd /Users/david/projects/wordgate/qtoolkit/mail && go test -run TestSendWithProviderError -v`
Expected: PASS (implementation already handles this)

- [ ] **Step 8: Write test — nil provider reverts to SMTP**

Add to `mail/mail_test.go`:

```go
func TestSetProviderNilRevertsToSMTP(t *testing.T) {
	resetMailer()

	var called bool
	mock := &mockSender{fn: func(msg *Message) error {
		called = true
		return nil
	}}

	SetProvider(mock)
	SetProvider(nil) // revert

	// Send should attempt SMTP now (will fail because no real SMTP, but should not call mock)
	_ = Send(&Message{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Hello",
	})
	if called {
		t.Fatal("provider should not be called after SetProvider(nil)")
	}
}
```

- [ ] **Step 9: Run test to verify it passes**

Run: `cd /Users/david/projects/wordgate/qtoolkit/mail && go test -run TestSetProviderNilRevertsToSMTP -v`
Expected: PASS (Send returns SMTP error, but mock was not called)

- [ ] **Step 10: Write test — validation runs before provider**

Add to `mail/mail_test.go`:

```go
func TestValidationRunsBeforeProvider(t *testing.T) {
	resetMailer()

	var called bool
	mock := &mockSender{fn: func(msg *Message) error {
		called = true
		return nil
	}}
	SetProvider(mock)

	// Missing To
	err := Send(&Message{Subject: "Test", Body: "Hello"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if called {
		t.Fatal("provider should not be called when validation fails")
	}

	// Missing Subject
	called = false
	err = Send(&Message{To: "user@example.com", Body: "Hello"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if called {
		t.Fatal("provider should not be called when validation fails")
	}
}
```

- [ ] **Step 11: Run test to verify it passes**

Run: `cd /Users/david/projects/wordgate/qtoolkit/mail && go test -run TestValidationRunsBeforeProvider -v`
Expected: PASS

- [ ] **Step 12: Run all mail tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit/mail && go test -v ./...`
Expected: All tests PASS (existing tests unaffected)

- [ ] **Step 13: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add mail/mail.go mail/mail_test.go
git commit -m "feat(mail): add Sender interface and SetProvider for pluggable backends"
```

---

### Task 2: Add Provider to aws/ses implementing mail.Sender

**Files:**
- Modify: `aws/ses/go.mod` (add mail dependency)
- Modify: `aws/ses/ses.go` (add Provider type, attachment mapping)
- Create: `aws/ses/provider_test.go`

- [ ] **Step 1: Add mail dependency to aws/ses/go.mod**

Run:
```bash
cd /Users/david/projects/wordgate/qtoolkit/aws/ses && go get github.com/wordgate/qtoolkit/mail
```

Then sync workspace:
```bash
cd /Users/david/projects/wordgate/qtoolkit && go work sync
```

- [ ] **Step 2: Write failing test — Provider type and buildProviderInput**

Create `aws/ses/provider_test.go`:

```go
package ses

import (
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/mail"
)

// resetSESState resets all package-level state for test isolation.
func resetSESState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	globalClient = nil
	initErr = nil
	clientOnce = sync.Once{}
}

func TestNewProviderCompiles(t *testing.T) {
	p := NewProvider()
	// Verify Provider satisfies mail.Sender at compile time
	var _ mail.Sender = p
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/david/projects/wordgate/qtoolkit/aws/ses && go test -run TestNewProviderCompiles -v`
Expected: FAIL — `NewProvider` undefined

- [ ] **Step 4: Implement Provider type and Send method**

Add to the end of `aws/ses/ses.go` (before the `Reset()` function):

```go
// Provider implements mail.Sender using AWS SES.
// Use NewProvider() to create, then pass to mail.SetProvider().
type Provider struct{}

// NewProvider creates a new SES email provider.
// SES configuration is loaded from viper on first use (aws.ses.* with aws.* fallback).
func NewProvider() *Provider {
	return &Provider{}
}

// Send implements mail.Sender. It maps mail.Message fields to SES API calls.
func (p *Provider) Send(msg *mail.Message) error {
	client, err := getClient()
	if err != nil {
		return fmt.Errorf("ses provider: %w", err)
	}

	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	from := cfg.DefaultFrom
	if from == "" {
		return fmt.Errorf("ses provider: aws.ses.default_from is required")
	}

	// Build SES input from mail.Message
	input := buildProviderInput(from, msg)

	ctx := context.Background()
	_, err = client.SendEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("ses provider: %w", err)
	}
	return nil
}

// buildProviderInput maps mail.Message to sesv2.SendEmailInput.
func buildProviderInput(from string, msg *mail.Message) *sesv2.SendEmailInput {
	input := &sesv2.SendEmailInput{
		FromEmailAddress: &from,
		Destination: &types.Destination{
			ToAddresses: []string{msg.To},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    &msg.Subject,
					Charset: strPtr("UTF-8"),
				},
				Body: &types.Body{},
			},
		},
	}

	// Body: text or HTML
	if msg.IsHTML {
		input.Content.Simple.Body.Html = &types.Content{
			Data:    &msg.Body,
			Charset: strPtr("UTF-8"),
		}
	} else {
		input.Content.Simple.Body.Text = &types.Content{
			Data:    &msg.Body,
			Charset: strPtr("UTF-8"),
		}
	}

	// CC
	if len(msg.Cc) > 0 {
		input.Destination.CcAddresses = msg.Cc
	}

	// ReplyTo
	if msg.ReplyTo != "" {
		input.ReplyToAddresses = []string{msg.ReplyTo}
	}

	// Attachments (SESv2 native support)
	for _, att := range msg.Attachments {
		input.Content.Simple.Attachments = append(input.Content.Simple.Attachments, types.Attachment{
			FileName:   strPtr(att.Filename),
			RawContent: att.Data,
		})
	}

	return input
}
```

Also add the `mail` import to the import block at the top of `ses.go`:

```go
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
	"github.com/wordgate/qtoolkit/mail"
)
```

- [ ] **Step 5: Run test to verify compilation passes**

Run: `cd /Users/david/projects/wordgate/qtoolkit/aws/ses && go test -run TestNewProviderCompiles -v`
Expected: PASS — Provider type exists and satisfies mail.Sender

- [ ] **Step 6: Write test — Provider.Send validates default_from**

Add to `aws/ses/provider_test.go`:

```go
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
```

- [ ] **Step 7: Run test**

Run: `cd /Users/david/projects/wordgate/qtoolkit/aws/ses && go test -run TestProviderRequiresDefaultFrom -v`
Expected: PASS

- [ ] **Step 8: Write test — buildProviderInput field mapping**

Add to `aws/ses/provider_test.go`:

```go
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

	// No CC, ReplyTo, or Attachments
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
```

- [ ] **Step 9: Run tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit/aws/ses && go test -run TestBuildProviderInput -v`
Expected: All 3 tests PASS

- [ ] **Step 10: Run all aws/ses tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit/aws/ses && go test -v ./...`
Expected: All tests PASS (existing example tests unaffected)

- [ ] **Step 11: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add aws/ses/ses.go aws/ses/provider_test.go aws/ses/go.mod aws/ses/go.sum
git commit -m "feat(aws/ses): add Provider implementing mail.Sender with attachment support"
```

---

### Task 3: Workspace Sync and Cross-Module Verification

**Files:**
- Verify: `go.work` (no changes needed, both modules already listed)

- [ ] **Step 1: Sync workspace**

Run:
```bash
cd /Users/david/projects/wordgate/qtoolkit && go work sync
```

Expected: No errors

- [ ] **Step 2: Verify cross-module compilation**

Run:
```bash
cd /Users/david/projects/wordgate/qtoolkit && go build ./mail/... && go build ./aws/ses/...
```

Expected: Both modules compile cleanly

- [ ] **Step 3: Run all tests across both modules**

Run:
```bash
cd /Users/david/projects/wordgate/qtoolkit/mail && go test -v ./...
cd /Users/david/projects/wordgate/qtoolkit/aws/ses && go test -v ./...
```

Expected: All tests PASS in both modules

- [ ] **Step 4: Verify mail module has no AWS SDK dependency**

Run:
```bash
cd /Users/david/projects/wordgate/qtoolkit/mail && grep -c "aws" go.mod
```

Expected: `0` — mail module must not depend on AWS SDK

- [ ] **Step 5: Update mail_config.yml with provider note**

Replace content of `mail/mail_config.yml` with:

```yaml
# Mail Configuration Template
# Add this to your main config.yml file

# SMTP provider (default, used when no custom provider is set)
mail:
  # Sender email address (required)
  send_from: YOUR_EMAIL@example.com

  # SMTP credentials
  username: YOUR_EMAIL@example.com
  password: YOUR_EMAIL_PASSWORD

  # SMTP server
  smtp_host: YOUR_SMTP_HOST        # e.g., smtp.gmail.com
  smtp_port: 465                    # 465 for SSL, 587 for TLS

# To use AWS SES instead of SMTP:
#   1. Configure aws.ses.* (see aws/ses/ses_config.yml)
#   2. In your code: mail.SetProvider(ses.NewProvider())
#   3. Then use mail.Send() as usual — it delegates to SES

# Security Notes:
# - Never commit real credentials to version control
# - Rotate passwords regularly
```

- [ ] **Step 6: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add mail/mail_config.yml
git commit -m "docs(mail): update config template with SES provider usage note"
```
