# mail: Multi-Sender Instance API

## Problem

`qtoolkit/mail` is currently a package-level singleton: `Send()` reads `mail.*` viper keys and initializes one `*gomail.Dialer` + one `from` address via `sync.Once`. This serves the common "one service, one sender" case but fails when an application needs **multiple independent sender identities** coexisting in the same process.

Downstream project `k2app` has two classes of outbound mail:

| Class | Config source | From address | SMTP account |
|---|---|---|---|
| Transactional (verification codes, login notices, ticket replies) | `mail.*` | `noreply@kaitu.io` | Account A |
| EDM / marketing | `edm.*` | `promo@kaitu.io` | Account B |

Separating transactional and marketing senders is standard practice for email deliverability — a promotional-domain reputation hit should not degrade delivery of transactional mail. Because the mail singleton only reads one config tree, the EDM path in `k2app` bypassed `qtoolkit` and hand-rolled `net/smtp`. That implementation forgot RFC 2047 Subject encoding, producing UTF-8 → GBK mojibake on Chinese mail clients (live incident).

## Design Decision

Keep the existing public API (`mail.Send(*Message)`, `type Message`, `type Attachment`) unchanged. Add one new entry point — `mail.Config(prefix string) *Sender` — that returns a per-prefix handle whose `Send(*Message)` behaves exactly like the package-level function but reads configuration from `<prefix>.*` instead of `mail.*`.

Internally, replace the four package-level globals (`dialer`, `from`, `useSES`, `once`) with a registry (`sync.Map[string]*sender`) keyed by prefix. The package-level `mail.Send(msg)` is rewritten as `mail.Config("mail").Send(msg)`.

Minimal surface change. Maximum coverage. Existing callers compile and run unchanged.

## Public API

```go
// Config returns a sender handle bound to the given viper key prefix.
// The handle is cacheable and safe for concurrent use.
// An empty prefix is legal to construct but fails at Send() with ErrEmptyPrefix.
func Config(prefix string) *Sender

// Sender.Send has the same signature and semantics as the package-level Send.
func (s *Sender) Send(msg *Message) error

// Equivalent forms
mail.Send(msg)                     // reads mail.*
mail.Config("mail").Send(msg)      // identical

mail.Config("edm").Send(msg)       // reads edm.*
mail.Config("notify").Send(msg)    // reads notify.*

// Sentinel errors
var (
    ErrEmptyPrefix   = errors.New("mail: empty config prefix")
    ErrMissingConfig = errors.New("mail: required config field missing")
)

// Test support — replaces the existing private resetMailer()
func ResetForTest()
```

### Unchanged surface

- `func Send(msg *Message) error`
- `type Message struct { To, Subject, Body string; IsHTML bool; ReplyTo string; Cc []string; Attachments []Attachment }`
- `type Attachment struct { Filename string; Data []byte }`

All field names and semantics preserved. Existing callers require zero code changes.

## Configuration Model

Each prefix is fully self-contained. No cascading fallback between prefixes (cascading across sender identities would make credential-leak troubleshooting much harder — if `edm.smtp_host` is absent, the correct answer is "fail loudly", not "silently send from the transactional account").

```yaml
# Default transactional sender
mail:
  provider: smtp              # smtp | ses
  send_from: noreply@kaitu.io
  smtp_host: smtp.a.com
  smtp_port: 465
  username: user-a
  password: pass-a

# EDM sender on a separate SMTP account
edm:
  provider: smtp
  send_from: promo@kaitu.io
  smtp_host: smtp.b.com
  smtp_port: 465
  username: user-b
  password: pass-b

# Alert sender on SES
notify:
  provider: ses
  send_from: alert@kaitu.io
  region: us-east-1
  access_key: AKIA...
  secret_key: REDACTED
  # or use_imds: true for EC2 instance role
```

Required fields per provider:

| Field | `provider: smtp` | `provider: ses` |
|---|---|---|
| `send_from` | required | required |
| `smtp_host` | required | — |
| `smtp_port` | required | — |
| `username` | required | — |
| `password` | required | — |
| `region` | — | required |
| `access_key` + `secret_key` | — | required unless `use_imds: true` |
| `use_imds` | — | optional (defaults false) |

`provider` defaults to `smtp` when absent or empty (preserves current behavior).

## Internal Structure

