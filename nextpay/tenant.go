package nextpay

import (
	"context"
	"fmt"
	"net/url"
)

// This file holds what a second tenant in the same process needs (an explicit client instead of
// the process-wide one) and the per-app capabilities added in NextPay's events 2.0.0: refunds to
// the original payment method and the hosted session that replaces a subscription's saved card.

type clientCtxKey struct{}

// NewClient builds a client from an explicit configuration, independent of the process-wide
// client configured through viper. Use it when one process talks to NextPay as more than one
// app: every package-level function uses this client when the context carries it (WithClient).
func NewClient(cfg Config) (*Client, error) {
	if cfg.AccessKey == "" {
		return nil, fmt.Errorf("%w: access key is required", ErrNotConfigured)
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://pay.arbella.group"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30
	}
	return createClient(&cfg)
}

// WithClient returns a context under which every package-level function (CreateSubscription,
// RefundOrder, ...) talks to NextPay as client's app instead of the process-wide one.
func WithClient(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, clientCtxKey{}, client)
}

// RefundRequest asks for a refund to the payment method an order was paid with.
type RefundRequest struct {
	// Amount in the smallest currency unit, always explicit (a full refund is the remaining
	// refundable amount).
	Amount uint64 `json:"amount"`
	// IdempotencyKey is unique per app. The same key with the same order and amount returns the
	// original refund; with a different order or amount it is an error (409).
	IdempotencyKey string `json:"idempotencyKey"`
	Note           string `json:"note,omitempty"`
	// CancelSubscription also terminates the order's subscription immediately.
	CancelSubscription bool `json:"cancelSubscription,omitempty"`
}

// RefundResult reports that a refund was accepted. Its outcome arrives as the order.refunded or
// order.refund_failed webhook.
type RefundResult struct {
	RefundID         string `json:"refundId"`
	OrderID          string `json:"orderId"`
	Amount           uint64 `json:"amount"`
	RefundedTotal    uint64 `json:"refundedTotal"`    // succeeded refunds only
	RefundableAmount uint64 `json:"refundableAmount"` // after this request
	Currency         string `json:"currency"`
	Status           string `json:"status"` // pending | succeeded | failed
}

// RefundOrder refunds part or all of a paid order to its original payment method. Only for apps
// with refunds to source enabled; every other app gets an API error (400).
func RefundOrder(ctx context.Context, orderUUID string, req *RefundRequest) (*RefundResult, error) {
	if orderUUID == "" || req == nil || req.Amount == 0 || req.IdempotencyKey == "" {
		return nil, fmt.Errorf("%w: order id, amount and idempotency key are required", ErrInvalidInput)
	}
	return do(ctx, func(ctx context.Context, c *Client) (*RefundResult, error) {
		resp, err := c.doRequest(ctx, "POST", "/api/orders/"+url.PathEscape(orderUUID)+"/refund", req)
		if err != nil {
			return nil, err
		}
		return decodeData[RefundResult](resp.Data)
	})
}

// PaymentMethodSessionRequest starts a hosted session in which the user saves a new card.
type PaymentMethodSessionRequest struct {
	ReturnURL string `json:"returnUrl"`
	CancelURL string `json:"cancelUrl,omitempty"` // defaults to ReturnURL
	Locale    string `json:"locale,omitempty"`    // defaults to the subscription's locale
}

// PaymentMethodSessionResult is the page to redirect the user to. Single use.
type PaymentMethodSessionResult struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expiresAt"` // unix seconds
}

// CreatePaymentMethodSession starts a hosted session that replaces the card a subscription
// renews with. Nothing is charged. The card changes when NextPay receives the processor's
// confirmation; subscription.payment_method_updated announces it.
func CreatePaymentMethodSession(ctx context.Context, subscriptionUUID string, req *PaymentMethodSessionRequest) (*PaymentMethodSessionResult, error) {
	if subscriptionUUID == "" || req == nil || req.ReturnURL == "" {
		return nil, fmt.Errorf("%w: subscription id and return URL are required", ErrInvalidInput)
	}
	return do(ctx, func(ctx context.Context, c *Client) (*PaymentMethodSessionResult, error) {
		resp, err := c.doRequest(ctx, "POST",
			"/api/subscriptions/"+url.PathEscape(subscriptionUUID)+"/payment-method/session", req)
		if err != nil {
			return nil, err
		}
		return decodeData[PaymentMethodSessionResult](resp.Data)
	})
}
