package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

var (
	globalConfig *Config      // Global configuration
	globalClient *Client      // Global client
	clientOnce   sync.Once    // Ensure initialization happens only once
	initErr      error        // Initialization error
	configMux    sync.RWMutex // Configuration read/write lock

	apiBaseURL = "https://v6.exchangerate-api.com"
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// ErrStale is wrapped in the error returned alongside last-known-good rates
// when refreshing an expired cache entry fails.
var ErrStale = errors.New("exchange rates are stale")

// Config represents exchange rate API configuration
type Config struct {
	APIKey       string `yaml:"api_key"`
	CacheTTL     int    `yaml:"cache_ttl"`     // Cache time-to-live in seconds
	BaseCurrency string `yaml:"base_currency"` // Default base currency
}

// Client represents the exchange rate API client
type Client struct {
	config *Config

	mu    sync.Mutex
	cache map[string]*cacheEntry // keyed by upper-case base currency
}

// cacheEntry holds the last successful rates for one base currency.
// sem (capacity 1) guards rates/fetchedAt and serializes refreshes.
type cacheEntry struct {
	sem       chan struct{}
	rates     map[string]float64
	fetchedAt time.Time
}

// ExchangeApiResp represents the API response structure
type ExchangeApiResp struct {
	TimeLastUpdateUnix int64              `json:"time_last_update_unix"`
	Result             string             `json:"result"`
	ErrorType          string             `json:"error-type"`
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

// GetRates returns conversion rates for the given base currency, cached
// in-process for Config.CacheTTL seconds per currency.
//
// If a refresh fails while an expired entry exists, the last successful rates
// are returned together with an error wrapping ErrStale and the cause, so the
// caller can decide whether stale data is acceptable:
//
//	rates, err := exchange.GetRates(ctx, "USD")
//	if errors.Is(err, exchange.ErrStale) { /* rates is the last good value */ }
//
// The returned map is a copy and may be modified freely.
func (c *Client) GetRates(ctx context.Context, currency string) (map[string]float64, error) {
	if c == nil || c.config == nil {
		return nil, fmt.Errorf("exchange rate client not initialized")
	}
	currency = strings.ToUpper(currency)
	if !isCurrencyCode(currency) {
		return nil, fmt.Errorf("invalid currency code %q", currency)
	}
	e := c.entry(currency)

	// Serialize refreshes per currency while still honoring ctx.
	select {
	case e.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-e.sem }()

	ttl := time.Duration(c.config.CacheTTL) * time.Second
	if e.rates != nil && time.Since(e.fetchedAt) < ttl {
		return maps.Clone(e.rates), nil
	}

	rates, err := c.fetch(ctx, currency)
	if err != nil {
		if e.rates != nil {
			return maps.Clone(e.rates), fmt.Errorf("%w: %w", ErrStale, err)
		}
		return nil, err
	}
	e.rates, e.fetchedAt = rates, time.Now()
	return maps.Clone(rates), nil
}

// entry returns the cache entry for currency, creating it if needed.
func (c *Client) entry(currency string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = make(map[string]*cacheEntry)
	}
	e, ok := c.cache[currency]
	if !ok {
		e = &cacheEntry{sem: make(chan struct{}, 1)}
		c.cache[currency] = e
	}
	return e
}

// fetch requests rates from ExchangeRate-API.
func (c *Client) fetch(ctx context.Context, currency string) (map[string]float64, error) {
	url := fmt.Sprintf("%s/v6/%s/latest/%s", apiBaseURL, c.config.APIKey, currency)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// *url.Error embeds the request URL, which contains the API key.
		var urlErr *neturl.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("exchange rate request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("exchange rate read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("exchange rate API HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}

	ex := &ExchangeApiResp{}
	if err := json.Unmarshal(body, ex); err != nil {
		return nil, fmt.Errorf("exchange rate decode: %w: %s", err, truncate(body, 200))
	}
	if ex.Result != "success" {
		return nil, fmt.Errorf("exchange rate API result %q: %s", ex.Result, ex.ErrorType)
	}
	if len(ex.ConversionRates) == 0 {
		return nil, fmt.Errorf("exchange rate API returned no rates")
	}
	return ex.ConversionRates, nil
}

// isCurrencyCode reports whether s is a 3-letter upper-case ISO 4217 code.
// This also bounds the cache to at most 26^3 entries.
func isCurrencyCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

// GetRates returns conversion rates for currency using the global client.
// See Client.GetRates for caching and ErrStale semantics.
func GetRates(ctx context.Context, currency string) (map[string]float64, error) {
	client := Get()
	if err := GetError(); err != nil {
		return nil, err
	}
	return client.GetRates(ctx, currency)
}
