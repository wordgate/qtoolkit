package sdk

import (
	"fmt"
)

// AppConfigSyncResponse 应用配置同步响应
type AppConfigSyncResponse struct {
	// Success 表示同步是否成功
	Success bool `json:"success"`
	// Message 操作消息
	Message string `json:"message"`
}

// SyncAppConfig 同步应用配置
//
// 将配置中定义的应用配置同步到服务器
// 返回同步结果和可能的错误
func (c *Client) SyncAppConfig() (*AppConfigSyncResponse, error) {
	// 同步基本信息
	if err := c.syncAppProfile(); err != nil {
		return nil, fmt.Errorf("同步应用基本信息失败: %w", err)
	}

	// 同步配置信息
	requestData := struct {
		Config AppConfig `json:"config"`
	}{
		Config: c.Config.Config,
	}

	// 创建响应结果
	var response AppConfigSyncResponse

	// 发送请求并解析响应
	if err := c.apiRequestJSON("PUT", "/app/config", requestData, &response); err != nil {
		return nil, fmt.Errorf("同步应用配置失败: %w", err)
	}

	// 设置成功标志
	response.Success = true

	return &response, nil
}

// syncAppProfile 同步应用基本信息
func (c *Client) syncAppProfile() error {
	requestData := struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Currency    string `json:"currency"`
	}{
		Name:        c.Config.App.Name,
		Description: c.Config.App.Description,
		Currency:    c.Config.App.Currency,
	}

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	if err := c.apiRequestJSON("PUT", "/app/profile", requestData, &response); err != nil {
		return fmt.Errorf("同步应用基本信息失败: %w", err)
	}

	return nil
}
