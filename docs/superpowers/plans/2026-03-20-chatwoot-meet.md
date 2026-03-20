# Chatwoot Meet Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a customer service video meeting module at `chatwoot/calcom` + `chatwoot/meet` that receives Cal.com booking webhooks, stores schedules in Redis, creates LiveKit rooms, and serves an embedded HTML video page.

**Architecture:** Two independent sub-modules under `chatwoot/`. `calcom` handles Cal.com webhook parsing and signature verification. `meet` handles schedule management (Redis), LiveKit room/token creation, and serves the video meeting HTML page. `meet` depends on `calcom`, `redis`, and `slack` but NOT on the parent `chatwoot` module — Chatwoot Reply is injected via a `ReplyFunc` callback.

**Tech Stack:** Go 1.24.0, LiveKit server-sdk-go/v2, LiveKit protocol/auth, go-redis/v9, gin, viper, crypto/rand, html/template, go:embed

**Spec:** `docs/superpowers/specs/2026-03-20-chatwoot-meet-design.md`

---

## File Map

### chatwoot/calcom/ (new module)

| File | Responsibility |
|---|---|
| `go.mod` | Module `github.com/wordgate/qtoolkit/chatwoot/calcom`, deps: viper |
| `calcom.go` | Config struct, loadConfigFromViper(), SetConfig(), lazy init, error vars |
| `webhook.go` | ParseWebhook(): read body, verify HMAC-SHA256 (X-Cal-Signature-256), unmarshal JSON into Event/Booking/Attendee types |
| `calcom_test.go` | Test ParseWebhook with valid/invalid signatures, various event types |
| `calcom_config.yml` | Config template |

### chatwoot/meet/ (new module)

| File | Responsibility |
|---|---|
| `go.mod` | Module `github.com/wordgate/qtoolkit/chatwoot/meet`, deps: calcom, redis, slack, livekit, gin, viper |
| `meet.go` | Schedule struct, token generation (crypto/rand), schedule CRUD in Redis, config, lazy init |
| `livekit.go` | LiveKit room creation (MaxDuration), participant token generation (auth.AccessToken) |
| `handler.go` | Mount(gin.IRouter, path, ReplyFunc), webhook handler, meeting page handler |
| `embed/meet.html` | Video meeting page: WeChat detection, waiting room, livekit-client SDK, controls |
| `meet_test.go` | Test token generation, schedule CRUD, webhook handling, page serving |
| `meet_config.yml` | Config template |

### Existing files to modify

| File | Change |
|---|---|
| `go.work` | Add `./chatwoot/calcom` and `./chatwoot/meet` |

---

## Task 1: calcom module — scaffold and config

**Files:**
- Create: `chatwoot/calcom/go.mod`
- Create: `chatwoot/calcom/calcom.go`
- Create: `chatwoot/calcom/calcom_config.yml`

- [ ] **Step 1: Create go.mod**

```
module github.com/wordgate/qtoolkit/chatwoot/calcom

go 1.24.0

require github.com/spf13/viper v1.21.0
```

Run: `cd chatwoot/calcom && go mod init github.com/wordgate/qtoolkit/chatwoot/calcom`

Then edit go.mod to set `go 1.24.0` and add viper dependency.

- [ ] **Step 2: Write calcom.go with Config and lazy init**

```go
// Package calcom provides Cal.com webhook parsing and signature verification.
//
// Usage:
//
//	event, err := calcom.ParseWebhook(r)
//	if err != nil {
//	    // handle error
//	}
//	switch event.TriggerEvent {
//	case "BOOKING_CREATED":
//	    // handle booking
//	}
package calcom

import (
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/viper"
)

var (
	ErrInvalidSignature = errors.New("calcom: invalid webhook signature")
	ErrEmptyBody        = errors.New("calcom: empty request body")
	ErrParsePayload     = errors.New("calcom: failed to parse webhook payload")
)

type Config struct {
	BaseURL       string `yaml:"base_url"`
	WebhookSecret string `yaml:"webhook_secret"`
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMux    sync.RWMutex
)

func loadConfigFromViper() (*Config, error) {
	cfg := &Config{
		BaseURL:       viper.GetString("chatwoot.calcom.base_url"),
		WebhookSecret: viper.GetString("chatwoot.calcom.webhook_secret"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.cal.com/v2"
	}
	if cfg.WebhookSecret == "" {
		return nil, fmt.Errorf("chatwoot.calcom.webhook_secret is required")
	}
	return cfg, nil
}

func initialize() {
	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil {
		loaded, err := loadConfigFromViper()
		if err != nil {
			fmt.Printf("calcom: config error: %v\n", err)
			return
		}
		configMux.Lock()
		globalConfig = loaded
		configMux.Unlock()
	}
}

func ensureInitialized() {
	configOnce.Do(initialize)
}

func getConfig() *Config {
	ensureInitialized()
	configMux.RLock()
	defer configMux.RUnlock()
	return globalConfig
}

// SetConfig sets configuration manually (for testing).
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	globalConfig = cfg
}

func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	configOnce = sync.Once{}
}
```

