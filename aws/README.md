# AWS Module

This module provides AWS services integration for qtoolkit, including S3 and SES operations.

**IMPORTANT**: This module has been migrated to AWS SDK for Go v2. AWS SDK v1 reaches end-of-support on July 31st, 2025.

## Features

### S3 - Server-Side Upload
- Direct file upload to S3 from server
- Byte data upload support
- Image upload handler with validation

### S3 - Client-Side Upload
- Presigned URLs for direct client uploads
- Both PUT and POST presigned URL generation
- Configurable expiration times
- Simple HTTP handlers for easy integration

### SES - Email Sending
- Simplified email sending via AWS SES
- Multiple authentication methods (credentials, EC2 IAM Role, environment variables)
- Support for text and HTML emails
- CC, BCC, and Reply-To support
- No configuration needed when running on EC2 with IAM Role

## Configuration

Create `aws_config.yml`:

```yaml
aws:
  access_key: "YOUR_AWS_ACCESS_KEY"
  secret_key: "YOUR_AWS_SECRET_KEY"
  region: "us-west-2"

  s3:
    bucket: "your-s3-bucket-name"
    region: "us-west-2"
    url_prefix: "https://your-s3-bucket-name.s3.us-west-2.amazonaws.com"

  ses:
    region: "us-east-1"  # Optional, uses global region if not set
    default_from: "noreply@yourdomain.com"  # Optional
```

**Note for EC2 Users**: When running on EC2 with an IAM Role, you don't need to configure `access_key` and `secret_key`. The SDK will automatically use the IAM Role credentials.

## Usage

### Basic Setup

```go
import "github.com/wordgate/qtoolkit/aws"

// Set configuration (optional on EC2 with IAM Role)
config := &aws.Config{
    AccessKey: "your-access-key",
    SecretKey: "your-secret-key",
    Region:    "us-west-2",
    S3: aws.S3Config{
        Bucket:    "my-bucket",
        Region:    "us-west-2",
        URLPrefix: "https://my-bucket.s3.us-west-2.amazonaws.com",
    },
    SES: aws.SESConfig{
        Region:      "us-east-1",
        DefaultFrom: "noreply@yourdomain.com",
    },
}
aws.SetConfig(config)
```

### Server-Side Upload

```go
// Upload file directly
url, err := aws.S3Upload("path/to/file.jpg", fileReader)

// Upload bytes
url, err := aws.S3UploadBytes("data.json", jsonBytes)

// Gin handler for image uploads
router.POST("/upload", aws.S3HandleImageUpload(
    func(c *gin.Context) string {
        return "uploads/" + uuid.New().String() + ".jpg"
    },
    nil, // no preprocessing
    func(c *gin.Context, url string) error {
        // Save URL to database
        return nil
    },
))
```

### Client-Side Upload (Presigned URLs)

```go
// Generate presigned PUT URL
url, err := aws.S3GeneratePresignedURL("client-upload.jpg", 15*time.Minute)

// Generate presigned POST URL with form data
presignedPost, err := aws.S3GeneratePresignedPOSTURL("client-upload.jpg", 15*time.Minute)

// Gin handlers
router.POST("/api/presign", aws.HandlePresignedURL())
router.POST("/api/presign-post", aws.HandlePresignedPOSTURL())

// Simple HTTP handler
http.HandleFunc("/presign", aws.SimplePresignedURLHandler())
```

### Client-Side Usage Example

```javascript
// Request presigned URL
const response = await fetch('/api/presign', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
        filename: 'my-file.jpg',
        expiration: 30 // minutes
    })
});

const { url, method } = await response.json();

// Upload file directly to S3
await fetch(url, {
    method: method, // 'PUT'
    headers: {
        'Content-Type': 'application/octet-stream'
    },
    body: fileBlob
});
```

## SES Email Sending

### Ultra-Simple Usage (with Default Sender)

If you configure a default sender in your config, you can use the simplest API:

```go
// Configure default sender once
config := &aws.Config{
    Region: "us-east-1",
    SES: aws.SESConfig{
        Region:      "us-east-1",
        DefaultFrom: "noreply@yourdomain.com",  // Set default sender
    },
}
aws.SetConfig(config)

// Then just use 3 parameters!
err := aws.SendMail("user@example.com", "Welcome", "Thank you for signing up!")

// Or send HTML email
err = aws.SendRichMail("user@example.com", "Newsletter", "<h1>Latest News</h1>")
```

### Simple Email (Text)

```go
// Send a simple text email
resp, err := aws.SendSimpleEmail(
    "sender@example.com",
    "recipient@example.com",
    "Hello World",
    "This is a test email from AWS SES!",
)

if err != nil {
    log.Printf("Failed to send email: %v", err)
} else {
    log.Printf("Email sent successfully! MessageID: %s", resp.MessageID)
}
```

