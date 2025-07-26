package qtoolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/log"
)

// WordgateConfig Wordgate配置信息
type WordgateConfig struct {
	// BaseURL API服务的基础URL
	BaseURL string `yaml:"base_url" json:"base_url"`
	// AppCode 应用代码，用于API认证
	AppCode string `yaml:"app_code" json:"app_code"`
	// AppSecret 应用密钥，用于API认证
	AppSecret string `yaml:"app_secret" json:"app_secret"`
}

// wordgateClient Wordgate客户端，用于与Wordgate API进行交互
type wordgateClient struct {
	// Config 存储Wordgate配置信息
	Config *WordgateConfig
	// HTTPClient 用于发送HTTP请求
	HTTPClient *http.Client
}

// WordgateOrderItem 订单项信息 - 严格按照order.go中的OrderItem定义
type WordgateOrderItem struct {
	// ItemCode 商品代码
	ItemCode string `json:"item_code"`
	// Quantity 数量
	Quantity int `json:"quantity"`
	// ItemType 商品类型（可选），如product或membership
	ItemType string `json:"item_type,omitempty"`
}

// WordgateOrderCustomer 订单客户信息 - 严格按照order.go中的OrderCustomer定义
type WordgateOrderCustomer struct {
	Provider string `json:"provider"`
	UID      string `json:"uid"`
}

// CreateOrderRequest 创建订单请求 - 严格按照order.go中的CreateOrderRequest定义
type CreateOrderRequest struct {
	// Items 订单项列表
	Items []WordgateOrderItem `json:"items"`
	// CouponCode 优惠券代码（可选）
	CouponCode string `json:"coupon_code,omitempty"`

	// AddressID 地址ID
	AddressID uint `json:"address_id"`
	// 客户信息
	Customer WordgateOrderCustomer `json:"customer"`
}

// WordgateOrderItemInfo 订单项详细信息 - 严格按照order.go中的OrderItemInfo定义
type WordgateOrderItemInfo struct {
	// ItemID 商品ID
	ItemID uint `json:"item_id"`
	// ItemName 商品名称
	ItemName string `json:"item_name"`
	// Quantity 数量
	Quantity int `json:"quantity"`
	// UnitPrice 单价（分）
	UnitPrice int64 `json:"unit_price"`
	// Subtotal 小计（分）
	Subtotal int64 `json:"subtotal"`
	// ItemType 商品类型
	ItemType string `json:"item_type"`
}

// WordgateOrderDetailResponse 订单详情响应 - 严格按照order.go中的OrderDetailResponse定义
type WordgateOrderDetailResponse struct {
	// ID 订单ID
	ID uint `json:"id"`
	// OrderNo 订单号
	OrderNo string `json:"order_no"`
	// UserID 用户ID
	UserID uint `json:"user_id"`
	// Amount 总金额（分）
	Amount int64 `json:"amount"`
	// Currency 货币
	Currency string `json:"currency"`
	// IsPaid 是否已支付
	IsPaid bool `json:"is_paid"`
	// CreatedAt 创建时间
	CreatedAt string `json:"created_at"`
	// PaidAt 支付时间（可为空）
	PaidAt *string `json:"paid_at"`
	// CouponCode 优惠券代码
	CouponCode string `json:"coupon_code"`
	// DiscountAmount 折扣金额（分）
	DiscountAmount int64 `json:"discount_amount"`
	// Items 订单项列表
	Items []WordgateOrderItemInfo `json:"items"`
	// PayURL 支付链接
	PayURL string `json:"pay_url"`
}

// WordgateOrderSummaryResponse 创建订单响应 - 严格按照order.go中的OrderResponse定义
type WordgateOrderSummaryResponse struct {
	// OrderNo 订单号
	OrderNo string `json:"order_no"`
	// Amount 总金额（分）
	Amount int64 `json:"amount"`
	// Currency 货币
	Currency string `json:"currency"`
	// IsPaid 是否已支付
	IsPaid bool `json:"is_paid"`
	// PaidAt 支付时间（可为空）
	PaidAt *time.Time `json:"paid_at"`
	// RedirectURL 支付页面URL
	RedirectURL string `json:"redirect_url"`
	// PayURL 支付链接
	PayURL string `json:"pay_url"`
}

