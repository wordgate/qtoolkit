package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// 商品类型常量
const (
	// ItemTypeProduct 表示普通商品类型
	ItemTypeProduct = "product"
	// ItemTypeMembership 表示会员商品类型
	ItemTypeMembership = "membership"
)

// ProductSyncRequest 产品同步请求
type ProductSyncRequest struct {
	// Products 要同步的产品列表
	Products []Product `json:"products"`
}

// ProductSyncResponse 产品同步响应
type ProductSyncResponse struct {
	// Success 表示同步是否成功
	Success bool `json:"success"`
	// Total 同步的产品总数
	Total int `json:"total"`
	// Created 新创建的产品数量
	Created int `json:"created"`
	// Updated 更新的产品数量
	Updated int `json:"updated"`
	// Unchanged 未发生变更的产品数量
	Unchanged int `json:"unchanged"`
	// Failed 同步失败的产品数量
	Failed int `json:"failed"`
	// Errors 失败的详细错误信息
	Errors []struct {
		// Code 错误代码
		Code string `json:"code"`
		// ProductCode 产品代码
		ProductCode string `json:"product_code"`
		// Message 错误消息
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// SyncProducts 同步产品(包括从文件和直接配置的产品)
//
// 该方法会收集配置中定义的所有产品信息，并将其同步到服务器
// 返回同步结果和可能的错误
func (c *Client) SyncProductsFromConfig() (*ProductSyncResponse, error) {
	var allProducts []Product

	// 1. 处理从文件中获取的产品
	if len(c.Config.Products.Files) > 0 {
		processor := NewContentProcessor(c.ConfigDir, c.Config)
		products, err := processor.Process()
		if err != nil {
			return nil, fmt.Errorf("处理产品文件失败: %w", err)
		}

		// 添加从文件中提取的产品
		allProducts = append(allProducts, products...)
	}

	// 2. 添加直接配置的产品
	allProducts = append(allProducts, c.Config.Products.Items...)

	// 检查是否有产品
	if len(allProducts) == 0 {
		return nil, fmt.Errorf("没有找到任何产品")
	}

	// 发送同步请求
	return c.SyncProducts(allProducts)
}

// SyncProduct 同步指定的产品列表
//
// products 参数包含要同步的产品列表
// 返回同步结果和可能的错误
func (c *Client) SyncProducts(products []Product) (*ProductSyncResponse, error) {
	// 如果产品列表为空，返回错误
	if len(products) == 0 {
		return nil, fmt.Errorf("产品列表为空")
	}

	// 构建请求体
	reqBody := ProductSyncRequest{
		Products: products,
	}

	// 发送请求
	resp, err := c.apiPost("/app/product/sync", reqBody)
	if err != nil {
		return nil, fmt.Errorf("同步产品失败: %w", err)
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
		Code int                 `json:"code"`
		Data ProductSyncResponse `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	// 当API返回code为0时，表示请求成功，强制将Success设置为true
	if apiResp.Code == 0 {
		apiResp.Data.Success = true
	}

	return &apiResp.Data, nil
}
