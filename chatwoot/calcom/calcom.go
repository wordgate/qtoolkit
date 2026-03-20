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
