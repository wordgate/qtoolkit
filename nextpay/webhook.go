package nextpay

// Inbound webhooks: NextPay POSTs event notifications to the App's configured
// WebhookURL. This file is the App-side receiving contract — signature
// verification, the event envelope, and typed accessors for each event's data.
//
// It is framework-agnostic on purpose: the App reads the raw request body and
// the X-NextPay-Signature header from whatever HTTP stack it uses (gin, echo,
// net/http, ...) and hands both to ParseWebhook. See the README for the ~8-line
// gin binding.
//
// Delivery is at-least-once: NextPay retries up to 15 times plus a sweeper
// cron, so the SAME event may arrive more than once. Handlers MUST be
// idempotent — dedupe on WebhookEvent.ID (an "evt_<uuid>" string).

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// WebhookEventType identifies an inbound webhook event.
type WebhookEventType string

const (
	// Order events carry WebhookOrderData.
	WebhookOrderPaid    WebhookEventType = "order.paid"
	WebhookOrderExpired WebhookEventType = "order.expired"
	WebhookOrderFailed  WebhookEventType = "order.failed"

	// Refunds to the original payment method (apps with refunds to source only).
	WebhookOrderRefunded     WebhookEventType = "order.refunded"
	WebhookOrderRefundFailed WebhookEventType = "order.refund_failed"
	// Card disputes. NextPay only forwards them.
	WebhookOrderDisputeOpened WebhookEventType = "order.dispute.opened"
	WebhookOrderDisputeClosed WebhookEventType = "order.dispute.closed"

	// Subscription events carry WebhookSubscriptionData.
	WebhookSubscriptionCreated   WebhookEventType = "subscription.created"
	WebhookSubscriptionRenewed   WebhookEventType = "subscription.renewed"
	WebhookSubscriptionCancelled WebhookEventType = "subscription.cancelled"
	WebhookSubscriptionExpired   WebhookEventType = "subscription.expired"
	WebhookSubscriptionPastDue   WebhookEventType = "subscription.past_due"
	WebhookSubscriptionPaused    WebhookEventType = "subscription.paused"
	WebhookSubscriptionResumed   WebhookEventType = "subscription.resumed"
	// An automatic collection attempt failed; see WebhookSubscriptionData.Reason.
	WebhookSubscriptionPaymentFailed WebhookEventType = "subscription.payment_failed"
	// The saved card was replaced; see WebhookSubscriptionData.PaymentMethod.
	WebhookSubscriptionPaymentMethodUpdated WebhookEventType = "subscription.payment_method_updated"

	// Contract events carry WebhookContractData.
	WebhookContractActivated WebhookEventType = "contract.activated"
	WebhookContractCancelled WebhookEventType = "contract.cancelled"

	// Charge events carry WebhookChargeData.
	WebhookChargeSucceeded WebhookEventType = "charge.succeeded"
	WebhookChargeFailed    WebhookEventType = "charge.failed"

	// Wallet events carry WebhookWalletData.
	WebhookWalletDeposited WebhookEventType = "wallet.deposited"
	WebhookWalletDeducted  WebhookEventType = "wallet.deducted"
)

// WebhookEvent is the envelope of every inbound webhook. Data is left as raw
// JSON so the caller decodes it into the matching type after switching on Type
// (use the typed accessors below).
type WebhookEvent struct {
	ID        string           `json:"id"`        // "evt_<uuid>"; dedupe key
	Type      WebhookEventType `json:"type"`      // event type
	Timestamp int64            `json:"timestamp"` // unix seconds (payload build time)
	Data      json.RawMessage  `json:"data"`      // event-specific payload
}

// WebhookOrderData is the data for order.* events.
type WebhookOrderData struct {
	OrderID        string `json:"orderId"`
	UserID         string `json:"userId"` // app-side (external) user id
	Amount         uint64 `json:"amount"` // cents
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	ProductName    string `json:"productName"`
	ObjectID       string `json:"objectId"`
	IsSubscription bool   `json:"isSubscription"`
	Plan           *Plan  `json:"plan,omitempty"` // present for subscription orders
	PaidAt         int64  `json:"paidAt,omitempty"`
	Metadata       string `json:"metadata,omitempty"`
	ExpiredAt      int64  `json:"expiredAt,omitempty"`
	// Renewal orders only: the subscription and the period this order pays for.
	SubscriptionID string `json:"subscriptionId,omitempty"`
	PeriodStart    int64  `json:"periodStart,omitempty"`
	PeriodEnd      int64  `json:"periodEnd,omitempty"`
	// Paid orders only: the wallet part and the part captured through Stripe.
	WalletAmount *uint64 `json:"walletAmount,omitempty"`
	CardAmount   *uint64 `json:"cardAmount,omitempty"`
}

