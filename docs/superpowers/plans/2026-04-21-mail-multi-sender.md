# mail Multi-Sender Instance API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `mail.Config(prefix) *Sender` so apps can run multiple independent sender identities (e.g. transactional on `mail.*`, EDM on `edm.*`) through qtoolkit without hand-rolling `net/smtp`.

**Architecture:** Replace the four mail-package globals (`dialer`, `from`, `useSES`, `once`) with a `sync.Map` registry keyed by viper prefix. Each prefix lazily loads a `*sender` holding either a `*gomail.Dialer` (SMTP) or a `*sesv2.Client` (SES). Package-level `mail.Send(msg)` becomes `Config("mail").Send(msg)`. Add two stateless helpers (`ses.NewClient`, `ses.SendEmailWith`) to `aws/ses` so each SES identity gets its own client without touching the global.

**Tech Stack:** Go 1.24.0, gomail.v2, AWS SDK v2 (sesv2), viper, `sync.Map` + `sync.Once`.

**Spec:** `docs/superpowers/specs/2026-04-21-mail-multi-sender-design.md`

---

## File Structure

**Modify:**
- `aws/ses/ses.go` — extract client-build logic into `NewClient(*Config)`; add `SendEmailWith(ctx, client, req)`; refactor existing `initialize()` and `SendEmail()` to reuse them.
- `aws/ses/ses_test.go` — add tests for `NewClient` + `SendEmailWith` that avoid the global.
- `mail/mail.go` — full rewrite: registry, `Sender`, `Config(prefix)`, `ResetForTest`, error sentinels, config loader, provider dispatch. `Message` / `Attachment` types unchanged.
- `mail/mail_test.go` — change `resetMailer()` body; rewrite the three legacy tests that inspected package globals.
- `mail/mail_config.yml` — add multi-prefix example block.
- `mail/README.md` — document `mail.Config(prefix)` usage + multi-identity example.
- `mail/CHANGELOG.md` — v2.1 entry (new API, non-breaking).
- `mail/go.mod` — promote `github.com/aws/aws-sdk-go-v2/service/sesv2` from indirect to direct.

**No new files.** Tests that need the private `senderFor` accessor live in the existing `mail_test.go` (same-package test file).

---

## Task 1: Add stateless SES helpers

Pull the AWS SDK config/client construction out of `initialize()` so `aws/ses` can mint clients on demand without mutating package globals. `SendEmail(req)` still works exactly as before.

**Files:**
- Modify: `aws/ses/ses.go` (around `initialize` lines 95–143, `SendEmail` lines 154–181)
- Test: `aws/ses/ses_test.go`

- [ ] **Step 1: Write failing test for `NewClient`**

Add to `aws/ses/ses_test.go`:

```go
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
```

- [ ] **Step 2: Run test, verify failure**

```bash
cd aws/ses && go test -run TestNewClient -v
```

Expected: FAIL — `NewClient` undefined.

- [ ] **Step 3: Implement `NewClient` in `aws/ses/ses.go`**

Replace the body of `initialize()` (lines 95–143) and add `NewClient`. New content:

```go
// NewClient constructs a sesv2.Client from an explicit Config.
// It does not touch package-level state.
func NewClient(cfg *Config) (*sesv2.Client, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	ctx := context.Background()
	var awsCfg awsv2.Config
	var err error

	if !cfg.UseIMDS {
		if cfg.AccessKey == "" || cfg.SecretKey == "" {
			return nil, fmt.Errorf("UseIMDS is false but AccessKey/SecretKey are not configured")
		}
		awsCfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.AccessKey,
				cfg.SecretKey,
				"",
			)),
		)
	} else {
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	return sesv2.NewFromConfig(awsCfg), nil
}

// initialize performs the actual SES client initialization for the global singleton.
func initialize() {
	cfg, err := loadConfigFromViper()
	if err != nil {
		initErr = fmt.Errorf("failed to load SES config: %v", err)
		return
	}

	configMux.Lock()
	globalConfig = cfg
	configMux.Unlock()

	client, err := NewClient(cfg)
	if err != nil {
		initErr = err
		return
	}
	globalClient = client
	initErr = nil
}
```

- [ ] **Step 4: Run `NewClient` tests, verify pass**

```bash
cd aws/ses && go test -run TestNewClient -v
```

Expected: PASS for both tests.

