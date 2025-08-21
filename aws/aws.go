package aws

import (
	"fmt"
	
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

// Config represents AWS configuration
type Config struct {
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	Region    string `yaml:"region" json:"region"`
	S3        S3Config `yaml:"s3" json:"s3"`
}

// S3Config represents S3 specific configuration
type S3Config struct {
	Bucket    string `yaml:"bucket" json:"bucket"`
	Region    string `yaml:"region" json:"region"`
	URLPrefix string `yaml:"url_prefix" json:"url_prefix"`
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

// createSession creates an AWS session with the given region
func createSession(region string) (*session.Session, error) {
	if globalConfig == nil {
		return nil, fmt.Errorf("AWS config not set")
	}

	return session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(globalConfig.AccessKey, globalConfig.SecretKey, ""),
		},
		SharedConfigState: session.SharedConfigEnable,
	})
}