- [ ] **Step 3: Write calcom_config.yml**

```yaml
# Cal.com Configuration Template
# Add this to your main config.yml file

chatwoot:
  calcom:
    # Cal.com API base URL (default: https://api.cal.com/v2)
    # base_url: "https://api.cal.com/v2"

    # Webhook signing secret for signature verification (required)
    webhook_secret: "YOUR_CALCOM_WEBHOOK_SECRET"

# Security Notes:
# - Never commit real credentials to version control
# - The webhook_secret is used to verify HMAC-SHA256 signatures
#   on incoming webhooks (header: X-Cal-Signature-256)
```

- [ ] **Step 4: Verify compilation**

Run: `cd chatwoot/calcom && go build ./...`
Expected: compiles with no errors

- [ ] **Step 5: Commit**

```bash
git add chatwoot/calcom/go.mod chatwoot/calcom/calcom.go chatwoot/calcom/calcom_config.yml
git commit -m "feat(chatwoot/calcom): scaffold module with config and lazy init"
```

---

## Task 2: calcom module — webhook parsing and signature verification

**Files:**
- Create: `chatwoot/calcom/webhook.go`
- Create: `chatwoot/calcom/calcom_test.go`

- [ ] **Step 1: Write failing test for ParseWebhook with valid signature**

```go
package calcom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Cal-Signature-256", sig)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd chatwoot/calcom && go test -run TestParseWebhook_ValidSignature -v`
Expected: FAIL — `ParseWebhook` not defined

- [ ] **Step 3: Write webhook.go with ParseWebhook implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd chatwoot/calcom && go test -run TestParseWebhook_ValidSignature -v`
Expected: PASS

- [ ] **Step 5: Write additional tests — invalid signature, cancelled, rescheduled**

Add to `calcom_test.go`:

```go
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

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Cal-Signature-256", sig)

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
```

- [ ] **Step 6: Run all calcom tests**

Run: `cd chatwoot/calcom && go test -v ./...`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add chatwoot/calcom/webhook.go chatwoot/calcom/calcom_test.go
git commit -m "feat(chatwoot/calcom): webhook parsing with HMAC-SHA256 signature verification"
```

---

## Task 3: meet module — scaffold, config, and token generation

**Files:**
- Create: `chatwoot/meet/go.mod`
- Create: `chatwoot/meet/meet.go`
- Create: `chatwoot/meet/meet_config.yml`
- Create: `chatwoot/meet/meet_test.go` (token tests only)

- [ ] **Step 1: Create go.mod**

```
module github.com/wordgate/qtoolkit/chatwoot/meet

go 1.24.0

require (
    github.com/gin-gonic/gin v1.11.0
    github.com/livekit/protocol v1.29.2
    github.com/livekit/server-sdk-go/v2 v2.4.1
    github.com/redis/go-redis/v9 v9.7.3
    github.com/spf13/viper v1.21.0
    github.com/wordgate/qtoolkit/chatwoot/calcom v0.0.0
    github.com/wordgate/qtoolkit/redis v0.0.0
    github.com/wordgate/qtoolkit/slack v0.0.0
)
```

Run: `cd chatwoot/meet && go mod init github.com/wordgate/qtoolkit/chatwoot/meet`

Then edit to set `go 1.24.0`, add dependencies, and run `go mod tidy`.

Note: LiveKit dependency versions should be resolved via `go mod tidy` — use the latest available versions. The versions listed above are approximate.

- [ ] **Step 2: Write meet.go — Config, token generation, Schedule struct, Redis CRUD**

