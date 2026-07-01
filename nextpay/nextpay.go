// Package nextpay provides integration with the NextPay payment gateway.
//
// NextPay supports subscriptions, one-time payments, usage-based (post-paid)
// billing, and auto-recharge contracts. This package wraps the App-facing
// endpoints under /api/*, authenticated with the app's AccessKey.
//
// Authentication: every request carries the app AccessKey in the X-Access-Key
// header. (The server treats Authorization: Bearer as a JWT, which is a
// different auth path — do not use it here.)
//
// Conventions that mirror the server contract:
//   - All amounts are unsigned integers in the smallest currency unit (cents).
//   - Timestamps are Unix seconds (int64), except the grant response, whose
//     period bounds are RFC3339 strings.
//   - A plan is referenced by its stable Code; plans no longer carry a uuid.
//   - List endpoints return {items, pagination}; the getters here return the
//     first page (up to 100 items).
//
// Usage:
//
//	// One-time payment
//	res, err := nextpay.CreateOrder(&nextpay.OrderRequest{
//	    UserID: "user123", Email: "u@example.com",
//	    ProductName: "Premium", Amount: 999, // $9.99
//	})
//
//	// Subscription checkout
//	res, err := nextpay.CreateSubscription(&nextpay.SubscriptionRequest{
//	    UserID: "user123", Email: "u@example.com", Code: "pro-monthly",
//	})
//
//	// Grant a comped subscription (no first payment)
//	g, err := nextpay.GrantSubscription(&nextpay.GrantSubscriptionRequest{
//	    UserID: "user123", Code: "pro-monthly",
//	    IdempotencyKey: "grant-2026-07-01-user123-pro",
//	})
//
//	// Plan management
//	plans, err := nextpay.ListPlans(false)
//	plan,  err := nextpay.CreatePlan(&nextpay.CreatePlanRequest{
//	    Code: "pro-monthly", Name: "Pro", Amount: 999, IntervalType: "month",
//	})
//
//	// Subscription lifecycle — one call per intent:
//	err := nextpay.SetAutoRenew("sub_123", false) // stop auto-renew, keep access to period end
//	sub, err := nextpay.PauseSubscription("sub_123")
//	sub, err := nextpay.ResumeSubscription("sub_123", true /* payWithWallet */)
//	err := nextpay.CancelSubscription("sub_123")  // hard cancel now (admin/support)
package nextpay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Errors
var (
	ErrNotConfigured = errors.New("nextpay: not configured")
	ErrInvalidInput  = errors.New("nextpay: invalid input")
)

// APIError represents a non-zero code returned by the NextPay API.
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
	cfg := &Config{
		AccessKey: viper.GetString("nextpay.access_key"),
		Endpoint:  viper.GetString("nextpay.endpoint"),
		Timeout:   viper.GetInt("nextpay.timeout"),
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://pay.arbella.group"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30
	}

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

// Response is the standard API envelope: {code, message, data}.
type Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// --- Data types (mirror the server DTOs) ---

// Plan is a subscription plan. It is identified by its stable Code.
type Plan struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Amount       uint64 `json:"amount"`       // in cents
	Currency     string `json:"currency"`     // always "usd"
	IntervalType string `json:"intervalType"` // month | quarter | year | lifetime
	TrialDays    int    `json:"trialDays"`
	IsActive     bool   `json:"isActive"`
	CreatedAt    int64  `json:"createdAt"` // unix seconds
}

// User is the slim user view embedded in orders, subscriptions and contracts.
// The App side only ever sees its own UserID (the external id it supplied); the
// server's internal uuid is never exposed on these App-facing endpoints.
type User struct {
	UserID    string `json:"userId"` // app-side (external) user id
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"`
}

// Order is a user's order.
type Order struct {
	UUID                    string `json:"uuid"`
	Amount                  uint64 `json:"amount"`
	Currency                string `json:"currency"`
	Status                  string `json:"status"` // pending, processing, paid, failed, refunded, cancelled
	PaymentMethod           string `json:"paymentMethod"`
	IsSubscription          bool   `json:"isSubscription"`
	SinglePaymentEquivalent bool   `json:"singlePaymentEquivalent"`
	ProductName             string `json:"productName,omitempty"`
	ProductDescription      string `json:"productDescription,omitempty"`
	ObjectID                string `json:"objectId,omitempty"`
	Metadata                string `json:"metadata,omitempty"`
	SuccessURL              string `json:"successUrl"`
	CancelURL               string `json:"cancelUrl"`
	CreatedAt               int64  `json:"createdAt"`
	PaidAt                  int64  `json:"paidAt,omitempty"`
	User                    *User  `json:"user,omitempty"`
	Plan                    *Plan  `json:"plan,omitempty"`
}

