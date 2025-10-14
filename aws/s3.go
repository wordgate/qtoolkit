package aws

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

// S3Upload uploads a file to S3 and returns the public URL
func S3Upload(objKey string, body io.Reader) (string, error) {
	if globalConfig == nil {
		return "", fmt.Errorf("AWS config not set")
	}

	bucket := globalConfig.S3.Bucket
	region := globalConfig.S3.Region
	urlPrefix := strings.TrimRight(globalConfig.S3.URLPrefix, "/") + "/"
	objKey = strings.TrimLeft(objKey, "/")

	cfg, err := loadConfig(region)
	if err != nil {
		return "", err
	}

	client := s3.NewFromConfig(cfg)
	ctx := context.Background()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: awsv2.String(bucket),
		Key:    awsv2.String(objKey),
		Body:   body,
	})
	if err != nil {
		return "", err
	}

	return urlPrefix + objKey, nil
}

// S3UploadBytes uploads byte data to S3
func S3UploadBytes(objKey string, data []byte) (string, error) {
	return S3Upload(objKey, bytes.NewReader(data))
}

// S3GeneratePresignedURL generates a presigned URL for client-side upload using SDK v2
func S3GeneratePresignedURL(objKey string, expiration time.Duration) (string, error) {
	if globalConfig == nil {
		return "", fmt.Errorf("AWS config not set")
	}

	bucket := globalConfig.S3.Bucket
	region := globalConfig.S3.Region
	objKey = strings.TrimLeft(objKey, "/")

	cfg, err := loadConfig(region)
	if err != nil {
		return "", err
	}

	client := s3.NewFromConfig(cfg)
	presignClient := s3.NewPresignClient(client)

	ctx := context.Background()
	presignResult, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: awsv2.String(bucket),
		Key:    awsv2.String(objKey),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})

	if err != nil {
		return "", err
	}

	return presignResult.URL, nil
}

// PresignedPostData represents presigned POST form data
type PresignedPostData struct {
	URL    string            `json:"url"`
	Fields map[string]string `json:"fields"`
}

// S3GeneratePresignedPOSTURL generates presigned POST URL and form data for client upload
// Note: AWS SDK v2 doesn't have direct POST presign support, so we use PUT presigned URL
func S3GeneratePresignedPOSTURL(objKey string, expiration time.Duration) (*PresignedPostData, error) {
	if globalConfig == nil {
		return nil, fmt.Errorf("AWS config not set")
	}

	// For SDK v2, we'll use PUT presigned URL (same as S3GeneratePresignedURL)
	url, err := S3GeneratePresignedURL(objKey, expiration)
	if err != nil {
		return nil, err
	}

	return &PresignedPostData{
		URL: url,
		Fields: map[string]string{
			"key": objKey,
		},
	}, nil
}

// S3HandleImageUpload handles image upload with validation and processing
func S3HandleImageUpload(
	keyFunc func(c *gin.Context) string,
	beforeUpload func(c *gin.Context, file io.Reader) (io.ReadCloser, error),
	afterUpload func(c *gin.Context, url string) error) gin.HandlerFunc {

	return func(c *gin.Context) {
		objKey := keyFunc(c)

		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(400, gin.H{"error": "file required"})
			return
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		if !(ext == ".jpg" || ext == ".png" || ext == ".jpeg" || ext == ".webp") {
			c.JSON(400, gin.H{"error": "invalid file type"})
			return
		}

		f, err := file.Open()
		if err != nil {
			c.JSON(400, gin.H{"error": "failed to open file"})
			return
		}

		var processedFile io.ReadCloser = f
		if beforeUpload != nil {
			processedFile, err = beforeUpload(c, f)
			if err != nil {
				c.JSON(400, gin.H{"error": "file processing failed"})
				return
			}
		}
		defer processedFile.Close()

		url, err := S3Upload(objKey, processedFile)
		if err != nil {
			c.JSON(500, gin.H{"error": "upload failed"})
			return
		}

		if afterUpload != nil {
			if err := afterUpload(c, url); err != nil {
				c.JSON(500, gin.H{"error": "post-upload processing failed"})
				return
			}
		}

		c.JSON(200, gin.H{"url": url})
	}
}
