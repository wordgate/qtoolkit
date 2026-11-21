package nextpay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A client carried by the context is used instead of the process-wide one, with its own key.
func TestWithClientUsesItsOwnApp(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("X-Access-Key"), r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"code":0,"data":{"refundId":"r1","orderId":"o1","amount":500,"refundableAmount":1500,"currency":"usd","status":"pending"}}`))
	}))
	defer srv.Close()

	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("a client without an access key must be refused")
	}
	client, err := NewClient(Config{AccessKey: "second-app-key", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithClient(context.Background(), client)

	got, err := RefundOrder(ctx, "o1", &RefundRequest{Amount: 500, IdempotencyKey: "k1", CancelSubscription: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.RefundID != "r1" || got.Status != "pending" || got.RefundableAmount != 1500 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if gotPath != "/api/orders/o1/refund" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "second-app-key" {
		t.Fatalf("request was not sent as the context's app: %q", gotAuth)
	}
	if gotBody["idempotencyKey"] != "k1" || gotBody["cancelSubscription"] != true {
		t.Fatalf("body = %v", gotBody)
	}

	if _, err := RefundOrder(ctx, "o1", &RefundRequest{Amount: 0, IdempotencyKey: "k"}); err == nil {
		t.Fatal("amount 0 must be refused before any request")
	}
	if _, err := CreatePaymentMethodSession(ctx, "s1", &PaymentMethodSessionRequest{}); err == nil {
		t.Fatal("a missing return URL must be refused before any request")
	}
}

func TestNewEventPayloads(t *testing.T) {
	failed := WebhookEvent{Type: WebhookSubscriptionPaymentFailed, Data: json.RawMessage(
		`{"subscriptionId":"s1","userId":"u1","status":"past_due","currentPeriodStart":1,"currentPeriodEnd":2,"autoRenew":true,` +
			`"metadata":"m","graceEndsAt":9,"reason":"card_declined","attempt":2,"nextRetryAt":5}`)}
	sub, err := failed.SubscriptionData()
	if err != nil {
		t.Fatal(err)
	}
	if sub.Reason != "card_declined" || sub.Attempt != 2 || sub.NextRetryAt != 5 || sub.GraceEndsAt != 9 || sub.Metadata != "m" {
		t.Fatalf("unexpected: %+v", sub)
	}

	renewed := WebhookEvent{Type: WebhookSubscriptionRenewed, Data: json.RawMessage(
		`{"subscriptionId":"s1","userId":"u1","status":"active","currentPeriodStart":1,"currentPeriodEnd":2,"autoRenew":true,` +
			`"orderId":"o9","amount":1000,"currency":"usd","walletAmount":0,"cardAmount":1000}`)}
	sub, err = renewed.SubscriptionData()
	if err != nil {
		t.Fatal(err)
	}
	if sub.OrderID != "o9" || sub.Amount != 1000 || sub.CardAmount != 1000 {
		t.Fatalf("unexpected: %+v", sub)
	}

	refunded := WebhookEvent{Type: WebhookOrderRefunded, Data: json.RawMessage(
		`{"orderId":"o1","userId":"u1","objectId":"x","refundId":"r1","amount":500,"refundedTotal":500,"orderAmount":2000,` +
			`"currency":"usd","orderStatus":"paid","initiator":"tenant","reason":"requested","idempotencyKey":"k1"}`)}
	r, err := refunded.RefundData()
	if err != nil {
		t.Fatal(err)
	}
	if r.RefundID != "r1" || r.RefundedTotal != 500 || r.IdempotencyKey != "k1" {
		t.Fatalf("unexpected: %+v", r)
	}
	if _, err := refunded.OrderData(); err == nil {
		t.Fatal("a refund event is not an order payload")
	}

	closed := WebhookEvent{Type: WebhookOrderDisputeClosed, Data: json.RawMessage(
		`{"orderId":"o1","userId":"u1","objectId":"x","disputeId":"d1","amount":2000,"currency":"usd","reason":"fraudulent","status":"closed","outcome":"lost"}`)}
	d, err := closed.DisputeData()
	if err != nil {
		t.Fatal(err)
	}
	if d.Outcome != "lost" || d.Status != "closed" {
		t.Fatalf("unexpected: %+v", d)
	}
}