// Subscription is a user's subscription.
type Subscription struct {
	UUID               string `json:"uuid"`
	Status             string `json:"status"` // active, trialing, past_due, paused, cancelled, expired
	AutoRenew          bool   `json:"autoRenew"`
	CurrentPeriodStart int64  `json:"currentPeriodStart"` // unix seconds
	CurrentPeriodEnd   int64  `json:"currentPeriodEnd"`   // unix seconds
	CancelAtPeriodEnd  bool   `json:"cancelAtPeriodEnd"`
	PauseAtPeriodEnd   bool   `json:"pauseAtPeriodEnd"`
	PaymentMethodLast4 string `json:"paymentMethodLast4,omitempty"`
	PaymentMethodBrand string `json:"paymentMethodBrand,omitempty"`
	PausedAt           int64  `json:"pausedAt,omitempty"`
	CancelledAt        int64  `json:"cancelledAt,omitempty"`
	CreatedAt          int64  `json:"createdAt"`
	Plan               *Plan  `json:"plan,omitempty"`
	User               *User  `json:"user,omitempty"`
}

// PendingCharge is a post-paid charge as returned by list/get.
type PendingCharge struct {
	UUID             string `json:"uuid"`
	SubscriptionUUID string `json:"subscriptionUuid"`
	Amount           uint64 `json:"amount"`
	Currency         string `json:"currency"`
	Description      string `json:"description"`
	BillingPeriod    string `json:"billingPeriod"` // YYYY-MM
	Status           string `json:"status"`
	BilledAt         int64  `json:"billedAt,omitempty"`
	Metadata         string `json:"metadata,omitempty"`
	CreatedAt        int64  `json:"createdAt"`
}

// RechargeContract is an auto-recharge contract as returned by get.
type RechargeContract struct {
	UUID                  string `json:"uuid"`
	DefaultAmount         uint64 `json:"defaultAmount"`
	Currency              string `json:"currency"`
	Status                string `json:"status"` // pending_setup, active, cancelled, failed
	PaymentMethodLast4    string `json:"paymentMethodLast4,omitempty"`
	PaymentMethodBrand    string `json:"paymentMethodBrand,omitempty"`
	PaymentMethodExpMonth int    `json:"paymentMethodExpMonth,omitempty"`
	PaymentMethodExpYear  int    `json:"paymentMethodExpYear,omitempty"`
	WebhookURL            string `json:"webhookUrl,omitempty"`
	ActivatedAt           int64  `json:"activatedAt,omitempty"`
	CreatedAt             int64  `json:"createdAt"`
	User                  *User  `json:"user,omitempty"`
}

// --- Request/result types ---

// OrderRequest creates a one-time payment order.
type OrderRequest struct {
	UserID             string `json:"userId"`
	Email              string `json:"email"`
	Name               string `json:"name,omitempty"`
	ProductName        string `json:"productName"`
	ProductDescription string `json:"productDescription,omitempty"`
	Amount             uint64 `json:"amount"` // in cents, > 0
	Currency           string `json:"currency,omitempty"`
	ObjectID           string `json:"objectId,omitempty"`
	SuccessURL         string `json:"successUrl,omitempty"`
	CancelURL          string `json:"cancelUrl,omitempty"`
	Metadata           string `json:"metadata,omitempty"`
}

// OrderResult is the result of CreateOrder.
type OrderResult struct {
	OrderID     string `json:"orderId"`
	PaymentURL  string `json:"paymentUrl"`
	Amount      uint64 `json:"amount"`
	Currency    string `json:"currency"`
	ProductName string `json:"productName"`
}

// SubscriptionRequest creates a subscription checkout order.
type SubscriptionRequest struct {
	UserID              string `json:"userId"`
	Email               string `json:"email"`
	Name                string `json:"name,omitempty"`
	Code                string `json:"code"`             // plan code (required)
	Period              string `json:"period,omitempty"` // monthly | quarterly | yearly | lifetime
	Amount              uint64 `json:"amount,omitempty"` // optional custom price (first-order discount)
	DiscountDescription string `json:"discountDescription,omitempty"`
	ObjectID            string `json:"objectId,omitempty"`
	SuccessURL          string `json:"successUrl,omitempty"`
	CancelURL           string `json:"cancelUrl,omitempty"`
}

