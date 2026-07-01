package nextpay

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// resetState resets global state for test isolation.
func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	clientOnce = sync.Once{}
	globalClient = nil
	initErr = nil
}

// testResponse is the success/error envelope helper for mock responses.
type testResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// items wraps a list payload the way the server's List/ItemsAll envelopes do.
func items(list ...any) map[string]any {
	return map[string]any{"items": list}
}

// mock spins up a test server that runs assert on the request and writes resp,
// wires SetConfig to point at it, and returns a cleanup func.
func mock(t *testing.T, assert func(*testing.T, *http.Request), resp testResponse) func() {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Access-Key") != "test-key" {
			t.Errorf("X-Access-Key = %q, want test-key", r.Header.Get("X-Access-Key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization must be empty; access-key auth uses X-Access-Key")
		}
		if assert != nil {
			assert(t, r)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	SetConfig(&Config{AccessKey: "test-key", Endpoint: server.URL})
	return server.Close
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode request body: %v", err)
	}
	return raw
}

// --- Checkout ---

func TestCreateOrder_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/checkout/order" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["userId"] != "user123" || body["email"] != "u@example.com" {
			t.Errorf("unexpected body: %v", body)
		}
		if body["amount"].(float64) != 999 {
			t.Errorf("amount = %v, want 999", body["amount"])
		}
	}, testResponse{Data: map[string]any{
		"orderId": "ord_1", "paymentUrl": "https://pay/ord_1", "amount": 999, "currency": "usd", "productName": "Premium",
	}})()

	res, err := CreateOrder(&OrderRequest{UserID: "user123", Email: "u@example.com", ProductName: "Premium", Amount: 999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrderID != "ord_1" || res.PaymentURL == "" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestCreateSubscription_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/checkout/subscription" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["code"] != "pro-monthly" {
			t.Errorf("code = %v, want pro-monthly", body["code"])
		}
		if _, bad := body["planId"]; bad {
			t.Error("found planId; subscription create must send code")
		}
	}, testResponse{Data: map[string]any{
		"orderId": "ord_2", "paymentUrl": "https://pay/ord_2", "amount": 4900,
		"plan": map[string]any{"code": "pro-monthly", "name": "Pro"},
	}})()

	res, err := CreateSubscription(&SubscriptionRequest{UserID: "user123", Email: "u@example.com", Code: "pro-monthly"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrderID != "ord_2" || res.Plan == nil || res.Plan.Code != "pro-monthly" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestGrantSubscription_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/checkout/subscription/grant" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["userId"] != "u_123" || body["code"] != "pro-monthly" || body["idempotencyKey"] != "grant-key" {
			t.Errorf("unexpected body: %v", body)
		}
		if _, bad := body["user_id"]; bad {
			t.Error("found snake_case user_id; grant must use camelCase")
		}
		if _, bad := body["planId"]; bad {
			t.Error("found planId; grant must send code")
		}
	}, testResponse{Data: map[string]any{
		"subscription": map[string]any{
			"uuid":               "sub_uuid_1",
			"plan":               map[string]any{"code": "pro-monthly", "name": "Pro"},
			"status":             "active",
			"currentPeriodStart": "2026-07-01T00:00:00Z",
			"currentPeriodEnd":   "2026-08-01T00:00:00Z",
		},
		"orderUuid": "order_uuid_1",
	}})()

	res, err := GrantSubscription(&GrantSubscriptionRequest{UserID: "u_123", Code: "pro-monthly", IdempotencyKey: "grant-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Subscription.UUID != "sub_uuid_1" || res.Subscription.Status != "active" {
		t.Errorf("unexpected subscription: %+v", res.Subscription)
	}
	if res.Subscription.Plan == nil || res.Subscription.Plan.Code != "pro-monthly" {
		t.Errorf("expected nested plan with code, got %+v", res.Subscription.Plan)
	}
	if res.Subscription.CurrentPeriodEnd != "2026-08-01T00:00:00Z" || res.OrderUUID != "order_uuid_1" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestGrantSubscription_APIErrors(t *testing.T) {
	cases := []struct {
		name string
		code int
	}{
		{"already active", 400403},
		{"idempotency conflict", 400404},
		{"plan not found", 400301},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetState()
			defer mock(t, nil, testResponse{Code: tc.code, Message: tc.name})()

			_, err := GrantSubscription(&GrantSubscriptionRequest{UserID: "u_123", Code: "pro-monthly", IdempotencyKey: "k"})
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *APIError, got %T (%v)", err, err)
			}
			if apiErr.Code != tc.code {
				t.Errorf("code = %d, want %d", apiErr.Code, tc.code)
			}
		})
	}
}