```go
type config struct {
    Provider  string
    SendFrom  string
    // SMTP
    SMTPHost  string
    SMTPPort  int
    Username  string
    Password  string
    // SES
    Region    string
    AccessKey string
    SecretKey string
    UseIMDS   bool
}

type sender struct {
    prefix string
    cfg    *config
    smtp   *gomail.Dialer     // when cfg.Provider == "smtp"
    ses    *sesv2.Client      // when cfg.Provider == "ses"
    initOnce sync.Once
    initErr  error
}

// Sender is the exported handle returned by Config().
type Sender struct {
    prefix string  // holds only the prefix; underlying *sender resolved lazily
}

var registry sync.Map  // map[string]*sender
```

Lazy load flow:

1. `Config(prefix)` allocates a lightweight `*Sender{prefix: prefix}` and returns it — **no viper read, no dialer construction**.
2. On first `Sender.Send(msg)`, the sender looks up (or creates-and-stores) a `*sender` in the registry via `LoadOrStore`.
3. The `*sender.initOnce.Do(...)` reads viper keys under `<prefix>.*`, validates required fields, constructs the SMTP dialer or SES client, and caches the result.
4. Subsequent `Send()` calls reuse the cached `*sender`.

Package-level `mail.Send(msg)` becomes:

```go
func Send(msg *Message) error {
    return Config("mail").Send(msg)
}
```

## SES Coupling Change (only change outside the mail module)

Current `mail.go` calls `ses.SendEmail(req)` — a package-level function in `aws/ses` backed by a global `sesv2.Client` singleton. This cannot support multiple SES identities in one process.

Add two stateless helpers to `aws/ses/ses.go`:

```go
// NewClient constructs a sesv2.Client from an explicit Config without touching
// package-level state. Useful for callers that need multiple SES identities.
func NewClient(cfg *Config) (*sesv2.Client, error)

// SendEmailWith sends an email using the provided client, without consulting
// any package-level singleton.
func SendEmailWith(ctx context.Context, client *sesv2.Client, req *EmailRequest) (*EmailResponse, error)
```

Existing `ses.SendEmail(req)` is rewritten as:

```go
func SendEmail(req *EmailRequest) (*EmailResponse, error) {
    clientOnce.Do(initialize)
    if initErr != nil { return nil, initErr }
    return SendEmailWith(context.Background(), globalClient, req)
}
```

Existing `ses.*` callers (direct SES users outside of mail) observe **no behavior change**.

The mail module constructs one `*sesv2.Client` per SES-provider prefix via `ses.NewClient(cfg)` and dispatches through `ses.SendEmailWith`.

## Error Handling

Errors from construction and validation surface at `Send()` — never at `Config()`. This keeps the call-site shape uniform: `mail.Config(x).Send(msg)` always returns exactly one error from exactly one place.

| Trigger | Returned error |
|---|---|
| `Config("")` then `Send(msg)` | `ErrEmptyPrefix` |
| `Config("typo")` where `typo.send_from` is absent | `fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, "typo", "send_from")` |
| `Config("x")` with `x.provider: ses` and `x.region` absent | `fmt.Errorf("%w: prefix=%q field=%q", ErrMissingConfig, "x", "region")` |
| `Config("x")` with `x.provider` present but unrecognized (e.g. `foo`) | `fmt.Errorf("%w: prefix=%q unknown provider=%q", ErrMissingConfig, "x", "foo")` |
| Message validation (existing: empty To / Subject / bad attachment) | Unchanged — same errors as today |
| SMTP send failure | `*gomail.Dialer.DialAndSend` error, unchanged shape |
| SES send failure | `ses.SendEmailWith` error, unchanged shape |

Recognized `provider` values: `smtp` (default when empty), `ses`. Anything else is a hard error at `Send()` — no silent fallback to `smtp`, because a typo like `provider: sees` silently picking SMTP with an unset username would dispatch unauthenticated mail.

## Testing Strategy

### Existing tests

`mail_test.go` has two categories of tests:

**Category A** — tests that only exercise public behavior through `Send()` or validate `*Message` field shape. These continue to work after replacing `resetMailer()`'s body with `registry = sync.Map{}`. Five of the eight tests fall here (`TestSendTextEmail`, `TestSendHtmlEmail`, `TestSendWithReplyTo`, `TestSendWithCc`, `TestSendWithAttachments`, `TestSendValidation`, `TestAttachBytesValidation`, `TestCompleteEmailWithAllFeatures`).

