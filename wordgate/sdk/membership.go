package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// MembershipSyncRequest 同步会员等级请求
type MembershipSyncRequest struct {
	// Tiers 要同步的会员等级列表
	Tiers []struct {
		// Code 会员等级代码
		Code string `json:"code"`
		// Name 会员等级名称
		Name string `json:"name"`
		// Level 等级值，用于排序
		Level int `json:"level"`
		// IsDefault 是否默认等级
		IsDefault bool `json:"is_default"`
		// Prices 会员价格配置列表
		Prices []struct {
			// PeriodType 周期类型，如 month、year 等
			PeriodType string `json:"period_type"`
			// Price 价格(单位:分)
			Price int64 `json:"price"`
			// OriginalPrice 原价(单位:分)，用于显示折扣信息
			OriginalPrice int64 `json:"original_price"`
		} `json:"prices,omitempty"`
	} `json:"tiers"`
}

// MembershipSyncResponse 同步会员等级响应
type MembershipSyncResponse struct {
	// Success 表示同步是否成功
	Success bool `json:"success"`
	// Total 同步的会员等级总数
	Total int `json:"total"`
	// Created 新创建的会员等级数量
	Created int `json:"created"`
	// Updated 更新的会员等级数量
	Updated int `json:"updated"`
	// Unchanged 未发生变更的会员等级数量
	Unchanged int `json:"unchanged"`
	// Failed 同步失败的会员等级数量
	Failed int `json:"failed"`
	// Errors 失败的详细错误信息
	Errors []struct {
		// Code 错误代码
		Code string `json:"code"`
		// TierCode 会员等级代码
		TierCode string `json:"tier_code"`
		// Message 错误消息
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// SyncMembershipTiers 同步会员等级
//
// 将配置中定义的会员等级同步到服务器
// 返回同步结果和可能的错误
func (c *Client) SyncMembershipTiers() (*MembershipSyncResponse, error) {
	// 检查会员等级配置
	if len(c.Config.Membership.Tiers) == 0 {
		return nil, fmt.Errorf("没有配置会员等级")
	}

	// 构建请求体
	syncRequest := MembershipSyncRequest{
		Tiers: make([]struct {
			Code      string `json:"code"`
			Name      string `json:"name"`
			Level     int    `json:"level"`
			IsDefault bool   `json:"is_default"`
			Prices    []struct {
				PeriodType    string `json:"period_type"`
				Price         int64  `json:"price"`
				OriginalPrice int64  `json:"original_price"`
			} `json:"prices,omitempty"`
		}, len(c.Config.Membership.Tiers)),
	}

	// 填充请求数据
	for i, tier := range c.Config.Membership.Tiers {
		syncRequest.Tiers[i].Code = tier.Code
		syncRequest.Tiers[i].Name = tier.Name
		syncRequest.Tiers[i].Level = tier.Level
		syncRequest.Tiers[i].IsDefault = tier.IsDefault

		syncRequest.Tiers[i].Prices = make([]struct {
			PeriodType    string `json:"period_type"`
			Price         int64  `json:"price"`
			OriginalPrice int64  `json:"original_price"`
		}, len(tier.Prices))

		for j, price := range tier.Prices {
			syncRequest.Tiers[i].Prices[j].PeriodType = price.PeriodType
			syncRequest.Tiers[i].Prices[j].Price = price.Price
			syncRequest.Tiers[i].Prices[j].OriginalPrice = price.OriginalPrice
		}
	}

	// 发送请求
	resp, err := c.apiPost("/app/membership/sync", syncRequest)
	if err != nil {
		return nil, fmt.Errorf("同步会员等级失败: %w", err)
	}
	defer resp.Body.Close()

	// 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析API响应
	var apiResp struct {
		Code int                    `json:"code"`
		Data MembershipSyncResponse `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		// 尝试作为直接响应解析
		var syncResponse MembershipSyncResponse
		if err2 := json.Unmarshal(body, &syncResponse); err2 != nil {
			return nil, fmt.Errorf("解析API响应失败: %w", err)
		}
		// 成功解析为直接响应
		return &syncResponse, nil
	}

	// 当API返回code为0时，表示请求成功，强制将Success设置为true
	if apiResp.Code == 0 {
		apiResp.Data.Success = true
	}

	return &apiResp.Data, nil
}
