package exchange

import (
	"sync"
	"testing"

	"github.com/spf13/viper"
)

func TestExchangeApiGet(t *testing.T) {
	viper.Set("exchange_rate.api_key", "YOUR_EXCHANGE_RATE_API_KEY")

	rates, err := ExchangeApiGet("usd")
	if err != nil {
		t.Errorf("exchange api, get failed: %v", err)
	} else if len(rates) == 0 {
		t.Errorf("exchange api, get failed: no rates got")
	}
}

func TestClientGetRates(t *testing.T) {
	// Test with SetConfig (deprecated method)
	cfg := &Config{
		APIKey:       "YOUR_EXCHANGE_RATE_API_KEY",
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
	// Test with viper configuration
	viper.Set("exchange_rate.api_key", "YOUR_EXCHANGE_RATE_API_KEY")
	viper.Set("exchange_rate.cache_ttl", 7200)
	viper.Set("exchange_rate.base_currency", "EUR")

	// Reset client for this test
	clientOnce = *new(sync.Once)
	globalClient = nil
	globalConfig = nil

	client := Get()
	if err := GetError(); err != nil {
		t.Fatalf("initialization failed: %v", err)
	}

	if client == nil {
		t.Fatal("client is nil")
	}

	if client.config.APIKey == "" {
		t.Error("APIKey not loaded from viper")
	}

	if client.config.CacheTTL != 7200 {
		t.Errorf("expected CacheTTL=7200, got %d", client.config.CacheTTL)
	}

	if client.config.BaseCurrency != "EUR" {
		t.Errorf("expected BaseCurrency=EUR, got %s", client.config.BaseCurrency)
	}
}

func TestConfigDefaults(t *testing.T) {
	viper.Set("exchange_rate.api_key", "test_key")

	// Reset for clean test
	clientOnce = *new(sync.Once)
	globalClient = nil
	globalConfig = nil

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