### HTML Email

```go
// Send an HTML email
resp, err := aws.SendHTMLEmail(
    "sender@example.com",
    "recipient@example.com",
    "Welcome!",
    "<h1>Welcome to our service!</h1><p>Thank you for signing up.</p>",
)
```

### Advanced Email (with CC, BCC, Reply-To)

```go
// Send email with all options
resp, err := aws.SendEmail(&aws.EmailRequest{
    From:     "noreply@example.com",
    To:       []string{"user1@example.com", "user2@example.com"},
    CC:       []string{"manager@example.com"},
    BCC:      []string{"archive@example.com"},
    ReplyTo:  []string{"support@example.com"},
    Subject:  "Important Update",
    BodyText: "This is the plain text version.",
    BodyHTML: "<h1>This is the HTML version</h1>",
})
```

### Running on EC2 with IAM Role

```go
// No configuration needed! Just call the function directly
// The SDK will automatically use the EC2 instance's IAM Role
resp, err := aws.SendSimpleEmail(
    "noreply@example.com",
    "user@example.com",
    "Test from EC2",
    "This email was sent from an EC2 instance using IAM Role!",
)
```

### Using Environment Variables

```bash
# Set environment variables
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"

# Then run your application - no config needed
```

```go
// The SDK will automatically pick up environment variables
resp, err := aws.SendSimpleEmail(...)
```

## API Reference

### Configuration Types

- `Config`: Main AWS configuration
- `S3Config`: S3-specific settings
- `SESConfig`: SES-specific settings
- `EmailRequest`: Email sending request
- `EmailResponse`: Email sending response

### Functions

#### Server Upload
- `S3Upload(objKey, body)`: Upload file to S3
- `S3UploadBytes(objKey, data)`: Upload byte data
- `S3HandleImageUpload(...)`: Gin handler for image uploads

#### Client Upload
- `S3GeneratePresignedURL(objKey, expiration)`: Generate PUT presigned URL
- `S3GeneratePresignedPOSTURL(objKey, expiration)`: Generate POST presigned URL
- `HandlePresignedURL()`: Gin handler for presigned URLs
- `SimplePresignedURLHandler()`: Simple HTTP handler

#### SES Email
- `SendMail(to, subject, body)`: Send text email using default sender (3 params)
- `SendRichMail(to, subject, htmlBody)`: Send HTML email using default sender (3 params)
- `SendSimpleEmail(from, to, subject, body)`: Send text email with explicit sender
- `SendHTMLEmail(from, to, subject, htmlBody)`: Send HTML email with explicit sender
- `SendEmail(req)`: Send email with full options (CC, BCC, Reply-To, etc.)

### Configuration Management
- `SetConfig(config)`: Set global configuration
- `GetConfig()`: Get current configuration

## Security

- Never commit real credentials to version control
- Use environment variables in production
- **Prefer EC2 IAM Roles** when running on AWS (most secure, no credentials needed)
- Set appropriate S3 bucket policies
- Use minimal IAM permissions
- Limit presigned URL expiration times (max 60 minutes)
- **SES**: Verify sender emails in AWS console
- **SES**: In sandbox mode, recipient emails must also be verified
- **SES**: Request production access to send to any email address

## Testing

```bash
cd aws
go test ./...
```

Note: Integration tests require valid AWS credentials and network access.

## Migration from AWS SDK v1

This module has been migrated to AWS SDK for Go v2. Key changes:

### Breaking Changes

1. **S3HandleImageUpload signature changed**:
   - Old: `beforeUpload func(c *gin.Context, file io.ReadSeeker) (io.ReadSeekCloser, error)`
   - New: `beforeUpload func(c *gin.Context, file io.Reader) (io.ReadCloser, error)`
   - Reason: SDK v2 uses `io.Reader` instead of `io.ReadSeeker`

2. **Internal implementation**:
   - Uses AWS SDK v2 configuration and clients
   - Presigned URLs generated using `s3.NewPresignClient`
   - SES uses `sesv2` service (SES API v2)

### Benefits of SDK v2

- Modular design - only import what you need
- Better performance and smaller binary size
- Continued security updates and support
- Access to newer AWS services and features
- Context-aware API calls

### No API Changes

The public API remains the same:
- `SendMail()`, `SendRichMail()`, `SendEmail()`
- `S3Upload()`, `S3UploadBytes()`
- `S3GeneratePresignedURL()`, `S3GeneratePresignedPOSTURL()`
- `SetConfig()`, `GetConfig()`

Your existing code will continue to work with minimal changes!