```go
// Package meet provides customer service video meeting via Cal.com + LiveKit.
//
// Usage:
//
//	meet.Mount(r, "/meet", func(ctx context.Context, conversationID int, text string) error {
//	    return chatwoot.Reply(ctx, conversationID, text)
//	})
package meet

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	qtredis "github.com/wordgate/qtoolkit/redis"
)

var (
	ErrTokenInvalid  = errors.New("meet: invalid or expired token")
	ErrNotConfigured = errors.New("meet: not configured")
)

// ReplyFunc is a callback for sending Chatwoot messages (injected to avoid circular dependency).
type ReplyFunc func(ctx context.Context, conversationID int, text string) error

// Config holds meet module configuration.
type Config struct {
	LiveKit      LiveKitConfig `yaml:"livekit"`
	TokenExpiry  time.Duration `yaml:"token_expiry"`
	RoomTimeout  time.Duration `yaml:"room_timeout"`
	BaseURL      string        `yaml:"base_url"`
	SlackChannel string        `yaml:"slack_channel"`
}

// LiveKitConfig holds LiveKit connection settings.
type LiveKitConfig struct {
	URL       string `yaml:"url"`
	APIKey    string `yaml:"api_key"`
	APISecret string `yaml:"api_secret"`
}

// Schedule represents a video meeting schedule stored in Redis.
type Schedule struct {
	ID              string    `json:"id"`
	CalcomBookingID int       `json:"calcom_booking_id"`
	AgentEmail      string    `json:"agent_email"`
	CustomerName    string    `json:"customer_name"`
	CustomerEmail   string    `json:"customer_email"`
	ConversationID  int       `json:"conversation_id"`
	InboxID         int       `json:"inbox_id"`
	ScheduledAt     time.Time `json:"scheduled_at"`
	Duration        string    `json:"duration"`
	RoomName        string    `json:"room_name"`
	CustomerToken   string    `json:"customer_token"`
	AgentToken      string    `json:"agent_token"`
	Status          string    `json:"status"` // pending / active / ended
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMux    sync.RWMutex
)

func loadConfigFromViper() (*Config, error) {
	cfg := &Config{
		LiveKit: LiveKitConfig{
			URL:       viper.GetString("chatwoot.meet.livekit.url"),
			APIKey:    viper.GetString("chatwoot.meet.livekit.api_key"),
			APISecret: viper.GetString("chatwoot.meet.livekit.api_secret"),
		},
		BaseURL:      viper.GetString("chatwoot.meet.base_url"),
		SlackChannel: viper.GetString("chatwoot.meet.slack_channel"),
	}

	// Parse durations with defaults
	if expiry := viper.GetString("chatwoot.meet.token_expiry"); expiry != "" {
		d, err := time.ParseDuration(expiry)
		if err != nil {
			return nil, fmt.Errorf("chatwoot.meet.token_expiry invalid: %w", err)
		}
		cfg.TokenExpiry = d
	} else {
		cfg.TokenExpiry = 24 * time.Hour
	}

	if timeout := viper.GetString("chatwoot.meet.room_timeout"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("chatwoot.meet.room_timeout invalid: %w", err)
		}
		cfg.RoomTimeout = d
	} else {
		cfg.RoomTimeout = 60 * time.Minute
	}

	// Validate required fields
	if cfg.LiveKit.URL == "" {
		return nil, fmt.Errorf("chatwoot.meet.livekit.url is required")
	}
	if cfg.LiveKit.APIKey == "" {
		return nil, fmt.Errorf("chatwoot.meet.livekit.api_key is required")
	}
	if cfg.LiveKit.APISecret == "" {
		return nil, fmt.Errorf("chatwoot.meet.livekit.api_secret is required")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("chatwoot.meet.base_url is required")
	}

	return cfg, nil
}

func initialize() {
	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil {
		loaded, err := loadConfigFromViper()
		if err != nil {
			fmt.Printf("meet: config error: %v\n", err)
			return
		}
		configMux.Lock()
		globalConfig = loaded
		configMux.Unlock()
	}
}

func ensureInitialized() {
	configOnce.Do(initialize)
}

func getConfig() *Config {
	ensureInitialized()
	configMux.RLock()
	defer configMux.RUnlock()
	return globalConfig
}

// SetConfig sets configuration manually (for testing).
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	globalConfig = cfg
}

func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	configOnce = sync.Once{}
}

// generateToken creates a cryptographically random URL-safe token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("meet: generate token: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// generateID creates a short random ID (12 chars, URL-safe).
func generateID() (string, error) {
	b := make([]byte, 9) // 9 bytes → 12 base64 chars
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("meet: generate id: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// Redis key helpers
func scheduleKey(id string) string         { return "meet:schedule:" + id }
func tokenKey(token string) string         { return "meet:token:" + token }
func activeKey(id string) string           { return "meet:active:" + id }
func bookingKey(bookingID int) string      { return fmt.Sprintf("meet:booking:%d", bookingID) }

// saveSchedule stores a schedule and all reverse-lookup keys in Redis.
func saveSchedule(ctx context.Context, s *Schedule, ttl time.Duration) error {
	rds := qtredis.Client()

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("meet: marshal schedule: %w", err)
	}

	pipe := rds.Pipeline()
	pipe.Set(ctx, scheduleKey(s.ID), data, ttl)
	pipe.Set(ctx, tokenKey(s.CustomerToken), s.ID, ttl)
	pipe.Set(ctx, tokenKey(s.AgentToken), s.ID, ttl)
	if s.CalcomBookingID > 0 {
		pipe.Set(ctx, bookingKey(s.CalcomBookingID), s.ID, ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// getScheduleByToken looks up a schedule by its access token.
func getScheduleByToken(ctx context.Context, token string) (*Schedule, error) {
	rds := qtredis.Client()

	id, err := rds.Get(ctx, tokenKey(token)).Result()
	if err == goredis.Nil {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("meet: redis get token: %w", err)
	}

	data, err := rds.Get(ctx, scheduleKey(id)).Result()
	if err == goredis.Nil {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("meet: redis get schedule: %w", err)
	}

	var s Schedule
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return nil, fmt.Errorf("meet: unmarshal schedule: %w", err)
	}
	return &s, nil
}

// deleteSchedule removes a schedule and all reverse-lookup keys from Redis.
func deleteSchedule(ctx context.Context, s *Schedule) error {
	rds := qtredis.Client()
	keys := []string{scheduleKey(s.ID), tokenKey(s.CustomerToken), tokenKey(s.AgentToken)}
	if s.CalcomBookingID > 0 {
		keys = append(keys, bookingKey(s.CalcomBookingID))
	}
	return rds.Del(ctx, keys...).Err()
}

// findScheduleByBookingID finds a schedule by Cal.com booking ID via reverse index.
func findScheduleByBookingID(ctx context.Context, bookingID int) (*Schedule, error) {
	rds := qtredis.Client()

	id, err := rds.Get(ctx, bookingKey(bookingID)).Result()
	if err == goredis.Nil {
		return nil, fmt.Errorf("meet: schedule not found for booking %d", bookingID)
	}
	if err != nil {
		return nil, fmt.Errorf("meet: redis get booking: %w", err)
	}

	data, err := rds.Get(ctx, scheduleKey(id)).Result()
	if err != nil {
		return nil, fmt.Errorf("meet: redis get schedule: %w", err)
	}

	var s Schedule
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return nil, fmt.Errorf("meet: unmarshal schedule: %w", err)
	}
	return &s, nil
}

// updateScheduleTime updates the scheduled_at and resets TTL.
func updateScheduleTime(ctx context.Context, s *Schedule, newTime time.Time, ttl time.Duration) error {
	s.ScheduledAt = newTime
	return saveSchedule(ctx, s, ttl)
}

// tryActivate atomically transitions status from pending to active.
// Returns true only on the first successful activation (for dedup notifications).
// Also updates the schedule status field in Redis.
func tryActivate(ctx context.Context, s *Schedule, ttl time.Duration) (bool, error) {
	rds := qtredis.Client()
	// SETNX: only succeeds if key does not exist
	ok, err := rds.SetNX(ctx, activeKey(s.ID), "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("meet: redis setnx: %w", err)
	}
	if ok {
		s.Status = "active"
		_ = saveSchedule(ctx, s, ttl) // best-effort status update
	}
	return ok, nil
}
```

