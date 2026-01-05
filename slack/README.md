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
  bot_token: "xoxb-YOUR-BOT-TOKEN"  # Optional, for DM functionality
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
