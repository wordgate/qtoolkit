package s3

import (
	"strings"
	"testing"
	"time"
)

func TestUpload_NoConfig(t *testing.T) {
	Reset()

	_, err := Upload("test.txt", strings.NewReader("test content"))
	if err == nil || !strings.Contains(err.Error(), "S3 config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestUploadBytes_NoConfig(t *testing.T) {
	Reset()

	_, err := UploadBytes("test.txt", []byte("test content"))
	if err == nil || !strings.Contains(err.Error(), "S3 config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestGeneratePresignedURL_NoConfig(t *testing.T) {
	Reset()

	_, err := GeneratePresignedURL("test.txt", 15*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "S3 config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestGeneratePresignedPOSTURL_NoConfig(t *testing.T) {
	Reset()

	_, err := GeneratePresignedPOSTURL("test.txt", 15*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "S3 config not set") {
		t.Error("Expected error when config is not set")
	}
}

func TestSetAndGetConfig(t *testing.T) {
	Reset()

	config := &Config{
		AccessKey: "AKIA_TEST_KEY",
		SecretKey: "test-secret-key",
		Bucket:    "test-bucket",
		Region:    "us-west-2",
		URLPrefix: "https://test-bucket.s3.us-west-2.amazonaws.com",
	}

	SetConfig(config)
	retrieved := GetConfig()

	if retrieved == nil {
		t.Error("Expected config to be set, got nil")
	}
	if retrieved.AccessKey != "AKIA_TEST_KEY" {
		t.Errorf("Expected AccessKey 'AKIA_TEST_KEY', got '%s'", retrieved.AccessKey)
	}
	if retrieved.Bucket != "test-bucket" {
		t.Errorf("Expected Bucket 'test-bucket', got '%s'", retrieved.Bucket)
	}
}

func TestLazyLoadOnlyOnce(t *testing.T) {
	Reset()

	// Set invalid config (missing region)
	SetConfig(&Config{
		Bucket: "test-bucket",
	})

	// Call multiple times
	_, err1 := Upload("test1.txt", strings.NewReader("test"))
	_, err2 := Upload("test2.txt", strings.NewReader("test"))

	// Both should fail with the same error (initialization happens once)
	if err1 == nil || err2 == nil {
		t.Error("Expected errors when region is missing")
	}
}

// Example of lazy load usage
func ExampleUpload() {
	// Set configuration once
	SetConfig(&Config{
		Region:    "us-east-1",
		Bucket:    "my-bucket",
		URLPrefix: "https://my-bucket.s3.us-east-1.amazonaws.com",
		UseIMDS:   true, // Use EC2 IAM role
	})

	// Upload is automatically initialized on first call
	url, err := Upload("uploads/image.jpg", strings.NewReader("image data"))
	if err != nil {
		panic(err)
	}
	_ = url
}
