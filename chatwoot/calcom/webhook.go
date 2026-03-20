package calcom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Event represents a Cal.com webhook event.
type Event struct {
	TriggerEvent string            `json:"triggerEvent"`
	Booking      Booking           `json:"-"`
	Metadata     map[string]string `json:"-"`
}

// Booking represents a Cal.com booking.
type Booking struct {
	ID        int        `json:"bookingId"`
	Title     string     `json:"title"`
	StartTime time.Time  `json:"startTime"`
	EndTime   time.Time  `json:"endTime"`
	Attendees []Attendee `json:"attendees"`
	Organizer Attendee   `json:"organizer"`
}

// Attendee represents a booking participant.
type Attendee struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// rawWebhook is the raw Cal.com webhook JSON structure.
type rawWebhook struct {
	TriggerEvent string          `json:"triggerEvent"`
	Payload      json.RawMessage `json:"payload"`
}

// rawPayload extracts fields from the nested payload.
type rawPayload struct {
	BookingID int               `json:"bookingId"`
	Title     string            `json:"title"`
	StartTime time.Time         `json:"startTime"`
	EndTime   time.Time         `json:"endTime"`
	Attendees []Attendee        `json:"attendees"`
	Organizer Attendee          `json:"organizer"`
	Metadata  map[string]string `json:"metadata"`
}

// ParseWebhook parses and verifies a Cal.com webhook request.
// Signature is verified using the configured webhook_secret (HMAC-SHA256).
// Signature header: X-Cal-Signature-256 (hex-encoded).
func ParseWebhook(r *http.Request) (*Event, error) {
	cfg := getConfig()
	if cfg == nil {
		return nil, fmt.Errorf("calcom: not configured")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmptyBody, err)
	}
	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	// Verify HMAC-SHA256 signature
	sig := r.Header.Get("X-Cal-Signature-256")
	if sig == "" {
		return nil, ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, ErrInvalidSignature
	}

	// Parse JSON
	var raw rawWebhook
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParsePayload, err)
	}

	var payload rawPayload
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParsePayload, err)
	}

	event := &Event{
		TriggerEvent: raw.TriggerEvent,
		Booking: Booking{
			ID:        payload.BookingID,
			Title:     payload.Title,
			StartTime: payload.StartTime,
			EndTime:   payload.EndTime,
			Attendees: payload.Attendees,
			Organizer: payload.Organizer,
		},
		Metadata: payload.Metadata,
	}

	return event, nil
}
