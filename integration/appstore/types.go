package appstore

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AppStoreServerRequest 表示从Apple接收的请求
type AppStoreServerRequest struct {
	SignedPayload string `json:"signedPayload"`
}

// 处理JWT头部相关类型
type JWTHeader struct {
	Alg string   `json:"alg"`
	X5c []string `json:"x5c"`
	Kid string   `json:"kid"`
}

// StandardClaims 实现jwt.Claims接口的自定义时间相关属性
type StandardClaims struct {
	ExpiresAt *int64   `json:"exp,omitempty"`
	IssuedAt  *int64   `json:"iat,omitempty"`
	NotBefore *int64   `json:"nbf,omitempty"`
	Issuer    *string  `json:"iss,omitempty"`
	Subject   *string  `json:"sub,omitempty"`
	Audience  []string `json:"aud,omitempty"`
}

// 实现jwt.Claims接口所需的方法
func (c *StandardClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c == nil || c.ExpiresAt == nil {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(*c.ExpiresAt, 0)), nil
}

func (c *StandardClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c == nil || c.IssuedAt == nil {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(*c.IssuedAt, 0)), nil
}

func (c *StandardClaims) GetNotBefore() (*jwt.NumericDate, error) {
	if c == nil || c.NotBefore == nil {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(*c.NotBefore, 0)), nil
}

// 实现GetAudience方法
func (c *StandardClaims) GetAudience() (jwt.ClaimStrings, error) {
	if c == nil || c.Audience == nil {
		return nil, nil
	}
	return jwt.ClaimStrings(c.Audience), nil
}

// 实现GetIssuer方法
func (c *StandardClaims) GetIssuer() (string, error) {
	if c == nil || c.Issuer == nil {
		return "", nil
	}
	return *c.Issuer, nil
}

// 实现GetSubject方法
func (c *StandardClaims) GetSubject() (string, error) {
	if c == nil || c.Subject == nil {
		return "", nil
	}
	return *c.Subject, nil
}

// AppStoreServerNotification 表示解析后的通知对象
type AppStoreServerNotification struct {
	appleRootCert   string
	Payload         *NotificationPayload
	TransactionInfo *TransactionInfo
	RenewalInfo     *RenewalInfo
	IsValid         bool
}

// NotificationPayload 表示解析后的通知载荷
type NotificationPayload struct {
	StandardClaims                  // 嵌入标准JWT声明
	NotificationType string         `json:"notificationType"`  // 通知类型
	Subtype          string         `json:"subtype"`           // 子类型
	NotificationUUID string         `json:"notificationUUID"`  // 通知唯一标识符
	Version          string         `json:"version"`           // API版本
	SignedDate       int64          `json:"signedDate"`        // 签名时间戳
	Data             PayloadData    `json:"data"`              // 通知数据
	Summary          PayloadSummary `json:"summary,omitempty"` // 摘要信息(可选)
}

// PayloadSummary 表示通知摘要信息
type PayloadSummary struct {
	RequestIdentifier      string   `json:"requestIdentifier"`      // 请求标识符
	AppAppleId             string   `json:"appAppleId"`             // Apple应用ID
	BundleId               string   `json:"bundleId"`               // 包ID
	ProductId              string   `json:"productId"`              // 产品ID
	Environment            string   `json:"environment"`            // 环境(Production/Sandbox)
	StorefrontCountryCodes []string `json:"storefrontCountryCodes"` // 商店国家/地区代码
	FailedCount            int64    `json:"failedCount"`            // 失败计数
	SucceededCount         int64    `json:"succeededCount"`         // 成功计数
}

// PayloadData 表示通知主体数据
type PayloadData struct {
	AppAppleId            int64  `json:"appAppleId"`            // Apple应用ID(数字)
	BundleId              string `json:"bundleId"`              // 包ID
	BundleVersion         string `json:"bundleVersion"`         // 应用版本
	Environment           string `json:"environment"`           // 环境
	SignedRenewalInfo     string `json:"signedRenewalInfo"`     // 签名的续期信息(JWT)
	SignedTransactionInfo string `json:"signedTransactionInfo"` // 签名的交易信息(JWT)
	Status                int32  `json:"status"`                // 状态码
	StatusError           string `json:"statusError,omitempty"` // 状态错误(可选)
}

