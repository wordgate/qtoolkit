# mail: Config-Driven Provider Selection — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `mail.SetProvider(ses.NewProvider())` with config-driven auto-selection so `mail.Send()` works with zero manual initialization.

**Architecture:** `mail` reads `mail.provider` from viper. When `"ses"`, it maps `mail.Message` → `ses.EmailRequest` and calls `ses.SendEmail()` internally. When empty/`"smtp"`, it uses SMTP as before. The dependency direction flips from `ses → mail` to `mail → ses`.

**Tech Stack:** Go, viper, gomail, aws-sdk-go-v2/sesv2

---

### Task 1: Add EmailAttachment to ses.EmailRequest

**Files:**
- Modify: `aws/ses/ses.go`

- [ ] **Step 1: Define EmailAttachment type and add to EmailRequest**

In `aws/ses/ses.go`, add the new type after `EmailResponse` (around line 43) and add the field to `EmailRequest`:

```go
// EmailAttachment represents an email attachment
type EmailAttachment struct {
	Filename string // Attachment filename
	Data     []byte // Attachment data
}
```

Add to `EmailRequest` struct (after BCC field):

```go
type EmailRequest struct {
	From        string            // Sender email (must be verified in SES)
	To          []string          // Recipient email addresses
	Subject     string            // Email subject
	BodyText    string            // Plain text body (optional if BodyHTML is provided)
	BodyHTML    string            // HTML body (optional if BodyText is provided)
	ReplyTo     []string          // Reply-to addresses (optional)
	CC          []string          // CC addresses (optional)
	BCC         []string          // BCC addresses (optional)
	Attachments []EmailAttachment // Attachments (optional)
}
```

- [ ] **Step 2: Update validateEmailRequest to validate attachments**

Add at the end of `validateEmailRequest()` in `aws/ses/ses.go` (before the final `return nil`):

```go
for _, att := range req.Attachments {
	if att.Filename == "" {
		return fmt.Errorf("attachment filename cannot be empty")
	}
	if len(att.Data) == 0 {
		return fmt.Errorf("attachment data cannot be empty")
	}
}
```

- [ ] **Step 3: Update buildSESv2Input to map attachments**

Add at the end of `buildSESv2Input()` in `aws/ses/ses.go` (before `return input`):

```go
for _, att := range req.Attachments {
	input.Content.Simple.Attachments = append(input.Content.Simple.Attachments, types.Attachment{
		FileName:   strPtr(att.Filename),
		RawContent: att.Data,
	})
}
```

- [ ] **Step 4: Commit**

```bash
git add aws/ses/ses.go
git commit -m "feat(ses): add EmailAttachment to EmailRequest with validation and SES mapping"
```

---

### Task 2: Add ses attachment tests

**Files:**
- Create: `aws/ses/ses_test.go`

- [ ] **Step 1: Write ses_test.go with attachment tests**

Create `aws/ses/ses_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd aws/ses && go test -run "TestBuildSESv2Input|TestValidateEmailRequestAttachments" -v`
Expected: PASS (3 tests)

- [ ] **Step 3: Commit**

```bash
git add aws/ses/ses_test.go
git commit -m "test(ses): add EmailAttachment and buildSESv2Input attachment tests"
```

---

### Task 3: Remove Provider from ses, delete provider_test.go

**Files:**
- Modify: `aws/ses/ses.go` (remove Provider, NewProvider, buildProviderInput)
- Delete: `aws/ses/provider_test.go`

- [ ] **Step 1: Remove Provider, NewProvider, buildProviderInput from ses.go**

Delete lines 299-381 in `aws/ses/ses.go` — the entire block containing:
- `Provider` struct
- `NewProvider()` function
- `Send()` method on Provider
- `buildProviderInput()` function

Also remove the `"github.com/wordgate/qtoolkit/mail"` import.

- [ ] **Step 2: Delete provider_test.go**

```bash
rm aws/ses/provider_test.go
```

- [ ] **Step 3: Run tests to verify ses module still works**

Run: `cd aws/ses && go test ./... -v`
Expected: PASS (ses_test.go tests only)

- [ ] **Step 4: Commit**

```bash
git add aws/ses/ses.go
git rm aws/ses/provider_test.go
git commit -m "refactor(ses): remove Provider type (mail will call ses.SendEmail directly)"
```

---

### Task 4: Modify mail — remove Sender/SetProvider, add config-driven SES

**Files:**
- Modify: `mail/mail.go`

- [ ] **Step 1: Update imports**

Replace the import block:

```go
import (
	"fmt"
	"io"
	"sync"

	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/aws/ses"
	"gopkg.in/gomail.v2"
)
```

- [ ] **Step 2: Replace var block**

Replace the entire var block:

```go
var (
	dialer *gomail.Dialer
	from   string
	useSES bool
	once   sync.Once
)
```

This removes: `provider`, `providerMux`, `Sender` interface, `SetProvider()`.

- [ ] **Step 3: Delete Sender interface and SetProvider function**

Remove the `Sender` interface definition (lines 22-24) and `SetProvider` function (lines 29-33).

- [ ] **Step 4: Replace Send function**

```go
func Send(msg *Message) error {
	if msg.To == "" {
		return fmt.Errorf("recipient (To) is required")
	}
	if msg.Subject == "" {
		return fmt.Errorf("subject is required")
	}

	// Validate attachments
	for _, att := range msg.Attachments {
		if att.Filename == "" {
			return fmt.Errorf("attachment filename cannot be empty")
		}
		if len(att.Data) == 0 {
			return fmt.Errorf("attachment data cannot be empty")
		}
	}

	initMailer()

	if useSES {
		return sendViaSES(msg)
	}
	return sendSMTP(msg)
}
```

- [ ] **Step 5: Update initMailer**

