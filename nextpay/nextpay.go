// Package nextpay provides integration with NextPay payment gateway.
//
// NextPay supports subscriptions, one-time payments, usage-based billing,
// and auto-recharge contracts.
//
// Usage:
//
//	// Create subscription
//	result, err := nextpay.CreateSubscription(&nextpay.SubscriptionRequest{
//	    UserID:     "user123",
//	    PlanID:     "plan_monthly",
//	    SuccessURL: "https://example.com/success",
//	    CancelURL:  "https://example.com/cancel",
//	})
//
//	// Create one-time payment
//	result, err := nextpay.CreateOrder(&nextpay.OrderRequest{
//	    UserID:  "user123",
//	    Amount:  999, // $9.99 in cents
//	    Currency: "USD",
//	})
//
//	// Query subscriptions
//	subs, err := nextpay.GetSubscriptions("user123")
package nextpay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Errors
var (
	ErrNotConfigured = errors.New("nextpay: not configured")
	ErrInvalidInput  = errors.New("nextpay: invalid input")
)

// APIError represents an error response from NextPay API.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("nextpay: API error %d: %s", e.Code, e.Message)
}

// Config holds NextPay configuration.
type Config struct {
	AccessKey string `yaml:"access_key"`
	Endpoint  string `yaml:"endpoint"`
	Timeout   int    `yaml:"timeout"` // seconds
}

var (
	globalConfig *Config
	globalClient *Client
	clientOnce   sync.Once
	initErr      error
	configMux    sync.RWMutex
)

// loadConfigFromViper loads configuration from viper.
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	cfg.AccessKey = viper.GetString("nextpay.access_key")
	cfg.Endpoint = viper.GetString("nextpay.endpoint")
	cfg.Timeout = viper.GetInt("nextpay.timeout")

	// Set defaults
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://pay.arbella.group"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30
	}

	// Validate
	if cfg.AccessKey == "" {
		return nil, fmt.Errorf("nextpay.access_key is required")
	}

	return cfg, nil
}

func initialize() {
	cfg, err := loadConfigFromViper()
	if err != nil {
		configMux.RLock()
		cfg = globalConfig
		configMux.RUnlock()

		if cfg == nil {
			initErr = fmt.Errorf("config not available: %v", err)
			return
		}
	} else {
		configMux.Lock()
		globalConfig = cfg
		configMux.Unlock()
	}

	globalClient, initErr = createClient(cfg)
}

func ensureInitialized() error {
	clientOnce.Do(initialize)
	return initErr
}

// SetConfig sets configuration manually (for testing).
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	globalConfig = cfg
	clientOnce = sync.Once{}
	globalClient = nil
	initErr = nil
}

func createClient(cfg *Config) (*Client, error) {
	return &Client{
		config: cfg,
		http: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}, nil
}

// Client is the NextPay API client.
type Client struct {
	config *Config
	http   *http.Client
}

// Get returns the initialized client.
func Get() (*Client, error) {
	if err := ensureInitialized(); err != nil {
		return nil, err
	}
	return globalClient, nil
}

// Response is the standard API response format.
type Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// --- Request/Response Types ---