// TransactionInfo 表示交易信息
type TransactionInfo struct {
	StandardClaims                     // 嵌入标准JWT声明
	AppAccountToken             string `json:"appAccountToken,omitempty"`             // 应用账户令牌(可选)
	BundleId                    string `json:"bundleId"`                              // 包ID
	Currency                    string `json:"currency,omitempty"`                    // 货币代码(可选)
	Environment                 string `json:"environment"`                           // 环境
	ExpiresDate                 int64  `json:"expiresDate,omitempty"`                 // 过期时间戳(可选)
	InAppOwnershipType          string `json:"inAppOwnershipType"`                    // 应用内所有权类型
	IsUpgraded                  bool   `json:"isUpgraded,omitempty"`                  // 是否已升级(可选)
	OfferIdentifier             string `json:"offerIdentifier,omitempty"`             // 优惠标识符(可选)
	OfferType                   int32  `json:"offerType,omitempty"`                   // 优惠类型(可选)
	OriginalPurchaseDate        int64  `json:"originalPurchaseDate"`                  // 原始购买时间戳
	OriginalTransactionId       string `json:"originalTransactionId"`                 // 原始交易ID
	Price                       int32  `json:"price,omitempty"`                       // 价格(可选)
	ProductId                   string `json:"productId"`                             // 产品ID
	PurchaseDate                int64  `json:"purchaseDate"`                          // 购买时间戳
	Quantity                    int32  `json:"quantity"`                              // 数量
	RevocationDate              int64  `json:"revocationDate,omitempty"`              // 撤销时间戳(可选)
	RevocationReason            int32  `json:"revocationReason,omitempty"`            // 撤销原因(可选)
	SignedDate                  int64  `json:"signedDate"`                            // 签名时间戳
	StoreFront                  string `json:"storefront,omitempty"`                  // 商店前端(可选)
	StoreFrontId                string `json:"storefrontId,omitempty"`                // 商店前端ID(可选)
	SubscriptionGroupIdentifier string `json:"subscriptionGroupIdentifier,omitempty"` // 订阅组标识符(可选)
	TransactionId               string `json:"transactionId"`                         // 交易ID
	TransactionReason           string `json:"transactionReason,omitempty"`           // 交易原因(可选)
	Type                        string `json:"type"`                                  // 交易类型
	WebOrderLineItemId          string `json:"webOrderLineItemId,omitempty"`          // 网络订单行项目ID(可选)
}

// RenewalInfo 表示订阅续期信息
type RenewalInfo struct {
	StandardClaims                     // 嵌入标准JWT声明
	AutoRenewProductId          string `json:"autoRenewProductId,omitempty"`          // 自动续订产品ID(可选)
	AutoRenewStatus             int32  `json:"autoRenewStatus"`                       // 自动续订状态
	Environment                 string `json:"environment"`                           // 环境
	ExpirationIntent            int32  `json:"expirationIntent,omitempty"`            // 过期意图(可选)
	GracePeriodExpiresDate      int64  `json:"gracePeriodExpiresDate,omitempty"`      // 宽限期过期时间戳(可选)
	IsInBillingRetryPeriod      bool   `json:"isInBillingRetryPeriod,omitempty"`      // 是否在账单重试期(可选)
	OfferIdentifier             string `json:"offerIdentifier,omitempty"`             // 优惠标识符(可选)
	OfferType                   int32  `json:"offerType,omitempty"`                   // 优惠类型(可选)
	OriginalTransactionId       string `json:"originalTransactionId"`                 // 原始交易ID
	PriceIncreaseStatus         int32  `json:"priceIncreaseStatus,omitempty"`         // 价格增长状态(可选)
	ProductId                   string `json:"productId"`                             // 产品ID
	RecentSubscriptionStartDate int64  `json:"recentSubscriptionStartDate,omitempty"` // 最近订阅开始时间戳(可选)
	RenewalDate                 int64  `json:"renewalDate,omitempty"`                 // 续期时间戳(可选)
	SignedDate                  int64  `json:"signedDate"`                            // 签名时间戳
}

