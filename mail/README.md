# Mail Module

Simple email functionality module for the qtoolkit library with SMTP email sending capabilities.

## Features

- Plain text email sending
- HTML email sending
- Lazy-loaded configuration from Viper
- Thread-safe initialization
- Minimal API - just call `SendMail()` or `SendRichMail()`

## Usage

### Ultra-Simple API

Just configure Viper and call the functions - no service initialization needed!

```go
package main

import (
    "github.com/spf13/viper"
    "github.com/wordgate/qtoolkit/mail"
)

func main() {
    // Configure Viper (once, at application startup)
    viper.Set("mail.send_from", "sender@example.com")
    viper.Set("mail.username", "smtp-user@example.com")
    viper.Set("mail.password", "smtp-password")
    viper.Set("mail.smtp_host", "smtp.example.com")
    viper.Set("mail.smtp_port", 587)

    // Then just send emails directly!
    err := mail.SendMail("recipient@example.com", "Welcome", "Thank you for signing up!")
    if err != nil {
        panic(err)
    }

    // Send HTML email
    htmlContent := "<h1>Hello</h1><p>This is an HTML email</p>"
    err = mail.SendRichMail("recipient@example.com", "Newsletter", htmlContent)
    if err != nil {
        panic(err)
    }
}
```

### Using Configuration File

Create a `mail_config.yml`:

```yaml
mail:
  send_from: YOUR_EMAIL@example.com
  username: YOUR_EMAIL@example.com
  password: YOUR_EMAIL_PASSWORD
  smtp_host: YOUR_SMTP_HOST
  smtp_port: 465
```

Load it with Viper:

```go
viper.SetConfigName("mail_config")
viper.SetConfigType("yaml")
viper.AddConfigPath(".")
viper.ReadInConfig()

// Now just call the functions
err := mail.SendMail("user@example.com", "Subject", "Content")
```

### Advanced Usage - Direct Dialer Access

For advanced use cases, you can get the underlying gomail dialer:

```go
dialer := mail.GetMailer()
// Use dialer for custom email operations
```

## Configuration

### Required Configuration Keys

- `mail.send_from` - Sender email address
- `mail.username` - SMTP username
- `mail.password` - SMTP password
- `mail.smtp_host` - SMTP server hostname
- `mail.smtp_port` - SMTP server port

### Environment Variables

You can override configuration using environment variables with Viper:

```go
viper.SetEnvPrefix("MAIL")
viper.AutomaticEnv()
```

Then set:
- `MAIL_SEND_FROM`
- `MAIL_USERNAME`
- `MAIL_PASSWORD`
- `MAIL_SMTP_HOST`
- `MAIL_SMTP_PORT`

## API Reference

### Functions

- `SendMail(to, subject, content string) error` - Send plain text email
- `SendRichMail(to, subject, html string) error` - Send HTML email
- `GetMailer() *gomail.Dialer` - Get the configured gomail dialer for advanced use

## How It Works

The mail module uses **lazy loading** with `sync.Once`:
- Configuration is loaded from Viper on first use
- The SMTP dialer is created once and reused
- Thread-safe initialization
- No manual service creation needed

## Testing

Run tests for the mail module:

```bash
go test ./mail/...
```

## Dependencies

- `github.com/spf13/viper` - Configuration management
- `gopkg.in/gomail.v2` - SMTP email sending

## Migration from v0.x

This module completely replaces the old mail functionality:

### Old Way (v0.x)
```go
config := &mail.MailConfig{...}
service := mail.NewMailService(config)
service.SendMail(to, subject, content)
```

### New Way (v1.x)
```go
// Just configure Viper
viper.Set("mail.send_from", "...")
// Then call directly
mail.SendMail(to, subject, content)
```

**Benefits:**
- No service struct to manage
- No initialization code needed
- Cleaner, simpler API
- Lazy loading for better performance

## License

Part of the qtoolkit library for the WordGate platform.
