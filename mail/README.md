# Mail Module

Email functionality module for the qtoolkit library, providing SMTP email sending capabilities.

## Features

- Plain text email sending
- HTML email sending
- Configurable SMTP settings
- Backward compatibility with legacy qtoolkit functions
- SMS placeholder functionality

## Usage

### Using the Service-Based API (Recommended)

```go
package main

import (
    "github.com/wordgate/qtoolkit/mail"
)

func main() {
    // Create mail service with configuration
    config := &mail.MailConfig{
        SendFrom: "sender@example.com",
        Username: "smtp-user@example.com",
        Password: "smtp-password",
        SMTPHost: "smtp.example.com",
        SMTPPort: 587,
    }

    service := mail.NewMailService(config)

    // Send plain text email
    err := service.SendMail("recipient@example.com", "Subject", "Plain text content")
    if err != nil {
        panic(err)
    }

    // Send HTML email
    htmlContent := "<h1>Hello</h1><p>This is an HTML email</p>"
    err = service.SendRichMail("recipient@example.com", "HTML Subject", htmlContent)
    if err != nil {
        panic(err)
    }
}
```

### Using Viper Configuration

```go
package main

import (
    "github.com/spf13/viper"
    "github.com/wordgate/qtoolkit/mail"
)

func main() {
    // Configure viper
    viper.Set("mail.send_from", "sender@example.com")
    viper.Set("mail.username", "smtp-user@example.com")
    viper.Set("mail.password", "smtp-password")
    viper.Set("mail.smtp_host", "smtp.example.com")
    viper.Set("mail.smtp_port", 587)

    // Create service from viper
    service := mail.NewMailServiceFromViper()

    err := service.SendMail("recipient@example.com", "Subject", "Content")
    if err != nil {
        panic(err)
    }
}
```

### Legacy API (Backward Compatibility)

```go
package main

import (
    "github.com/spf13/viper"
    "github.com/wordgate/qtoolkit/mail"
)

func main() {
    // Configure viper
    viper.Set("mail.send_from", "sender@example.com")
    viper.Set("mail.username", "smtp-user@example.com")
    viper.Set("mail.password", "smtp-password")
    viper.Set("mail.smtp_host", "smtp.example.com")
    viper.Set("mail.smtp_port", 587)

    // Use legacy functions (automatically initializes from viper)
    err := mail.SendMail("recipient@example.com", "Subject", "Content")
    if err != nil {
        panic(err)
    }

    err = mail.SendRichMail("recipient@example.com", "HTML Subject", "<h1>HTML Content</h1>")
    if err != nil {
        panic(err)
    }
}
```

## Configuration

### Configuration File

Create a `mail_config.yml` file:

```yaml
mail:
  send_from: YOUR_EMAIL@example.com
  username: YOUR_EMAIL@example.com
  password: YOUR_EMAIL_PASSWORD
  smtp_host: YOUR_SMTP_HOST
  smtp_port: 465
```

### Environment Variables

You can override configuration using environment variables:
- `MAIL_SEND_FROM`
- `MAIL_USERNAME`
- `MAIL_PASSWORD`
- `MAIL_SMTP_HOST`
- `MAIL_SMTP_PORT`

## Testing

Run tests for the mail module:

```bash
go test ./mail/...
```

## Dependencies

- `github.com/spf13/viper` - Configuration management
- `gopkg.in/gomail.v2` - SMTP email sending

## Migration from v0.x

This module replaces the mail functionality that was previously in the root qtoolkit package. The legacy functions are still available for backward compatibility but it's recommended to use the new service-based API for new code.

### Migration Steps

1. Replace `qtoolkit.SendMail()` with `mail.SendMail()`
2. Replace `qtoolkit.SendRichMail()` with `mail.SendRichMail()`
3. Consider migrating to the service-based API for better testability and configuration management

## License

Part of the qtoolkit library for the WordGate platform.