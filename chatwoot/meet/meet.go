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
func scheduleKey(id string) string    { return "meet:schedule:" + id }
func tokenKey(token string) string    { return "meet:token:" + token }
func activeKey(id string) string      { return "meet:active:" + id }
func bookingKey(bookingID int) string { return fmt.Sprintf("meet:booking:%d", bookingID) }

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

// Ensure unexported functions are referenced to avoid "declared and not used" errors in tests.
var _ = getConfig
var _ = resetState
var _ = saveSchedule
var _ = getScheduleByToken
var _ = deleteSchedule
var _ = findScheduleByBookingID
var _ = updateScheduleTime
var _ = tryActivate
