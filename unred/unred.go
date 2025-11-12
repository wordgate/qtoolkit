package unred

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// 全局单例客户端
var (
	defaultClient *Client
	clientOnce    sync.Once
)

// Client Unred API 客户端
type Client struct {
	apiEndpoint string
	secretKey   string
	httpClient  *http.Client
}

// CreateLinkRequest 创建短链接请求
type CreateLinkRequest struct {
	TargetURL string `json:"target_url"`           // 目标 URL
	ExpireAt  int64  `json:"expire_at,omitempty"`  // 过期时间戳（可选）
}

// CreateLinkResponse 创建短链接响应
type CreateLinkResponse struct {
	Success   bool   `json:"success"`
	Subdomain string `json:"subdomain,omitempty"`
	Path      string `json:"path,omitempty"`
	URL       string `json:"url,omitempty"`
	Message   string `json:"message,omitempty"`
}

// DeleteLinkResponse 删除短链接响应
type DeleteLinkResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// initClient initializes the singleton client from viper configuration (lazy load)
func initClient() *Client {
	clientOnce.Do(func() {
		apiEndpoint := viper.GetString("unred.api_endpoint")
		secretKey := viper.GetString("unred.secret_key")

		if apiEndpoint == "" {
			return
		}

		defaultClient = &Client{
			apiEndpoint: strings.TrimSuffix(apiEndpoint, "/"),
			secretKey:   secretKey,
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		}
	})
	return defaultClient
}

// CreateLink 创建短链接
// path: 短链接路径，如 "/s/test" 或 "s/test"
// targetURL: 目标 URL
// expireAt: 过期时间戳（可选，0 表示不设置）
// Configuration is automatically loaded from viper on first use
func CreateLink(path string, targetURL string, expireAt int64) (*CreateLinkResponse, error) {
	client := initClient()
	if client == nil {
		return nil, fmt.Errorf("unred client not configured")
	}

	// 确保 path 以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 构建请求体
	reqBody := CreateLinkRequest{
		TargetURL: targetURL,
	}
	if expireAt > 0 {
		reqBody.ExpireAt = expireAt
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建请求 URL
	url := fmt.Sprintf("https://%s%s", client.apiEndpoint, path)

	// 创建 HTTP 请求
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Secret-Key", client.secretKey)

	// 发送请求
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析响应
	var result CreateLinkResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(respBody))
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &result, fmt.Errorf("api error: status=%d, message=%s", resp.StatusCode, result.Message)
	}

	return &result, nil
}

// DeleteLink 删除短链接
// path: 短链接路径，如 "/s/test" 或 "s/test"
// Configuration is automatically loaded from viper on first use
func DeleteLink(path string) (*DeleteLinkResponse, error) {
	client := initClient()
	if client == nil {
		return nil, fmt.Errorf("unred client not configured")
	}

	// 确保 path 以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 构建请求 URL
	url := fmt.Sprintf("https://%s%s", client.apiEndpoint, path)

	// 创建 HTTP 请求
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Secret-Key", client.secretKey)

	// 发送请求
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析响应
	var result DeleteLinkResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(respBody))
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return &result, fmt.Errorf("api error: status=%d, message=%s", resp.StatusCode, result.Message)
	}

	return &result, nil
}

// NewClient 创建自定义客户端（不使用全局单例）
func NewClient(apiEndpoint, secretKey string) *Client {
	return &Client{
		apiEndpoint: strings.TrimSuffix(apiEndpoint, "/"),
		secretKey:   secretKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateLink 使用自定义客户端创建短链接
func (c *Client) CreateLink(path string, targetURL string, expireAt int64) (*CreateLinkResponse, error) {
	// 确保 path 以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 构建请求体
	reqBody := CreateLinkRequest{
		TargetURL: targetURL,
	}
	if expireAt > 0 {
		reqBody.ExpireAt = expireAt
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建请求 URL
	url := fmt.Sprintf("https://%s%s", c.apiEndpoint, path)

	// 创建 HTTP 请求
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Secret-Key", c.secretKey)

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析响应
	var result CreateLinkResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(respBody))
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return &result, fmt.Errorf("api error: status=%d, message=%s", resp.StatusCode, result.Message)
	}

	return &result, nil
}

// DeleteLink 使用自定义客户端删除短链接
func (c *Client) DeleteLink(path string) (*DeleteLinkResponse, error) {
	// 确保 path 以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 构建请求 URL
	url := fmt.Sprintf("https://%s%s", c.apiEndpoint, path)

	// 创建 HTTP 请求
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Secret-Key", c.secretKey)

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析响应
	var result DeleteLinkResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(respBody))
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return &result, fmt.Errorf("api error: status=%d, message=%s", resp.StatusCode, result.Message)
	}

	return &result, nil
}
