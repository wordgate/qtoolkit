package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

// Config represents S3 configuration
type Config struct {
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	UseIMDS   bool   `yaml:"use_imds" json:"use_imds"`
	Bucket    string `yaml:"bucket" json:"bucket"`
	Region    string `yaml:"region" json:"region"`
	URLPrefix string `yaml:"url_prefix" json:"url_prefix"`
}

var (
	globalConfig *Config
	globalClient *s3.Client
	clientOnce   sync.Once
	initErr      error
	configMux    sync.RWMutex
)

// SetConfig sets the S3 configuration for lazy loading
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	globalConfig = cfg
}

// GetConfig returns the current S3 configuration
func GetConfig() *Config {
	configMux.RLock()
	defer configMux.RUnlock()
	return globalConfig
}

// initialize performs the actual S3 client initialization
func initialize() {
	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil {
		initErr = fmt.Errorf("S3 config not set, call SetConfig() first")
		return
	}

	if cfg.Region == "" {
		initErr = fmt.Errorf("S3 region is required")
		return
	}

	ctx := context.Background()

	// If UseIMDS is explicitly set to false, use static credentials
	var awsCfg awsv2.Config
	var err error

	if !cfg.UseIMDS {
		if cfg.AccessKey != "" && cfg.SecretKey != "" {
			awsCfg, err = config.LoadDefaultConfig(ctx,
				config.WithRegion(cfg.Region),
				config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
					cfg.AccessKey,
					cfg.SecretKey,
					"",
				)),
			)
		} else {
			initErr = fmt.Errorf("UseIMDS is false but AccessKey/SecretKey are not configured")
			return
		}
	} else {
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	}

	if err != nil {
		initErr = fmt.Errorf("failed to load AWS config: %v", err)
		return
	}

	globalClient = s3.NewFromConfig(awsCfg)
	initErr = nil
}

// getClient returns the S3 client with lazy initialization
func getClient() (*s3.Client, error) {
	clientOnce.Do(initialize)
	if initErr != nil {
		return nil, initErr
	}
	return globalClient, nil
}

// Upload uploads a file to S3 and returns the public URL
func Upload(objKey string, body io.Reader) (string, error) {
	client, err := getClient()
	if err != nil {
		return "", err
	}

	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	bucket := cfg.Bucket
	urlPrefix := strings.TrimRight(cfg.URLPrefix, "/") + "/"
	objKey = strings.TrimLeft(objKey, "/")

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

// UploadBytes uploads byte data to S3
func UploadBytes(objKey string, data []byte) (string, error) {
	return Upload(objKey, bytes.NewReader(data))
}

// GeneratePresignedURL generates a presigned URL for client-side upload using SDK v2
func GeneratePresignedURL(objKey string, expiration time.Duration) (string, error) {
	client, err := getClient()
	if err != nil {
		return "", err
	}

	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	bucket := cfg.Bucket
	objKey = strings.TrimLeft(objKey, "/")

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

// GeneratePresignedPOSTURL generates presigned POST URL and form data for client upload
func GeneratePresignedPOSTURL(objKey string, expiration time.Duration) (*PresignedPostData, error) {
	url, err := GeneratePresignedURL(objKey, expiration)
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

// HandleImageUpload handles image upload with validation and processing
func HandleImageUpload(
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

		url, err := Upload(objKey, processedFile)
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

// Reset resets the S3 client and configuration
// This is mainly useful for testing
func Reset() {
	configMux.Lock()
	defer configMux.Unlock()

	globalConfig = nil
	globalClient = nil
	initErr = nil
	clientOnce = sync.Once{}
}
