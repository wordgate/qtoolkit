# mail: Configuration-Driven Provider Selection

## Problem

The current `mail.SetProvider(ses.NewProvider())` pattern requires manual initialization at application startup. This violates qtoolkit's core principle: "viper.ReadInConfig() once, all modules lazy-load and are automatically ready to use."

Every other qtoolkit module (slack, db, redis, s3, sqs...) works without manual initialization. mail should too.

## Design Decision

Remove the `Sender` interface and `SetProvider()`. Instead, `mail` reads `mail.provider` from viper config and internally delegates to the correct backend. `mail` directly depends on `aws/ses`.

This trades compile isolation for configuration-driven auto-selection — consistent with how every other qtoolkit module works.

## Configuration

```yaml
# SES mode
mail:
  provider: ses
  send_from: user@example.com

# SMTP mode (default, when provider is absent or "smtp")
mail:
  send_from: user@example.com
  username: user@example.com
  password: xxx
  smtp_host: smtp.gmail.com
  smtp_port: 465
```

- `mail.send_from` is the sender address for both SMTP and SES
- SES credentials/region: read from `aws.ses.*` → `aws.*` (cascading fallback, unchanged)
- `aws.ses.default_from` is no longer needed when using mail — `mail.send_from` takes over

## Dependency Direction Change

```
Before: ses → mail  (ses imported mail.Message)
After:  mail → ses  (mail calls ses.SendEmail)
```

No circular dependency. ses has zero knowledge of mail.

## Changes to `mail/`

### Removed

- `Sender` interface
- `SetProvider()` function
- `provider` / `providerMux` variables

### Modified: `Send()`

```go
func Send(msg *Message) error {
    if msg.To == "" {
        return fmt.Errorf("recipient (To) is required")
    }
    if msg.Subject == "" {
        return fmt.Errorf("subject is required")
    }

    initMailer()

    if useSES {
        return sendViaSES(msg)
    }
    return sendSMTP(msg)
}
```

### Modified: `initMailer()`

```go
var (
    dialer *gomail.Dialer
    from   string
    useSES bool
    once   sync.Once
)

func initMailer() {
    once.Do(func() {
        from = viper.GetString("mail.send_from")
        provider := viper.GetString("mail.provider")
        useSES = provider == "ses"

        if !useSES {
            // SMTP initialization (unchanged)
            username := viper.GetString("mail.username")
            password := viper.GetString("mail.password")
            smtpHost := viper.GetString("mail.smtp_host")
            smtpPort := viper.GetInt("mail.smtp_port")
            dialer = gomail.NewDialer(smtpHost, smtpPort, username, password)
        }
    })
}
```

### New: `sendViaSES()`

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

## Changes to `aws/ses/`

### Removed

- `Provider` struct
- `NewProvider()` function
- `buildProviderInput()` function
- `provider_test.go` file

### Added: Attachment support to `EmailRequest`

```go
type EmailAttachment struct {
    Filename string
    Data     []byte
}

type EmailRequest struct {
    From        string
    To          []string
    Subject     string
    BodyText    string
    BodyHTML    string
    ReplyTo     []string
    CC          []string
    BCC         []string
    Attachments []EmailAttachment  // new
}
```

### Modified: `buildSESv2Input()`

Extended to map `EmailAttachment` → `types.Attachment`:

```go
for _, att := range req.Attachments {
    input.Content.Simple.Attachments = append(input.Content.Simple.Attachments, types.Attachment{
        FileName:   strPtr(att.Filename),
        RawContent: att.Data,
    })
}
```

### Modified: `validateEmailRequest()`

Add attachment validation:

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

## Dependency Changes

### mail/go.mod

Add:
```
require github.com/wordgate/qtoolkit/aws/ses v0.0.0
```

Plus transitive AWS SDK dependencies.

### aws/ses/go.mod

Remove:
```
require github.com/wordgate/qtoolkit/mail v0.0.0
```

## Testing

### mail/mail_test.go

- Remove: `mockSender`, `TestSendWithProvider`, `TestSendWithProviderError`, `TestSetProviderNilRevertsToSMTP`, `TestValidationRunsBeforeProvider`
- Update: `resetMailer()` — remove provider/providerMux reset, add useSES reset
- Add: `TestSendProviderConfig` — verify `mail.provider: ses` sets useSES flag

### aws/ses/provider_test.go

- Delete entire file

### aws/ses/ses_test.go

- Add: `EmailAttachment` field tests
- Add: `buildSESv2Input` attachment mapping tests

## Usage

```go
// Before
mail.SetProvider(ses.NewProvider())
mail.Send(&mail.Message{To: "user@example.com", Subject: "Hi", Body: "Hello"})

// After
mail.Send(&mail.Message{To: "user@example.com", Subject: "Hi", Body: "Hello"})
// provider auto-selected from config
```

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `mail/mail.go` | Modify | Remove Sender/SetProvider/providerMux; add useSES, sendViaSES() |
| `mail/mail_test.go` | Modify | Remove mock/provider tests; add config test |
| `mail/go.mod` | Modify | Add aws/ses dependency |
| `mail/mail_config.yml` | Modify | Add provider config docs |
| `aws/ses/ses.go` | Modify | Remove Provider/NewProvider/buildProviderInput; add EmailAttachment to EmailRequest |
| `aws/ses/provider_test.go` | Delete | Entire file |
| `aws/ses/go.mod` | Modify | Remove mail dependency |
