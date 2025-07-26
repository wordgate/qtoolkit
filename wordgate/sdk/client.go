/*
package sdk 提供了与Wordgate API进行交互的客户端工具包。

该包可以用于同步产品信息、会员等级信息以及应用配置信息到Wordgate服务。
它支持从配置文件加载配置，从Markdown文件中提取产品信息，以及直接在配置文件中定义产品。

基本用法示例:

	// 加载配置
	config, err := sdk.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建客户端
	client := sdk.NewClient(config, "/path/to/config/dir")

	// 执行同步
	result, err := client.SyncAll()
	if err != nil {
		log.Fatalf("同步失败: %v", err)
	}

	// 处理结果
	fmt.Printf("同步状态: %v\n", result.Success)

Configuration File Format:

	wordgate:
	  base_url: "https://api.wordgate.example.com"
	  appCode: "your-app-code"
	  app_secret: "your-app-secret"
	  enable_purchase: true

	  app:
	    name: "您的应用名称"
	    description: "应用描述"
	    currency: "CNY"

	  products:
	    files:
	      - "content/courses/*.md"
	    items:
	      - code: "PRODUCT001"
	        name: "产品名称"
	        price: 9900

详细使用说明和完整的API文档请参考 https://github.com/wordgate/qtoolkit/wordgate/sdk
*/
package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client Wordgate客户端，用于与Wordgate API进行交互
type Client struct {
	// Config 存储Wordgate配置信息
	Config *WordgateConfig
	// HTTPClient 用于发送HTTP请求
	HTTPClient *http.Client
	// ConfigDir 配置文件所在目录，用于解析相对路径
	ConfigDir string
}

// SyncAllResponse 包含全部同步操作的响应信息
type SyncAllResponse struct {
	// Success 表示整体同步是否成功
	Success bool `json:"success"`
	// AppConfig 包含应用配置同步的结果
	AppConfig AppConfigSyncResponse `json:"app_config"`
	// Memberships 包含会员等级同步的结果
	Memberships *MembershipSyncResponse `json:"memberships"`
	// Products 包含产品同步的结果
	Products *ProductSyncResponse `json:"products"`
	// ErrorMessage 当同步失败时的错误信息
	ErrorMessage string `json:"error_message,omitempty"`
}

// DryRunResult 包含干运行操作的结果，不会实际发送同步请求
type DryRunResult struct {
	// AppConfig 应用配置信息
	AppConfig *AppInfo `json:"app_config"`
	// Memberships 会员等级信息列表
	Memberships []MembershipTier `json:"memberships"`
	// Products 产品信息列表
	Products []Product `json:"products"`
}

// APIError 表示API请求返回的错误信息
type APIError struct {
	// Code 错误代码
	Code int `json:"code"`
	// Message 错误信息
	Message string `json:"message"`
}

// NewClient 创建并返回一个新的Wordgate客户端实例
//
// config 参数包含Wordgate的配置信息
// configDir 参数指定配置文件所在的目录，用于解析相对路径
func NewClient(config *WordgateConfig, configDir string) *Client {
	// 验证配置
	err := ValidateConfig(config)
	if err != nil {
		// 在生产环境中，应该返回错误而不是panic
		panic(fmt.Sprintf("无效的Wordgate配置: %v", err))
	}

	// 创建HTTP客户端
	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}

	return &Client{
		Config:     config,
		HTTPClient: httpClient,
		ConfigDir:  configDir,
	}
}

// SyncAll 执行完整的同步流程，包括应用配置、会员等级和产品
//
// 返回一个包含所有同步操作结果的SyncAllResponse和可能的错误
func (c *Client) SyncAll() (*SyncAllResponse, error) {
	result := &SyncAllResponse{
		Success: true,
	}

	// 1. 同步应用配置
	appConfigResp, err := c.SyncAppConfig()
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("同步应用配置失败: %v", err)
		return result, err
	}
	result.AppConfig = *appConfigResp

	// 2. 同步会员等级（可选项）
	if len(c.Config.Membership.Tiers) > 0 {
		membershipResp, err := c.SyncMembershipTiers()
		if err != nil {
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("同步会员等级失败: %v", err)
			return result, err
		}
		result.Memberships = membershipResp
	}

	// 3. 同步产品（可选项）
	hasProducts := len(c.Config.Products.Items) > 0 || len(c.Config.Products.Files) > 0
	if hasProducts {
		productResp, err := c.SyncProductsFromConfig()
		if err != nil {
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("同步产品失败: %v", err)
			return result, err
		}
		result.Products = productResp
	}

	return result, nil
}

// DryRun 执行干运行操作，不发送实际请求，但返回将要同步的数据
//
// 返回一个包含将要同步的所有数据的DryRunResult和可能的错误
func (c *Client) DryRun() (*DryRunResult, error) {
	result := &DryRunResult{
		AppConfig:   &c.Config.App,
		Memberships: []MembershipTier{},
		Products:    []Product{},
	}

	// 添加会员等级，仅当存在时
	if len(c.Config.Membership.Tiers) > 0 {
		result.Memberships = c.Config.Membership.Tiers
	}

	// 处理从文件中获取的产品，仅当文件配置存在时
	if len(c.Config.Products.Files) > 0 {
		processor := NewContentProcessor(c.ConfigDir, c.Config)
		products, err := processor.Process()
		if err != nil {
			return nil, fmt.Errorf("处理产品文件失败: %w", err)
		}

		// 添加从文件中提取的产品
		result.Products = append(result.Products, products...)
	}

	// 添加直接配置的产品，仅当产品项存在时
	if len(c.Config.Products.Items) > 0 {
		result.Products = append(result.Products, c.Config.Products.Items...)
	}

	return result, nil
}

// apiPost 发送POST请求到API
//
// path 参数指定API路径
// body 参数包含请求体数据
func (c *Client) apiPost(path string, body interface{}) (*http.Response, error) {
	return c.apiRequest("POST", path, body)
}

// apiRequest 发送通用请求到API
//
// method 参数指定HTTP方法(GET、POST等)
// path 参数指定API路径
// body 参数包含请求体数据(对于GET请求可以为nil)
func (c *Client) apiRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader

	// 如果有请求体，序列化为JSON
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// 构建完整URL
	url := fmt.Sprintf("%s%s", c.Config.BaseURL, path)

	// 创建HTTP请求
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 设置请求头
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-App-Code", c.Config.AppCode)
	req.Header.Set("X-App-Secret", c.Config.AppSecret)

	// 发送请求
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		fmt.Printf("%s %s -> fail:%s\n", method, url, err.Error())
	}
	fmt.Printf("%s %s -> done\n", method, url)
	return res, err
}

// apiRequestJSON 发送请求并解析JSON响应
//
// method 参数指定HTTP方法(GET、POST等)
// path 参数指定API路径
// body 参数包含请求体数据
// result 参数是用于存储响应的结构体指针
func (c *Client) apiRequestJSON(method, path string, body interface{}, result interface{}) error {
	resp, err := c.apiRequest(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 读取响应内容
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		// 尝试解析错误信息
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return fmt.Errorf("API错误(%d): %s", apiErr.Code, apiErr.Message)
		}
		// 如果无法解析为APIError，返回HTTP错误
		return fmt.Errorf("API请求失败: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析JSON响应到结果结构体
	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("解析响应JSON失败: %w", err)
	}

	return nil
}
