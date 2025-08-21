# AWS Module

This module provides AWS services integration for qtoolkit, specifically focused on S3 operations.

## Features

### Server-Side Upload
- Direct file upload to S3 from server
- Byte data upload support
- Image upload handler with validation

### Client-Side Upload  
- Presigned URLs for direct client uploads
- Both PUT and POST presigned URL generation
- Configurable expiration times
- Simple HTTP handlers for easy integration

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
```

## Usage

### Basic Setup

```go
import "github.com/wordgate/qtoolkit/aws"

// Set configuration
config := &aws.Config{
    AccessKey: "your-access-key",
    SecretKey: "your-secret-key", 
    Region:    "us-west-2",
    S3: aws.S3Config{
        Bucket:    "my-bucket",
        Region:    "us-west-2",
        URLPrefix: "https://my-bucket.s3.us-west-2.amazonaws.com",
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

## API Reference

### Configuration Types

- `Config`: Main AWS configuration
- `S3Config`: S3-specific settings

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

### Configuration Management
- `SetConfig(config)`: Set global configuration
- `GetConfig()`: Get current configuration

## Security

- Never commit real credentials to version control
- Use environment variables in production
- Set appropriate S3 bucket policies
- Use minimal IAM permissions
- Limit presigned URL expiration times (max 60 minutes)

## Testing

```bash
cd aws
go test ./...
```

Note: Integration tests require valid AWS credentials and network access.