// --- Orders ---

func TestGetOrders_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/users/user123/orders" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: items(
		map[string]any{"uuid": "ord_1", "status": "paid", "amount": 999},
	)})()

	orders, err := GetOrders("user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(orders) != 1 || orders[0].UUID != "ord_1" || orders[0].Status != "paid" {
		t.Errorf("unexpected orders: %+v", orders)
	}
}

func TestGetOrder_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/orders/ord_1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: map[string]any{
		"uuid": "ord_1", "status": "paid", "amount": 999,
		"user": map[string]any{"userId": "user123", "email": "u@example.com"},
	}})()

	order, err := GetOrder("ord_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.UUID != "ord_1" {
		t.Errorf("uuid = %s", order.UUID)
	}
	// App-facing responses expose only the external UserID, never the server uuid.
	if order.User == nil || order.User.UserID != "user123" {
		t.Errorf("unexpected user view: %+v", order.User)
	}
}

// --- Plans ---

func TestListPlans_ActiveOnly(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/plans" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("active-only list must send no query, got %q", r.URL.RawQuery)
		}
	}, testResponse{Data: items(
		map[string]any{"code": "pro-monthly", "name": "Pro", "amount": 999, "intervalType": "month", "isActive": true},
	)})()

	plans, err := ListPlans(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 1 || plans[0].Code != "pro-monthly" || !plans[0].IsActive {
		t.Errorf("unexpected plans: %+v", plans)
	}
}

func TestListPlans_IncludeInactive(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.URL.Query().Get("activeOnly") != "false" {
			t.Errorf("activeOnly = %q, want false", r.URL.Query().Get("activeOnly"))
		}
	}, testResponse{Data: items()})()

	if _, err := ListPlans(true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetPlan_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/plans/pro-monthly" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: map[string]any{"code": "pro-monthly", "name": "Pro", "amount": 999, "intervalType": "month"}})()

	plan, err := GetPlan("pro-monthly")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Code != "pro-monthly" || plan.Amount != 999 {
		t.Errorf("unexpected plan: %+v", plan)
	}
}

func TestCreatePlan_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/plans" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["code"] != "pro-monthly" || body["intervalType"] != "month" {
			t.Errorf("unexpected body: %v", body)
		}
		if body["amount"].(float64) != 999 {
			t.Errorf("amount = %v", body["amount"])
		}
	}, testResponse{Data: map[string]any{"code": "pro-monthly", "name": "Pro", "amount": 999, "intervalType": "month", "isActive": true}})()

	plan, err := CreatePlan(&CreatePlanRequest{Code: "pro-monthly", Name: "Pro", Amount: 999, IntervalType: "month"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Code != "pro-monthly" {
		t.Errorf("unexpected plan: %+v", plan)
	}
}

func TestUpdatePlan_Success(t *testing.T) {
	resetState()
	newAmount := uint64(1299)
	inactive := false
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/api/plans/pro-monthly" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["amount"].(float64) != 1299 {
			t.Errorf("amount = %v, want 1299", body["amount"])
		}
		if body["isActive"] != false {
			t.Errorf("isActive = %v, want false", body["isActive"])
		}
		// nil pointers must be omitted
		if _, sent := body["name"]; sent {
			t.Error("nil Name should be omitted from update body")
		}
	}, testResponse{Data: map[string]any{}})()

	if err := UpdatePlan("pro-monthly", &UpdatePlanRequest{Amount: &newAmount, IsActive: &inactive}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeletePlan_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/plans/pro-monthly" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: map[string]any{}})()

	if err := DeletePlan("pro-monthly"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetPlan_NotFound(t *testing.T) {
	resetState()
	defer mock(t, nil, testResponse{Code: 404, Message: "plan not found"})()

	_, err := GetPlan("nope")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 404 {
		t.Fatalf("expected *APIError 404, got %v", err)
	}
}

// --- Subscriptions ---

func TestGetSubscriptions_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/users/user123/subscriptions" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: items(
		map[string]any{
			"uuid": "sub_1", "status": "active", "autoRenew": true,
			"currentPeriodStart": 1000, "currentPeriodEnd": 2000,
			"plan": map[string]any{"code": "pro-monthly"},
		},
	)})()

	subs, err := GetSubscriptions("user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("len = %d, want 1", len(subs))
	}
	s := subs[0]
	if s.UUID != "sub_1" || s.Status != "active" || !s.AutoRenew {
		t.Errorf("unexpected subscription: %+v", s)
	}
	if s.CurrentPeriodEnd != 2000 {
		t.Errorf("currentPeriodEnd = %d, want 2000", s.CurrentPeriodEnd)
	}
	if s.Plan == nil || s.Plan.Code != "pro-monthly" {
		t.Errorf("expected nested plan code, got %+v", s.Plan)
	}
}

