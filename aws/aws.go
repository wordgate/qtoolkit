package aws

import (
	"context"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// Config represents AWS configuration
type Config struct {
	AccessKey string    `yaml:"access_key" json:"access_key"`
	SecretKey string    `yaml:"secret_key" json:"secret_key"`
	Region    string    `yaml:"region" json:"region"`
	S3        S3Config  `yaml:"s3" json:"s3"`
	SES       SESConfig `yaml:"ses" json:"ses"`
}

// S3Config represents S3 specific configuration
type S3Config struct {
	Bucket    string `yaml:"bucket" json:"bucket"`
	Region    string `yaml:"region" json:"region"`
	URLPrefix string `yaml:"url_prefix" json:"url_prefix"`
}

// SESConfig represents SES specific configuration
type SESConfig struct {
	Region      string `yaml:"region" json:"region"`              // SES region (optional, uses global region if not set)
	DefaultFrom string `yaml:"default_from" json:"default_from"`  // Default sender email (optional)
}

var globalConfig *Config

// SetConfig sets the global AWS configuration
func SetConfig(config *Config) {
	globalConfig = config
}

// GetConfig returns the global AWS configuration
func GetConfig() *Config {
	return globalConfig
}

// loadConfig loads AWS configuration (v2 SDK style)
// It returns awsv2.Config that can be used with AWS SDK v2 clients
func loadConfig(region string) (awsv2.Config, error) {
	ctx := context.Background()

	// If explicit credentials are configured, use them
	if globalConfig != nil && globalConfig.AccessKey != "" && globalConfig.SecretKey != "" {
		return config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				globalConfig.AccessKey,
				globalConfig.SecretKey,
				"",
			)),
		)
	}

	// Otherwise, let AWS SDK auto-discover credentials
	// This will work with:
	// - EC2 IAM Roles (automatic)
	// - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
	// - AWS credentials file (~/.aws/credentials)
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}