// WordgateResponse 通用API响应结构
type WordgateResponse struct {
	// Code 返回码，0为成功，非0为失败
	Code int `json:"code"`
	// Message 错误信息或提示信息
	Message string `json:"message,omitempty"`
	// Data 响应数据
	Data interface{} `json:"data,omitempty"`
}

// 商品类型常量
const (
	// ItemTypeProduct 表示普通商品类型
	ItemTypeProduct = "product"
	// ItemTypeMembership 表示会员商品类型
	ItemTypeMembership = "membership"
)

// WordgateProduct 产品定义
type WordgateProduct struct {
	// Code 产品代码，唯一标识一个产品
	Code string `json:"code"`
	// Name 产品名称
	Name string `json:"name"`
	// Price 产品价格(单位:分)
	Price int `json:"price"`
	// RequireAddress 是否需要地址
	RequireAddress bool `json:"require_address,omitempty"`
}

// WordgateProductDetail 产品详细信息 - 对应 api/model_product.go 中的 Product 结构
type WordgateProductDetail struct {
	// ID 产品ID
	ID uint64 `json:"id"`
	// CreatedAt 创建时间
	CreatedAt string `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt string `json:"updated_at"`
	// AppID 应用ID
	AppID uint64 `json:"app_id"`
	// Code 产品代码
	Code string `json:"code"`
	// Name 产品名称
	Name string `json:"name"`
	// Price 产品价格(分)
	Price int64 `json:"price"`
	// Status 产品状态
	Status string `json:"status"`
	// RequireAddress 是否需要地址
	RequireAddress bool `json:"require_address"`
	// Version 版本号
	Version int `json:"version"`
}

// CreateProductRequest 创建产品请求 - 对应 app_product.go 中的 CreateProductRequest
type CreateProductRequest struct {
	// Code 产品代码
	Code string `json:"code"`
	// Name 产品名称
	Name string `json:"name"`
	// Price 产品价格(分)
	Price int64 `json:"price"`
	// RequireAddress 是否需要地址
	RequireAddress bool `json:"require_address"`
}

// UpdateProductRequest 更新产品请求 - 对应 app_product.go 中的 UpdateProductRequest
type UpdateProductRequest struct {
	// Name 产品名称
	Name string `json:"name"`
	// Price 产品价格(分)
	Price int64 `json:"price"`
	// RequireAddress 是否需要地址
	RequireAddress bool `json:"require_address"`
}

// WordgateProductSyncRequest 产品同步请求
type WordgateProductSyncRequest struct {
	// Products 要同步的产品列表
	Products []WordgateProduct `json:"products"`
}

// WordgateProductSyncResponse 产品同步响应
type WordgateProductSyncResponse struct {
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

// WordgateProductListQuery 产品列表查询参数
type WordgateProductListQuery struct {
	// Status 产品状态（active/inactive）
	Status string `json:"status,omitempty"`
	// ShowDeleted 是否显示已删除的产品
	ShowDeleted bool `json:"show_deleted,omitempty"`
	// Page 页码，默认1
	Page int `json:"page,omitempty"`
	// Limit 每页数量，默认20
	Limit int `json:"limit,omitempty"`
}

// WordgateProductListResponse 产品列表响应 - 对应 api/response.go 中的 ListResult
type WordgateProductListResponse struct {
	// Items 产品列表
	Items []WordgateProductDetail `json:"items"`
	// Pagination 分页信息
	Pagination struct {
		// Page 当前页码
		Page int `json:"page"`
		// Limit 每页数量
		Limit int `json:"limit"`
		// Total 总记录数
		Total int64 `json:"total"`
	} `json:"pagination"`
}

func wordgateGetResponseData[T any](ctx context.Context, resp *WordgateResponse) (T, error) {
	var data T
	if resp.Data != nil {
		dataBytes, _ := json.Marshal(resp.Data)
		err := json.Unmarshal(dataBytes, &data)
		return data, err
	}
	log.Warnf(ctx, "[wordgate] data is nil")
	return *new(T), fmt.Errorf("no response data")
}

// WordgateClient 创建并返回一个新的Wordgate客户端实例
func WordgateClient() *wordgateClient {
	// 获取配置
	baseURL := viper.GetString("wordgate.base_url")
	appCode := viper.GetString("wordgate.app_code")
	appSecret := viper.GetString("wordgate.app_secret")

	// 验证配置
	if baseURL == "" {
		log.Errorf(context.Background(), "wordgate.base_url is not configured")
		return nil
	}
	if appCode == "" {
		log.Errorf(context.Background(), "wordgate.app_code is not configured")
		return nil
	}
	if appSecret == "" {
		log.Errorf(context.Background(), "wordgate.app_secret is not configured")
		return nil
	}

	// 创建HTTP客户端
	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}
	config := &WordgateConfig{
		BaseURL:   baseURL,
		AppCode:   appCode,
		AppSecret: appSecret,
	}

	return &wordgateClient{
		Config:     config,
		HTTPClient: httpClient,
	}
}

// apiPost 发送POST请求到API
func (c *wordgateClient) apiPost(ctx context.Context, path string, body interface{}) (*WordgateResponse, error) {
	return c.apiRequest(ctx, "POST", path, body)
}

// apiRequest 发送通用请求到API
func (c *wordgateClient) apiRequest(ctx context.Context, method, path string, body interface{}) (*WordgateResponse, error) {
	var reqBody io.Reader
	var reqBodyStr string

	// 如果有请求体，序列化为JSON
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reqBodyStr = string(jsonData)
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
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		log.Warnf(ctx, "%s %s -> fail:%s\n", method, url, err.Error())
	} else {
		log.Debugf(ctx, "%s %s -> done\n", method, url)
	}

	if err != nil {
		return nil, fmt.Errorf("获取订单失败: %w", err)
	}
	defer resp.Body.Close()

	// 解析响应
	resp_body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	log.Debugf(ctx, "[wordgate] [request] api request=(%s) response=(%s)", reqBodyStr, string(resp_body))

	// 解析API响应
	var apiResp WordgateResponse
	if err := json.Unmarshal(resp_body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}
	return &apiResp, err
}

// CreateOrder 创建订单
func (c *wordgateClient) CreateOrder(ctx context.Context, request *CreateOrderRequest) (*WordgateOrderSummaryResponse, error) {
	// 发送POST请求
	resp, err := c.apiPost(ctx, "/app/orders/create", request)
	if err != nil {
		return nil, fmt.Errorf("创建订单失败: %w", err)
	}
	return wordgateGetResponseData[*WordgateOrderSummaryResponse](ctx, resp)
}

// GetOrder 获取订单详情
//
// orderNo 参数指定要查询的订单号
// 返回订单详情和可能的错误
func (c *wordgateClient) GetOrder(ctx context.Context, orderNo string) (*WordgateOrderDetailResponse, error) {
	// 构建URL
	path := fmt.Sprintf("/app/orders/%s", orderNo)

	// 发送GET请求
	resp, err := c.apiRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("获取订单失败: %w", err)
	}
	return wordgateGetResponseData[*WordgateOrderDetailResponse](ctx, resp)
}

// CreateProduct 创建单个产品
//
// request 参数包含要创建的产品信息
// 返回创建的产品详情和可能的错误
func (c *wordgateClient) CreateProduct(ctx context.Context, request *CreateProductRequest) (*WordgateProductDetail, error) {
	// 发送POST请求
	resp, err := c.apiPost(ctx, "/app/products", request)
	if err != nil {
		return nil, fmt.Errorf("创建产品失败: %w", err)
	}
	return wordgateGetResponseData[*WordgateProductDetail](ctx, resp)
}

// GetProduct 获取单个产品详情
//
// productCode 参数指定要查询的产品代码
// 返回产品详情和可能的错误
func (c *wordgateClient) GetProduct(ctx context.Context, productCode string) (*WordgateProductDetail, error) {
	// 构建URL
	path := fmt.Sprintf("/app/products/%s", productCode)

	// 发送GET请求
	resp, err := c.apiRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("获取产品失败: %w", err)
	}
	return wordgateGetResponseData[*WordgateProductDetail](ctx, resp)
}

// UpdateProduct 更新单个产品
//
// productCode 参数指定要更新的产品代码
// request 参数包含要更新的产品信息
// 返回更新后的产品详情和可能的错误
func (c *wordgateClient) UpdateProduct(ctx context.Context, productCode string, request *UpdateProductRequest) (*WordgateProductDetail, error) {
	// 构建URL
	path := fmt.Sprintf("/app/products/%s", productCode)

	// 发送PUT请求
	resp, err := c.apiRequest(ctx, "PUT", path, request)
	if err != nil {
		return nil, fmt.Errorf("更新产品失败: %w", err)
	}
	if resp.Code == 404 {
		log.Warnf(ctx, "[wordgate] product %s not found, try create", productCode)
		return c.CreateProduct(ctx, &CreateProductRequest{
			Code:           productCode,
			Name:           request.Name,
			Price:          request.Price,
			RequireAddress: request.RequireAddress,
		})
	}
	return wordgateGetResponseData[*WordgateProductDetail](ctx, resp)
}

// DeleteProduct 删除单个产品
//
// productCode 参数指定要删除的产品代码
// 返回删除结果和可能的错误
func (c *wordgateClient) DeleteProduct(ctx context.Context, productCode string) error {
	// 构建URL
	path := fmt.Sprintf("/app/products/%s", productCode)

	// 发送DELETE请求
	resp, err := c.apiRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("删除产品失败: %w", err)
	}

	// 检查响应状态
	if resp.Code != 0 {
		return fmt.Errorf("删除产品失败: %s", resp.Message)
	}

	return nil
}

// RestoreProduct 恢复已删除的产品
//
// productCode 参数指定要恢复的产品代码
// 返回恢复后的产品详情和可能的错误
func (c *wordgateClient) RestoreProduct(ctx context.Context, productCode string) (*WordgateProductDetail, error) {
	// 构建URL
	path := fmt.Sprintf("/app/products/%s/restore", productCode)

	// 发送POST请求
	resp, err := c.apiPost(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("恢复产品失败: %w", err)
	}
	return wordgateGetResponseData[*WordgateProductDetail](ctx, resp)
}

// SyncProducts 同步指定的产品列表
//
// products 参数包含要同步的产品列表
// 返回同步结果和可能的错误
func (c *wordgateClient) SyncProducts(ctx context.Context, products []WordgateProduct) (*WordgateProductSyncResponse, error) {
	// 如果产品列表为空，返回错误
	if len(products) == 0 {
		return nil, fmt.Errorf("产品列表为空")
	}

	// 构建请求体
	reqBody := WordgateProductSyncRequest{
		Products: products,
	}

	// 发送POST请求
	resp, err := c.apiPost(ctx, "/app/product/sync", reqBody)
	if err != nil {
		return nil, fmt.Errorf("同步产品失败: %w", err)
	}
	return wordgateGetResponseData[*WordgateProductSyncResponse](ctx, resp)
}

// ListProducts 获取产品列表
//
// query 参数包含查询条件，如状态、分页等
// 返回产品列表和可能的错误
func (c *wordgateClient) ListProducts(ctx context.Context, query *WordgateProductListQuery) (*WordgateProductListResponse, error) {
	// 构建查询参数
	params := make(map[string]string)

	if query.Status != "" {
		params["status"] = query.Status
	}

	if query.ShowDeleted {
		params["show_deleted"] = "true"
	}

	if query.Page > 0 {
		params["page"] = fmt.Sprintf("%d", query.Page)
	}

	if query.Limit > 0 {
		params["limit"] = fmt.Sprintf("%d", query.Limit)
	}

	// 构建URL
	path := "/app/products"
	if len(params) > 0 {
		queryStr := ""
		for key, value := range params {
			if queryStr != "" {
				queryStr += "&"
			}
			queryStr += fmt.Sprintf("%s=%s", key, value)
		}
		path += "?" + queryStr
	}

	// 发送GET请求
	resp, err := c.apiRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("获取产品列表失败: %w", err)
	}
	return wordgateGetResponseData[*WordgateProductListResponse](ctx, resp)
}