// SubscriptionResult is the result of CreateSubscription.
type SubscriptionResult struct {
	OrderID    string `json:"orderId"`
	PaymentURL string `json:"paymentUrl"`
	Amount     uint64 `json:"amount"`
	Plan       *Plan  `json:"plan"`
}

// GrantSubscriptionRequest directly grants a user an active subscription with
// no first payment (comps, partnerships, operational make-goods).
type GrantSubscriptionRequest struct {
	UserID         string `json:"userId,omitempty"` // app-side user id; one of UserID/Email required
	Email          string `json:"email,omitempty"`  // used to create the user if UserID absent
	Name           string `json:"name,omitempty"`   // optional display name
	Code           string `json:"code"`             // plan code (required)
	ObjectID       string `json:"objectId,omitempty"`
	Reason         string `json:"reason,omitempty"` // audit note
	IdempotencyKey string `json:"idempotencyKey"`   // required; unique per app, dedupes retries
}

// GrantedSubscription is the subscription returned by a grant. Its period bounds
// are RFC3339 strings (unlike Subscription, whose bounds are unix seconds).
type GrantedSubscription struct {
	UUID               string `json:"uuid"`
	Plan               *Plan  `json:"plan"`
	Status             string `json:"status"` // "active"
	CurrentPeriodStart string `json:"currentPeriodStart"`
	CurrentPeriodEnd   string `json:"currentPeriodEnd"`
}

// GrantResult is the result of GrantSubscription.
type GrantResult struct {
	Subscription GrantedSubscription `json:"subscription"`
	OrderUUID    string              `json:"orderUuid"`
}

// CreatePlanRequest creates or upserts a plan. When Code is set the call is an
// idempotent upsert on (app, code); omit Code for a one-off plan with no key.
type CreatePlanRequest struct {
	Code         string `json:"code,omitempty"`
	Name         string `json:"name"` // required
	Description  string `json:"description,omitempty"`
	Amount       uint64 `json:"amount"`             // required, > 0, in cents
	Currency     string `json:"currency,omitempty"` // defaults to "usd" (only usd supported)
	IntervalType string `json:"intervalType"`       // required: month|quarter|year|lifetime
	TrialDays    int    `json:"trialDays,omitempty"`
}

// UpdatePlanRequest patches a plan. Nil pointers are left unchanged; empty
// strings for Name/Description are ignored by the server.
type UpdatePlanRequest struct {
	Name         string  `json:"name,omitempty"`
	Description  string  `json:"description,omitempty"`
	Amount       *uint64 `json:"amount,omitempty"`
	IntervalType *string `json:"intervalType,omitempty"`
	IsActive     *bool   `json:"isActive,omitempty"`
}

// RenewResult is the result of RenewSubscription (a renewal order + payment URL).
type RenewResult struct {
	OrderID    string `json:"orderId"`
	PaymentURL string `json:"paymentUrl"`
}

// PendingChargeRequest creates a post-paid charge.
type PendingChargeRequest struct {
	SubscriptionID string `json:"subscriptionId"`
	Amount         uint64 `json:"amount"` // in cents, > 0
	Description    string `json:"description"`
	Metadata       string `json:"metadata,omitempty"`
}

// PendingChargeResult is the result of CreatePendingCharge (distinct from the
// PendingCharge shape returned by list/get).
type PendingChargeResult struct {
	ChargeID            string `json:"chargeId"`
	AppID               string `json:"appId"`
	UserID              string `json:"userId"`
	SubscriptionID      string `json:"subscriptionId"`
	Amount              uint64 `json:"amount"`
	Currency            string `json:"currency"`
	Description         string `json:"description"`
	BillingPeriod       string `json:"billingPeriod"`
	Status              string `json:"status"`
	StripeInvoiceItemID string `json:"stripeInvoiceItemId,omitempty"`
	CreatedAt           int64  `json:"createdAt"`
}

