package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OrderItem 订单项信息
type OrderItem struct {
	// ItemCode 商品代码
	ItemCode string `json:"item_code"`
	// Quantity 数量
	Quantity int `json:"quantity"`
	// ItemType 商品类型（可选），如product或membership
	ItemType string `json:"item_type,omitempty"`
}

type OrderCustomer struct {
	Provider string `json:"provider"`
	UID      string `json:"uid"`
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	// Items 订单项列表
	Items []OrderItem `json:"items"`
	// CouponCode 优惠券代码（可选）
	CouponCode string `json:"coupon_code,omitempty"`

	// AddressID 地址ID (可选)
	AddressID uint `json:"address_id"`
	// 客户信息
	Customer OrderCustomer `json:"customer"`
}

// OrderItemInfo 订单项详细信息
type OrderItemInfo struct {
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

// OrderDetailResponse 订单详情响应
type OrderDetailResponse struct {
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
	Items []OrderItemInfo `json:"items"`
	// PayURL 支付链接
	PayURL string `json:"pay_url"`
}

// OrderResponse 创建订单响应
type OrderResponse struct {
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

// GetOrder 获取订单详情
//
// orderNo 参数指定要查询的订单号
// 返回订单详情和可能的错误
func (c *Client) GetOrder(orderNo string) (*OrderDetailResponse, error) {
	// 构建URL
	path := fmt.Sprintf("/app/orders/%s", orderNo)

	// 发送GET请求
	resp, err := c.apiRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("获取订单失败: %w", err)
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
		Data OrderDetailResponse `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	return &apiResp.Data, nil
}

// CreateOrder 创建订单
//
// request 参数包含创建订单所需的信息
// 返回创建的订单信息和可能的错误
func (c *Client) CreateOrder(request *CreateOrderRequest) (*OrderResponse, error) {
	// 发送POST请求
	resp, err := c.apiPost("/app/orders/create", request)
	if err != nil {
		return nil, fmt.Errorf("创建订单失败: %w", err)
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
		Code int           `json:"code"`
		Data OrderResponse `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	return &apiResp.Data, nil
}