// SubscriptionRequest represents a subscription creation request.
type SubscriptionRequest struct {
	UserID      string            `json:"userId"`
	PlanID      string            `json:"planId"`
	SuccessURL  string            `json:"successUrl"`
	CancelURL   string            `json:"cancelUrl"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	TrialDays   int               `json:"trialDays,omitempty"`
	CouponCode  string            `json:"couponCode,omitempty"`
}

// CheckoutResult represents the result of checkout operations.
type CheckoutResult struct {
	OrderID    string `json:"orderId"`
	PaymentURL string `json:"paymentUrl"`
}

// OrderRequest represents a one-time order creation request.
type OrderRequest struct {
	UserID      string            `json:"userId"`
	Amount      int               `json:"amount"` // in cents
	Currency    string            `json:"currency"`
	ProductID   string            `json:"productId,omitempty"`
	ProductName string            `json:"productName,omitempty"`
	SuccessURL  string            `json:"successUrl"`
	CancelURL   string            `json:"cancelUrl"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Subscription represents a user's subscription.
type Subscription struct {
	ID                string `json:"id"`
	UserID            string `json:"userId"`
	PlanID            string `json:"planId"`
	Status            string `json:"status"` // active, trialing, past_due, cancelled, expired
	CurrentPeriodEnd  string `json:"currentPeriodEnd,omitempty"`
	CancelAtPeriodEnd bool   `json:"cancelAtPeriodEnd,omitempty"`
}

// Order represents a user's order.
type Order struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Amount    int    `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"` // pending, paid, failed, cancelled, refunded
	CreatedAt string `json:"createdAt,omitempty"`
}

// PendingChargeRequest represents a usage-based billing charge.
type PendingChargeRequest struct {
	SubscriptionID string `json:"subscriptionId"`
	Amount         int    `json:"amount"` // in cents
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// PendingCharge represents a pending charge.
type PendingCharge struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscriptionId"`
	Amount         int    `json:"amount"`
	Status         string `json:"status"` // pending, billed, failed, cancelled
	Description    string `json:"description"`
}

// RechargeContractRequest represents an auto-recharge contract creation.
type RechargeContractRequest struct {
	UserID         string            `json:"userId"`
	MaxAmount      int               `json:"maxAmount"` // maximum per charge in cents
	SuccessURL     string            `json:"successUrl"`
	CancelURL      string            `json:"cancelUrl"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// RechargeContractResult represents the contract creation result.
type RechargeContractResult struct {
	ContractID       string `json:"contractId"`
	AuthorizationURL string `json:"authorizationUrl"`
}

// RechargeContract represents an auto-recharge contract.
type RechargeContract struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Status    string `json:"status"` // pending, active, cancelled
	MaxAmount int    `json:"maxAmount"`
}

// ContractChargeRequest represents a charge against a contract.
type ContractChargeRequest struct {
	Amount         int    `json:"amount"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	Description    string `json:"description,omitempty"`
}

// ContractChargeResult represents the result of a contract charge.
type ContractChargeResult struct {
	ChargeID string `json:"chargeId"`
	Status   string `json:"status"`
}

// --- Public API ---

// CreateSubscription creates a new subscription checkout session.
func CreateSubscription(req *SubscriptionRequest) (*CheckoutResult, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.createSubscription(req)
}

// CreateOrder creates a new one-time payment order.
func CreateOrder(req *OrderRequest) (*CheckoutResult, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.createOrder(req)
}

// GetSubscriptions returns subscriptions for a user.
func GetSubscriptions(userID string) ([]Subscription, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.getSubscriptions(userID)
}

// GetOrders returns orders for a user.
func GetOrders(userID string) ([]Order, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.getOrders(userID)
}

// CreatePendingCharge creates a usage-based pending charge.
func CreatePendingCharge(req *PendingChargeRequest) (*PendingCharge, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.createPendingCharge(req)
}

// GetPendingCharges returns pending charges for a subscription.
func GetPendingCharges(subscriptionID string) ([]PendingCharge, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.getPendingCharges(subscriptionID)
}

// CreateRechargeContract creates a new auto-recharge contract.
func CreateRechargeContract(req *RechargeContractRequest) (*RechargeContractResult, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.createRechargeContract(req)
}

// GetRechargeContract returns a recharge contract by ID.
func GetRechargeContract(contractID string) (*RechargeContract, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.getRechargeContract(contractID)
}

// ChargeContract executes a charge against a contract.
func ChargeContract(contractID string, req *ContractChargeRequest) (*ContractChargeResult, error) {
	client, err := Get()
	if err != nil {
		return nil, err
	}
	return client.chargeContract(contractID, req)
}