func TestGetSubscription_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/subscriptions/sub_1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: map[string]any{"uuid": "sub_1", "status": "active"}})()

	sub, err := GetSubscription("sub_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.UUID != "sub_1" {
		t.Errorf("uuid = %s", sub.UUID)
	}
}

func TestSetAutoRenew_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/subscriptions/sub_1/auto-renew" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["enabled"] != false {
			t.Errorf("enabled = %v, want false", body["enabled"])
		}
	}, testResponse{Data: map[string]any{}})()

	if err := SetAutoRenew("sub_1", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetAutoRenew_APIError(t *testing.T) {
	resetState()
	defer mock(t, nil, testResponse{Code: 404, Message: "subscription not found"})()

	err := SetAutoRenew("sub_404", false)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 404 {
		t.Fatalf("expected *APIError 404, got %v", err)
	}
}

func TestCancelSubscription_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/subscriptions/sub_1/cancel" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		// Immediate-only; "cancel at period end" is SetAutoRenew(id,false)'s job.
		if body["cancelAtPeriodEnd"] != false {
			t.Errorf("cancelAtPeriodEnd = %v, want false", body["cancelAtPeriodEnd"])
		}
	}, testResponse{Data: map[string]any{}})()

	if err := CancelSubscription("sub_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPauseSubscription_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/subscriptions/sub_1/pause" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if r.ContentLength > 0 {
			t.Error("pause must send no body")
		}
	}, testResponse{Data: map[string]any{"uuid": "sub_1", "status": "active", "pauseAtPeriodEnd": true, "autoRenew": true}})()

	sub, err := PauseSubscription("sub_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.UUID != "sub_1" || !sub.PauseAtPeriodEnd {
		t.Errorf("unexpected subscription: %+v", sub)
	}
}

func TestResumeSubscription_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/subscriptions/sub_1/resume" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["payWithWallet"] != true {
			t.Errorf("payWithWallet = %v, want true", body["payWithWallet"])
		}
	}, testResponse{Data: map[string]any{"uuid": "sub_1", "status": "active", "autoRenew": false}})()

	sub, err := ResumeSubscription("sub_1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.UUID != "sub_1" || sub.Status != "active" {
		t.Errorf("unexpected subscription: %+v", sub)
	}
}

func TestRenewSubscription_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/subscriptions/sub_1/renew" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: map[string]any{"orderId": "ord_r", "paymentUrl": "https://pay/ord_r"}})()

	res, err := RenewSubscription("sub_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrderID != "ord_r" || res.PaymentURL == "" {
		t.Errorf("unexpected result: %+v", res)
	}
}

// --- Billing ---

func TestCreatePendingCharge_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/billing/pending-charges" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["subscriptionId"] != "sub_1" || body["description"] != "API usage" {
			t.Errorf("unexpected body: %v", body)
		}
	}, testResponse{Data: map[string]any{"chargeId": "chg_1", "subscriptionId": "sub_1", "amount": 500, "status": "pending"}})()

	charge, err := CreatePendingCharge(&PendingChargeRequest{SubscriptionID: "sub_1", Amount: 500, Description: "API usage"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if charge.ChargeID != "chg_1" || charge.Status != "pending" {
		t.Errorf("unexpected charge: %+v", charge)
	}
}

func TestGetPendingCharges_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/billing/pending-charges" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("subscriptionId") != "sub_1" {
			t.Errorf("subscriptionId = %q", r.URL.Query().Get("subscriptionId"))
		}
	}, testResponse{Data: items(
		map[string]any{"uuid": "pc_1", "subscriptionUuid": "sub_1", "amount": 500, "status": "pending"},
	)})()

	charges, err := GetPendingCharges("sub_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(charges) != 1 || charges[0].UUID != "pc_1" {
		t.Errorf("unexpected charges: %+v", charges)
	}
}

func TestGetPendingCharge_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.URL.Path != "/api/billing/pending-charges/pc_1" {
			t.Errorf("path = %s", r.URL.Path)
		}
	}, testResponse{Data: map[string]any{"uuid": "pc_1", "status": "billed"}})()

	charge, err := GetPendingCharge("pc_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if charge.UUID != "pc_1" {
		t.Errorf("uuid = %s", charge.UUID)
	}
}

// --- Recharge contracts ---

