# qtoolkit Module Code Patterns

## Directory Structure

```
<module_name>/
├── go.mod                      # go 1.24.0, github.com/wordgate/qtoolkit/<module_name>
├── <module_name>.go            # Main implementation
├── <module_name>_test.go       # Tests
├── <module_name>_config.yml    # Configuration template
└── README.md                   # Module documentation
```

## go.mod Template

```go
module github.com/wordgate/qtoolkit/<module_name>

go 1.24.0

require (
    github.com/spf13/viper v1.19.0
    // Add third-party SDK dependencies here
)
```

## Main Implementation Template

```go
// Package <module_name> provides <brief description>.
//
// Usage:
//
//	<module_name>.FunctionName(args)
//
//	<module_name>.Builder().
//	    Option1(value).
//	    Option2(value).
//	    Execute()
package <module_name>

import (
    "errors"
    "fmt"
    "sync"

    "github.com/spf13/viper"
)

// Errors
var (
    ErrNotConfigured = errors.New("<module_name>: not configured")
    ErrInvalidInput  = errors.New("<module_name>: invalid input")
    // Add service-specific errors
)

// Config holds module configuration.
type Config struct {
    // Required fields
    APIKey    string `yaml:"api_key"`
    // Optional fields with defaults
    Endpoint  string `yaml:"endpoint"`
    Timeout   int    `yaml:"timeout"` // seconds
}

var (
    globalConfig *Config
    globalClient *Client  // or appropriate client type
    clientOnce   sync.Once
    initErr      error
    configMux    sync.RWMutex
)

// loadConfigFromViper loads configuration from viper.
// Configuration path priority (cascading fallback):
// 1. <service>.<module>.field - Module-specific config
// 2. <service>.field - Global service config (if applicable)
func loadConfigFromViper() (*Config, error) {
    cfg := &Config{}

    // Load module-specific config
    cfg.APIKey = viper.GetString("<module_name>.api_key")
    cfg.Endpoint = viper.GetString("<module_name>.endpoint")
    cfg.Timeout = viper.GetInt("<module_name>.timeout")

    // Set defaults for optional fields
    if cfg.Endpoint == "" {
        cfg.Endpoint = "https://api.example.com"
    }
    if cfg.Timeout == 0 {
        cfg.Timeout = 30
    }

    // Validate required fields
    if cfg.APIKey == "" {
        return nil, fmt.Errorf("<module_name>.api_key is required")
    }

    return cfg, nil
}

// initialize performs the actual initialization.
// Called once via sync.Once.
func initialize() {
    cfg, err := loadConfigFromViper()
    if err != nil {
        configMux.RLock()
        cfg = globalConfig
        configMux.RUnlock()

        if cfg == nil {
            initErr = fmt.Errorf("config not available: %v", err)
            return
        }
    } else {
        configMux.Lock()
        globalConfig = cfg
        configMux.Unlock()
    }

    // Initialize client with config
    globalClient, initErr = createClient(cfg)
}

func ensureInitialized() error {
    clientOnce.Do(initialize)
    return initErr
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
    // Reset initialization state for testing
    clientOnce = sync.Once{}
    globalClient = nil
    initErr = nil
}

// createClient creates the actual client (implement based on SDK)
func createClient(cfg *Config) (*Client, error) {
    // Initialize third-party SDK client here
    return &Client{config: cfg}, nil
}

// Client wraps the third-party client.
type Client struct {
    config *Config
    // Add SDK client fields
}

// Get returns the initialized client.
func Get() (*Client, error) {
    if err := ensureInitialized(); err != nil {
        return nil, err
    }
    return globalClient, nil
}

// --- Public API Functions ---
// Keep API minimal, expose only essential operations

// DoSomething performs the primary operation.
func DoSomething(input string) (string, error) {
    client, err := Get()
    if err != nil {
        return "", err
    }
    return client.doSomething(input)
}

func (c *Client) doSomething(input string) (string, error) {
    if input == "" {
        return "", ErrInvalidInput
    }
    // Implementation using third-party SDK
    return "result", nil
}
```

## Builder Pattern (for complex operations)

```go
// Builder for fluent API
type RequestBuilder struct {
    option1 string
    option2 int
    option3 bool
}

// NewRequest creates a new request builder.
func NewRequest() *RequestBuilder {
    return &RequestBuilder{}
}

// Option1 sets option1.
func (b *RequestBuilder) Option1(val string) *RequestBuilder {
    b.option1 = val
    return b
}

// Option2 sets option2.
func (b *RequestBuilder) Option2(val int) *RequestBuilder {
    b.option2 = val
    return b
}

// Execute performs the request.
func (b *RequestBuilder) Execute() (Result, error) {
    client, err := Get()
    if err != nil {
        return Result{}, err
    }
    // Build and execute request
    return client.execute(b)
}
```