// CancelRechargeContract cancels a recharge contract.
func CancelRechargeContract(contractID string) error {
	client, err := Get()
	if err != nil {
		return err
	}
	return client.cancelRechargeContract(contractID)
}

// --- Client Methods ---

func (c *Client) doRequest(method, path string, body interface{}) (*Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.config.Endpoint+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.AccessKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, &APIError{
			Code:    apiResp.Code,
			Message: apiResp.Message,
		}
	}

	return &apiResp, nil
}

func (c *Client) createSubscription(req *SubscriptionRequest) (*CheckoutResult, error) {
	resp, err := c.doRequest("POST", "/api/checkout/subscription", req)
	if err != nil {
		return nil, err
	}

	var result CheckoutResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode result: %w", err)
	}
	return &result, nil
}

func (c *Client) createOrder(req *OrderRequest) (*CheckoutResult, error) {
	resp, err := c.doRequest("POST", "/api/checkout/order", req)
	if err != nil {
		return nil, err
	}

	var result CheckoutResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode result: %w", err)
	}
	return &result, nil
}

func (c *Client) getSubscriptions(userID string) ([]Subscription, error) {
	resp, err := c.doRequest("GET", "/api/subscriptions?userId="+userID, nil)
	if err != nil {
		return nil, err
	}

	var subs []Subscription
	if err := json.Unmarshal(resp.Data, &subs); err != nil {
		return nil, fmt.Errorf("failed to decode subscriptions: %w", err)
	}
	return subs, nil
}

func (c *Client) getOrders(userID string) ([]Order, error) {
	resp, err := c.doRequest("GET", "/api/orders?userId="+userID, nil)
	if err != nil {
		return nil, err
	}

	var orders []Order
	if err := json.Unmarshal(resp.Data, &orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %w", err)
	}
	return orders, nil
}

func (c *Client) createPendingCharge(req *PendingChargeRequest) (*PendingCharge, error) {
	resp, err := c.doRequest("POST", "/api/billing/pending-charges", req)
	if err != nil {
		return nil, err
	}

	var charge PendingCharge
	if err := json.Unmarshal(resp.Data, &charge); err != nil {
		return nil, fmt.Errorf("failed to decode charge: %w", err)
	}
	return &charge, nil
}

func (c *Client) getPendingCharges(subscriptionID string) ([]PendingCharge, error) {
	resp, err := c.doRequest("GET", "/api/billing/pending-charges?subscriptionId="+subscriptionID, nil)
	if err != nil {
		return nil, err
	}

	var charges []PendingCharge
	if err := json.Unmarshal(resp.Data, &charges); err != nil {
		return nil, fmt.Errorf("failed to decode charges: %w", err)
	}
	return charges, nil
}

func (c *Client) createRechargeContract(req *RechargeContractRequest) (*RechargeContractResult, error) {
	resp, err := c.doRequest("POST", "/api/recharge-contracts", req)
	if err != nil {
		return nil, err
	}

	var result RechargeContractResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode result: %w", err)
	}
	return &result, nil
}

func (c *Client) getRechargeContract(contractID string) (*RechargeContract, error) {
	resp, err := c.doRequest("GET", "/api/recharge-contracts/"+contractID, nil)
	if err != nil {
		return nil, err
	}

	var contract RechargeContract
	if err := json.Unmarshal(resp.Data, &contract); err != nil {
		return nil, fmt.Errorf("failed to decode contract: %w", err)
	}
	return &contract, nil
}

func (c *Client) chargeContract(contractID string, req *ContractChargeRequest) (*ContractChargeResult, error) {
	resp, err := c.doRequest("POST", "/api/recharge-contracts/"+contractID+"/charge", req)
	if err != nil {
		return nil, err
	}

	var result ContractChargeResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode result: %w", err)
	}
	return &result, nil
}

func (c *Client) cancelRechargeContract(contractID string) error {
	_, err := c.doRequest("DELETE", "/api/recharge-contracts/"+contractID, nil)
	return err
}
