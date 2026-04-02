# mail: SES Provider Support via Sender Interface

## Problem

`mail` module only supports SMTP. Users who want AWS SES must use `aws/ses` directly with a different API. Switching providers requires changing all call sites.

## Design Decision

Introduce a `Sender` interface in `mail/`. The `aws/ses` module implements this interface. Callers inject the provider explicitly via `mail.SetProvider()`. Without injection, SMTP is used (100% backward compatible).

This avoids:
- `mail` depending on AWS SDK (compile-on-demand principle)
- Hidden configuration magic (explicit > implicit)
- A third aggregation package (less is more)

## Dependency Direction

```
caller  -->  mail     (interface + SMTP implementation)
caller  -->  aws/ses  (SES implementation + mail.Sender)
aws/ses -->  mail     (Message type only)
```

No circular dependency. `mail` has zero knowledge of `ses`.

## Changes to `mail/`

### New Types

```go
// Sender defines the interface for sending emails.
type Sender interface {
    Send(msg *Message) error
}
```

### New Functions

```go
// SetProvider sets a custom email sender. When set, Send() delegates to it
// instead of using the built-in SMTP sender.
// Must be called before Send() (typically at application startup).
func SetProvider(s Sender)
```

### Concurrency Safety

`provider` is protected by `sync.RWMutex` (consistent with `SetConfig` pattern in other modules):

```go
var (
    provider    Sender
    providerMux sync.RWMutex
)

func SetProvider(s Sender) {
    providerMux.Lock()
    defer providerMux.Unlock()
    provider = s
}
```

`Send()` acquires a read lock to check provider:

```go
func Send(msg *Message) error {
    providerMux.RLock()
    p := provider
    providerMux.RUnlock()
    if p != nil {
        return p.Send(msg)
    }
    return sendSMTP(msg)
}
```

The existing SMTP logic moves from `Send()` to a private `sendSMTP()` function. No behavioral change.

### Message Struct

No changes. Existing `Message` and `Attachment` types remain as-is.

## Changes to `aws/ses/`

### Attachment Support (Simple Path - Native)

SESv2 `types.Message` already supports attachments natively via its `Attachments []Attachment` field. No need for `RawMessage` or manual MIME construction.

```go
// SESv2 types (already in SDK)
type Message struct {
    Body        *Body
    Subject     *Content
    Attachments []Attachment  // native attachment support
}

type Attachment struct {
    FileName    *string
    RawContent  []byte   // SDK handles base64 encoding
    ContentType *string  // e.g., "application/pdf"
}
```

The existing `buildSESv2Input` will be extended to map `mail.Attachment` -> `types.Attachment` when attachments are present. Same Simple path, no branching logic.

### New Provider Type

```go
// Provider implements mail.Sender using AWS SES.
type Provider struct{}

func NewProvider() *Provider {
    return &Provider{}
}

func (p *Provider) Send(msg *mail.Message) error {
    // 1. Validate msg (To, Subject, Body required)
    // 2. Get SES client via getClient() (lazy-loaded)
    // 3. Build sesv2.SendEmailInput:
    //      msg.To       -> Destination.ToAddresses ([]string{msg.To})
    //      msg.Subject  -> Content.Simple.Subject
    //      msg.Body     -> Body.Text (IsHTML=false) or Body.Html (IsHTML=true)
    //      msg.ReplyTo  -> ReplyToAddresses ([]string{msg.ReplyTo})
    //      msg.Cc       -> Destination.CcAddresses
    //      msg.Attachments -> Content.Simple.Attachments (types.Attachment)
    //      From: aws.ses.default_from config
    // 4. Send via client.SendEmail()
}
```

### Dependency Addition

`aws/ses/go.mod` adds:

```
require github.com/wordgate/qtoolkit/mail v0.0.0
```

With workspace (`go.work`), this resolves locally during development.

## Configuration

No new configuration keys. Each module reads its own config:

- SMTP: `mail.*` (send_from, username, password, smtp_host, smtp_port)
- SES: `aws.ses.*` with `aws.*` fallback (access_key, secret_key, region, default_from)

Provider selection is code-level, not config-level. This is intentional: the caller explicitly chooses which backend to use.

## Usage

### SMTP (unchanged)

```go
viper.ReadInConfig()
mail.Send(&mail.Message{To: "user@example.com", Subject: "Hi", Body: "Hello"})
```

### SES

```go
viper.ReadInConfig()
mail.SetProvider(ses.NewProvider())
mail.Send(&mail.Message{To: "user@example.com", Subject: "Hi", Body: "Hello"})
```

### SES with Attachments

```go
mail.SetProvider(ses.NewProvider())
mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "Report",
    Body:    "<h1>See attached</h1>",
    IsHTML:  true,
    Attachments: []mail.Attachment{
        {Filename: "report.pdf", Data: pdfBytes},
    },
})
```

## Testing Strategy

### mail/
- Test that `Send()` without provider uses SMTP (existing tests unchanged)
- Test that `Send()` with a mock `Sender` delegates correctly
- Test `SetProvider(nil)` reverts to SMTP

### aws/ses/
- Test `Provider.Send()` field mapping (Message -> SES input)
- Test attachment mapping (mail.Attachment -> types.Attachment)
- Test that `NewProvider()` uses lazy-loaded SES client
- Test From defaults to aws.ses.default_from config
- Mock SES API via httptest

## Scope Exclusions

- No `mail.provider` config key (provider selection is code-level)
- No build tags or init() magic
- No changes to `mail.Message` struct
- Existing `aws/ses` public API (`SendEmail`, `SendSimpleEmail`, etc.) unchanged