- [ ] **Step 5: Write failing test for `SendEmailWith`**

Add to `aws/ses/ses_test.go`:

```go
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

	// Validation path — sends request to AWS would require credentials,
	// so we assert that validation runs against the request and that
	// SendEmailWith accepts the client we constructed.
	_, err = SendEmailWith(context.Background(), client, &EmailRequest{
		// Missing From → validation error expected, NOT a network error.
	})
	if err == nil {
		t.Fatal("SendEmailWith must reject invalid request")
	}
	if !strings.Contains(err.Error(), "sender email") {
		t.Errorf("expected validation error about sender, got: %v", err)
	}
}
```

Also add `"strings"` to the test file imports if not present.

- [ ] **Step 6: Run test, verify failure**

```bash
cd aws/ses && go test -run TestSendEmailWith -v
```

Expected: FAIL — `SendEmailWith` undefined.

- [ ] **Step 7: Implement `SendEmailWith` and refactor `SendEmail`**

Replace `SendEmail` (lines 154–181) with:

```go
// SendEmailWith sends an email using the provided client, without consulting
// any package-level singleton. Callers that need multiple SES identities in
// one process should use NewClient + SendEmailWith directly.
func SendEmailWith(ctx context.Context, client *sesv2.Client, req *EmailRequest) (*EmailResponse, error) {
	if err := validateEmailRequest(req); err != nil {
		return &EmailResponse{Success: false, Error: err}, err
	}

	input := buildSESv2Input(req)
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

// SendEmail sends an email using the global SES client (lazy-initialized from viper).
// Existing callers are unaffected by SendEmailWith's introduction.
func SendEmail(req *EmailRequest) (*EmailResponse, error) {
	client, err := getClient()
	if err != nil {
		return &EmailResponse{Success: false, Error: err}, err
	}
	return SendEmailWith(context.Background(), client, req)
}
```

- [ ] **Step 8: Run the entire ses test suite**

```bash
cd aws/ses && go test ./... -v
```

Expected: all existing tests PASS, plus three new ones (`TestNewClient_NoGlobalMutation`, `TestNewClient_RejectsMissingCreds`, `TestSendEmailWith_UsesProvidedClient`).

- [ ] **Step 9: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add aws/ses/ses.go aws/ses/ses_test.go
git commit -m "feat(ses): add NewClient and SendEmailWith for multi-identity callers"
```

---

## Task 2: Refactor mail module to sender registry

Replace the four package-level globals (`dialer`, `from`, `useSES`, `once`) with a registry keyed by viper prefix. Add `Config(prefix) *Sender`. Rewrite `mail.Send(msg)` as `Config("mail").Send(msg)`. Preserve all public types and behaviors. Update the existing test file so the three tests that inspected globals now read through a private `senderFor` accessor.

**Files:**
- Modify: `mail/mail.go` (full rewrite)
- Modify: `mail/mail_test.go` (rewrite `resetMailer` body + three legacy tests)
- Modify: `mail/go.mod` (promote sesv2 to direct)

- [ ] **Step 1: Replace `mail/mail.go` with the new implementation**

```go
package mail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/aws/ses"
	"gopkg.in/gomail.v2"
)

// Sentinel errors.
var (
	ErrEmptyPrefix   = errors.New("mail: empty config prefix")
	ErrMissingConfig = errors.New("mail: required config field missing")
)

// Message is the outbound email payload.
type Message struct {
	To          string       // Recipient
	Subject     string       // Subject line
	Body        string       // Plain text or HTML body
	IsHTML      bool         // Body is HTML when true
	ReplyTo     string       // Optional Reply-To header
	Cc          []string     // Optional CC recipients
	Attachments []Attachment // Optional attachments
}

// Attachment is an in-memory file attached to a Message.
type Attachment struct {
	Filename string
	Data     []byte
}

// Sender is a handle bound to a viper config prefix.
// Returned by Config(prefix). Safe for concurrent use.
type Sender struct {
	prefix string
}

type config struct {
	Provider  string
	SendFrom  string
	SMTPHost  string
	SMTPPort  int
	Username  string
	Password  string
	Region    string
	AccessKey string
	SecretKey string
	UseIMDS   bool
}

type sender struct {
	prefix   string
	cfg      *config
	smtp     *gomail.Dialer
	ses      *sesv2.Client
	initOnce sync.Once
	initErr  error
}

var registry sync.Map // string -> *sender

