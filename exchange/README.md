# Exchange Rate API Module

Exchange rate API client for fetching real-time currency conversion rates using [ExchangeRate-API](https://www.exchangerate-api.com/).

## Features

- ✅ Real-time exchange rate data from 160+ currencies
- ✅ Lazy loading with `sync.Once` for thread-safe initialization
- ✅ Viper-based configuration auto-loading
- ✅ In-process per-currency cache (`cache_ttl`), concurrency-safe, one upstream request per refresh
- ✅ Stale-on-error: returns last good rates with an error wrapping `ErrStale`
- ✅ 10s HTTP timeout plus `context.Context` cancellation

## Installation

```bash
go get github.com/wordgate/qtoolkit/exchange
```

## Configuration

### Using Viper (Recommended)

Add to your `config.yml`:

```yaml
exchange_rate:
  api_key: "YOUR_EXCHANGE_RATE_API_KEY"  # Required
  cache_ttl: 3600                        # Optional: Cache TTL in seconds (default: 3600)
  base_currency: "USD"                   # Optional: Default base currency (default: "USD")
```

Load configuration at startup:

```go
import "github.com/spf13/viper"

viper.SetConfigFile("config.yml")
if err := viper.ReadInConfig(); err != nil {
    panic(err)
}
```

### Using SetConfig (Deprecated)

```go
import "github.com/wordgate/qtoolkit/exchange"

exchange.SetConfig(&exchange.Config{
    APIKey:       "YOUR_EXCHANGE_RATE_API_KEY",
    CacheTTL:     3600,
    BaseCurrency: "USD",
})
```

## Usage

```go
import "github.com/wordgate/qtoolkit/exchange"

// Cached for cache_ttl seconds per base currency
rates, err := exchange.GetRates(ctx, "USD")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("USD to CNY: %.4f\n", rates["CNY"])
```

The returned map is a copy; callers may modify it.

## API Response Format

```go
map[string]float64{
    "CNY": 7.2345,    // Chinese Yuan
    "EUR": 0.9234,    // Euro
    "GBP": 0.7891,    // British Pound
    "JPY": 149.56,    // Japanese Yen
    // ... 160+ currencies
}
```

## Supported Currencies

The API supports 160+ currencies including:
- Major: USD, EUR, GBP, JPY, CNY, CHF, CAD, AUD
- Cryptocurrencies: BTC, ETH (if supported by API)
- All ISO 4217 currency codes

## Error Handling

```go
rates, err := exchange.GetRates(ctx, "USD")
switch {
case errors.Is(err, exchange.ErrStale):
    // Refresh failed; rates holds the last successful (expired) value.
    // err also wraps the underlying cause. Decide whether to accept it.
case err != nil:
    // No rates available: network error, timeout, non-2xx HTTP status,
    // API error result (e.g. invalid-key), or ctx canceled.
    return err
}
```

## Configuration Paths

Configuration follows v1.0 cascading architecture:

| Field | Config Path | Default |
|-------|-------------|---------|
| API Key | `exchange_rate.api_key` | Required |
| Cache TTL | `exchange_rate.cache_ttl` | 3600 |
| Base Currency | `exchange_rate.base_currency` | "USD" |

## Testing

```bash
# Run tests
go test ./...

# With coverage
go test -cover ./...
```

Tests use an `httptest` mock server. The live API test runs only when `EXCHANGE_RATE_API_KEY` is set.

## Rate Limits

Free tier limits (as of 2025):
- 1,500 requests/month
- Updates: Once per day
- No credit card required

Check [ExchangeRate-API pricing](https://www.exchangerate-api.com/pricing) for current limits.

## Security Notes

- Never commit real API keys to version control
- Rotate API keys periodically
- Monitor usage to avoid rate limit overages

## Thread Safety

The module uses:
- `sync.Once` for lazy initialization (only once, thread-safe)
- `sync.RWMutex` for configuration access (concurrent reads, exclusive writes)
- A per-currency lock for the rate cache, so concurrent callers share one upstream request

Safe for concurrent use across goroutines.

## Module Architecture

Part of qtoolkit v1.0 modular architecture:
- Independent `go.mod` with minimal dependencies
- Viper-based auto-loading configuration
- Lazy initialization pattern
- Workspace-compatible for development

## Dependencies

- `github.com/spf13/viper` - Configuration management
- Standard library: `encoding/json`, `net/http`, `sync`

## Getting an API Key

1. Visit [ExchangeRate-API](https://www.exchangerate-api.com/)
2. Sign up for free account
3. Get API key from dashboard
4. Add to your configuration file

## Example Integration

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/spf13/viper"
    "github.com/wordgate/qtoolkit/exchange"
)

func main() {
    // Load configuration
    viper.SetConfigFile("config.yml")
    if err := viper.ReadInConfig(); err != nil {
        log.Fatal(err)
    }

    // Fetch exchange rates
    rates, err := exchange.GetRates(context.Background(), "USD")
    if err != nil {
        log.Fatalf("Failed to fetch rates: %v", err)
    }

    // Display rates
    currencies := []string{"CNY", "EUR", "GBP", "JPY"}
    for _, curr := range currencies {
        fmt.Printf("1 USD = %.4f %s\n", rates[curr], curr)
    }
}
```

## License

Part of the WordGate qtoolkit library.