// 通知类型常量
const (
	// 订阅通知类型
	NotificationType_CONSUMPTION_REQUEST       = "CONSUMPTION_REQUEST"
	NotificationType_DID_CHANGE_RENEWAL_PREF   = "DID_CHANGE_RENEWAL_PREF"
	NotificationType_DID_CHANGE_RENEWAL_STATUS = "DID_CHANGE_RENEWAL_STATUS"
	NotificationType_DID_FAIL_TO_RENEW         = "DID_FAIL_TO_RENEW"
	NotificationType_DID_RENEW                 = "DID_RENEW"
	NotificationType_EXPIRED                   = "EXPIRED"
	NotificationType_GRACE_PERIOD_EXPIRED      = "GRACE_PERIOD_EXPIRED"
	NotificationType_OFFER_REDEEMED            = "OFFER_REDEEMED"
	NotificationType_PRICE_INCREASE            = "PRICE_INCREASE"
	NotificationType_REFUND                    = "REFUND"
	NotificationType_REFUND_DECLINED           = "REFUND_DECLINED"
	NotificationType_RENEWAL_EXTENDED          = "RENEWAL_EXTENDED"
	NotificationType_REVOKE                    = "REVOKE"
	NotificationType_SUBSCRIBED                = "SUBSCRIBED"

	// 子类型
	Subtype_INITIAL_BUY          = "INITIAL_BUY"
	Subtype_RESUBSCRIBE          = "RESUBSCRIBE"
	Subtype_DOWNGRADE            = "DOWNGRADE"
	Subtype_UPGRADE              = "UPGRADE"
	Subtype_AUTO_RENEW_ENABLED   = "AUTO_RENEW_ENABLED"
	Subtype_AUTO_RENEW_DISABLED  = "AUTO_RENEW_DISABLED"
	Subtype_VOLUNTARY            = "VOLUNTARY"
	Subtype_BILLING_RETRY        = "BILLING_RETRY"
	Subtype_PRICE_INCREASE       = "PRICE_INCREASE"
	Subtype_PRODUCT_NOT_FOR_SALE = "PRODUCT_NOT_FOR_SALE"
	Subtype_PENDING              = "PENDING"
	Subtype_ACCEPTED             = "ACCEPTED"

	// 环境
	Environment_Production = "Production"
	Environment_Sandbox    = "Sandbox"

	// 自动续期状态
	AutoRenewStatus_Off = 0
	AutoRenewStatus_On  = 1

	// 过期意图
	ExpirationIntent_Cancelled           = 1 // 客户取消
	ExpirationIntent_BillingError        = 2 // 账单错误
	ExpirationIntent_PriceIncreaseDenied = 3 // 客户不同意涨价
	ExpirationIntent_ProductUnavailable  = 4 // 产品不再可用
	ExpirationIntent_Unknown             = 5 // 其他原因
)

// 交易类型常量 - 对应TransactionInfo中的Type字段
const (
	// 交易类型
	TransactionType_AutoRenewableSubscription = "Auto-Renewable Subscription" // 自动续费订阅
	TransactionType_NonConsumable             = "Non-Consumable"              // 非消耗型项目
	TransactionType_Consumable                = "Consumable"                  // 消耗型项目
	TransactionType_NonRenewingSubscription   = "Non-Renewing Subscription"   // 非续订订阅
)

// 通知数据状态码 - 对应PayloadData中的Status字段
const (
	// 状态码
	NotificationStatus_OK    = 0 // 正常
	NotificationStatus_Error = 1 // 错误
)

// 应用内购买所有权类型 - 对应TransactionInfo中的InAppOwnershipType字段
const (
	OwnershipType_PURCHASED       = "PURCHASED"       // 已购买
	OwnershipType_FAMILY_SHARED   = "FAMILY_SHARED"   // 家庭共享
	OwnershipType_PURCHASED_TRIAL = "PURCHASED_TRIAL" // 试用购买
)

// 其他相关类型定义...