// WebhookSubscriptionData is the data for subscription.* events.
type WebhookSubscriptionData struct {
	SubscriptionID     string `json:"subscriptionId"`
	UserID             string `json:"userId"`
	Plan               *Plan  `json:"plan,omitempty"`
	ObjectID           string `json:"objectId,omitempty"`
	Status             string `json:"status"`
	CurrentPeriodStart int64  `json:"currentPeriodStart"`
	CurrentPeriodEnd   int64  `json:"currentPeriodEnd"`
	AutoRenew          bool   `json:"autoRenew"` // = !cancelAtPeriodEnd
	// Metadata is the opaque string given at subscription creation.
	Metadata string `json:"metadata,omitempty"`
	// GraceEndsAt: when the subscription expires if still unpaid. Present while past_due, and on
	// payment_failed once the period has ended and the app grants a grace period.
	GraceEndsAt int64 `json:"graceEndsAt,omitempty"`

	// Payment of the period [CurrentPeriodStart, CurrentPeriodEnd): subscription.created and
	// subscription.renewed. OrderID is "" on events that carry no payment.
	OrderID      string `json:"orderId,omitempty"`
	Amount       uint64 `json:"amount,omitempty"`
	Currency     string `json:"currency,omitempty"`
	WalletAmount uint64 `json:"walletAmount,omitempty"`
	CardAmount   uint64 `json:"cardAmount,omitempty"`

	// subscription.payment_failed: why, the 1-based count of real attempts for this period, and
	// when the next automatic attempt is due (0 = none).
	Reason      string `json:"reason,omitempty"`
	Attempt     int    `json:"attempt,omitempty"`
	NextRetryAt int64  `json:"nextRetryAt,omitempty"`

	// subscription.payment_method_updated: the new card.
	PaymentMethod *WebhookCard `json:"paymentMethod,omitempty"`
}

// WebhookCard is the saved card in subscription.payment_method_updated.
type WebhookCard struct {
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int64  `json:"expMonth,omitempty"`
	ExpYear  int64  `json:"expYear,omitempty"`
}

// WebhookRefundData is the payload of order.refunded and order.refund_failed.
type WebhookRefundData struct {
	OrderID        string `json:"orderId"`
	UserID         string `json:"userId"`
	ObjectID       string `json:"objectId"`
	SubscriptionID string `json:"subscriptionId,omitempty"`
	Metadata       string `json:"metadata,omitempty"`
	RefundID       string `json:"refundId"`
	Amount         uint64 `json:"amount"`        // this refund
	RefundedTotal  uint64 `json:"refundedTotal"` // all succeeded refunds of the order
	OrderAmount    uint64 `json:"orderAmount"`
	Currency       string `json:"currency"`
	OrderStatus    string `json:"orderStatus"`
	Initiator      string `json:"initiator"` // tenant | system
	Reason         string `json:"reason"`    // requested | late_payment | duplicate_payment | ...
	Note           string `json:"note,omitempty"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"` // the tenant's key; absent on system refunds
	FailureReason  string `json:"failureReason,omitempty"`
}

// WebhookDisputeData is the payload of order.dispute.opened and order.dispute.closed.
type WebhookDisputeData struct {
	OrderID        string `json:"orderId"`
	UserID         string `json:"userId"`
	ObjectID       string `json:"objectId"`
	SubscriptionID string `json:"subscriptionId,omitempty"`
	Metadata       string `json:"metadata,omitempty"`
	DisputeID      string `json:"disputeId"`
	Amount         uint64 `json:"amount"`
	Currency       string `json:"currency"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`            // open | closed
	Outcome        string `json:"outcome,omitempty"` // won | lost, once closed
	EvidenceDueBy  int64  `json:"evidenceDueBy,omitempty"`
}

// WebhookContractData is the data for contract.* events.
type WebhookContractData struct {
	ContractID    string `json:"contractId"`
	UserID        string `json:"userId"`
	DefaultAmount uint64 `json:"defaultAmount"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
}

// WebhookChargeData is the data for charge.* events.
type WebhookChargeData struct {
	ChargeID       string `json:"chargeId"`
	ContractID     string `json:"contractId"`
	UserID         string `json:"userId"`
	Amount         uint64 `json:"amount"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotencyKey"`
	Description    string `json:"description,omitempty"`
}

