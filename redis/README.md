# Redis Module

Redis module for qtoolkit providing Redis client management, caching utilities, and broadcast services.

## Features

- **Redis Client Management**: Single Redis client with singleton pattern
- **Cache Operations**: JSON-based caching with TTL support
- **Hash Operations**: Redis hash field operations
- **Distributed Locking**: Atomic distributed lock implementation
- **Broadcast Service**: WebSocket and HTTP long-polling support
- **Pub/Sub**: Redis publish/subscribe functionality

## Installation

```bash
go get github.com/wordgate/qtoolkit/redis
```

## Configuration

### Configuration File

Create a `redis_config.yml` file:

```yaml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
```

### Environment Variables

```bash
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD="your_password"
```


## Usage

### Basic Redis Operations

```go
import "github.com/wordgate/qtoolkit/redis"

// Get Redis client
client := redis.Client()

// Publish/Subscribe
ch := redis.Subscribe("notifications")
redis.Publish("notifications", "Hello World")
```

### Cache Operations

```go
// Basic cache operations
data := map[string]interface{}{"name": "test", "age": 25}
redis.CacheSet("user:1", data, 3600) // Cache for 1 hour

var result map[string]interface{}
exists, err := redis.CacheGet("user:1", &result)

// Hash operations
redis.CacheHSet("user:settings", "theme", "dark")
var theme string
exists, err := redis.CacheHGet("user:settings", "theme", &theme)
```

### Distributed Locking

```go
// Try to acquire lock
acquired, err := redis.TryLock("resource:1", 30) // 30 seconds TTL
if acquired {
    defer redis.ReleaseLock("resource:1")
    // Critical section
}
```

### Broadcast Service

```go
import "github.com/gin-gonic/gin"

// Create broadcast instance
broadcast := redis.NewBroadcast(10) // 10 seconds cache

// Run broadcast service (in goroutine)
go broadcast.Run()

// WebSocket subscription endpoint
router.GET("/ws/:channel", broadcast.WsSub("channel"))

// HTTP long-polling endpoint
router.GET("/sub/:channel", broadcast.HttpSub("channel"))

// Publish message
ctx := context.Background()
broadcast.Pub(ctx, "notifications", map[string]interface{}{
    "type": "user_update",
    "user_id": 123,
})

// Get metrics
router.GET("/metrics", broadcast.GetMetrics)
```

## API Reference

### Redis Client Management

- `Client() *redis.Client` - Get Redis client
- `Close() error` - Close Redis connection

### Cache Operations

- `CacheGet(key string, val interface{}) (bool, error)`
- `CacheSet(key string, value interface{}, seconds int) error`
- `CacheDel(key string) error`
- `CacheHSet(key, field string, value interface{}) error`
- `CacheHGet(key, field string, val interface{}) (bool, error)`
- `CacheHKeys(key string) ([]string, error)`

### Distributed Locking

- `TryLock(key string, expireSeconds int) (bool, error)`
- `ReleaseLock(key string) error`

### Pub/Sub

- `Subscribe(channel string) chan string`
- `Publish(channel, payload string) error`

### Broadcast Service

```go
type Broadcast struct {
    // Methods
    Pub(ctx context.Context, channel string, payload interface{}) error
    WsSubChannel(c *gin.Context, channel string) error
    WsSub(paramName string) gin.HandlerFunc
    HttpSub(paramName string) gin.HandlerFunc
    Run()
    GetMetrics(c *gin.Context)
    Delete(channel string)
}
```

## Testing

Run tests (requires Redis server):

```bash
cd redis
go test ./...
```

Skip tests if Redis is not available:

```bash
export REDIS_TEST_SKIP=1
go test ./...
```

## Configuration Reference

### Redis Configuration

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `addr` | string | Redis server address | `localhost:6379` |
| `password` | string | Redis password | `""` |
| `db` | int | Redis database number | `0` |

### Broadcast Configuration

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `cacheSecondsForLated` | int64 | Message cache duration for late subscribers | `10` |

## Architecture

This module follows the qtoolkit v1.0 modular architecture:

- **Independent Module**: Has its own `go.mod` with minimal dependencies
- **Configuration-Driven**: Can be enabled/disabled through configuration
- **Single Redis Instance**: Simple singleton pattern for Redis connection
- **KISS Principle**: Simplified architecture with single Redis instance
- **Graceful Shutdown**: Proper cleanup of connections and resources

## Dependencies

- `github.com/redis/go-redis/v9` - Redis client
- `github.com/gin-gonic/gin` - HTTP framework (for broadcast)
- `github.com/gorilla/websocket` - WebSocket support (for broadcast)
- `gopkg.in/yaml.v3` - YAML configuration parsing

## Migration from v0.x

The module maintains backward compatibility with the original qtoolkit Redis functions:

- `RedisDefault()` → `redis.Client()`
- `Redis(app)` → `redis.Client()` (simplified, no app parameter)
- `RedisSubscribe(app, channel)` → `redis.Subscribe(channel)` (simplified)
- `RedisPublish(app, channel, payload)` → `redis.Publish(channel, payload)` (simplified)
- Cache functions remain the same
- Broadcast service: `NewBroadcast(app, cache)` → `NewBroadcast(cache)` (simplified)

## License

Part of the qtoolkit project.