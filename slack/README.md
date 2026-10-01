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

### Channel Management

Same conventions as the Bot API above (`ErrNoBotToken`, `ErrAPIFailed`,
`*RateLimitError`).

```go
channelID, err := slack.CreateChannel(ctx, "support-42", false) // conversations.create; true = private
if errors.Is(err, slack.ErrChannelNameTaken) {
    // pick another name (the error also wraps ErrAPIFailed)
}

err = slack.InviteToChannel(ctx, channelID, []string{"U0123", "U0456"}) // conversations.invite
err = slack.SetChannelTopic(ctx, channelID, "Visitor #42")              // conversations.setTopic
members, err := slack.ChannelMembers(ctx, channelID)                    // conversations.members, all pages
err = slack.ArchiveChannel(ctx, channelID)                              // conversations.archive

botID, err := slack.BotUserID(ctx) // auth.test; cached after the first success
```

- `CreateChannel` sends `name` as is: it must already be lowercase, at most
  80 characters, without spaces.
- `InviteToChannel` sends at most 1000 user ids per call and splits longer
  lists; an empty list is a no-op. Every call sets `force: true`, so Slack
  skips ids that are invalid or already in the channel and invites the rest.
  `already_in_channel` is treated as success.
- `ArchiveChannel` treats `already_archived` as success.
- `SetChannelTopic` cuts topics longer than 250 characters to the first 250.
- `BotUserID` caches per bot token for the life of the process; failures are
  not cached.

Scopes (public / private channels):

| Operation | Scopes |
|-----------|--------|
| Create, archive, set topic | `channels:manage` / `groups:write` |
| Invite | `channels:write.invites` / `groups:write.invites` |
| List members | `channels:read` / `groups:read` |

For any other Slack error code, use `*APIError`:

```go
var apiErr *slack.APIError
if errors.As(err, &apiErr) && apiErr.Code == "not_in_channel" {
    // apiErr.Method is the Web API method, e.g. "conversations.invite"
}
```

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

### Channel Management Functions

- `CreateChannel(ctx, name, private) (channelID, error)` - Create a public or private channel
- `InviteToChannel(ctx, channelID, userIDs) error` - Add users to a channel
- `ArchiveChannel(ctx, channelID) error` - Archive a channel
- `ChannelMembers(ctx, channelID) ([]string, error)` - User ids of all members
- `SetChannelTopic(ctx, channelID, topic) error` - Set the channel topic
- `BotUserID(ctx) (string, error)` - The bot's own user id

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