// Config returns a Sender bound to the given viper key prefix.
//
// The underlying dialer / SES client is lazy-loaded on first Send.
// Passing an empty prefix is legal; it fails at Send() with ErrEmptyPrefix.
//
// Example:
//
//	err := mail.Config("edm").Send(&mail.Message{...})
func Config(prefix string) *Sender {
	return &Sender{prefix: prefix}
}

// Send dispatches msg using the sender identity bound to s.prefix.
func (s *Sender) Send(msg *Message) error {
	if s.prefix == "" {
		return ErrEmptyPrefix
	}
	if err := validateMessage(msg); err != nil {
		return err
	}
	snd, err := resolveSender(s.prefix)
	if err != nil {
		return err
	}
	if snd.cfg.Provider == "ses" {
		return sendViaSES(snd, msg)
	}
	return sendViaSMTP(snd, msg)
}

// Send is the package-level shortcut for Config("mail").Send(msg).
//
// Example:
//
//	// 纯文本邮件
//	mail.Send(&mail.Message{
//	    To:      "user@example.com",
//	    Subject: "Hello",
//	    Body:    "Hello World",
//	})
//
//	// HTML 邮件带附件
//	mail.Send(&mail.Message{
//	    To:      "user@example.com",
//	    Subject: "Report",
//	    Body:    "<h1>Monthly Report</h1>",
//	    IsHTML:  true,
//	    ReplyTo: "noreply@example.com",
//	    Cc:      []string{"boss@example.com"},
//	    Attachments: []mail.Attachment{
//	        {Filename: "report.csv", Data: csvData},
//	    },
//	})
func Send(msg *Message) error {
	return Config("mail").Send(msg)
}

// ResetForTest clears the sender registry. Intended for tests only.
func ResetForTest() {
	registry = sync.Map{}
}

// resolveSender returns (and lazy-initializes) the *sender for prefix.
func resolveSender(prefix string) (*sender, error) {
	v, _ := registry.LoadOrStore(prefix, &sender{prefix: prefix})
	snd := v.(*sender)
	snd.initOnce.Do(func() {
		cfg, err := loadConfig(prefix)
		if err != nil {
			snd.initErr = err
			return
		}
		snd.cfg = cfg
		switch cfg.Provider {
		case "smtp":
			snd.smtp = gomail.NewDialer(cfg.SMTPHost, cfg.SMTPPort, cfg.Username, cfg.Password)
		case "ses":
			client, err := ses.NewClient(&ses.Config{
				AccessKey:   cfg.AccessKey,
				SecretKey:   cfg.SecretKey,
				UseIMDS:     cfg.UseIMDS,
				Region:      cfg.Region,
				DefaultFrom: cfg.SendFrom,
			})
			if err != nil {
				snd.initErr = err
				return
			}
			snd.ses = client
		}
	})
	if snd.initErr != nil {
		return nil, snd.initErr
	}
	return snd, nil
}

// senderFor is a private accessor used by tests.
func senderFor(prefix string) *sender {
	v, _ := registry.Load(prefix)
	if v == nil {
		return nil
	}
	return v.(*sender)
}

// loadConfig reads and validates <prefix>.* from viper.
func loadConfig(prefix string) (*config, error) {
	cfg := &config{
		Provider:  viper.GetString(prefix + ".provider"),
		SendFrom:  viper.GetString(prefix + ".send_from"),
		SMTPHost:  viper.GetString(prefix + ".smtp_host"),
		SMTPPort:  viper.GetInt(prefix + ".smtp_port"),
		Username:  viper.GetString(prefix + ".username"),
		Password:  viper.GetString(prefix + ".password"),
		Region:    viper.GetString(prefix + ".region"),
		AccessKey: viper.GetString(prefix + ".access_key"),
		SecretKey: viper.GetString(prefix + ".secret_key"),
		UseIMDS:   viper.GetBool(prefix + ".use_imds"),
	}
	if cfg.Provider == "" {
		cfg.Provider = "smtp"
	}
	if cfg.SendFrom == "" {
		return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "send_from")
	}
	switch cfg.Provider {
	case "smtp":
		for _, pair := range []struct {
			name, val string
		}{
			{"smtp_host", cfg.SMTPHost},
			{"username", cfg.Username},
			{"password", cfg.Password},
		} {
			if pair.val == "" {
				return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, pair.name)
			}
		}
		if cfg.SMTPPort == 0 {
			return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "smtp_port")
		}
	case "ses":
		if cfg.Region == "" {
			return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "region")
		}
		if !cfg.UseIMDS {
			if cfg.AccessKey == "" {
				return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "access_key")
			}
			if cfg.SecretKey == "" {
				return nil, fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, prefix, "secret_key")
			}
		}
	default:
		return nil, fmt.Errorf("%w: prefix=%q unknown provider=%q", ErrMissingConfig, prefix, cfg.Provider)
	}
	return cfg, nil
}