## Test Template

```go
package <module_name>

import (
    "net/http"
    "net/http/httptest"
    "sync"
    "testing"
)

// resetState resets global state for test isolation
func resetState() {
    configMux.Lock()
    globalConfig = nil
    configMux.Unlock()
    clientOnce = sync.Once{}
    globalClient = nil
    initErr = nil
}

func TestDoSomething_Success(t *testing.T) {
    resetState()

    // Setup mock server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request
        if r.Header.Get("Authorization") == "" {
            t.Error("missing Authorization header")
        }
        // Return mock response
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"result": "success"}`))
    }))
    defer server.Close()

    // Configure module to use mock server
    SetConfig(&Config{
        APIKey:   "test-key",
        Endpoint: server.URL,
    })

    // Test
    result, err := DoSomething("test-input")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != "success" {
        t.Errorf("result = %q, want %q", result, "success")
    }
}

func TestDoSomething_NotConfigured(t *testing.T) {
    resetState()
    // Don't set config

    _, err := DoSomething("test")
    if err == nil {
        t.Error("expected error when not configured")
    }
}

func TestDoSomething_InvalidInput(t *testing.T) {
    resetState()
    SetConfig(&Config{APIKey: "test-key"})

    _, err := DoSomething("")
    if err != ErrInvalidInput {
        t.Errorf("error = %v, want %v", err, ErrInvalidInput)
    }
}

func TestDoSomething_ServerError(t *testing.T) {
    resetState()

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer server.Close()

    SetConfig(&Config{
        APIKey:   "test-key",
        Endpoint: server.URL,
    })

    _, err := DoSomething("test")
    if err == nil {
        t.Error("expected error on server error")
    }
}
```

## Configuration Template

```yaml
# <Module Name> Configuration
# Add to your main config.yml

<module_name>:
  # API key (required)
  api_key: "YOUR_API_KEY"

  # API endpoint (optional, defaults to production)
  # endpoint: "https://api.example.com"

  # Request timeout in seconds (optional, default: 30)
  # timeout: 30

# Usage:
#   <module_name>.DoSomething("input")
#   <module_name>.NewRequest().Option1("val").Execute()

# Security Notes:
# - Never commit real API keys to version control
# - Use environment variables for production
```

## AWS Service Pattern (2-level fallback)

For AWS services, use cascading config fallback:

```go
func loadConfigFromViper() (*Config, error) {
    cfg := &Config{}

    // 1. Service-specific config (aws.<service>.*)
    cfg.Region = viper.GetString("aws.<service>.region")
    cfg.AccessKey = viper.GetString("aws.<service>.access_key")
    cfg.SecretKey = viper.GetString("aws.<service>.secret_key")

    // 2. Global AWS config fallback (aws.*)
    if cfg.Region == "" {
        cfg.Region = viper.GetString("aws.region")
    }
    if cfg.AccessKey == "" {
        cfg.AccessKey = viper.GetString("aws.access_key")
    }
    if cfg.SecretKey == "" {
        cfg.SecretKey = viper.GetString("aws.secret_key")
    }

    // Check for IMDS (EC2 instance metadata)
    cfg.UseIMDS = viper.GetBool("aws.use_imds")

    return cfg, nil
}
```

## Multi-Instance Pattern (like SQS queues)

For services with multiple instances:

```go
var (
    clients    = make(map[string]*Client)
    clientsMux sync.RWMutex
)

// Get returns client for the named instance.
func Get(name string) (*Client, error) {
    clientsMux.RLock()
    client, ok := clients[name]
    clientsMux.RUnlock()

    if ok {
        return client, nil
    }

    // Load config for this instance
    cfg, err := loadConfigForInstance(name)
    if err != nil {
        return nil, err
    }

    client, err = createClient(cfg)
    if err != nil {
        return nil, err
    }

    clientsMux.Lock()
    clients[name] = client
    clientsMux.Unlock()

    return client, nil
}

func loadConfigForInstance(name string) (*Config, error) {
    cfg := &Config{Name: name}

    // 1. Instance-specific config
    instancePath := fmt.Sprintf("<service>.instances.%s", name)
    if viper.IsSet(instancePath) {
        cfg.Field = viper.GetString(instancePath + ".field")
    }

    // 2. Service-level fallback
    if cfg.Field == "" {
        cfg.Field = viper.GetString("<service>.field")
    }

    return cfg, nil
}
```