// RechargeContractRequest creates an auto-recharge contract.
type RechargeContractRequest struct {
	UserID        string `json:"userId"`
	Email         string `json:"email"`
	Name          string `json:"name,omitempty"`
	DefaultAmount uint64 `json:"defaultAmount"` // in cents, > 0
	Currency      string `json:"currency,omitempty"`
	OrderUUID     string `json:"orderUuid"`               // required; related order used on fallback
	AllowFallback *bool  `json:"allowFallback,omitempty"` // allow degrade to one-time payment; default true
	SuccessURL    string `json:"successUrl,omitempty"`
	CancelURL     string `json:"cancelUrl,omitempty"`
	WebhookURL    string `json:"webhookUrl,omitempty"`
}

// RechargeContractResult is the result of CreateRechargeContract.
type RechargeContractResult struct {
	ContractID       string `json:"contractId"`
	UserID           string `json:"userId"`
	DefaultAmount    uint64 `json:"defaultAmount"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
	PaymentURL       string `json:"paymentUrl"`
	AuthorizationURL string `json:"authorizationUrl"`
	CreatedAt        int64  `json:"createdAt"`
}

// ChargeRequest executes a charge against a contract.
type ChargeRequest struct {
	Amount         uint64 `json:"amount"` // in cents, > 0
	IdempotencyKey string `json:"idempotencyKey"`
	Description    string `json:"description,omitempty"`
	Metadata       string `json:"metadata,omitempty"`
}

// ChargeResult is the result of ChargeContract.
type ChargeResult struct {
	ChargeID              string `json:"chargeId"`
	Status                string `json:"status"`
	Amount                uint64 `json:"amount"`
	Currency              string `json:"currency"`
	StripePaymentIntentID string `json:"stripePaymentIntentId,omitempty"`
}

// WalletDepositRequest credits a user's wallet. The user (identified by UserID,
// the app-side user id) is created on demand, so Email is required here.
type WalletDepositRequest struct {
	UserID         string `json:"userId"` // app-side user id (external), auto-created
	Email          string `json:"email"`
	Name           string `json:"name,omitempty"`
	Amount         uint64 `json:"amount"` // in cents, > 0
	IdempotencyKey string `json:"idempotencyKey"`
	Description    string `json:"description,omitempty"`
	RelatedOrderID string `json:"relatedOrderId,omitempty"` // order uuid, optional
}

// WalletDeductRequest debits a user's wallet. Fails if the balance is
// insufficient or the user does not exist.
type WalletDeductRequest struct {
	UserID         string `json:"userId"` // app-side user id (external); must already exist
	Amount         uint64 `json:"amount"` // in cents, > 0
	IdempotencyKey string `json:"idempotencyKey"`
	Description    string `json:"description,omitempty"`
	RelatedOrderID string `json:"relatedOrderId,omitempty"` // order uuid, optional
}

// WalletOperation is the result of a deposit or deduct.
type WalletOperation struct {
	TransactionID string `json:"transactionId"`
	Amount        uint64 `json:"amount"`
	Balance       uint64 `json:"balance"` // balance after the operation
}

// WalletBalance is a user's current wallet balance.
type WalletBalance struct {
	UserID   string `json:"userId"` // app-side user id (external)
	Balance  uint64 `json:"balance"`
	Currency string `json:"currency"`
}

// WalletTransaction is one wallet ledger entry. Amount is signed: negative for
// deductions, positive for deposits.
type WalletTransaction struct {
	UUID           string `json:"uuid"`
	Type           string `json:"type"`
	Amount         int64  `json:"amount"`
	BalanceAfter   uint64 `json:"balanceAfter"`
	IdempotencyKey string `json:"idempotencyKey"`
	RelatedOrderID string `json:"relatedOrderId,omitempty"`
	Description    string `json:"description,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
}

// --- Public API ---

// CreateOrder creates a one-time payment order.
func CreateOrder(req *OrderRequest) (*OrderResult, error) {
	return do(func(c *Client) (*OrderResult, error) { return c.createOrder(req) })
}

// CreateSubscription creates a subscription checkout order.
func CreateSubscription(req *SubscriptionRequest) (*SubscriptionResult, error) {
	return do(func(c *Client) (*SubscriptionResult, error) { return c.createSubscription(req) })
}