func validateMessage(msg *Message) error {
	if msg.To == "" {
		return fmt.Errorf("recipient (To) is required")
	}
	if msg.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	for _, att := range msg.Attachments {
		if att.Filename == "" {
			return fmt.Errorf("attachment filename cannot be empty")
		}
		if len(att.Data) == 0 {
			return fmt.Errorf("attachment data cannot be empty")
		}
	}
	return nil
}

func sendViaSMTP(snd *sender, msg *Message) error {
	m := gomail.NewMessage()
	m.SetHeader("From", snd.cfg.SendFrom)
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
	return snd.smtp.DialAndSend(m)
}

func sendViaSES(snd *sender, msg *Message) error {
	req := &ses.EmailRequest{
		From:    snd.cfg.SendFrom,
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
	_, err := ses.SendEmailWith(context.Background(), snd.ses, req)
	return err
}

func attachBytes(m *gomail.Message, filename string, data []byte) error {
	if filename == "" {
		return fmt.Errorf("attachment filename cannot be empty")
	}
	if len(data) == 0 {
		return fmt.Errorf("attachment data cannot be empty")
	}
	m.Attach(filename, gomail.SetCopyFunc(func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}))
	return nil
}
```

- [ ] **Step 2: Rewrite `mail/mail_test.go` helper and legacy tests**

Open `mail/mail_test.go`. Replace the `resetMailer` function (lines 13–18) with:

```go
// resetMailer clears the sender registry. Kept as a local alias so existing
// tests compile with minimal churn.
func resetMailer() {
	ResetForTest()
}
```

Replace `TestMailerInitialization` (lines 260–288) with:

```go
func TestMailerInitialization(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "") // defensive reset; no dependency on prior tests
	viper.Set("mail.send_from", "init@example.com")
	viper.Set("mail.username", "init@example.com")
	viper.Set("mail.password", "initpass")
	viper.Set("mail.smtp_host", "smtp.init.com")
	viper.Set("mail.smtp_port", 465)

	snd1, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd1.smtp == nil {
		t.Fatal("SMTP dialer should be initialized")
	}
	if snd1.cfg.SendFrom != "init@example.com" {
		t.Errorf("expected from 'init@example.com', got %q", snd1.cfg.SendFrom)
	}

	// Second resolve must return the cached *sender.
	snd2, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd2 != snd1 {
		t.Error("resolveSender should return cached sender (one *sender per prefix)")
	}
}
```

Replace `TestSendProviderConfig` (lines 290–306) with:

```go
func TestSendProviderConfig(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "ses")
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.region", "us-east-1")
	viper.Set("mail.access_key", "AKIA_TEST")
	viper.Set("mail.secret_key", "secret_test")

	snd, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd.cfg.Provider != "ses" {
		t.Errorf("expected provider 'ses', got %q", snd.cfg.Provider)
	}
	if snd.cfg.SendFrom != "test@example.com" {
		t.Errorf("expected from 'test@example.com', got %q", snd.cfg.SendFrom)
	}
	if snd.smtp != nil {
		t.Error("SMTP dialer should be nil when provider is 'ses'")
	}
	if snd.ses == nil {
		t.Error("SES client should be non-nil when provider is 'ses'")
	}
}
```

Replace `TestSendProviderDefaultSMTP` (lines 308–325) with:

```go
func TestSendProviderDefaultSMTP(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "")
	viper.Set("mail.send_from", "test@example.com")
	viper.Set("mail.username", "test@example.com")
	viper.Set("mail.password", "testpass")
	viper.Set("mail.smtp_host", "smtp.example.com")
	viper.Set("mail.smtp_port", 587)

	snd, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender returned error: %v", err)
	}
	if snd.cfg.Provider != "smtp" {
		t.Errorf("expected provider to default to 'smtp', got %q", snd.cfg.Provider)
	}
	if snd.smtp == nil {
		t.Error("SMTP dialer should be initialized for SMTP provider")
	}
	if snd.ses != nil {
		t.Error("SES client should be nil for SMTP provider")
	}
}
```

Remove the now-unused `sync` import line check — verify imports at the top of the file still include:

```go
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)
```

(drop the `sync` import; the new `resetMailer` doesn't use it.)

- [ ] **Step 3: Update `mail/go.mod` to promote sesv2 to direct**

Open `mail/go.mod`. Move `github.com/aws/aws-sdk-go-v2/service/sesv2 v1.54.4` from the indirect block into the direct `require` block. The direct block should look like:

```
require (
	github.com/aws/aws-sdk-go-v2/service/sesv2 v1.54.4
	github.com/spf13/viper v1.21.0
	github.com/wordgate/qtoolkit/aws/ses v1.5.25
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
)
```

- [ ] **Step 4: Sync workspace and verify build**

```bash
cd /Users/david/projects/wordgate/qtoolkit
go work sync
cd mail && go build ./...
```

Expected: clean build, no errors.

- [ ] **Step 5: Run existing mail tests**

```bash
cd /Users/david/projects/wordgate/qtoolkit/mail && go test ./... -v
```

Expected: all existing tests PASS — including the three rewritten ones. Fix any issues before proceeding.

- [ ] **Step 6: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add mail/mail.go mail/mail_test.go mail/go.mod mail/go.sum
git commit -m "refactor(mail): replace singleton with prefix-keyed sender registry"
```

