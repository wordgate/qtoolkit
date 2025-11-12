package unred

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// 测试 viper 配置加载
func TestClientFromViper(t *testing.T) {
	// 重置单例
	clientOnce = sync.Once{}
	defaultClient = nil

	// 设置测试配置
	viper.Set("unred.api_endpoint", "api.x.all7.cc")
	viper.Set("unred.secret_key", "test-secret-key")
	defer viper.Set("unred.api_endpoint", "")
	defer viper.Set("unred.secret_key", "")

	client := initClient()
	if client == nil {
		t.Fatalf("Failed to create client")
	}

	if client.apiEndpoint != "api.x.all7.cc" {
		t.Errorf("Expected api_endpoint 'api.x.all7.cc', got '%s'", client.apiEndpoint)
	}

	if client.secretKey != "test-secret-key" {
		t.Errorf("Expected secret_key 'test-secret-key', got '%s'", client.secretKey)
	}
}

// 测试自定义客户端创建
func TestNewClient(t *testing.T) {
	client := NewClient("api.x.all7.cc", "my-secret-key")

	if client.apiEndpoint != "api.x.all7.cc" {
		t.Errorf("Expected api_endpoint 'api.x.all7.cc', got '%s'", client.apiEndpoint)
	}

	if client.secretKey != "my-secret-key" {
		t.Errorf("Expected secret_key 'my-secret-key', got '%s'", client.secretKey)
	}

	if client.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}

// 测试路径规范化
func TestPathNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test", "/test"},
		{"/test", "/test"},
		{"s/test", "/s/test"},
		{"/s/test", "/s/test"},
	}

	for _, tt := range tests {
		// 这里只是测试路径处理逻辑，不实际发送请求
		path := tt.input
		if path != "" && path[0] != '/' {
			path = "/" + path
		}

		if path != tt.expected {
			t.Errorf("Path normalization failed: input='%s', expected='%s', got='%s'",
				tt.input, tt.expected, path)
		}
	}
}

// 集成测试 - 创建和删除短链接（需要真实的 API 密钥）
func TestCreateAndDeleteLink(t *testing.T) {
	// 跳过集成测试（除非设置了环境变量）
	apiEndpoint := os.Getenv("UNRED_TEST_API_ENDPOINT")
	secretKey := os.Getenv("UNRED_TEST_SECRET_KEY")

	if apiEndpoint == "" || secretKey == "" {
		t.Skip("Skipping integration test: UNRED_TEST_API_ENDPOINT and UNRED_TEST_SECRET_KEY not set")
	}

	client := NewClient(apiEndpoint, secretKey)

	// 创建短链接
	testPath := "/test/integration"
	targetURL := "https://example.com"
	expireAt := time.Now().Add(24 * time.Hour).Unix()

	createResp, err := client.CreateLink(testPath, targetURL, expireAt)
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	if !createResp.Success {
		t.Fatalf("Create link failed: %s", createResp.Message)
	}

	t.Logf("Created link: %s", createResp.URL)

	// 删除短链接
	deleteResp, err := client.DeleteLink(testPath)
	if err != nil {
		t.Fatalf("Failed to delete link: %v", err)
	}

	if !deleteResp.Success {
		t.Fatalf("Delete link failed: %s", deleteResp.Message)
	}

	t.Logf("Deleted link successfully")
}

// 测试批量创建短链接
func TestBatchCreateLinks(t *testing.T) {
	apiEndpoint := os.Getenv("UNRED_TEST_API_ENDPOINT")
	secretKey := os.Getenv("UNRED_TEST_SECRET_KEY")

	if apiEndpoint == "" || secretKey == "" {
		t.Skip("Skipping batch test: UNRED_TEST_API_ENDPOINT and UNRED_TEST_SECRET_KEY not set")
	}

	client := NewClient(apiEndpoint, secretKey)

	// 批量创建
	links := []struct {
		path      string
		targetURL string
	}{
		{"/batch/link1", "https://example.com/1"},
		{"/batch/link2", "https://example.com/2"},
		{"/batch/link3", "https://example.com/3"},
	}

	expireAt := time.Now().Add(24 * time.Hour).Unix()

	for _, link := range links {
		resp, err := client.CreateLink(link.path, link.targetURL, expireAt)
		if err != nil {
			t.Errorf("Failed to create link %s: %v", link.path, err)
			continue
		}

		if !resp.Success {
			t.Errorf("Create link %s failed: %s", link.path, resp.Message)
			continue
		}

		t.Logf("Created link: %s -> %s", link.path, resp.URL)
	}

	// 批量删除
	for _, link := range links {
		resp, err := client.DeleteLink(link.path)
		if err != nil {
			t.Errorf("Failed to delete link %s: %v", link.path, err)
			continue
		}

		if !resp.Success {
			t.Errorf("Delete link %s failed: %s", link.path, resp.Message)
		}
	}
}
