package sdk

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WordgateConfig 表示完整的Wordgate配置
// 包含应用信息、API认证信息、产品配置、会员配置等
type WordgateConfig struct {
	// BaseURL API服务的基础URL
	BaseURL string `yaml:"base_url" json:"base_url"`
	// AppCode 应用代码，用于API认证
	AppCode string `yaml:"app_code" json:"app_code"`
	// AppSecret 应用密钥，用于API认证
	AppSecret string `yaml:"app_secret" json:"app_secret"`
	// EnablePurchase 是否启用支付功能
	EnablePurchase bool `yaml:"enable_purchase" json:"enable_purchase"`
	// Products 产品相关配置
	Products ProductConfig `yaml:"products" json:"products"`
	// App 应用基本信息
	App AppInfo `yaml:"app" json:"app"`
	// Config 应用配置
	Config AppConfig `yaml:"config" json:"config"`
	// Membership 会员系统配置
	Membership MembershipConfig `yaml:"membership" json:"membership"`
}

// AppInfo 应用基本信息
type AppInfo struct {
	// Name 应用名称
	Name string `yaml:"name" json:"name"`
	// Description 应用描述
	Description string `yaml:"description" json:"description"`
	// Currency 结算货币代码(如CNY、USD等)
	Currency string `yaml:"currency" json:"currency"`
}

// ProductConfig 产品配置
type ProductConfig struct {
	// Files 文件匹配模式列表，用于从文件中提取产品信息
	// 路径相对于配置文件所在目录
	Files []string `yaml:"files" json:"files"`
	// Items 直接在配置文件中定义的产品列表
	Items []Product `yaml:"items" json:"items"`
}

// Product 产品定义
type Product struct {
	// Code 产品代码，唯一标识一个产品
	Code string `yaml:"code" json:"code"`
	// Name 产品名称
	Name string `yaml:"name" json:"name"`
	// Price 产品价格(单位:分)
	Price int `yaml:"price" json:"price"`
}

// AppConfig 应用配置
type AppConfig struct {
	// SMTP 邮件配置
	SMTP SMTPConfig `yaml:"smtp" json:"smtp"`
	// SMS 短信配置
	SMS SMSConfig `yaml:"sms" json:"sms"`
	// Security 安全配置
	Security SecurityConfig `yaml:"security" json:"security"`
	// Purchase 支付配置
	Purchase PurchaseConfig `yaml:"purchase" json:"purchase"`
	// Site 网站配置
	Site SiteConfig `yaml:"site" json:"site"`
}

// SiteConfig 网站配置
type SiteConfig struct {
	// BaseURL 网站基础URL
	BaseURL string `yaml:"base_url" json:"base_url"`
	// PayPagePath 支付页面路径
	PayPagePath string `yaml:"pay_page_path" json:"pay_page_path"`
	// PayResultPagePath 支付结果页面路径
	PayResultPagePath string `yaml:"pay_result_page_path" json:"pay_result_page_path"`
}

// GeneratePurchaseURL 生成支付页面URL
func (c *SiteConfig) GeneratePurchaseURL(orderNo string) string {
	// 如果路径已经是完整URL，则直接使用
	path := c.PayPagePath
	if path == "" {
		path = "/pay"
	}

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return fmt.Sprintf("%s?order_no=%s", path, orderNo)
	}

	// 规范化baseURL和path的连接
	base := c.BaseURL
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	url := base + path
	return fmt.Sprintf("%s?order_no=%s", url, orderNo)
}

// GeneratePayResultURL 生成支付结果页面URL
func (c *SiteConfig) GeneratePayResultURL(orderNo string, queryParams map[string]string) string {
	path := c.PayResultPagePath
	if path == "" {
		path = "/pay-result"
	}

	baseURL := ""
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		baseURL = path
	} else {
		// 规范化baseURL和path的连接
		base := c.BaseURL
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}

		if strings.HasPrefix(path, "/") {
			path = path[1:]
		}

		baseURL = base + path
	}

	// 构建带查询参数的URL
	result := fmt.Sprintf("%s?order_no=%s", baseURL, orderNo)

	// 添加其他查询参数
	if len(queryParams) > 0 {
		u, err := url.Parse(result)
		if err == nil {
			q := u.Query()
			for k, v := range queryParams {
				q.Set(k, v)
			}
			u.RawQuery = q.Encode()
			result = u.String()
		}
	}

	return result
}

// SMTPConfig 邮件配置
type SMTPConfig struct {
	// Host SMTP服务器地址
	Host string `yaml:"host" json:"host"`
	// Port SMTP服务器端口
	Port int `yaml:"port" json:"port"`
	// Username SMTP用户名
	Username string `yaml:"username" json:"username"`
	// Password SMTP密码
	Password string `yaml:"password" json:"password"`
	// FromEmail 发件人邮箱
	FromEmail string `yaml:"from_email" json:"from_email"`
	// FromName 发件人名称
	FromName string `yaml:"from_name" json:"from_name"`
	// ReplyToEmail 回复邮箱
	ReplyToEmail string `yaml:"reply_to_email" json:"reply_to_email"`
}