---

## Task 3: Add multi-sender public API tests

Verify `Config(prefix)`, error sentinels, prefix isolation, and lazy init through the public surface.

**Files:**
- Modify: `mail/mail_test.go` (append new tests)

- [ ] **Step 1: Add prefix-isolation test**

Append to `mail/mail_test.go`:

```go
func TestConfig_PrefixIsolation(t *testing.T) {
	resetMailer()
	// Explicit provider resets — viper keys persist across tests, and a prior
	// test may have set mail.provider to "ses". Set both to empty so loadConfig
	// defaults to "smtp".
	viper.Set("mail.provider", "")
	viper.Set("edm.provider", "")

	viper.Set("mail.send_from", "tx@example.com")
	viper.Set("mail.username", "tx_user")
	viper.Set("mail.password", "tx_pass")
	viper.Set("mail.smtp_host", "smtp.tx.com")
	viper.Set("mail.smtp_port", 465)

	viper.Set("edm.send_from", "edm@example.com")
	viper.Set("edm.username", "edm_user")
	viper.Set("edm.password", "edm_pass")
	viper.Set("edm.smtp_host", "smtp.edm.com")
	viper.Set("edm.smtp_port", 587)

	txSnd, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolve mail: %v", err)
	}
	edmSnd, err := resolveSender("edm")
	if err != nil {
		t.Fatalf("resolve edm: %v", err)
	}

	if txSnd == edmSnd {
		t.Fatal("mail and edm must resolve to distinct *sender instances")
	}
	if txSnd.cfg.SMTPHost != "smtp.tx.com" {
		t.Errorf("mail host = %q, want smtp.tx.com", txSnd.cfg.SMTPHost)
	}
	if edmSnd.cfg.SMTPHost != "smtp.edm.com" {
		t.Errorf("edm host = %q, want smtp.edm.com", edmSnd.cfg.SMTPHost)
	}
	if txSnd.smtp == edmSnd.smtp {
		t.Error("mail and edm must have distinct *gomail.Dialer instances")
	}
}
```

- [ ] **Step 2: Add empty-prefix test**

```go
func TestConfig_EmptyPrefix(t *testing.T) {
	resetMailer()
	err := Config("").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrEmptyPrefix) {
		t.Errorf("expected ErrEmptyPrefix, got %v", err)
	}
}
```

Add `"errors"` import to the test file if missing.

- [ ] **Step 3: Add missing-config test**

```go
func TestConfig_MissingSendFrom(t *testing.T) {
	resetMailer()
	// ghost prefix has no config at all
	err := Config("ghost").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Errorf("expected ErrMissingConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "send_from") {
		t.Errorf("error should name missing field send_from, got %v", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should name prefix 'ghost', got %v", err)
	}
}

func TestConfig_MissingSMTPHost(t *testing.T) {
	resetMailer()
	viper.Set("partial.send_from", "p@example.com")
	// intentionally omit smtp_host/username/password/smtp_port
	err := Config("partial").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Errorf("expected ErrMissingConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "smtp_host") {
		t.Errorf("error should name missing field smtp_host, got %v", err)
	}
}
```

