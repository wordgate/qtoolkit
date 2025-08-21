package aws

import (
	"strings"
	"testing"
	"time"
)

func TestSetAndGetConfig(t *testing.T) {
	config := &Config{
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Region:    "us-west-2",
		S3: S3Config{
			Bucket:    "test-bucket",
			Region:    "us-west-2",
			URLPrefix: "https://test-bucket.s3.us-west-2.amazonaws.com",
		},
	}

	SetConfig(config)
	retrievedConfig := GetConfig()

	if retrievedConfig.AccessKey != "test-key" {
		t.Errorf("Expected access key 'test-key', got '%s'", retrievedConfig.AccessKey)
	}
	if retrievedConfig.S3.Bucket != "test-bucket" {
		t.Errorf("Expected bucket 'test-bucket', got '%s'", retrievedConfig.S3.Bucket)
	}
}

func TestS3Upload_NoConfig(t *testing.T) {
	// Clear config
	SetConfig(nil)
	
	_, err := S3Upload("test.txt", strings.NewReader("test content"))
	if err == nil || !strings.Contains(err.Error(), "AWS config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestS3UploadBytes_NoConfig(t *testing.T) {
	// Clear config  
	SetConfig(nil)
	
	_, err := S3UploadBytes("test.txt", []byte("test content"))
	if err == nil || !strings.Contains(err.Error(), "AWS config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestS3GeneratePresignedURL_NoConfig(t *testing.T) {
	// Clear config
	SetConfig(nil)
	
	_, err := S3GeneratePresignedURL("test.txt", 15*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "AWS config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestS3GeneratePresignedPOSTURL_NoConfig(t *testing.T) {
	// Clear config
	SetConfig(nil)
	
	_, err := S3GeneratePresignedPOSTURL("test.txt", 15*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "AWS config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestCreateSession_NoConfig(t *testing.T) {
	// Clear config
	SetConfig(nil)
	
	_, err := createSession("us-west-2")
	if err == nil || !strings.Contains(err.Error(), "AWS config not set") {
		t.Error("Expected error when config is not set")
	}
}

// Mock tests with config set
func TestWithMockConfig(t *testing.T) {
	config := &Config{
		AccessKey: "AKIA_TEST_KEY",
		SecretKey: "test-secret-key", 
		Region:    "us-west-2",
		S3: S3Config{
			Bucket:    "test-bucket",
			Region:    "us-west-2", 
			URLPrefix: "https://test-bucket.s3.us-west-2.amazonaws.com",
		},
	}
	SetConfig(config)

	t.Run("CreateSession", func(t *testing.T) {
		session, err := createSession("us-west-2")
		if err != nil {
			t.Errorf("Unexpected error creating session: %v", err)
		}
		if session == nil {
			t.Error("Expected non-nil session")
		}
	})

	// Note: Actual S3 operations would require valid AWS credentials and network access
	// In a real test environment, you might use mocked S3 service or localstack
}