// SMSConfig 短信配置
type SMSConfig struct {
	// Provider 短信服务提供商
	Provider string `yaml:"provider" json:"provider"`
	// APIKey API密钥
	APIKey string `yaml:"api_key" json:"api_key"`
	// APISecret API密钥对应的密钥
	APISecret string `yaml:"api_secret" json:"api_secret"`
	// SignName 短信签名
	SignName string `yaml:"sign_name" json:"sign_name"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	// CodeExpire 验证码过期时间(秒)
	CodeExpire int `yaml:"code_expire" json:"code_expire"`
}

// PurchaseConfig 支付配置
type PurchaseConfig struct {
	// GatewayMode 网关模式配置
	GatewayMode GatewayModeConfig `yaml:"gateway_mode" json:"gateway_mode"`
	// Antom Antom支付配置
	Antom AntomConfig `yaml:"antom" json:"antom"`
	// Stripe Stripe支付配置
	Stripe StripeConfig `yaml:"stripe" json:"stripe"`
	// Payssion Payssion支付配置
	Payssion PayssionConfig `yaml:"payssion" json:"payssion"`
	// TronPay TronPay配置
	TronPay TronPayConfig `yaml:"tronpay" json:"tronpay"`
}

// GatewayModeConfig 网关模式配置
type GatewayModeConfig struct {
	// Enabled 是否启用网关模式
	Enabled bool `yaml:"enabled" json:"enabled"`
	// NotifyURL 通知URL
	NotifyURL string `yaml:"notify_url" json:"notify_url"`
	// RedirectURL 重定向URL
	RedirectURL string `yaml:"redirect_url" json:"redirect_url"`
}

// AntomConfig Antom支付配置
type AntomConfig struct {
	// Enabled 是否启用Antom支付
	Enabled bool `yaml:"enabled" json:"enabled"`
	// ClientID Antom客户端ID
	ClientID string `yaml:"client_id" json:"client_id"`
	// AntomPublicKey Antom公钥
	AntomPublicKey string `yaml:"antom_public_key" json:"antom_public_key"`
	// YourPublicKey 您的公钥
	YourPublicKey string `yaml:"your_public_key" json:"your_public_key"`
	// YourPrivateKey 您的私钥
	YourPrivateKey string `yaml:"your_private_key" json:"your_private_key"`
	// IsSandbox 是否使用沙箱环境
	IsSandbox bool `yaml:"is_sandbox" json:"is_sandbox"`
	// Domain 域名
	Domain string `yaml:"domain" json:"domain"`
}

// StripeConfig Stripe支付配置
type StripeConfig struct {
	// Enabled 是否启用Stripe支付
	Enabled bool `yaml:"enabled" json:"enabled"`
	// PublicKey Stripe公钥(前端使用)
	PublicKey string `yaml:"public_key" json:"public_key"`
	// SecretKey Stripe密钥(后端使用)
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	// WebhookSecret Stripe Webhook密钥
	WebhookSecret string `yaml:"webhook_secret" json:"webhook_secret"`
}

// PayssionConfig Payssion支付配置
type PayssionConfig struct {
	// Enabled 是否启用Payssion支付
	Enabled bool `yaml:"enabled" json:"enabled"`
	// ApiKey Payssion API密钥
	ApiKey string `yaml:"api_key" json:"api_key"`
	// SecretKey Payssion 密钥
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	// LiveMode 是否使用正式环境
	LiveMode bool `yaml:"live_mode" json:"live_mode"`
	// PmListIDs 支持的支付方式列表
	PmListIDs []string `yaml:"pm_list_ids" json:"pm_list_ids"`
}

// TronPayConfig TronPay配置
type TronPayConfig struct {
	// Enabled 是否启用TronPay
	Enabled bool `yaml:"enabled" json:"enabled"`
	// MainAddress 主钱包地址
	MainAddress string `yaml:"main_address" json:"main_address"`
	// XPub 主钱包的扩展公钥
	XPub string `yaml:"xpub" json:"xpub"`
}

// MembershipConfig 会员系统配置
type MembershipConfig struct {
	// Tiers 会员等级列表
	Tiers []MembershipTier `yaml:"tiers" json:"tiers"`
}

// MembershipTier 会员等级
type MembershipTier struct {
	// Code 会员等级代码
	Code string `yaml:"code" json:"code"`
	// Name 会员等级名称
	Name string `yaml:"name" json:"name"`
	// Level 等级值，用于排序
	Level int `yaml:"level" json:"level"`
	// IsDefault 是否默认等级
	IsDefault bool `yaml:"is_default" json:"is_default"`
	// Prices 会员价格配置列表
	Prices []MembershipPrice `yaml:"prices" json:"prices"`
}

// MembershipPrice 会员价格
type MembershipPrice struct {
	// PeriodType 周期类型，如 month、year 等
	PeriodType string `yaml:"period_type" json:"period_type"`
	// Price 价格(单位:分)
	Price int64 `yaml:"price" json:"price"`
	// OriginalPrice 原价(单位:分)，用于显示折扣信息
	OriginalPrice int64 `yaml:"original_price" json:"original_price"`
}

// LoadConfig 从文件加载Wordgate配置
//
// filePath 参数指定配置文件的路径
// 返回加载的配置和可能的错误
func LoadConfig(filePath string) (*WordgateConfig, error) {
	// 读取配置文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 根据文件扩展名选择解析方法
	ext := filepath.Ext(filePath)
	var config *WordgateConfig
	switch strings.ToLower(ext) {
	case ".yaml", ".yml":
		config, err = loadFromYAML(data)
	case ".json":
		config, err = loadFromJSON(data)
	default:
		return nil, fmt.Errorf("不支持的配置文件格式: %s", ext)
	}

	if err != nil {
		return nil, err
	}

	// 从环境变量覆盖Antom配置
	overrideAntomConfigFromEnv(config)

	return config, nil
}

// overrideAntomConfigFromEnv 从环境变量读取并覆盖配置中的Antom设置
func overrideAntomConfigFromEnv(config *WordgateConfig) {
	// 如果配置未初始化，不进行任何操作
	if config == nil {
		return
	}

	fmt.Println("[配置] 检查环境变量中的Antom支付配置...")
	changed := false

	// 检查并覆盖ANTOM_CLIENT_ID
	if clientID := os.Getenv("ANTOM_CLIENT_ID"); clientID != "" {
		fmt.Printf("[配置] 从环境变量覆盖 Antom ClientID: %s\n", clientID)
		config.Config.Purchase.Antom.ClientID = clientID
		// 如果设置了客户端ID，确保启用Antom支付
		config.Config.Purchase.Antom.Enabled = true
		changed = true
	}

	// 检查并覆盖ANTOM_PUBLIC_KEY
	if publicKey := os.Getenv("ANTOM_PUBLIC_KEY"); publicKey != "" {
		maskedKey := maskSensitiveValue(publicKey)
		fmt.Printf("[配置] 从环境变量覆盖 Antom PublicKey: %s\n", maskedKey)
		config.Config.Purchase.Antom.AntomPublicKey = publicKey
		changed = true
	}

	// 检查并覆盖ANTOM_YOUR_PUBLIC_KEY
	if yourPublicKey := os.Getenv("ANTOM_YOUR_PUBLIC_KEY"); yourPublicKey != "" {
		maskedKey := maskSensitiveValue(yourPublicKey)
		fmt.Printf("[配置] 从环境变量覆盖 Your PublicKey: %s\n", maskedKey)
		config.Config.Purchase.Antom.YourPublicKey = yourPublicKey
		changed = true
	}

	// 检查并覆盖ANTOM_YOUR_PRIVATE_KEY
	if yourPrivateKey := os.Getenv("ANTOM_YOUR_PRIVATE_KEY"); yourPrivateKey != "" {
		fmt.Println("[配置] 从环境变量覆盖 Your PrivateKey: [已隐藏]")
		config.Config.Purchase.Antom.YourPrivateKey = yourPrivateKey
		changed = true
	}

	// 检查并覆盖ANTOM_DOMAIN
	if domain := os.Getenv("ANTOM_DOMAIN"); domain != "" {
		fmt.Printf("[配置] 从环境变量覆盖 Antom Domain: %s\n", domain)
		config.Config.Purchase.Antom.Domain = domain
		changed = true
	}

	// 检查是否启用沙箱模式
	if sandboxStr := os.Getenv("ANTOM_SANDBOX"); sandboxStr != "" {
		isSandbox := (sandboxStr == "true" || sandboxStr == "1" || sandboxStr == "yes")
		fmt.Printf("[配置] 从环境变量覆盖 Antom Sandbox模式: %v\n", isSandbox)
		config.Config.Purchase.Antom.IsSandbox = isSandbox
		changed = true
	}

	if changed {
		fmt.Println("[配置] Antom支付配置已从环境变量更新")
	} else {
		fmt.Println("[配置] 未发现环境变量中的Antom支付配置")
	}
}

// maskSensitiveValue 隐藏敏感值，只显示前几个和后几个字符
func maskSensitiveValue(value string) string {
	if len(value) <= 10 {
		return "****" // 对于短字符串，完全隐藏
	}
	prefix := value[:5]
	suffix := value[len(value)-5:]
	return prefix + "..." + suffix
}

// loadFromYAML 从YAML数据加载配置
//
// data 参数包含YAML格式的配置数据
// 返回解析的配置和可能的错误
func loadFromYAML(data []byte) (*WordgateConfig, error) {
	// 创建顶级配置结构
	var topLevelConfig struct {
		Wordgate WordgateConfig `yaml:"wordgate"`
	}

	// 解析YAML数据
	err := yaml.Unmarshal(data, &topLevelConfig)
	if err != nil {
		return nil, fmt.Errorf("解析YAML配置失败: %w", err)
	}

	// 检查是否包含wordgate配置
	if isEmpty(topLevelConfig.Wordgate) {
		return nil, fmt.Errorf("配置文件中缺少wordgate配置或配置不完整")
	}

	return &topLevelConfig.Wordgate, nil
}

// loadFromJSON 从JSON数据加载配置
//
// data 参数包含JSON格式的配置数据
// 返回解析的配置和可能的错误
func loadFromJSON(data []byte) (*WordgateConfig, error) {
	// 创建顶级配置结构
	var topLevelConfig struct {
		Wordgate WordgateConfig `json:"wordgate"`
	}

	// 解析JSON数据
	err := json.Unmarshal(data, &topLevelConfig)
	if err != nil {
		return nil, fmt.Errorf("解析JSON配置失败: %w", err)
	}

	// 检查是否包含wordgate配置
	if isEmpty(topLevelConfig.Wordgate) {
		return nil, fmt.Errorf("配置文件中缺少wordgate配置或配置不完整")
	}

	return &topLevelConfig.Wordgate, nil
}

// isEmpty 判断WordgateConfig是否为空
//
// 当所有关键字段都为空时，认为配置为空
func isEmpty(config WordgateConfig) bool {
	return config.BaseURL == "" && config.AppCode == "" && config.AppSecret == ""
}

// ValidateConfig 验证配置是否合法
func ValidateConfig(config *WordgateConfig) error {
	if config == nil {
		return fmt.Errorf("配置不能为空")
	}

	// 检查基础配置
	if config.BaseURL == "" {
		return fmt.Errorf("BaseURL不能为空")
	}

	if config.AppCode == "" {
		return fmt.Errorf("AppCode不能为空")
	}

	if config.AppSecret == "" {
		return fmt.Errorf("AppSecret不能为空")
	}

	// 检查应用信息
	if config.App.Name == "" {
		return fmt.Errorf("应用名称不能为空")
	}

	if config.App.Currency == "" {
		return fmt.Errorf("结算货币不能为空")
	}

	// 检查支付配置
	if config.Config.Purchase.Antom.Enabled {
		if config.Config.Purchase.Antom.ClientID == "" {
			return fmt.Errorf("Antom支付ClientID不能为空")
		}
		if config.Config.Purchase.Antom.AntomPublicKey == "" {
			return fmt.Errorf("Antom公钥不能为空")
		}
		if config.Config.Purchase.Antom.YourPublicKey == "" {
			return fmt.Errorf("商户公钥不能为空")
		}
		if config.Config.Purchase.Antom.YourPrivateKey == "" {
			return fmt.Errorf("商户私钥不能为空")
		}
		if config.Config.Purchase.Antom.Domain == "" {
			return fmt.Errorf("Antom域名不能为空")
		}
	}

	// 检查Stripe配置
	if config.Config.Purchase.Stripe.Enabled {
		if config.Config.Purchase.Stripe.PublicKey == "" {
			return fmt.Errorf("Stripe公钥不能为空")
		}
		if config.Config.Purchase.Stripe.SecretKey == "" {
			return fmt.Errorf("Stripe密钥不能为空")
		}
		if config.Config.Purchase.Stripe.WebhookSecret == "" {
			return fmt.Errorf("Stripe Webhook密钥不能为空")
		}
	}

	// 检查Payssion配置
	if config.Config.Purchase.Payssion.Enabled {
		if config.Config.Purchase.Payssion.ApiKey == "" {
			return fmt.Errorf("Payssion API Key不能为空")
		}
		if config.Config.Purchase.Payssion.SecretKey == "" {
			return fmt.Errorf("Payssion Secret Key不能为空")
		}
	}

	// 检查网关模式配置
	if config.Config.Purchase.GatewayMode.Enabled {
		if config.Config.Purchase.GatewayMode.NotifyURL == "" {
			return fmt.Errorf("网关模式通知URL不能为空")
		}
		if config.Config.Purchase.GatewayMode.RedirectURL == "" {
			return fmt.Errorf("网关模式重定向URL不能为空")
		}
	}

	return nil
}