Add `"strings"` import if missing.

- [ ] **Step 3b: Add SES missing-region test**

```go
func TestConfig_SESMissingRegion(t *testing.T) {
	resetMailer()
	viper.Set("ses_bad.provider", "ses")
	viper.Set("ses_bad.send_from", "x@example.com")
	// intentionally omit region / access_key / secret_key
	err := Config("ses_bad").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("expected ErrMissingConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "region") {
		t.Errorf("error should name missing field region, got %v", err)
	}
}
```

- [ ] **Step 4: Add unknown-provider test**

```go
func TestConfig_UnknownProvider(t *testing.T) {
	resetMailer()
	viper.Set("weird.provider", "pigeon")
	viper.Set("weird.send_from", "p@example.com")
	err := Config("weird").Send(&Message{
		To:      "user@example.com",
		Subject: "test",
		Body:    "test",
	})
	if !errors.Is(err, ErrMissingConfig) {
		t.Errorf("expected ErrMissingConfig for unknown provider, got %v", err)
	}
	if !strings.Contains(err.Error(), "pigeon") {
		t.Errorf("error should name the unknown provider, got %v", err)
	}
}
```

- [ ] **Step 5: Add lazy-init test**

```go
func TestConfig_LazyInit(t *testing.T) {
	resetMailer()
	// Create a Sender handle without triggering resolveSender.
	_ = Config("edm")
	// Registry must still be empty.
	if senderFor("edm") != nil {
		t.Error("Config(prefix) must not populate the registry before Send()")
	}
}
```

- [ ] **Step 6: Add package-level shortcut equivalence test**

```go
func TestSend_EquivalentToConfigMail(t *testing.T) {
	resetMailer()
	viper.Set("mail.provider", "") // defensive: prior tests may have set "ses"
	viper.Set("mail.send_from", "a@example.com")
	viper.Set("mail.username", "a")
	viper.Set("mail.password", "b")
	viper.Set("mail.smtp_host", "smtp.a.com")
	viper.Set("mail.smtp_port", 25)

	// Force init via package-level Send's validation path (we do not
	// actually dial — we only care that both paths hit the same *sender).
	if err := Send(&Message{
		To:      "u@example.com",
		Subject: "s",
		Body:    "b",
	}); err == nil {
		t.Log("Send attempted dial (expected SMTP failure or success); ignoring error")
	}

	direct := senderFor("mail")
	if direct == nil {
		t.Fatal("package-level Send should have resolved the 'mail' sender")
	}

	// Config("mail") must produce a handle whose Send targets the same *sender.
	viaConfig, err := resolveSender("mail")
	if err != nil {
		t.Fatalf("resolveSender: %v", err)
	}
	if viaConfig != direct {
		t.Error("Send(msg) and Config(\"mail\").Send(msg) must share the same *sender")
	}
}
```

- [ ] **Step 7: Run new tests**

```bash
cd /Users/david/projects/wordgate/qtoolkit/mail && go test -run 'TestConfig_|TestSend_Equivalent' -v
```

Expected: all six new tests PASS.

- [ ] **Step 8: Run full mail test suite with race detector**

```bash
cd /Users/david/projects/wordgate/qtoolkit/mail && go test -race ./...
```

Expected: PASS, no race reports.