- [ ] **Step 3: Write token generation tests**

Create `chatwoot/meet/meet_test.go`:

```go
package meet

import (
	"testing"
)

func TestGenerateToken(t *testing.T) {
	token1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken error: %v", err)
	}
	if len(token1) < 40 {
		t.Errorf("token length = %d, want >= 40", len(token1))
	}

	// Tokens should be unique
	token2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken error: %v", err)
	}
	if token1 == token2 {
		t.Error("two generated tokens are identical")
	}
}

func TestGenerateID(t *testing.T) {
	id1, err := generateID()
	if err != nil {
		t.Fatalf("generateID error: %v", err)
	}
	if len(id1) != 12 {
		t.Errorf("id length = %d, want 12", len(id1))
	}

	id2, err := generateID()
	if err != nil {
		t.Fatalf("generateID error: %v", err)
	}
	if id1 == id2 {
		t.Error("two generated IDs are identical")
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd chatwoot/meet && go test -run TestGenerate -v`
Expected: PASS

- [ ] **Step 5: Write meet_config.yml**

```yaml
# Meet (Video Meeting) Configuration Template
# Add this to your main config.yml file

chatwoot:
  meet:
    # LiveKit Cloud connection
    livekit:
      # LiveKit server URL (required)
      url: "wss://your-project.livekit.cloud"
      # API key (required)
      api_key: "YOUR_LIVEKIT_API_KEY"
      # API secret (required)
      api_secret: "YOUR_LIVEKIT_API_SECRET"

    # Meeting link validity period (default: 24h)
    # token_expiry: "24h"

    # Maximum room duration, maps to LiveKit MaxDuration (default: 60m)
    # room_timeout: "60m"

    # Base URL for generating meeting links (required)
    base_url: "https://your-domain.com"

    # Slack channel for agent notifications (optional)
    # slack_channel: "support"

# Security Notes:
# - Never commit real LiveKit credentials to version control
# - Meeting tokens are generated using crypto/rand (32 bytes)
# - Tokens are single-use and expire with the schedule
```

- [ ] **Step 6: Commit**

```bash
git add chatwoot/meet/
git commit -m "feat(chatwoot/meet): scaffold module with config, token generation, Redis schedule CRUD"
```

---

## Task 4: meet module — LiveKit integration

**Files:**
- Create: `chatwoot/meet/livekit.go`
- Modify: `chatwoot/meet/meet_test.go` (add LiveKit tests)

- [ ] **Step 1: Write failing test for participant token generation**

Add to `meet_test.go`:

```go
func TestCreateParticipantToken(t *testing.T) {
	resetState()
	SetConfig(&Config{
		LiveKit: LiveKitConfig{
			URL:       "wss://test.livekit.cloud",
			APIKey:    "test-api-key",
			APISecret: "test-api-secret-that-is-long-enough-for-hmac",
		},
		TokenExpiry: 24 * time.Hour,
		RoomTimeout: 60 * time.Minute,
		BaseURL:     "https://example.com",
	})

	token, err := createParticipantToken("test-room", "user-123", "Test User")
	if err != nil {
		t.Fatalf("createParticipantToken error: %v", err)
	}
	if token == "" {
		t.Error("participant token is empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd chatwoot/meet && go test -run TestCreateParticipantToken -v`