// GrantSubscription directly grants a user an active subscription with no first
// payment. The subscription is active immediately (no payment page, no Stripe
// session, no wallet deduction); the first period is comped.
//
// Renewal semantics — read this before relying on "it keeps charging":
// a granted subscription carries NO saved card (grant never runs a Stripe
// authorization), so at period end the hourly renewal only has the wallet rail:
//   - paid plan, wallet funded (>= plan amount): auto-renews from wallet;
//   - paid plan, wallet short and no card: expires at period end (no grace);
//   - free plan (amount 0): renews forever.
//
// So to make a granted paid subscription keep auto-charging, keep the user's
// wallet topped up (see WalletDeposit) — renewal is wallet-first. Off-session
// card auto-renewal is NOT possible after a grant: that requires a card mandate
// the user established through normal checkout or a recharge contract.
//
// Idempotent on req.IdempotencyKey: a retry with the same key and parameters
// returns the same subscription. No webhook is emitted — the caller already has
// the result from this HTTP response. Errors surface as *APIError (use
// errors.As, then switch on Code):
//   - 400404: the idempotency key was reused with different parameters
//   - 400403: the user already has an active subscription to this plan
//   - 400301: the plan does not exist
func GrantSubscription(req *GrantSubscriptionRequest) (*GrantResult, error) {
	return do(func(c *Client) (*GrantResult, error) { return c.grantSubscription(req) })
}

// GetOrders returns the first page of orders for a user.
func GetOrders(userID string) ([]Order, error) {
	return do(func(c *Client) ([]Order, error) { return c.getOrders(userID) })
}

// GetOrder returns a single order by its uuid.
func GetOrder(orderUUID string) (*Order, error) {
	return do(func(c *Client) (*Order, error) { return c.getOrder(orderUUID) })
}

// ListPlans returns the app's plans. When includeInactive is false only active
// plans are returned.
func ListPlans(includeInactive bool) ([]Plan, error) {
	return do(func(c *Client) ([]Plan, error) { return c.listPlans(includeInactive) })
}

// GetPlan returns a single plan by its code.
func GetPlan(code string) (*Plan, error) {
	return do(func(c *Client) (*Plan, error) { return c.getPlan(code) })
}

// CreatePlan creates or upserts a plan. When req.Code is set, an existing plan
// with that code is updated (idempotent upsert); otherwise a new plan is created.
func CreatePlan(req *CreatePlanRequest) (*Plan, error) {
	return do(func(c *Client) (*Plan, error) { return c.createPlan(req) })
}

// UpdatePlan patches a plan identified by its code.
func UpdatePlan(code string, req *UpdatePlanRequest) error {
	_, err := do(func(c *Client) (struct{}, error) { return struct{}{}, c.updatePlan(code, req) })
	return err
}

// DeletePlan soft-deletes a plan identified by its code.
func DeletePlan(code string) error {
	_, err := do(func(c *Client) (struct{}, error) { return struct{}{}, c.deletePlan(code) })
	return err
}

// GetSubscriptions returns the first page of subscriptions for a user.
func GetSubscriptions(userID string) ([]Subscription, error) {
	return do(func(c *Client) ([]Subscription, error) { return c.getSubscriptions(userID) })
}

// GetSubscription returns a single subscription by its uuid.
func GetSubscription(subscriptionUUID string) (*Subscription, error) {
	return do(func(c *Client) (*Subscription, error) { return c.getSubscription(subscriptionUUID) })
}

// SetAutoRenew enables or disables automatic renewal for a subscription.
// Passing false turns off auto-renew (cancel at period end); access is kept
// until the current period ends.
func SetAutoRenew(subscriptionUUID string, enabled bool) error {
	_, err := do(func(c *Client) (struct{}, error) { return struct{}{}, c.setAutoRenew(subscriptionUUID, enabled) })
	return err
}

// CancelSubscription immediately and permanently terminates a subscription.
//
// This is the hard "cancel now" path — typically an admin/support action
// (refund, fraud, account deletion). For everyday customer flows prefer:
//   - SetAutoRenew(id, false): stop auto-renew, keep access until period end
//   - PauseSubscription(id):   suspend with the option to resume later
func CancelSubscription(subscriptionUUID string) error {
	_, err := do(func(c *Client) (struct{}, error) { return struct{}{}, c.cancelSubscription(subscriptionUUID) })
	return err
}

// PauseSubscription pauses an active subscription.
func PauseSubscription(subscriptionUUID string) (*Subscription, error) {
	return do(func(c *Client) (*Subscription, error) { return c.pauseSubscription(subscriptionUUID) })
}

// ResumeSubscription resumes a paused subscription.
func ResumeSubscription(subscriptionUUID string, payWithWallet bool) (*Subscription, error) {
	return do(func(c *Client) (*Subscription, error) { return c.resumeSubscription(subscriptionUUID, payWithWallet) })
}