- [ ] **Step 9: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add mail/mail_test.go
git commit -m "test(mail): cover Config prefix isolation, errors, lazy init"
```

---

## Task 4: Add SES multi-identity test

Prove two SES prefixes build distinct clients with distinct regions, without hitting AWS.

**Files:**
- Modify: `mail/mail_test.go` (append one more test)

- [ ] **Step 1: Add test**

Append to `mail/mail_test.go`:

```go
func TestConfig_SESMultiIdentity(t *testing.T) {
	resetMailer()

	viper.Set("ses_a.provider", "ses")
	viper.Set("ses_a.send_from", "a@example.com")
	viper.Set("ses_a.region", "us-east-1")
	viper.Set("ses_a.access_key", "AKIA_A")
	viper.Set("ses_a.secret_key", "secret_a")

	viper.Set("ses_b.provider", "ses")
	viper.Set("ses_b.send_from", "b@example.com")
	viper.Set("ses_b.region", "eu-west-1")
	viper.Set("ses_b.access_key", "AKIA_B")
	viper.Set("ses_b.secret_key", "secret_b")

	a, err := resolveSender("ses_a")
	if err != nil {
		t.Fatalf("resolve ses_a: %v", err)
	}
	b, err := resolveSender("ses_b")
	if err != nil {
		t.Fatalf("resolve ses_b: %v", err)
	}

	if a.ses == nil || b.ses == nil {
		t.Fatal("both SES senders must have non-nil clients")
	}
	if a.ses == b.ses {
		t.Error("each SES prefix must own its own *sesv2.Client")
	}
	if a.ses.Options().Region != "us-east-1" {
		t.Errorf("ses_a region = %q, want us-east-1", a.ses.Options().Region)
	}
	if b.ses.Options().Region != "eu-west-1" {
		t.Errorf("ses_b region = %q, want eu-west-1", b.ses.Options().Region)
	}
}
```

- [ ] **Step 2: Run test**

```bash
cd /Users/david/projects/wordgate/qtoolkit/mail && go test -run TestConfig_SESMultiIdentity -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add mail/mail_test.go
git commit -m "test(mail): verify SES multi-identity holds distinct clients"
```

---

## Task 5: Update documentation

Reflect the new API in the config template, README, and CHANGELOG. No code changes.

**Files:**
- Modify: `mail/mail_config.yml`
- Modify: `mail/README.md`
- Modify: `mail/CHANGELOG.md`

- [ ] **Step 1: Update `mail/mail_config.yml`**

Replace the entire file with:

```yaml
# Mail Configuration Template
# Add one or more top-level blocks to your main config.yml.
# Each block defines one independent sender identity, addressable via
# mail.Config("<prefix>"). The package-level mail.Send reads the "mail" prefix.

# Default / transactional sender (used by mail.Send)
mail:
  # Provider: "smtp" (default) or "ses"
  # provider: smtp

  # Sender email address (required)
  send_from: YOUR_EMAIL@example.com

  # SMTP settings (required when provider is smtp)
  username: YOUR_EMAIL@example.com
  password: YOUR_EMAIL_PASSWORD
  smtp_host: YOUR_SMTP_HOST          # e.g. smtp.gmail.com
  smtp_port: 465                      # 465 for SSL, 587 for TLS

# Example: separate EDM / marketing sender on a different SMTP account.
# Accessed via mail.Config("edm").Send(&mail.Message{...}).
# edm:
#   provider: smtp
#   send_from: promo@example.com
#   username: promo-user
#   password: promo-pass
#   smtp_host: smtp.promo.example.com
#   smtp_port: 465

# Example: SES-backed sender. No cascading fallback to aws.ses.* —
# each prefix is self-contained.
# notify:
#   provider: ses
#   send_from: alert@example.com
#   region: us-east-1
#   access_key: YOUR_AWS_ACCESS_KEY
#   secret_key: YOUR_AWS_SECRET_KEY
#   # Or, for EC2 instance role:
#   # use_imds: true

# Security Notes:
# - Never commit real credentials to version control.
# - Rotate passwords and API keys regularly.
# - Separate transactional and marketing senders to protect deliverability.
```

- [ ] **Step 2: Update `mail/README.md`**

Insert a new section after the existing "配置" block and before "使用示例":

```markdown
## 多发件身份（Multi-Sender）

默认情况下 `mail.Send` 读取 `mail.*` 配置。若应用需要多个独立发件身份（如事务邮件与 EDM 分账号），在配置里新增顶级块，然后通过 `mail.Config(prefix)` 取回一个 `*Sender` 句柄：

```go
// 事务邮件（读 mail.*）
mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "验证码",
    Body:    "123456",
})

// EDM 邮件（读 edm.*）
mail.Config("edm").Send(&mail.Message{
    To:      "user@example.com",
    Subject: "五月活动",
    Body:    "<h1>Hello</h1>",
    IsHTML:  true,
})

// 句柄可复用，内部按 prefix 缓存 dialer/SES client
edm := mail.Config("edm")
edm.Send(msg1)
edm.Send(msg2)
```

等价：`mail.Send(msg) ≡ mail.Config("mail").Send(msg)`。