// WebhookWalletData is the data for wallet.* events. Amount is signed: positive
// on deposit, negative on deduct.
type WebhookWalletData struct {
	UUID           string `json:"uuid"` // wallet uuid
	UserID         string `json:"userId"`
	Amount         int64  `json:"amount"`  // signed delta, cents
	Balance        uint64 `json:"balance"` // balance after, cents
	TransactionID  string `json:"transactionId"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	RelatedOrderID string `json:"relatedOrderId,omitempty"`
}

// ErrInvalidWebhookSignature is returned when the X-NextPay-Signature header is
// missing, malformed, or does not match the payload under the app secret.
var ErrInvalidWebhookSignature = errors.New("nextpay: invalid webhook signature")

// VerifyWebhookSignature checks the X-NextPay-Signature header against rawBody
// using the app's webhook secret. The header format is "t=<unix>,v1=<hex>" and
// the signature is HMAC-SHA256(secret, "<t>.<rawBody>").
//
// rawBody must be the exact bytes received — verify BEFORE any JSON decode or
// re-encode, or the HMAC will not match. Replay protection is the caller's job
// via dedupe on WebhookEvent.ID; there is deliberately no timestamp tolerance.
func VerifyWebhookSignature(rawBody []byte, signatureHeader, secret string) error {
	t, v1, ok := parseSignatureHeader(signatureHeader)
	if !ok {
		return ErrInvalidWebhookSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t))
	mac.Write([]byte("."))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(v1)) {
		return ErrInvalidWebhookSignature
	}
	return nil
}

// ParseWebhook verifies the signature and decodes the envelope in one step. It
// is the single entry point for an inbound webhook handler: on success the
// returned event is authentic and ready to dispatch on Type.
func ParseWebhook(rawBody []byte, signatureHeader, secret string) (*WebhookEvent, error) {
	if err := VerifyWebhookSignature(rawBody, signatureHeader, secret); err != nil {
		return nil, err
	}
	var evt WebhookEvent
	if err := json.Unmarshal(rawBody, &evt); err != nil {
		return nil, fmt.Errorf("nextpay: decode webhook envelope: %w", err)
	}
	return &evt, nil
}

// OrderData decodes the payload of an order.* event.
func (e *WebhookEvent) OrderData() (*WebhookOrderData, error) {
	return decodeWebhookData[WebhookOrderData](e, WebhookOrderPaid, WebhookOrderExpired, WebhookOrderFailed)
}

// SubscriptionData decodes the payload of a subscription.* event.
func (e *WebhookEvent) SubscriptionData() (*WebhookSubscriptionData, error) {
	return decodeWebhookData[WebhookSubscriptionData](e,
		WebhookSubscriptionCreated, WebhookSubscriptionRenewed, WebhookSubscriptionCancelled,
		WebhookSubscriptionExpired, WebhookSubscriptionPastDue,
		WebhookSubscriptionPaused, WebhookSubscriptionResumed,
		WebhookSubscriptionPaymentFailed, WebhookSubscriptionPaymentMethodUpdated)
}

// RefundData decodes the payload of order.refunded / order.refund_failed.
func (e *WebhookEvent) RefundData() (*WebhookRefundData, error) {
	return decodeWebhookData[WebhookRefundData](e, WebhookOrderRefunded, WebhookOrderRefundFailed)
}

// DisputeData decodes the payload of order.dispute.opened / order.dispute.closed.
func (e *WebhookEvent) DisputeData() (*WebhookDisputeData, error) {
	return decodeWebhookData[WebhookDisputeData](e, WebhookOrderDisputeOpened, WebhookOrderDisputeClosed)
}

// ContractData decodes the payload of a contract.* event.
func (e *WebhookEvent) ContractData() (*WebhookContractData, error) {
	return decodeWebhookData[WebhookContractData](e, WebhookContractActivated, WebhookContractCancelled)
}

// ChargeData decodes the payload of a charge.* event.
func (e *WebhookEvent) ChargeData() (*WebhookChargeData, error) {
	return decodeWebhookData[WebhookChargeData](e, WebhookChargeSucceeded, WebhookChargeFailed)
}

// WalletData decodes the payload of a wallet.* event.
func (e *WebhookEvent) WalletData() (*WebhookWalletData, error) {
	return decodeWebhookData[WebhookWalletData](e, WebhookWalletDeposited, WebhookWalletDeducted)
}

// decodeWebhookData decodes e.Data into T after confirming e.Type is one of the
// event types that carry T, so a caller that reaches for the wrong accessor
// gets a clear error instead of a silently zero-valued struct.
func decodeWebhookData[T any](e *WebhookEvent, want ...WebhookEventType) (*T, error) {
	if !slices.Contains(want, e.Type) {
		return nil, fmt.Errorf("nextpay: event type %q does not carry %T", e.Type, *new(T))
	}
	var d T
	if err := json.Unmarshal(e.Data, &d); err != nil {
		return nil, fmt.Errorf("nextpay: decode %q data: %w", e.Type, err)
	}
	return &d, nil
}

// parseSignatureHeader splits "t=<unix>,v1=<hex>" into its parts. Order is not
// assumed; both fields must be present and non-empty. The raw t string is
// returned verbatim so the HMAC is computed over exactly what was signed.
func parseSignatureHeader(header string) (t, v1 string, ok bool) {
	for part := range strings.SplitSeq(header, ",") {
		key, val, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch key {
		case "t":
			t = val
		case "v1":
			v1 = val
		}
	}
	if t == "" || v1 == "" {
		return "", "", false
	}
	return t, v1, true
}