// RenewSubscription creates a manual renewal order and returns its payment URL.
func RenewSubscription(subscriptionUUID string) (*RenewResult, error) {
	return do(func(c *Client) (*RenewResult, error) { return c.renewSubscription(subscriptionUUID) })
}

// CreatePendingCharge creates a post-paid charge for a subscription.
func CreatePendingCharge(req *PendingChargeRequest) (*PendingChargeResult, error) {
	return do(func(c *Client) (*PendingChargeResult, error) { return c.createPendingCharge(req) })
}

// GetPendingCharges returns the first page of pending charges for a subscription.
func GetPendingCharges(subscriptionUUID string) ([]PendingCharge, error) {
	return do(func(c *Client) ([]PendingCharge, error) { return c.getPendingCharges(subscriptionUUID) })
}

// GetPendingCharge returns a single pending charge by its uuid.
func GetPendingCharge(chargeUUID string) (*PendingCharge, error) {
	return do(func(c *Client) (*PendingCharge, error) { return c.getPendingCharge(chargeUUID) })
}

// CreateRechargeContract creates a new auto-recharge contract.
func CreateRechargeContract(req *RechargeContractRequest) (*RechargeContractResult, error) {
	return do(func(c *Client) (*RechargeContractResult, error) { return c.createRechargeContract(req) })
}

// GetRechargeContract returns a recharge contract by its uuid.
func GetRechargeContract(contractUUID string) (*RechargeContract, error) {
	return do(func(c *Client) (*RechargeContract, error) { return c.getRechargeContract(contractUUID) })
}

// ChargeContract executes a charge against a contract.
func ChargeContract(contractUUID string, req *ChargeRequest) (*ChargeResult, error) {
	return do(func(c *Client) (*ChargeResult, error) { return c.chargeContract(contractUUID, req) })
}

// CancelRechargeContract cancels a recharge contract.
func CancelRechargeContract(contractUUID string) error {
	_, err := do(func(c *Client) (struct{}, error) { return struct{}{}, c.cancelRechargeContract(contractUUID) })
	return err
}

// WalletDeposit credits a user's wallet (creating the user on demand).
func WalletDeposit(req *WalletDepositRequest) (*WalletOperation, error) {
	return do(func(c *Client) (*WalletOperation, error) { return c.walletDeposit(req) })
}

// WalletDeduct debits a user's wallet.
func WalletDeduct(req *WalletDeductRequest) (*WalletOperation, error) {
	return do(func(c *Client) (*WalletOperation, error) { return c.walletDeduct(req) })
}

// GetWalletBalance returns a user's wallet balance. userID is the app-side user id.
func GetWalletBalance(userID string) (*WalletBalance, error) {
	return do(func(c *Client) (*WalletBalance, error) { return c.walletBalance(userID) })
}

// GetWalletTransactions returns the first page of a user's wallet ledger. userID
// is the app-side user id.
func GetWalletTransactions(userID string) ([]WalletTransaction, error) {
	return do(func(c *Client) ([]WalletTransaction, error) { return c.walletTransactions(userID) })
}

// --- Transport ---

// do resolves the client and runs fn, so every public method shares one
// initialization + error path.
func do[T any](fn func(*Client) (T, error)) (T, error) {
	var zero T
	client, err := Get()
	if err != nil {
		return zero, err
	}
	return fn(client)
}

// doRequest performs an authenticated request and returns the decoded envelope.
// A non-zero response code is mapped to *APIError.
func (c *Client) doRequest(method, path string, body any) (*Response, error) {
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

	// AccessKey auth: the server reads X-Access-Key. (Authorization: Bearer is
	// parsed as a JWT and would be rejected.)
	req.Header.Set("X-Access-Key", c.config.AccessKey)
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
		return nil, &APIError{Code: apiResp.Code, Message: apiResp.Message}
	}

	return &apiResp, nil
}

// decodeData unmarshals a single-object data payload into T.
func decodeData[T any](raw json.RawMessage) (*T, error) {
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("failed to decode result: %w", err)
	}
	return &out, nil
}