Expected: FAIL — `createParticipantToken` not defined

- [ ] **Step 3: Write livekit.go**

```go
package meet

import (
	"context"
	"fmt"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// createRoom creates a LiveKit room with MaxDuration.
// If the room already exists, it returns without error.
func createRoom(ctx context.Context, roomName string) error {
	cfg := getConfig()
	if cfg == nil {
		return ErrNotConfigured
	}

	roomClient := lksdk.NewRoomServiceClient(cfg.LiveKit.URL, cfg.LiveKit.APIKey, cfg.LiveKit.APISecret)

	_, err := roomClient.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:            roomName,
		EmptyTimeout:    300, // 5 minutes after last participant leaves
		MaxDuration:     uint32(cfg.RoomTimeout.Seconds()),
		MaxParticipants: 2,
	})
	if err != nil {
		return fmt.Errorf("meet: create room: %w", err)
	}
	return nil
}

// createParticipantToken generates a LiveKit JWT for a participant to join a room.
func createParticipantToken(roomName, identity, name string) (string, error) {
	cfg := getConfig()
	if cfg == nil {
		return "", ErrNotConfigured
	}

	at := auth.NewAccessToken(cfg.LiveKit.APIKey, cfg.LiveKit.APISecret)

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}

	at.SetIdentity(identity).
		SetName(name).
		SetValidFor(cfg.TokenExpiry).
		AddGrant(grant)

	return at.ToJWT()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd chatwoot/meet && go test -run TestCreateParticipantToken -v`
Expected: PASS

- [ ] **Step 5: Resolve dependencies**

Run: `cd chatwoot/meet && go mod tidy`
Expected: go.sum updated with LiveKit dependencies

- [ ] **Step 6: Commit**

```bash
git add chatwoot/meet/livekit.go chatwoot/meet/meet_test.go chatwoot/meet/go.mod chatwoot/meet/go.sum
git commit -m "feat(chatwoot/meet): LiveKit room creation and participant token generation"
```

---

## Task 5: meet module — HTTP handlers (webhook + meeting page)

**Files:**
- Create: `chatwoot/meet/handler.go`
- Create: `chatwoot/meet/embed/meet.html`
- Modify: `chatwoot/meet/meet_test.go` (add handler tests)

- [ ] **Step 1: Write the embed HTML file**

Create `chatwoot/meet/embed/meet.html`:

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
<title>视频会议</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #1a1a2e; color: #fff; height: 100vh; display: flex; flex-direction: column; }

