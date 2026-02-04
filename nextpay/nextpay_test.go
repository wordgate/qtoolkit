package nextpay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// resetState resets global state for test isolation
func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	clientOnce = sync.Once{}
	globalClient = nil
	initErr = nil
}

// testResponse is a helper for mock API responses
type testResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func TestCreateSubscription_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/checkout/subscription" {
			t.Errorf("path = %s, want /api/checkout/subscription", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %s, want Bearer test-key", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", r.Header.Get("Content-Type"))
		}

		// Decode request body
		var req SubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.UserID != "user123" {
			t.Errorf("userId = %s, want user123", req.UserID)
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(testResponse{
			Code: 0,
			Data: map[string]interface{}{
				"orderId":    "order_123",
				"paymentUrl": "https://pay.example.com/checkout/123",
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	result, err := CreateSubscription(&SubscriptionRequest{
		UserID:     "user123",
		PlanID:     "plan_monthly",
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OrderID == "" {
		t.Error("expected orderId in response")
	}
	if result.PaymentURL == "" {
		t.Error("expected paymentUrl in response")
	}
}

func TestCreateOrder_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/checkout/order" {
			t.Errorf("path = %s, want /api/checkout/order", r.URL.Path)
		}

		var req OrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Amount != 999 {
			t.Errorf("amount = %d, want 999", req.Amount)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{
			Code: 0,
			Data: map[string]interface{}{
				"orderId":    "order_456",
				"paymentUrl": "https://pay.example.com/checkout/456",
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	result, err := CreateOrder(&OrderRequest{
		UserID:     "user123",
		Amount:     999, // $9.99
		Currency:   "USD",
		ProductID:  "product_123",
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OrderID == "" {
		t.Error("expected orderId in response")
	}
}

func TestGetSubscriptions_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/subscriptions" {
			t.Errorf("path = %s, want /api/subscriptions", r.URL.Path)
		}
		if r.URL.Query().Get("userId") != "user123" {
			t.Errorf("userId = %s, want user123", r.URL.Query().Get("userId"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{
			Code: 0,
			Data: []map[string]interface{}{
				{
					"id":     "sub_123",
					"status": "active",
					"planId": "plan_monthly",
				},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	subs, err := GetSubscriptions("user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 1 {
		t.Errorf("len(subs) = %d, want 1", len(subs))
	}
	if subs[0].Status != "active" {
		t.Errorf("status = %s, want active", subs[0].Status)
	}
}

func TestGetOrders_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/orders" {
			t.Errorf("path = %s, want /api/orders", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{
			Code: 0,
			Data: []map[string]interface{}{
				{
					"id":     "order_123",
					"status": "paid",
					"amount": 999,
				},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	orders, err := GetOrders("user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(orders) != 1 {
		t.Errorf("len(orders) = %d, want 1", len(orders))
	}
	if orders[0].Status != "paid" {
		t.Errorf("status = %s, want paid", orders[0].Status)
	}
}

func TestCreatePendingCharge_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/billing/pending-charges" {
			t.Errorf("path = %s, want /api/billing/pending-charges", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{
			Code: 0,
			Data: map[string]interface{}{
				"id":     "charge_123",
				"status": "pending",
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	charge, err := CreatePendingCharge(&PendingChargeRequest{
		SubscriptionID: "sub_123",
		Amount:         500,
		Description:    "API usage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if charge.ID == "" {
		t.Error("expected charge ID")
	}
}

func TestCreateRechargeContract_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/recharge-contracts" {
			t.Errorf("path = %s, want /api/recharge-contracts", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{
			Code: 0,
			Data: map[string]interface{}{
				"contractId":       "contract_123",
				"authorizationUrl": "https://pay.example.com/authorize/123",
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	result, err := CreateRechargeContract(&RechargeContractRequest{
		UserID:     "user123",
		MaxAmount:  10000,
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContractID == "" {
		t.Error("expected contractId")
	}
	if result.AuthorizationURL == "" {
		t.Error("expected authorizationUrl")
	}
}

func TestChargeContract_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/recharge-contracts/contract_123/charge" {
			t.Errorf("path = %s, want /api/recharge-contracts/contract_123/charge", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{
			Code: 0,
			Data: map[string]interface{}{
				"chargeId": "charge_789",
				"status":   "succeeded",
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	result, err := ChargeContract("contract_123", &ContractChargeRequest{
		Amount:         500,
		IdempotencyKey: "unique_key_123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ChargeID == "" {
		t.Error("expected chargeId")
	}
}

func TestNotConfigured(t *testing.T) {
	resetState()

	_, err := GetSubscriptions("user123")
	if err == nil {
		t.Error("expected error when not configured")
	}
}

func TestAPIError(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(testResponse{
			Code:    400801,
			Message: "Missing required fields",
		})
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	_, err := CreateSubscription(&SubscriptionRequest{
		UserID: "user123",
	})
	if err == nil {
		t.Error("expected error on API error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != 400801 {
		t.Errorf("code = %d, want 400801", apiErr.Code)
	}
}

func TestServerError(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	SetConfig(&Config{
		AccessKey: "test-key",
		Endpoint:  server.URL,
	})

	_, err := GetSubscriptions("user123")
	if err == nil {
		t.Error("expected error on server error")
	}
}