// decodeItems unmarshals a list data payload ({items:[...]}) into []T.
func decodeItems[T any](raw json.RawMessage) ([]T, error) {
	var wrap struct {
		Items []T `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("failed to decode list: %w", err)
	}
	return wrap.Items, nil
}

// --- Client methods: checkout ---

func (c *Client) createOrder(req *OrderRequest) (*OrderResult, error) {
	resp, err := c.doRequest("POST", "/api/checkout/order", req)
	if err != nil {
		return nil, err
	}
	return decodeData[OrderResult](resp.Data)
}

func (c *Client) createSubscription(req *SubscriptionRequest) (*SubscriptionResult, error) {
	resp, err := c.doRequest("POST", "/api/checkout/subscription", req)
	if err != nil {
		return nil, err
	}
	return decodeData[SubscriptionResult](resp.Data)
}

func (c *Client) grantSubscription(req *GrantSubscriptionRequest) (*GrantResult, error) {
	resp, err := c.doRequest("POST", "/api/checkout/subscription/grant", req)
	if err != nil {
		return nil, err
	}
	return decodeData[GrantResult](resp.Data)
}

// --- Client methods: orders ---

func (c *Client) getOrders(userID string) ([]Order, error) {
	resp, err := c.doRequest("GET", "/api/users/"+url.PathEscape(userID)+"/orders?pageSize=100", nil)
	if err != nil {
		return nil, err
	}
	return decodeItems[Order](resp.Data)
}

func (c *Client) getOrder(orderUUID string) (*Order, error) {
	resp, err := c.doRequest("GET", "/api/orders/"+url.PathEscape(orderUUID), nil)
	if err != nil {
		return nil, err
	}
	return decodeData[Order](resp.Data)
}

// --- Client methods: plans ---

func (c *Client) listPlans(includeInactive bool) ([]Plan, error) {
	path := "/api/plans"
	if includeInactive {
		path += "?activeOnly=false"
	}
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	return decodeItems[Plan](resp.Data)
}

func (c *Client) getPlan(code string) (*Plan, error) {
	resp, err := c.doRequest("GET", "/api/plans/"+url.PathEscape(code), nil)
	if err != nil {
		return nil, err
	}
	return decodeData[Plan](resp.Data)
}

func (c *Client) createPlan(req *CreatePlanRequest) (*Plan, error) {
	resp, err := c.doRequest("POST", "/api/plans", req)
	if err != nil {
		return nil, err
	}
	return decodeData[Plan](resp.Data)
}

func (c *Client) updatePlan(code string, req *UpdatePlanRequest) error {
	_, err := c.doRequest("PUT", "/api/plans/"+url.PathEscape(code), req)
	return err
}

func (c *Client) deletePlan(code string) error {
	_, err := c.doRequest("DELETE", "/api/plans/"+url.PathEscape(code), nil)
	return err
}

// --- Client methods: subscriptions ---

func (c *Client) getSubscriptions(userID string) ([]Subscription, error) {
	resp, err := c.doRequest("GET", "/api/users/"+url.PathEscape(userID)+"/subscriptions?pageSize=100", nil)
	if err != nil {
		return nil, err
	}
	return decodeItems[Subscription](resp.Data)
}

func (c *Client) getSubscription(subscriptionUUID string) (*Subscription, error) {
	resp, err := c.doRequest("GET", "/api/subscriptions/"+url.PathEscape(subscriptionUUID), nil)
	if err != nil {
		return nil, err
	}
	return decodeData[Subscription](resp.Data)
}

func (c *Client) setAutoRenew(subscriptionUUID string, enabled bool) error {
	_, err := c.doRequest("POST", "/api/subscriptions/"+url.PathEscape(subscriptionUUID)+"/auto-renew",
		map[string]any{"enabled": enabled})
	return err
}

func (c *Client) cancelSubscription(subscriptionUUID string) error {
	// cancelAtPeriodEnd=false -> terminate immediately. The "cancel at period
	// end" intent is owned solely by SetAutoRenew(id, false).
	_, err := c.doRequest("POST", "/api/subscriptions/"+url.PathEscape(subscriptionUUID)+"/cancel",
		map[string]any{"cancelAtPeriodEnd": false})
	return err
}

func (c *Client) pauseSubscription(subscriptionUUID string) (*Subscription, error) {
	resp, err := c.doRequest("POST", "/api/subscriptions/"+url.PathEscape(subscriptionUUID)+"/pause", nil)
	if err != nil {
		return nil, err
	}
	return decodeData[Subscription](resp.Data)
}

func (c *Client) resumeSubscription(subscriptionUUID string, payWithWallet bool) (*Subscription, error) {
	resp, err := c.doRequest("POST", "/api/subscriptions/"+url.PathEscape(subscriptionUUID)+"/resume",
		map[string]any{"payWithWallet": payWithWallet})
	if err != nil {
		return nil, err
	}
	return decodeData[Subscription](resp.Data)
}

func (c *Client) renewSubscription(subscriptionUUID string) (*RenewResult, error) {
	resp, err := c.doRequest("POST", "/api/subscriptions/"+url.PathEscape(subscriptionUUID)+"/renew", nil)
	if err != nil {
		return nil, err
	}
	return decodeData[RenewResult](resp.Data)
}

// --- Client methods: billing (post-paid) ---

func (c *Client) createPendingCharge(req *PendingChargeRequest) (*PendingChargeResult, error) {
	resp, err := c.doRequest("POST", "/api/billing/pending-charges", req)
	if err != nil {
		return nil, err
	}
	return decodeData[PendingChargeResult](resp.Data)
}

func (c *Client) getPendingCharges(subscriptionUUID string) ([]PendingCharge, error) {
	resp, err := c.doRequest("GET", "/api/billing/pending-charges?"+listQuery("subscriptionId", subscriptionUUID), nil)
	if err != nil {
		return nil, err
	}
	return decodeItems[PendingCharge](resp.Data)
}

func (c *Client) getPendingCharge(chargeUUID string) (*PendingCharge, error) {
	resp, err := c.doRequest("GET", "/api/billing/pending-charges/"+url.PathEscape(chargeUUID), nil)
	if err != nil {
		return nil, err
	}
	return decodeData[PendingCharge](resp.Data)
}

// --- Client methods: recharge contracts ---

func (c *Client) createRechargeContract(req *RechargeContractRequest) (*RechargeContractResult, error) {
	resp, err := c.doRequest("POST", "/api/recharge-contracts", req)
	if err != nil {
		return nil, err
	}
	return decodeData[RechargeContractResult](resp.Data)
}

func (c *Client) getRechargeContract(contractUUID string) (*RechargeContract, error) {
	resp, err := c.doRequest("GET", "/api/recharge-contracts/"+url.PathEscape(contractUUID), nil)
	if err != nil {
		return nil, err
	}
	return decodeData[RechargeContract](resp.Data)
}

func (c *Client) chargeContract(contractUUID string, req *ChargeRequest) (*ChargeResult, error) {
	resp, err := c.doRequest("POST", "/api/recharge-contracts/"+url.PathEscape(contractUUID)+"/charge", req)
	if err != nil {
		return nil, err
	}
	return decodeData[ChargeResult](resp.Data)
}

func (c *Client) cancelRechargeContract(contractUUID string) error {
	_, err := c.doRequest("DELETE", "/api/recharge-contracts/"+url.PathEscape(contractUUID), nil)
	return err
}

// --- Client methods: wallets ---

// walletPath builds the user-scoped wallet path. The user (external id) lives in
// the URL; the server takes it from there and ignores any userId in the body.
func walletPath(userID, action string) string {
	return "/api/users/" + url.PathEscape(userID) + "/wallet/" + action
}

func (c *Client) walletDeposit(req *WalletDepositRequest) (*WalletOperation, error) {
	resp, err := c.doRequest("POST", walletPath(req.UserID, "deposit"), req)
	if err != nil {
		return nil, err
	}
	return decodeData[WalletOperation](resp.Data)
}

func (c *Client) walletDeduct(req *WalletDeductRequest) (*WalletOperation, error) {
	resp, err := c.doRequest("POST", walletPath(req.UserID, "deduct"), req)
	if err != nil {
		return nil, err
	}
	return decodeData[WalletOperation](resp.Data)
}

func (c *Client) walletBalance(userID string) (*WalletBalance, error) {
	resp, err := c.doRequest("GET", walletPath(userID, "balance"), nil)
	if err != nil {
		return nil, err
	}
	return decodeData[WalletBalance](resp.Data)
}

func (c *Client) walletTransactions(userID string) ([]WalletTransaction, error) {
	resp, err := c.doRequest("GET", walletPath(userID, "transactions")+"?pageSize=100", nil)
	if err != nil {
		return nil, err
	}
	return decodeItems[WalletTransaction](resp.Data)
}

// listQuery builds a filtered list query with the max page size, so a single
// call returns as many items as the server allows in one page (100).
func listQuery(key, value string) string {
	q := url.Values{}
	if value != "" {
		q.Set(key, value)
	}
	q.Set("pageSize", "100")
	return q.Encode()
}