每个 prefix 完全自包含，**不走级联兜底**。prefix 配置缺失 / provider 不识别时，`Send()` 返回包裹 `ErrMissingConfig` / `ErrEmptyPrefix` 的错误。

配置示例见 `mail_config.yml`。
```

Also update the "函数" table near the end by inserting a row for `Config`:

```markdown
| `Config(prefix string) *Sender` | 返回绑定到 viper 前缀的 sender 句柄 |
| `(*Sender).Send(msg *Message) error` | 用该身份发送邮件 |
```

- [ ] **Step 3: Update `mail/CHANGELOG.md`**

Prepend a new entry:

```markdown
## v2.1.0 - 多发件身份 (2026-04-21)

### ✨ 新增

`mail.Config(prefix string) *Sender` —— 按 viper 前缀获取发件身份句柄。同一进程可并存多套发件配置（事务 vs EDM vs 告警），每套独立 SMTP 账号或 SES 凭证。

```go
// 事务（读 mail.*，与原有行为一致）
mail.Send(&mail.Message{...})

// EDM（读 edm.*）
mail.Config("edm").Send(&mail.Message{...})
```

等价关系：`mail.Send(msg) ≡ mail.Config("mail").Send(msg)`。

### 💥 非破坏性

- `mail.Send(*Message)` 签名、`Message`、`Attachment` 字段布局完全保留，下游调用方零改动。
- 内部包级全局变量 `dialer / from / useSES / once` 被 prefix-keyed `sync.Map` 替代（对外不可见）。
- `aws/ses` 新增两个 stateless 辅助：`ses.NewClient(cfg)`、`ses.SendEmailWith(ctx, client, req)`。原 `ses.SendEmail(req)` 保留不变。

### 🎯 配置模型

每个 prefix 顶格自包含，**不走级联兜底**：

```yaml
mail:
  provider: smtp
  send_from: noreply@kaitu.io
  smtp_host: smtp.a.com
  smtp_port: 465
  username: user-a
  password: pass-a

edm:
  provider: smtp
  send_from: promo@kaitu.io
  smtp_host: smtp.b.com
  smtp_port: 465
  username: user-b
  password: pass-b
```

### 🔍 错误

- `mail.ErrEmptyPrefix` —— `Config("")` 再 `Send()` 时返回。
- `mail.ErrMissingConfig` —— 必填字段缺失、`provider` 取值不识别。

```

- [ ] **Step 4: Commit**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git add mail/mail_config.yml mail/README.md mail/CHANGELOG.md
git commit -m "docs(mail): document Config() multi-sender API"
```

---

## Task 6: Workspace-wide verification

Make sure the whole repo still builds and tests cleanly.

- [ ] **Step 1: `go work sync` and build everything**

```bash
cd /Users/david/projects/wordgate/qtoolkit
go work sync
go build ./...
```

Expected: no errors.

- [ ] **Step 2: Run all tests across the workspace**

```bash
cd /Users/david/projects/wordgate/qtoolkit
go test ./...
```

Expected: all module test suites PASS. If `aws/ses` or `mail` tests fail, fix and re-commit before proceeding.

- [ ] **Step 3: If any change was needed in step 2, commit it**

```bash
cd /Users/david/projects/wordgate/qtoolkit
git status
# if there are unstaged edits from fixing workspace-level issues:
# git add <files> && git commit -m "fix(mail|ses): resolve workspace build issue"
```

Otherwise this step is a no-op.

---

## Summary of Deliverables

| File | Change |
|---|---|
| `aws/ses/ses.go` | +`NewClient`, +`SendEmailWith`; refactor `initialize` and `SendEmail` to reuse them |
| `aws/ses/ses_test.go` | +3 tests for new helpers |
| `mail/mail.go` | Full rewrite: registry, `Sender`, `Config`, `ResetForTest`, sentinel errors, per-prefix config loader |
| `mail/mail_test.go` | `resetMailer` body swap; 3 legacy tests rewritten; 7 new tests |
| `mail/mail_config.yml` | Multi-prefix example |
| `mail/README.md` | New "多发件身份" section |
| `mail/CHANGELOG.md` | v2.1.0 entry |
| `mail/go.mod` | Promote `sesv2` indirect → direct |
| `docs/superpowers/plans/2026-04-21-mail-multi-sender.md` | This file |

**Six commits** (one per task), no amend.