func TestCreateRechargeContract_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/recharge-contracts" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["defaultAmount"].(float64) != 10000 || body["orderUuid"] != "ord_1" {
			t.Errorf("unexpected body: %v", body)
		}
	}, testResponse{Data: map[string]any{
		"contractId": "rc_1", "status": "pending_setup", "paymentUrl": "https://pay/rc_1", "authorizationUrl": "https://auth/rc_1",
	}})()

	res, err := CreateRechargeContract(&RechargeContractRequest{
		UserID: "user123", Email: "u@example.com", DefaultAmount: 10000, OrderUUID: "ord_1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ContractID != "rc_1" || res.PaymentURL == "" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestGetRechargeContract_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.URL.Path != "/api/recharge-contracts/rc_1" {
			t.Errorf("path = %s", r.URL.Path)
		}
	}, testResponse{Data: map[string]any{"uuid": "rc_1", "status": "active", "defaultAmount": 10000}})()

	contract, err := GetRechargeContract("rc_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contract.UUID != "rc_1" || contract.Status != "active" {
		t.Errorf("unexpected contract: %+v", contract)
	}
}

func TestChargeContract_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/recharge-contracts/rc_1/charge" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["idempotencyKey"] != "key_1" {
			t.Errorf("idempotencyKey = %v", body["idempotencyKey"])
		}
	}, testResponse{Data: map[string]any{"chargeId": "chg_9", "status": "succeeded", "amount": 500}})()

	res, err := ChargeContract("rc_1", &ChargeRequest{Amount: 500, IdempotencyKey: "key_1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ChargeID != "chg_9" || res.Status != "succeeded" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestCancelRechargeContract_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/recharge-contracts/rc_1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: map[string]any{}})()

	if err := CancelRechargeContract("rc_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Wallets ---

func TestWalletDeposit_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/users/user123/wallet/deposit" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["idempotencyKey"] != "dep_1" {
			t.Errorf("unexpected body: %v", body)
		}
	}, testResponse{Data: map[string]any{"transactionId": "tx_1", "amount": 1000, "balance": 1500}})()

	res, err := WalletDeposit(&WalletDepositRequest{
		UserID: "user123", Email: "u@example.com", Amount: 1000, IdempotencyKey: "dep_1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TransactionID != "tx_1" || res.Balance != 1500 {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestWalletDeduct_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/users/user123/wallet/deduct" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body := decodeBody(t, r)
		if body["amount"].(float64) != 500 {
			t.Errorf("unexpected body: %v", body)
		}
	}, testResponse{Data: map[string]any{"transactionId": "tx_2", "amount": 500, "balance": 1000}})()

	res, err := WalletDeduct(&WalletDeductRequest{UserID: "user123", Amount: 500, IdempotencyKey: "ded_1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Balance != 1000 {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestWalletDeduct_InsufficientBalance(t *testing.T) {
	resetState()
	defer mock(t, nil, testResponse{Code: 400901, Message: "insufficient balance"})()

	_, err := WalletDeduct(&WalletDeductRequest{UserID: "user123", Amount: 999999, IdempotencyKey: "ded_x"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 400901 {
		t.Fatalf("expected *APIError 400901, got %v", err)
	}
}

func TestGetWalletBalance_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/users/user123/wallet/balance" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: map[string]any{"userId": "user123", "balance": 1500, "currency": "usd"}})()

	bal, err := GetWalletBalance("user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bal.UserID != "user123" || bal.Balance != 1500 {
		t.Errorf("unexpected balance: %+v", bal)
	}
}

func TestGetWalletTransactions_Success(t *testing.T) {
	resetState()
	defer mock(t, func(t *testing.T, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/users/user123/wallet/transactions" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
	}, testResponse{Data: items(
		map[string]any{"uuid": "tx_1", "type": "deposit", "amount": 1000, "balanceAfter": 1500},
		map[string]any{"uuid": "tx_2", "type": "deduct", "amount": -500, "balanceAfter": 1000},
	)})()

	txs, err := GetWalletTransactions("user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(txs) != 2 || txs[1].Amount != -500 {
		t.Errorf("unexpected transactions: %+v", txs)
	}
}

// --- Transport ---

func TestNotConfigured(t *testing.T) {
	resetState()
	if _, err := GetSubscriptions("user123"); err == nil {
		t.Error("expected error when not configured")
	}
}

func TestServerError(t *testing.T) {
	resetState()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	SetConfig(&Config{AccessKey: "test-key", Endpoint: server.URL})

	if _, err := GetSubscriptions("user123"); err == nil {
		t.Error("expected error on server error")
	}
}
