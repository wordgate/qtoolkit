# AWS SQS Module

Simple Queue Service (SQS) client for WordGate qtoolkit.

## Features

- Automatic configuration loading from viper
- Per-queue configuration support
- Message retry mechanism with exponential backoff
- Type-safe message parameter parsing
- Support for both static credentials and EC2 IAM roles (IMDS)

## Configuration

Add to your main `config.yml`:

```yaml
aws:
  sqs:
    # Global settings (used by all queues unless overridden)
    access_key: "YOUR_AWS_ACCESS_KEY"
    secret_key: "YOUR_AWS_SECRET_KEY"
    use_imds: false  # Set true to use EC2 IAM role
    region: "us-east-1"

    # Per-queue settings (optional)
    queues:
      notifications:
        region: "us-east-1"

      background-jobs:
        region: "us-west-2"
        # Can override credentials per queue if needed
        # access_key: "DIFFERENT_KEY"
        # secret_key: "DIFFERENT_SECRET"
```

## Usage

### Basic Usage

```go
package main

import (
    "github.com/wordgate/qtoolkit/aws/sqs"
    "github.com/spf13/viper"
)

func main() {
    // Load your config file
    viper.SetConfigFile("config.yml")
    viper.ReadInConfig()

    // Get SQS client (config is loaded automatically)
    client, err := sqs.Get("notifications")
    if err != nil {
        panic(err)
    }

    // Send a message
    err = client.Send("user.registered", map[string]interface{}{
        "user_id": 123,
        "email": "user@example.com",
    })
}
```

### Message Handling

```go
// Define your message params struct
type UserRegisteredParams struct {
    UserID int    `json:"user_id"`
    Email  string `json:"email"`
}

// Consume messages
client, _ := sqs.Get("notifications")

client.Consume(func(msg sqs.Message) error {
    // Parse typed parameters
    var params UserRegisteredParams
    err := msg.ParseParams(&params)
    if err != nil {
        return err
    }

    // Process the message
    fmt.Printf("User %d registered: %s\n", params.UserID, params.Email)

    // Return error to trigger retry
    // Return nil to mark as successful
    return nil
})
```

### Custom Retry

```go
// Send with custom max retries (default is 3)
err := client.SendWithRetry("task.heavy", params, 5)
```

## Configuration Priority

The module follows this configuration lookup order:

1. Queue-specific config: `aws.sqs.queues.<queueName>`
2. Global SQS config: `aws.sqs`

Each queue can override global settings by specifying values in its own configuration block.

## Error Handling

- Failed messages are automatically retried with exponential backoff
- Retry delays: 1min, 2min, 4min, etc.
- After max retries, errors are logged (implement dead-letter queue as needed)

## Examples

See `sqs_test.go` for more examples.
