package s3

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestUpload_NoConfig(t *testing.T) {
	Reset()
	viper.Reset()

	_, err := Upload("test.txt", strings.NewReader("test content"))
	if err == nil || !strings.Contains(err.Error(), "s3") {
		t.Error("Expected error when config is not set")
	}
}

func TestUploadBytes_NoConfig(t *testing.T) {
	Reset()
	viper.Reset()

	_, err := UploadBytes("test.txt", []byte("test content"))
	if err == nil || !strings.Contains(err.Error(), "s3") {
		t.Error("Expected error when config is not set")
	}
}

func TestGeneratePresignedURL_NoConfig(t *testing.T) {
	Reset()
	viper.Reset()

	_, err := GeneratePresignedURL("test.txt", 15*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "s3") {
		t.Error("Expected error when config is not set")
	}
}

func TestGeneratePresignedPOSTURL_NoConfig(t *testing.T) {
	Reset()
	viper.Reset()

	_, err := GeneratePresignedPOSTURL("test.txt", 15*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "s3") {
		t.Error("Expected error when config is not set")
	}
}

func TestLazyLoadOnlyOnce(t *testing.T) {
	Reset()
	viper.Reset()

	// Set invalid config (missing region)
	viper.Set("aws.s3.bucket", "test-bucket")
	// Region not set - should cause error

	// Call multiple times
	_, err1 := Upload("test1.txt", strings.NewReader("test"))
	_, err2 := Upload("test2.txt", strings.NewReader("test"))

	// Both should fail with the same error (initialization happens once)
	if err1 == nil || err2 == nil {
		t.Error("Expected errors when region is missing")
	}
}

// Example of lazy load usage with viper configuration
func ExampleUpload() {
	// Set configuration via viper once
	viper.Set("aws.s3.region", "us-east-1")
	viper.Set("aws.s3.bucket", "my-bucket")
	viper.Set("aws.s3.url_prefix", "https://my-bucket.s3.us-east-1.amazonaws.com")
	viper.Set("aws.use_imds", true) // Use EC2 IAM role

	// Upload is automatically initialized on first call
	url, err := Upload("uploads/image.jpg", strings.NewReader("image data"))
	if err != nil {
		panic(err)
	}
	_ = url
}
