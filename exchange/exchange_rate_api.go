package exchange

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

var (
	globalConfig *Config      // Global configuration
	globalClient *Client      // Global client
	clientOnce   sync.Once    // Ensure initialization happens only once
	initErr      error        // Initialization error
	configMux    sync.RWMutex // Configuration read/write lock
)

// Config represents exchange rate API configuration
type Config struct {
	APIKey       string `yaml:"api_key"`
	CacheTTL     int    `yaml:"cache_ttl"`      // Cache time-to-live in seconds
	BaseCurrency string `yaml:"base_currency"`  // Default base currency
}

// Client represents the exchange rate API client
type Client struct {
	config *Config
}

// ExchangeApiResp represents the API response structure
type ExchangeApiResp struct {
	TimeLastUpdateUnix int64              `json:"time_last_update_unix"`
	Result             string             `json:"result"`
	ConversionRates    map[string]float64 `json:"conversion_rates"` // upper case map
}

// loadConfigFromViper loads configuration from viper
// Configuration path: exchange_rate.*
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	// Load exchange rate config
	cfg.APIKey = viper.GetString("exchange_rate.api_key")
	cfg.CacheTTL = viper.GetInt("exchange_rate.cache_ttl")
	cfg.BaseCurrency = viper.GetString("exchange_rate.base_currency")

	// Set defaults
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 3600 // Default: 1 hour
	}
	if cfg.BaseCurrency == "" {
		cfg.BaseCurrency = "USD"
	}

	// Validate required fields
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("exchange_rate.api_key is required")
	}

	return cfg, nil
}

// initialize performs the actual initialization
// Called once via sync.Once
func initialize() {
	// Try to load from viper first
	cfg, err := loadConfigFromViper()
	if err != nil {
		// Fall back to SetConfig if viper config not available
		configMux.RLock()
		cfg = globalConfig
		configMux.RUnlock()

		if cfg == nil {
			initErr = fmt.Errorf("config not available: %v", err)
			return
		}
	} else {
		// Store loaded config
		configMux.Lock()
		globalConfig = cfg
		configMux.Unlock()
	}

	// Initialize client with config
	globalClient = &Client{config: cfg}
}

// Get returns the client with lazy initialization
func Get() *Client {
	clientOnce.Do(initialize)
	return globalClient
}

// GetError returns the initialization error if any
func GetError() error {
	return initErr
}

// SetConfig sets the configuration for lazy loading (deprecated)
// Use viper configuration instead
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	globalConfig = cfg
}

// GetRates fetches exchange rates for the given currency
func (c *Client) GetRates(currency string) (map[string]float64, error) {
	if c == nil || c.config == nil {
		return nil, fmt.Errorf("exchange rate client not initialized")
	}

	url := fmt.Sprintf("https://v6.exchangerate-api.com/v6/%s/latest/%s",
		c.config.APIKey, strings.ToUpper(currency))

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	ex := &ExchangeApiResp{}
	if err := json.Unmarshal(body, ex); err != nil {
		return nil, err
	}

	if ex.Result != "success" {
		return nil, fmt.Errorf("API returned non-success result: %s", ex.Result)
	}

	return ex.ConversionRates, nil
}

// ExchangeApiGet fetches exchange rates (backward compatible function)
// Deprecated: Use Get().GetRates(currency) instead
func ExchangeApiGet(currency string) (map[string]float64, error) {
	client := Get()
	if err := GetError(); err != nil {
		return nil, err
	}
	return client.GetRates(currency)
}