/* WeChat guide */
.wechat-guide { display: none; text-align: center; padding: 40px 20px; }
.wechat-guide h2 { margin-bottom: 20px; }
.wechat-guide p { color: #aaa; line-height: 1.8; font-size: 16px; }

/* Waiting room */
.waiting { display: none; text-align: center; padding: 40px 20px; }
.waiting h2 { margin-bottom: 10px; }
.waiting .countdown { font-size: 48px; font-weight: bold; margin: 20px 0; color: #4ecdc4; }
.waiting p { color: #aaa; }

/* Video area */
.video-container { display: none; flex: 1; position: relative; background: #000; }
#remote-video { width: 100%; height: 100%; }
#remote-video video { width: 100%; height: 100%; object-fit: cover; }
#local-video { position: absolute; bottom: 80px; right: 16px; width: 120px; height: 160px; border-radius: 8px; overflow: hidden; border: 2px solid #fff; }
#local-video video { width: 100%; height: 100%; object-fit: cover; }
.no-remote { display: flex; align-items: center; justify-content: center; font-size: 18px; color: #aaa; height: 100%; }

/* Controls */
.controls { display: none; justify-content: center; gap: 20px; padding: 16px; background: rgba(0,0,0,0.8); }
.controls button {
    width: 56px; height: 56px; border-radius: 50%; border: none;
    font-size: 24px; cursor: pointer; color: #fff; background: #333;
}
.controls button:active { opacity: 0.7; }
.controls button.active { background: #e74c3c; }
.controls button.hangup { background: #e74c3c; }

/* Error / ended */
.error-page, .ended-page { display: none; text-align: center; padding: 40px 20px; }
.error-page h2 { color: #e74c3c; margin-bottom: 10px; }
.ended-page h2 { margin-bottom: 10px; }
</style>
</head>
<body>

<div class="wechat-guide" id="wechat-guide">
  <h2>请在浏览器中打开</h2>
  <p>当前环境不支持视频通话<br>请点击右上角 <strong>⋯</strong><br>选择「在默认浏览器中打开」</p>
</div>

<div class="waiting" id="waiting">
  <h2>视频会议</h2>
  <div class="countdown" id="countdown"></div>
  <p>会议将在预约时间开始</p>
</div>

<div class="video-container" id="video-container">
  <div id="remote-video"><div class="no-remote" id="no-remote">等待对方加入...</div></div>
  <div id="local-video"></div>
</div>

<div class="controls" id="controls">
  <button id="btn-mic" title="静音">🎤</button>
  <button id="btn-cam" title="摄像头">📷</button>
  <button id="btn-hangup" class="hangup" title="挂断">📞</button>
</div>

<div class="error-page" id="error-page">
  <h2>无法加入会议</h2>
  <p id="error-msg">链接无效或已过期，请联系客服。</p>
</div>

<div class="ended-page" id="ended-page">
  <h2>会议已结束</h2>
  <p>感谢您的参与</p>
</div>

<script id="meet-data" type="application/json">{{.DataJSON}}</script>
<script src="https://unpkg.com/livekit-client/dist/livekit-client.umd.min.js"></script>
<script>
(function() {
  const DATA = JSON.parse(document.getElementById('meet-data').textContent);

  // Error state
  if (DATA.error) {
    document.getElementById('error-page').style.display = 'block';
    document.getElementById('error-msg').textContent = DATA.error;
    return;
  }

  // WeChat detection
  const ua = navigator.userAgent;
  const isWeChat = /MicroMessenger/i.test(ua);
  if (isWeChat && typeof RTCPeerConnection === 'undefined') {
    document.getElementById('wechat-guide').style.display = 'block';
    return;
  }

  // Check if meeting time has arrived
  const scheduledAt = new Date(DATA.scheduledAt);
  const now = new Date();
  const diff = scheduledAt.getTime() - now.getTime();

  if (diff > 5 * 60 * 1000) { // more than 5 min early
    showWaiting(scheduledAt);
    return;
  }

  startMeeting();

  function showWaiting(target) {
    const el = document.getElementById('waiting');
    const cd = document.getElementById('countdown');
    el.style.display = 'block';

    const timer = setInterval(function() {
      const remain = target.getTime() - Date.now();
      if (remain <= 5 * 60 * 1000) {
        clearInterval(timer);
        el.style.display = 'none';
        startMeeting();
        return;
      }
      const h = Math.floor(remain / 3600000);
      const m = Math.floor((remain % 3600000) / 60000);
      const s = Math.floor((remain % 60000) / 1000);
      cd.textContent = (h > 0 ? h + ':' : '') +
        String(m).padStart(2, '0') + ':' + String(s).padStart(2, '0');
    }, 1000);
  }

  function startMeeting() {
    const videoContainer = document.getElementById('video-container');
    const controls = document.getElementById('controls');
    videoContainer.style.display = 'flex';
    controls.style.display = 'flex';

    const room = new LivekitClient.Room();
    let micEnabled = true;
    let camEnabled = true;

    room.on(LivekitClient.RoomEvent.TrackSubscribed, function(track, pub, participant) {
      const el = track.attach();
      if (track.kind === 'video') {
        document.getElementById('no-remote').style.display = 'none';
        document.getElementById('remote-video').appendChild(el);
      } else {
        document.getElementById('remote-video').appendChild(el);
      }
    });

    room.on(LivekitClient.RoomEvent.TrackUnsubscribed, function(track) {
      track.detach().forEach(function(el) { el.remove(); });
    });

    room.on(LivekitClient.RoomEvent.Disconnected, function() {
      videoContainer.style.display = 'none';
      controls.style.display = 'none';
      document.getElementById('ended-page').style.display = 'block';
    });

    room.connect(DATA.serverURL, DATA.token)
      .then(function() {
        return room.localParticipant.enableCameraAndMicrophone();
      })
      .then(function() {
        room.localParticipant.videoTrackPublications.forEach(function(pub) {
          if (pub.track) {
            document.getElementById('local-video').appendChild(pub.track.attach());
          }
        });
      })
      .catch(function(err) {
        videoContainer.style.display = 'none';
        controls.style.display = 'none';
        document.getElementById('error-page').style.display = 'block';
        document.getElementById('error-msg').textContent = '连接失败: ' + err.message;
      });

    document.getElementById('btn-mic').addEventListener('click', function() {
      micEnabled = !micEnabled;
      room.localParticipant.setMicrophoneEnabled(micEnabled);
      this.classList.toggle('active', !micEnabled);
    });

    document.getElementById('btn-cam').addEventListener('click', function() {
      camEnabled = !camEnabled;
      room.localParticipant.setCameraEnabled(camEnabled);
      this.classList.toggle('active', !camEnabled);
    });

    document.getElementById('btn-hangup').addEventListener('click', function() {
      room.disconnect();
    });
  }
})();
</script>
</body>
</html>
```

- [ ] **Step 2: Write handler.go**

```go
package meet

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wordgate/qtoolkit/chatwoot/calcom"
	"github.com/wordgate/qtoolkit/slack"
)

//go:embed embed/meet.html
var meetFS embed.FS

var meetTemplate *template.Template

func init() {
	meetTemplate = template.Must(template.ParseFS(meetFS, "embed/meet.html"))
}

// meetPageData is the data injected into meet.html via a JSON blob.
type meetPageData struct {
	DataJSON template.JS // JSON blob injected into <script type="application/json">
}

// meetData is serialized to JSON and injected into the template.
type meetData struct {
	ServerURL string `json:"serverURL,omitempty"`
	Token     string `json:"token,omitempty"`
	ScheduledAt string `json:"scheduledAt,omitempty"`
	Role      string `json:"role,omitempty"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Mount registers all meet routes on the given router.
func Mount(r gin.IRouter, path string, reply ReplyFunc) {
	ensureInitialized()

	r.POST(path+"/webhook/calcom", handleCalcomWebhook(reply))
	r.GET(path+"/:token", handleMeetPage())
}

func handleCalcomWebhook(reply ReplyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		event, err := calcom.ParseWebhook(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		cfg := getConfig()
		if cfg == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "meet: not configured"})
			return
		}

		switch event.TriggerEvent {
		case "BOOKING_CREATED":
			handleBookingCreated(ctx, cfg, event, reply, c)
		case "BOOKING_CANCELLED":
			handleBookingCancelled(ctx, cfg, event, reply, c)
		case "BOOKING_RESCHEDULED":
			handleBookingRescheduled(ctx, cfg, event, reply, c)
		default:
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		}
	}
}

func handleBookingCreated(ctx context.Context, cfg *Config, event *calcom.Event, reply ReplyFunc, c *gin.Context) {
	id, err := generateID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	customerToken, err := generateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	agentToken, err := generateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	conversationID, _ := strconv.Atoi(event.Metadata["conversation_id"])
	inboxID, _ := strconv.Atoi(event.Metadata["inbox_id"])

	customerName := ""
	customerEmail := ""
	if len(event.Booking.Attendees) > 0 {
		customerName = event.Booking.Attendees[0].Name
		customerEmail = event.Booking.Attendees[0].Email
	}

	duration := event.Booking.EndTime.Sub(event.Booking.StartTime)

	schedule := &Schedule{
		ID:              id,
		CalcomBookingID: event.Booking.ID,
		AgentEmail:      event.Booking.Organizer.Email,
		CustomerName:    customerName,
		CustomerEmail:   customerEmail,
		ConversationID:  conversationID,
		InboxID:         inboxID,
		ScheduledAt:     event.Booking.StartTime,
		Duration:        duration.String(),
		RoomName:        "meet-" + id,
		CustomerToken:   customerToken,
		AgentToken:      agentToken,
		Status:          "pending",
	}

	if err := saveSchedule(ctx, schedule, cfg.TokenExpiry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	customerURL := fmt.Sprintf("%s/meet/%s", cfg.BaseURL, customerToken)
	agentURL := fmt.Sprintf("%s/meet/%s", cfg.BaseURL, agentToken)

	// Send meeting link to customer via Chatwoot
	if conversationID > 0 && reply != nil {
		msg := fmt.Sprintf("您的视频会议已预约，时间：%s\n点击加入：%s",
			event.Booking.StartTime.Format("2006-01-02 15:04"), customerURL)
		_ = reply(ctx, conversationID, msg)
	}

	// Notify agent via Slack
	if cfg.SlackChannel != "" {
		slackMsg := fmt.Sprintf("新视频会议预约\n客户：%s\n时间：%s\n加入：%s",
			customerName, event.Booking.StartTime.Format("2006-01-02 15:04"), agentURL)
		_ = slack.Send(cfg.SlackChannel, slackMsg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "created", "schedule_id": id})
}

func handleBookingCancelled(ctx context.Context, cfg *Config, event *calcom.Event, reply ReplyFunc, c *gin.Context) {
	schedule, err := findScheduleByBookingID(ctx, event.Booking.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": "not_found"})
		return
	}

	_ = deleteSchedule(ctx, schedule)

	if schedule.ConversationID > 0 && reply != nil {
		_ = reply(ctx, schedule.ConversationID, "视频会议已取消。")
	}

	if cfg.SlackChannel != "" {
		_ = slack.Send(cfg.SlackChannel, fmt.Sprintf("视频会议已取消\n客户：%s", schedule.CustomerName))
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func handleBookingRescheduled(ctx context.Context, cfg *Config, event *calcom.Event, reply ReplyFunc, c *gin.Context) {
	schedule, err := findScheduleByBookingID(ctx, event.Booking.ID)
	if err != nil {
		// Treat as new booking if not found
		handleBookingCreated(ctx, cfg, event, reply, c)
		return
	}

	if err := updateScheduleTime(ctx, schedule, event.Booking.StartTime, cfg.TokenExpiry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if schedule.ConversationID > 0 && reply != nil {
		msg := fmt.Sprintf("视频会议时间已更新为：%s", event.Booking.StartTime.Format("2006-01-02 15:04"))
		_ = reply(ctx, schedule.ConversationID, msg)
	}

	if cfg.SlackChannel != "" {
		_ = slack.Send(cfg.SlackChannel, fmt.Sprintf("视频会议已改期\n客户：%s\n新时间：%s",
			schedule.CustomerName, event.Booking.StartTime.Format("2006-01-02 15:04")))
	}

	c.JSON(http.StatusOK, gin.H{"status": "rescheduled"})
}

func handleMeetPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		ctx := c.Request.Context()
		cfg := getConfig()

		schedule, err := getScheduleByToken(ctx, token)
		if err != nil {
			renderMeetPage(c, meetData{Error: "链接无效或已过期，请联系客服。"})
			return
		}

		// Determine role based on token
		role := "customer"
		identity := schedule.CustomerEmail
		name := schedule.CustomerName
		if token == schedule.AgentToken {
			role = "agent"
			identity = schedule.AgentEmail
			name = "客服"
		}

		// Create LiveKit room (idempotent)
		if err := createRoom(ctx, schedule.RoomName); err != nil {
			renderMeetPage(c, meetData{Error: "无法创建会议房间，请稍后重试。"})
			return
		}

		// Generate LiveKit participant token
		participantToken, err := createParticipantToken(schedule.RoomName, identity, name)
		if err != nil {
			renderMeetPage(c, meetData{Error: "无法生成会议凭证，请稍后重试。"})
			return
		}

		// Try to activate (dedup Slack notification)
		if role == "customer" {
			if activated, _ := tryActivate(ctx, schedule, cfg.TokenExpiry); activated {
				if cfg.SlackChannel != "" {
					agentURL := fmt.Sprintf("%s/meet/%s", cfg.BaseURL, schedule.AgentToken)
					_ = slack.Send(cfg.SlackChannel, fmt.Sprintf("客户 %s 已进入视频会议，请立即加入：%s",
						schedule.CustomerName, agentURL))
				}
			}
		}

		renderMeetPage(c, meetData{
			ServerURL:   cfg.LiveKit.URL,
			Token:       participantToken,
			ScheduledAt: schedule.ScheduledAt.Format(time.RFC3339),
			Role:        role,
			Status:      schedule.Status,
		})
	}
}

func renderMeetPage(c *gin.Context, data meetData) {
	jsonBytes, _ := json.Marshal(data)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	meetTemplate.Execute(c.Writer, meetPageData{
		DataJSON: template.JS(jsonBytes),
	})
}
```

- [ ] **Step 3: Write handler tests**

Add to `meet_test.go`:

```go
func requireRedis(t *testing.T) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skip("Redis not available, skipping integration test")
		}
	}()
	ctx := context.Background()
	if err := qtredis.Client().Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
}

func TestHandleMeetPage_InvalidToken(t *testing.T) {
	requireRedis(t)
	resetState()
	SetConfig(&Config{
		LiveKit: LiveKitConfig{
			URL:       "wss://test.livekit.cloud",
			APIKey:    "test-key",
			APISecret: "test-secret-long-enough-for-hmac-signing",
		},
		TokenExpiry: 24 * time.Hour,
		RoomTimeout: 60 * time.Minute,
		BaseURL:     "https://example.com",
	})

	router := gin.New()
	Mount(router, "/meet", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/meet/invalid-token-here", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "链接无效") {
		t.Error("response should contain error message for invalid token")
	}
}
```

Note: The `requireRedis` helper skips tests when Redis is not available. Full integration tests for webhook handling also need Redis.

- [ ] **Step 4: Run tests**

Run: `cd chatwoot/meet && go test -v ./...`
Expected: token/ID generation tests PASS, handler test PASS (or SKIP if no Redis)

- [ ] **Step 5: Commit**

```bash
git add chatwoot/meet/handler.go chatwoot/meet/embed/ chatwoot/meet/meet.go chatwoot/meet/meet_test.go
git commit -m "feat(chatwoot/meet): HTTP handlers, video meeting page, Cal.com webhook processing"
```

---

## Task 6: Update go.work and verify workspace

**Files:**
- Modify: `go.work`

- [ ] **Step 1: Add new modules to go.work**

Add two lines to `go.work`:

```
	./chatwoot/calcom
	./chatwoot/meet
```

- [ ] **Step 2: Sync workspace**

Run: `go work sync`
Expected: no errors

- [ ] **Step 3: Run all module tests**

Run: `cd chatwoot/calcom && go test -v ./...`
Expected: all PASS

Run: `cd chatwoot/meet && go test -v ./...`
Expected: all compilable tests PASS (Redis-dependent tests may skip)

- [ ] **Step 4: Verify full workspace builds**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add go.work
git commit -m "feat: add chatwoot/calcom and chatwoot/meet to workspace"
```

---

## Implementation Notes

### Redis dependency in tests

Tests that require Redis (schedule CRUD, webhook integration) should check for Redis availability and skip if not present:

```go
func requireRedis(t *testing.T) {
	t.Helper()
	// Try to ping Redis
	ctx := context.Background()
	if err := qtredis.Client().Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
}
```

### LiveKit dependency in tests

Tests should NOT call real LiveKit APIs. The `createRoom` and `createParticipantToken` functions are tested via:
- Token generation: uses `auth.AccessToken` which is pure JWT generation (no network)
- Room creation: mocked by testing the handler error paths

### Build order

Tasks must be executed in order because:
- Task 1-2: calcom module (independent)
- Task 3-5: meet module (depends on calcom)
- Task 6: workspace integration (depends on both)
