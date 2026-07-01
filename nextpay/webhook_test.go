package nextpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
)

// signBody reproduces the server's signing scheme so the tests exercise the
// real header format the App will receive:
//
//	signature = HMAC-SHA256(secret, "<t>.<body>"), header = "t=<t>,v1=<sig>".
func signBody(t int64, body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", t, string(body))
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"order.paid","timestamp":100,"data":{}}`)
	secret := "whsec_test"
	header := signBody(1700000000, body, secret)

	if err := VerifyWebhookSignature(body, header, secret); err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
}

func TestVerifyWebhookSignature_TamperedBody(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"order.paid","timestamp":100,"data":{}}`)
	secret := "whsec_test"
	header := signBody(1700000000, body, secret)

	tampered := []byte(`{"id":"evt_1","type":"order.paid","timestamp":100,"data":{"x":1}}`)
	if err := VerifyWebhookSignature(tampered, header, secret); !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Fatalf("expected ErrInvalidWebhookSignature, got %v", err)
	}
}

func TestVerifyWebhookSignature_WrongSecret(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	header := signBody(1700000000, body, "whsec_real")

	if err := VerifyWebhookSignature(body, header, "whsec_wrong"); !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Fatalf("expected ErrInvalidWebhookSignature, got %v", err)
	}
}

func TestVerifyWebhookSignature_MalformedHeader(t *testing.T) {
	body := []byte(`{}`)
	for _, h := range []string{"", "garbage", "t=123", "v1=abc", "t=,v1=", "t=123,v2=abc"} {
		if err := VerifyWebhookSignature(body, h, "s"); !errors.Is(err, ErrInvalidWebhookSignature) {
			t.Errorf("header %q: expected ErrInvalidWebhookSignature, got %v", h, err)
		}
	}
}

func TestVerifyWebhookSignature_HeaderOrderIndependent(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	secret := "whsec_test"
	// server emits t first; prove verification does not depend on ordering.
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", int64(42), string(body))
	sig := hex.EncodeToString(mac.Sum(nil))
	header := fmt.Sprintf("v1=%s,t=%d", sig, 42)

	if err := VerifyWebhookSignature(body, header, secret); err != nil {
		t.Fatalf("expected valid signature regardless of field order, got %v", err)
	}
}

func TestParseWebhook_OrderPaid(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{
		"id": "evt_abc",
		"type": "order.paid",
		"timestamp": 1700000000,
		"data": {
			"orderId": "ord_1",
			"userId": "user123",
			"amount": 1999,
			"currency": "usd",
			"status": "paid",
			"productName": "Pro",
			"objectId": "obj_1",
			"isSubscription": true,
			"plan": {"code": "pro", "amount": 1999, "intervalType": "month"},
			"paidAt": 1700000001
		}
	}`)
	header := signBody(1700000000, body, secret)

	evt, err := ParseWebhook(body, header, secret)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if evt.ID != "evt_abc" || evt.Type != WebhookOrderPaid {
		t.Fatalf("unexpected envelope: %+v", evt)
	}

	d, err := evt.OrderData()
	if err != nil {
		t.Fatalf("OrderData: %v", err)
	}
	if d.OrderID != "ord_1" || d.UserID != "user123" || d.Amount != 1999 || !d.IsSubscription {
		t.Errorf("unexpected order data: %+v", d)
	}
	if d.Plan == nil || d.Plan.Code != "pro" || d.Plan.IntervalType != "month" {
		t.Errorf("unexpected plan: %+v", d.Plan)
	}
}

func TestParseWebhook_InvalidSignatureRejected(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"order.paid","data":{}}`)
	if _, err := ParseWebhook(body, "t=1,v1=deadbeef", "whsec_test"); !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Fatalf("expected ErrInvalidWebhookSignature, got %v", err)
	}
}

func TestWebhookEvent_SubscriptionData(t *testing.T) {
	evt := &WebhookEvent{
		Type: WebhookSubscriptionRenewed,
		Data: []byte(`{"subscriptionId":"sub_1","userId":"user123","status":"active","currentPeriodStart":100,"currentPeriodEnd":200,"autoRenew":true,"plan":{"code":"pro"}}`),
	}
	d, err := evt.SubscriptionData()
	if err != nil {
		t.Fatalf("SubscriptionData: %v", err)
	}
	if d.SubscriptionID != "sub_1" || d.Status != "active" || !d.AutoRenew || d.CurrentPeriodEnd != 200 {
		t.Errorf("unexpected subscription data: %+v", d)
	}
	if d.Plan == nil || d.Plan.Code != "pro" {
		t.Errorf("unexpected plan: %+v", d.Plan)
	}
}

func TestWebhookEvent_ChargeData(t *testing.T) {
	evt := &WebhookEvent{
		Type: WebhookChargeSucceeded,
		Data: []byte(`{"chargeId":"chg_1","contractId":"rc_1","userId":"user123","amount":500,"currency":"usd","status":"succeeded","idempotencyKey":"idem_1"}`),
	}
	d, err := evt.ChargeData()
	if err != nil {
		t.Fatalf("ChargeData: %v", err)
	}
	if d.ChargeID != "chg_1" || d.ContractID != "rc_1" || d.Amount != 500 || d.IdempotencyKey != "idem_1" {
		t.Errorf("unexpected charge data: %+v", d)
	}
}

func TestWebhookEvent_WalletData_SignedAmount(t *testing.T) {
	evt := &WebhookEvent{
		Type: WebhookWalletDeducted,
		Data: []byte(`{"uuid":"wal_1","userId":"user123","amount":-300,"balance":700,"transactionId":"txn_1"}`),
	}
	d, err := evt.WalletData()
	if err != nil {
		t.Fatalf("WalletData: %v", err)
	}
	if d.Amount != -300 || d.Balance != 700 || d.TransactionID != "txn_1" {
		t.Errorf("unexpected wallet data: %+v", d)
	}
}

func TestWebhookEvent_ContractData(t *testing.T) {
	evt := &WebhookEvent{
		Type: WebhookContractActivated,
		Data: []byte(`{"contractId":"rc_1","userId":"user123","defaultAmount":1000,"currency":"usd","status":"active"}`),
	}
	d, err := evt.ContractData()
	if err != nil {
		t.Fatalf("ContractData: %v", err)
	}
	if d.ContractID != "rc_1" || d.DefaultAmount != 1000 || d.Status != "active" {
		t.Errorf("unexpected contract data: %+v", d)
	}
}

func TestWebhookEvent_WrongAccessorRejected(t *testing.T) {
	evt := &WebhookEvent{Type: WebhookOrderPaid, Data: []byte(`{}`)}
	if _, err := evt.SubscriptionData(); err == nil {
		t.Fatal("expected error when decoding order event as subscription data")
	}
}