**Category B** — tests that reach into the package globals `dialer`, `from`, `useSES` to verify initialization (`TestMailerInitialization`, `TestSendProviderConfig`, `TestSendProviderDefaultSMTP`). Those globals disappear, so these tests must be rewritten to assert through a private accessor:

```go
// mail_internal_test.go (same package, not exported)
func senderFor(prefix string) *sender {
    v, _ := registry.Load(prefix)
    if v == nil { return nil }
    return v.(*sender)
}
```

The rewritten tests call `Config(prefix).Send(...)` (or a trivial forced-init helper), then assert on `senderFor(prefix).smtp` / `.ses` / `.cfg.SendFrom`. Behavior being verified is identical; only the accessor changes.

### New tests

1. **Prefix isolation** — set `mail.*` and `edm.*` to different SMTP hosts, confirm `Config("mail")` and `Config("edm")` initialize distinct dialers pointed at the correct hosts.
2. **Default equivalence** — confirm `mail.Send(msg)` and `mail.Config("mail").Send(msg)` produce byte-identical SMTP traffic against a loopback server.
3. **Missing config** — `Config("ghost").Send(msg)` returns `ErrMissingConfig` with the right field name.
4. **Empty prefix** — `Config("").Send(msg)` returns `ErrEmptyPrefix`.
5. **SES multi-identity** — set `mail.provider: ses, mail.region: us-east-1` and `edm.provider: ses, edm.region: eu-west-1`, confirm each sender holds its own `*sesv2.Client` with the right region.
6. **Lazy init** — `Config("edm")` alone (no Send) does not read viper or construct a dialer; registry is untouched.
7. **Concurrent init** — 100 goroutines calling `Config("x").Send(msg)` concurrently produce exactly one dialer construction (use atomic counter in test-only hook).

Loopback SMTP server: in-process `net.Listen` + minimal SMTP state machine (read `EHLO`, `MAIL FROM`, `RCPT TO`, `DATA`, terminate). Validates host routing without network.

SES path: the test asserts the cached client's `Options().Region` and credential provider identity — no AWS network calls.

## Breaking Changes

For `qtoolkit/mail` callers: **none**. All public types and functions preserved.

For `qtoolkit/aws/ses` callers: **none**. `ses.SendEmail(req)` keeps its signature; two new functions added.

Internal mail module: `resetMailer()` is private; renamed/replaced `ResetForTest()` is new public. No external consumer affected.

## Migration Guide (k2app)

### Transactional mail (already using qtoolkit)

**No change required.** Existing `mail.Send(&mail.Message{...})` calls work unchanged, reading `mail.*` exactly as before.

### EDM path (currently hand-rolled `net/smtp`)

Delete the hand-rolled SMTP code. Replace with:

```go
err := mail.Config("edm").Send(&mail.Message{
    To:      recipient,
    Subject: subject,
    Body:    htmlBody,
    IsHTML:  true,
    Attachments: []mail.Attachment{
        {Filename: "promo.pdf", Data: pdfData},
    },
})
```

Add `edm.*` block to the application config file (see Configuration Model above).

### Automatic benefits

- **Subject header RFC 2047 encoding** — `gomail` applies it automatically. The Chinese-subject mojibake incident is root-caused and fixed by migration.
- **Uniform observability surface** — both mail paths now flow through the same qtoolkit module. Future additions (metrics, structured logging, retries, bounced-address handling) live in one place.
- **Deliverability hygiene** — the two senders stay operationally separated at the account / domain / IP level; the shared code path does not merge reputation.

## Non-Goals

- **No config cascading across prefixes.** `edm.smtp_host` absent does not fall back to `mail.smtp_host`.
- **No new message features.** BCC, inline images, custom headers beyond `Reply-To` / `Cc` are out of scope.
- **No builder-style message API.** `mail.Send(*Message)` struct-literal form is preserved verbatim.
- **No `io.Reader` attachments.** `Attachment{Filename, Data []byte}` unchanged.
- **No public config struct injection.** viper is the only entry point, consistent with qtoolkit's single-source-of-truth principle.

## Open Questions

None. All decisions locked during brainstorming.
