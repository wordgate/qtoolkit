package exchange

import (
	"os"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

// resetClient resets the global client state for testing
func resetClient() {
	clientOnce = sync.Once{}
	globalClient = nil
	globalConfig = nil
	initErr = nil
}

// getTestAPIKey returns a real API key if available, empty string otherwise
func getTestAPIKey() string {
	// Check environment variable first
	if key := os.Getenv("EXCHANGE_RATE_API_KEY"); key != "" {
		return key
	}
	return ""
}

// skipIfNoAPIKey skips the test if no real API key is available
func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if getTestAPIKey() == "" {
		t.Skip("Skipping test: EXCHANGE_RATE_API_KEY not configured")
	}
}

func TestExchangeApiGet(t *testing.T) {
	skipIfNoAPIKey(t)

	resetClient()
	viper.Reset()
	viper.Set("exchange_rate.api_key", getTestAPIKey())

	rates, err := ExchangeApiGet("usd")
	if err != nil {
		t.Errorf("exchange api, get failed: %v", err)
	} else if len(rates) == 0 {
		t.Errorf("exchange api, get failed: no rates got")
	}
}

func TestClientGetRates(t *testing.T) {
	skipIfNoAPIKey(t)

	resetClient()
	viper.Reset()

	// Test with SetConfig (deprecated method)
	cfg := &Config{
		APIKey:       getTestAPIKey(),
		CacheTTL:     3600,
		BaseCurrency: "USD",
	}
	SetConfig(cfg)

	client := Get()
	if client == nil {
		t.Fatal("failed to get client")
	}

	rates, err := client.GetRates("USD")
	if err != nil {
		t.Errorf("GetRates failed: %v", err)
	} else if len(rates) == 0 {
		t.Errorf("GetRates returned no rates")
	}
}

func TestViperConfig(t *testing.T) {
	// This test does NOT need real API key - it only tests config loading
	resetClient()
	viper.Reset()

	viper.Set("exchange_rate.api_key", "test_api_key_12345")
	viper.Set("exchange_rate.cache_ttl", 7200)
	viper.Set("exchange_rate.base_currency", "EUR")

	client := Get()
	if err := GetError(); err != nil {
		t.Fatalf("initialization failed: %v", err)
	}

	if client == nil {
		t.Fatal("client is nil")
	}

	if client.config.APIKey != "test_api_key_12345" {
		t.Errorf("expected APIKey='test_api_key_12345', got %s", client.config.APIKey)
	}

	if client.config.CacheTTL != 7200 {
		t.Errorf("expected CacheTTL=7200, got %d", client.config.CacheTTL)
	}

	if client.config.BaseCurrency != "EUR" {
		t.Errorf("expected BaseCurrency=EUR, got %s", client.config.BaseCurrency)
	}
}

func TestConfigDefaults(t *testing.T) {
	// This test does NOT need real API key - it only tests config defaults
	resetClient()
	viper.Reset()

	viper.Set("exchange_rate.api_key", "test_key")
	// Don't set cache_ttl and base_currency to test defaults

	client := Get()
	if err := GetError(); err != nil {
		t.Fatalf("initialization failed: %v", err)
	}

	// Check defaults
	if client.config.CacheTTL != 3600 {
		t.Errorf("expected default CacheTTL=3600, got %d", client.config.CacheTTL)
	}

	if client.config.BaseCurrency != "USD" {
		t.Errorf("expected default BaseCurrency=USD, got %s", client.config.BaseCurrency)
	}
}

func TestConfigNotSet(t *testing.T) {
	resetClient()
	viper.Reset()

	// Don't set any config
	client := Get()

	// Should be nil because config is not set
	if client != nil {
		t.Error("Expected nil client when config not set")
	}

	// Should have an error
	if GetError() == nil {
		t.Error("Expected error when config not set")
	}
}
