package exchange

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	return os.Getenv("EXCHANGE_RATE_API_KEY")
}

// skipIfNoAPIKey skips the test if no real API key is available
func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if getTestAPIKey() == "" {
		t.Skip("Skipping test: EXCHANGE_RATE_API_KEY not configured")
	}
}

// mockAPI starts a fake ExchangeRate-API server. handler decides the response
// for each request; the request count is returned for cache assertions.
func mockAPI(t *testing.T, handler http.HandlerFunc) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	old := apiBaseURL
	apiBaseURL = server.URL
	t.Cleanup(func() { apiBaseURL = old })

	resetClient()
	viper.Reset()
	SetConfig(&Config{APIKey: "test_key", CacheTTL: 3600, BaseCurrency: "USD"})
	return &calls
}

func writeRates(w http.ResponseWriter, cny float64) {
	fmt.Fprintf(w, `{"result":"success","time_last_update_unix":1,"conversion_rates":{"USD":1,"CNY":%v}}`, cny)
}

func TestGetRatesCached(t *testing.T) {
	calls := mockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v6/test_key/latest/USD" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		writeRates(w, 7.1)
	})

	for i := 0; i < 3; i++ {
		rates, err := GetRates(context.Background(), "usd")
		if err != nil {
			t.Fatalf("GetRates: %v", err)
		}
		if rates["CNY"] != 7.1 {
			t.Fatalf("CNY = %v, want 7.1", rates["CNY"])
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("upstream called %d times, want 1", n)
	}
}

func TestGetRatesCachePerCurrency(t *testing.T) {
	calls := mockAPI(t, func(w http.ResponseWriter, r *http.Request) { writeRates(w, 7.1) })

	GetRates(context.Background(), "USD")
	GetRates(context.Background(), "EUR")
	GetRates(context.Background(), "EUR")
	if n := calls.Load(); n != 2 {
		t.Errorf("upstream called %d times, want 2", n)
	}
}

func TestGetRatesReturnsCopy(t *testing.T) {
	mockAPI(t, func(w http.ResponseWriter, r *http.Request) { writeRates(w, 7.1) })

	rates, _ := GetRates(context.Background(), "USD")
	rates["CNY"] = 999
	delete(rates, "USD")

	again, _ := GetRates(context.Background(), "USD")
	if again["CNY"] != 7.1 || again["USD"] != 1 {
		t.Errorf("cache was mutated by caller: %v", again)
	}
}

func TestGetRatesExpiresAndRefreshes(t *testing.T) {
	var cny atomic.Int32
	cny.Store(7)
	calls := mockAPI(t, func(w http.ResponseWriter, r *http.Request) { writeRates(w, float64(cny.Load())) })

	GetRates(context.Background(), "USD")
	expireCache()
	cny.Store(8)

	rates, err := GetRates(context.Background(), "USD")
	if err != nil {
		t.Fatalf("GetRates: %v", err)
	}
	if rates["CNY"] != 8 {
		t.Errorf("CNY = %v, want refreshed 8", rates["CNY"])
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("upstream called %d times, want 2", n)
	}
}

func TestGetRatesStaleOnError(t *testing.T) {
	var fail atomic.Bool
	mockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("<html>bad gateway</html>"))
			return
		}
		writeRates(w, 7.1)
	})

	GetRates(context.Background(), "USD")
	expireCache()
	fail.Store(true)

	rates, err := GetRates(context.Background(), "USD")
	if !errors.Is(err, ErrStale) {
		t.Fatalf("err = %v, want ErrStale", err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error should carry upstream cause, got %v", err)
	}
	if rates["CNY"] != 7.1 {
		t.Errorf("stale rates = %v, want last good value", rates)
	}
}

func TestGetRatesNoCacheError(t *testing.T) {
	mockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<html>oops</html>"))
	})

	rates, err := GetRates(context.Background(), "USD")
	if err == nil || rates != nil {
		t.Fatalf("want error and nil rates, got %v, %v", rates, err)
	}
	if errors.Is(err, ErrStale) {
		t.Error("must not be ErrStale when nothing cached")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "oops") {
		t.Errorf("error should include status and body, got %v", err)
	}
}

func TestGetRatesAPIErrorResult(t *testing.T) {
	mockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":"error","error-type":"invalid-key"}`))
	})

	_, err := GetRates(context.Background(), "USD")
	if err == nil || !strings.Contains(err.Error(), "invalid-key") {
		t.Errorf("want invalid-key error, got %v", err)
	}
}

func TestGetRatesContextCancel(t *testing.T) {
	release := make(chan struct{})
	mockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := GetRates(ctx, "USD")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("GetRates did not honor context deadline")
	}
}

func TestGetRatesConcurrentSingleFetch(t *testing.T) {
	calls := mockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		writeRates(w, 7.1)
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rates, err := GetRates(context.Background(), "USD")
			if err != nil || rates["CNY"] != 7.1 {
				t.Errorf("GetRates: %v %v", rates, err)
			}
			rates["CNY"] = 0 // concurrent writes to returned maps must be safe
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("upstream called %d times, want 1", n)
	}
}

func TestGetRatesNotInitialized(t *testing.T) {
	resetClient()
	viper.Reset()

	if _, err := GetRates(context.Background(), "USD"); err == nil {
		t.Error("want error when config not set")
	}
}

// expireCache marks every cached entry as expired.
func expireCache() {
	c := Get()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.cache {
		e.sem <- struct{}{}
		e.fetchedAt = time.Time{}
		<-e.sem
	}
}

func TestGetRatesRealAPI(t *testing.T) {
	skipIfNoAPIKey(t)

	resetClient()
	viper.Reset()
	viper.Set("exchange_rate.api_key", getTestAPIKey())

	rates, err := GetRates(context.Background(), "usd")
	if err != nil {
		t.Fatalf("GetRates: %v", err)
	}
	if len(rates) == 0 {
		t.Error("no rates returned")
	}
}

func TestViperConfig(t *testing.T) {
	resetClient()
	viper.Reset()

	viper.Set("exchange_rate.api_key", "test_api_key_12345")
	viper.Set("exchange_rate.cache_ttl", 7200)
	viper.Set("exchange_rate.base_currency", "EUR")

	client := Get()
	if err := GetError(); err != nil {
		t.Fatalf("initialization failed: %v", err)
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
	resetClient()
	viper.Reset()

	viper.Set("exchange_rate.api_key", "test_key")

	client := Get()
	if err := GetError(); err != nil {
		t.Fatalf("initialization failed: %v", err)
	}

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

	if client := Get(); client != nil {
		t.Error("Expected nil client when config not set")
	}
	if GetError() == nil {
		t.Error("Expected error when config not set")
	}
}
