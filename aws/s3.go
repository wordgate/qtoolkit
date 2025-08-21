package aws

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

// S3Upload uploads a file to S3 and returns the public URL
func S3Upload(objKey string, body io.ReadSeeker) (string, error) {
	if globalConfig == nil {
		return "", fmt.Errorf("AWS config not set")
	}

	bucket := globalConfig.S3.Bucket
	region := globalConfig.S3.Region
	urlPrefix := strings.TrimRight(globalConfig.S3.URLPrefix, "/") + "/"
	objKey = strings.TrimLeft(objKey, "/")

	sess, err := createSession(region)
	if err != nil {
		return "", err
	}

	svc := s3.New(sess)
	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objKey),
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

// S3GeneratePresignedURL generates a presigned URL for client-side upload
func S3GeneratePresignedURL(objKey string, expiration time.Duration) (string, error) {
	if globalConfig == nil {
		return "", fmt.Errorf("AWS config not set")
	}

	bucket := globalConfig.S3.Bucket
	region := globalConfig.S3.Region
	objKey = strings.TrimLeft(objKey, "/")

	sess, err := createSession(region)
	if err != nil {
		return "", err
	}

	svc := s3.New(sess)
	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objKey),
	})

	return req.Presign(expiration)
}

// PresignedPostData represents presigned POST form data
type PresignedPostData struct {
	URL    string            `json:"url"`
	Fields map[string]string `json:"fields"`
}

// S3GeneratePresignedPOSTURL generates presigned POST URL and form data for client upload
func S3GeneratePresignedPOSTURL(objKey string, expiration time.Duration) (*PresignedPostData, error) {
	if globalConfig == nil {
		return nil, fmt.Errorf("AWS config not set")
	}

	bucket := globalConfig.S3.Bucket
	region := globalConfig.S3.Region
	objKey = strings.TrimLeft(objKey, "/")

	sess, err := createSession(region)
	if err != nil {
		return nil, err
	}

	svc := s3.New(sess)
	
	// Create a presigned POST request using the S3 service
	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objKey),
	})

	// For POST uploads, we'll generate a PUT presigned URL instead
	// as AWS SDK v1 doesn't have PresignedPost method
	url, err := req.Presign(expiration)
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
	beforeUpload func(c *gin.Context, file io.ReadSeeker) (io.ReadSeekCloser, error),
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

		var processedFile io.ReadSeekCloser = f
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