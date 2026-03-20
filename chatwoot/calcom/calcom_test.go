package calcom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func signBody(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestParseWebhook_ValidSignature(t *testing.T) {
	resetState()

	secret := "test-webhook-secret"
	SetConfig(&Config{WebhookSecret: secret})

	body := `{
		"triggerEvent": "BOOKING_CREATED",
		"payload": {
			"bookingId": 123,
			"title": "Video Support",
			"startTime": "2026-03-20T14:00:00.000Z",
			"endTime": "2026-03-20T14:30:00.000Z",
			"organizer": {"email": "agent@example.com", "name": "Agent"},
			"attendees": [{"email": "customer@example.com", "name": "Customer"}],
			"metadata": {"conversation_id": "42", "inbox_id": "5"}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Cal-Signature-256", signBody(secret, body))

	event, err := ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook error: %v", err)
	}
	if event.TriggerEvent != "BOOKING_CREATED" {
		t.Errorf("TriggerEvent = %q, want BOOKING_CREATED", event.TriggerEvent)
	}
	if event.Booking.ID != 123 {
		t.Errorf("Booking.ID = %d, want 123", event.Booking.ID)
	}
	if len(event.Booking.Attendees) != 1 {
		t.Fatalf("Attendees count = %d, want 1", len(event.Booking.Attendees))
	}
	if event.Booking.Attendees[0].Email != "customer@example.com" {
		t.Errorf("Attendee email = %q, want customer@example.com", event.Booking.Attendees[0].Email)
	}
	if event.Metadata["conversation_id"] != "42" {
		t.Errorf("Metadata conversation_id = %q, want 42", event.Metadata["conversation_id"])
	}
}

func TestParseWebhook_InvalidSignature(t *testing.T) {
	resetState()
	SetConfig(&Config{WebhookSecret: "secret"})

	body := `{"triggerEvent": "BOOKING_CREATED", "payload": {}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Cal-Signature-256", "invalid-sig")

	_, err := ParseWebhook(req)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("error = %v, want ErrInvalidSignature", err)
	}
}

func TestParseWebhook_MissingSignature(t *testing.T) {
	resetState()
	SetConfig(&Config{WebhookSecret: "secret"})

	body := `{"triggerEvent": "BOOKING_CREATED", "payload": {}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))

	_, err := ParseWebhook(req)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("error = %v, want ErrInvalidSignature", err)
	}
}

func TestParseWebhook_BookingCancelled(t *testing.T) {
	resetState()

	secret := "test-secret"
	SetConfig(&Config{WebhookSecret: secret})

	body := `{
		"triggerEvent": "BOOKING_CANCELLED",
		"payload": {
			"bookingId": 456,
			"title": "Cancelled Meeting",
			"startTime": "2026-03-20T14:00:00.000Z",
			"endTime": "2026-03-20T14:30:00.000Z",
			"organizer": {"email": "agent@example.com", "name": "Agent"},
			"attendees": [],
			"metadata": {"conversation_id": "99"}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Cal-Signature-256", signBody(secret, body))

	event, err := ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook error: %v", err)
	}
	if event.TriggerEvent != "BOOKING_CANCELLED" {
		t.Errorf("TriggerEvent = %q, want BOOKING_CANCELLED", event.TriggerEvent)
	}
}

func TestParseWebhook_EmptyBody(t *testing.T) {
	resetState()
	SetConfig(&Config{WebhookSecret: "secret"})

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(""))

	_, err := ParseWebhook(req)
	if !errors.Is(err, ErrEmptyBody) {
		t.Errorf("error = %v, want ErrEmptyBody", err)
	}
}
