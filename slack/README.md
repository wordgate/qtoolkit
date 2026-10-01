# Slack Module

Minimal Slack client for webhook messages and direct messages.

## Features

- **Webhook Messages** - Send messages to Slack channels via webhooks
- **Direct Messages** - Send DMs to users by email address
- **Rich Formatting** - Attachments, colors, fields, timestamps

## Configuration

```yaml
# config.yml
slack:
  webhooks:
    alert: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
    notify: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
  bot_token: "xoxb-YOUR-BOT-TOKEN"  # Optional, for DM and bot API functionality
```

See `slack_config.yml` for the configuration template.

## Usage

### Webhook Messages

```go
import "github.com/wordgate/qtoolkit/slack"

// Simple message
slack.Send("alert", "Server is down!")

// Formatted message
slack.Sendf("alert", "Deploy %s completed", version)

// Rich message with builder
slack.To("alert").
    Text("Deployment completed").
    Color(slack.ColorGood).
    Field("Environment", "production", true).
    Field("Version", "v1.2.3", true).
    Send()
```

### Direct Messages

Requires `bot_token` with `users:read.email` and `chat:write` scopes.

```go
// Simple DM
slack.SendDM("user@example.com", "Hello!")

// Rich DM with builder
slack.DM("user@example.com").
    Text("Your report is ready").
    Color(slack.ColorGood).
    Field("Status", "Complete", true).
    Send()
```

### Bot API (channels, threads, events)

All of these use `bot_token` and return `ErrNoBotToken` if it is empty. A
Slack response of HTTP 200 with `"ok": false` is returned as an error wrapping
`ErrAPIFailed` and carrying Slack's error code. HTTP 429 is returned as
`*RateLimitError`.

```go
// Post to a channel; ts identifies the message (and its thread)
ts, err := slack.PostMessage(ctx, "C0123456789", "New conversation", nil)

// Reply in that thread (ReplyBroadcast also shows it in the channel)
_, err = slack.PostMessage(ctx, "C0123456789", "Visitor: hi", &slack.PostOptions{ThreadTS: ts})

err = slack.UpdateMessage(ctx, "C0123456789", ts, "Conversation (closed)") // chat.update
link, err := slack.GetPermalink(ctx, "C0123456789", ts)                    // chat.getPermalink
err = slack.PinMessage(ctx, "C0123456789", ts)                             // pins.add
email, err := slack.UserEmail(ctx, "U0123456789")                          // users.info

var rl *slack.RateLimitError
if errors.As(err, &rl) {
    time.Sleep(rl.RetryAfter) // 0 if Slack sent no Retry-After header
}
```

Scopes: `chat:write` (post/update), `pins:write` (pin), `users:read` +
`users:read.email` (`UserEmail`).

Verify incoming Events API / slash command / interactivity requests before
parsing them. Pass the **raw** request body:

```go
body, _ := io.ReadAll(r.Body)
if err := slack.VerifySignature(signingSecret, r.Header, body, time.Now()); err != nil {
    http.Error(w, "invalid signature", http.StatusUnauthorized) // errors.Is(err, slack.ErrInvalidSignature)
    return
}
```

Requests whose `X-Slack-Request-Timestamp` is more than 5 minutes from `now`
are rejected. The signing secret is passed by the caller; it is not part of
this module's configuration.

### Colors

```go
slack.ColorGood    // Green
slack.ColorWarning // Yellow
slack.ColorDanger  // Red
```

## API Reference

### Webhook Functions

- `Send(channel, text)` - Send simple text message
- `Sendf(channel, format, args...)` - Send formatted text message
- `To(channel)` - Create message builder

### DM Functions

- `SendDM(email, text)` - Send simple DM
- `DM(email)` - Create DM builder

### Bot API Functions

- `PostMessage(ctx, channelID, text, opts) (ts, error)` - Post a message or thread reply
- `UpdateMessage(ctx, channelID, ts, text) error` - Edit a message
- `GetPermalink(ctx, channelID, ts) (string, error)` - Message permalink
- `PinMessage(ctx, channelID, ts) error` - Pin a message
- `UserEmail(ctx, userID) (string, error)` - Profile email of a user
- `VerifySignature(signingSecret, header, body, now) error` - Verify a Slack request signature

### MessageBuilder Methods

- `.Text(string)` / `.Textf(format, args...)` - Set message text
- `.Color(string)` - Set attachment color
- `.Title(string)` - Set attachment title
- `.Field(title, value, short)` - Add field
- `.Footer(text, icon)` - Set footer
- `.Timestamp(time.Time)` - Set timestamp
- `.Send()` - Send the message

### Utility Functions

- `GetWebhookURL(channel)` - Get webhook URL for channel
- `IsConfigured(channel)` - Check if channel has webhook configured