```go
func initMailer() {
	once.Do(func() {
		from = viper.GetString("mail.send_from")
		provider := viper.GetString("mail.provider")
		useSES = provider == "ses"

		if !useSES {
			username := viper.GetString("mail.username")
			password := viper.GetString("mail.password")
			smtpHost := viper.GetString("mail.smtp_host")
			smtpPort := viper.GetInt("mail.smtp_port")
			dialer = gomail.NewDialer(smtpHost, smtpPort, username, password)
		}
	})
}
```

- [ ] **Step 6: Add sendViaSES function**

Add after `sendSMTP`:

```go
func sendViaSES(msg *Message) error {
	req := &ses.EmailRequest{
		From:    from,
		To:      []string{msg.To},
		Subject: msg.Subject,
	}
	if msg.IsHTML {
		req.BodyHTML = msg.Body
	} else {
		req.BodyText = msg.Body
	}
	if len(msg.Cc) > 0 {
		req.CC = msg.Cc
	}
	if msg.ReplyTo != "" {
		req.ReplyTo = []string{msg.ReplyTo}
	}
	for _, att := range msg.Attachments {
		req.Attachments = append(req.Attachments, ses.EmailAttachment{
			Filename: att.Filename,
			Data:     att.Data,
		})
	}
	_, err := ses.SendEmail(req)
	return err
}
```

- [ ] **Step 7: Verify it compiles**

Run: `cd mail && go build ./...`
Expected: Compilation succeeds (may need `go mod tidy` first — see Task 6)

---

### Task 5: Update mail tests

**Files:**
- Modify: `mail/mail_test.go`

- [ ] **Step 1: Remove mock and provider-related code**

Remove from `mail/mail_test.go`:
- `mockSender` struct (lines 13-19)
- `TestSendWithProvider` (lines 271-298)
- `TestSendWithProviderError` (lines 300-319)
- `TestSetProviderNilRevertsToSMTP` (lines 321-342)
- `TestValidationRunsBeforeProvider` (lines 344-372)

- [ ] **Step 2: Update resetMailer**

```go
func resetMailer() {
	once = sync.Once{}
	dialer = nil
	from = ""
	useSES = false
}
```

- [ ] **Step 3: Add provider config test**

```go
func TestSendProviderConfig(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "ses")
	viper.Set("mail.send_from", "test@example.com")

	initMailer()

	if !useSES {
		t.Error("useSES should be true when mail.provider is 'ses'")
	}
	if from != "test@example.com" {
		t.Errorf("expected from 'test@example.com', got '%s'", from)
	}
	if dialer != nil {
		t.Error("dialer should be nil when using SES")
	}
}

func TestSendProviderDefaultSMTP(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "")
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)

	initMailer()

	if useSES {
		t.Error("useSES should be false when mail.provider is empty")
	}
	if dialer == nil {
		t.Error("dialer should be initialized for SMTP")
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd mail && go test ./... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add mail/mail.go mail/mail_test.go
git commit -m "feat(mail): config-driven provider selection, remove Sender/SetProvider"
```

---

### Task 6: Update go.mod for both modules

**Files:**
- Modify: `mail/go.mod`
- Modify: `aws/ses/go.mod`

- [ ] **Step 1: Add ses dependency to mail/go.mod**

```bash
cd mail && go mod tidy
```

This will add `github.com/wordgate/qtoolkit/aws/ses` and its transitive deps (AWS SDK).

- [ ] **Step 2: Clean ses/go.mod**

```bash
cd aws/ses && go mod tidy
```

This will remove `github.com/wordgate/qtoolkit/mail` if it was present.

- [ ] **Step 3: Sync workspace**

```bash
go work sync
```

- [ ] **Step 4: Run all tests across both modules**

```bash
cd mail && go test ./... -v
cd ../aws/ses && go test ./... -v
```

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add mail/go.mod mail/go.sum aws/ses/go.mod aws/ses/go.sum
git commit -m "chore: sync go.mod dependencies for mail→ses direction change"
```

---

### Task 7: Update config files

**Files:**
- Modify: `mail/mail_config.yml`

- [ ] **Step 1: Update mail_config.yml**

```yaml
# Mail Configuration Template
# Add this to your main config.yml file

mail:
  # Provider: "ses" for AWS SES, or omit/leave empty for SMTP (default)
  # provider: ses

  # Sender email address (required for both SMTP and SES)
  send_from: YOUR_EMAIL@example.com

  # SMTP credentials (only needed when provider is not "ses")
  username: YOUR_EMAIL@example.com
  password: YOUR_EMAIL_PASSWORD

  # SMTP server (only needed when provider is not "ses")
  smtp_host: YOUR_SMTP_HOST        # e.g., smtp.gmail.com
  smtp_port: 465                    # 465 for SSL, 587 for TLS

# To use AWS SES:
#   1. Set mail.provider to "ses"
#   2. Configure aws.ses.* (see aws/ses/ses_config.yml)
#   3. Use mail.Send() as usual — it delegates to SES automatically

# Security Notes:
# - Never commit real credentials to version control
# - Rotate passwords regularly
```

- [ ] **Step 2: Commit**

```bash
git add mail/mail_config.yml
git commit -m "docs(mail): update config template for provider selection"
```

---

### Task 8: Final verification

- [ ] **Step 1: Run all workspace tests**

```bash
go test ./mail/... ./aws/ses/... -v
```

Expected: All PASS

- [ ] **Step 2: Verify clean build with workspace**

```bash
go build ./mail/... ./aws/ses/...
```

Expected: No errors

- [ ] **Step 3: Verify no circular dependency**

```bash
cd mail && go mod graph | grep ses
cd ../aws/ses && go mod graph | grep mail
```

Expected: `mail` depends on `ses`, `ses` does NOT depend on `mail`.
