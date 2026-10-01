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

#### Namespaces, slow consumers, shutdown

Redis pub/sub ignores the `db` number, and `NewBroadcast` always uses the key
`broadcast` — every service using it on the same Redis sees every other
service's messages. New services should use a namespace:

```go
// Pub/sub key "broadcast:chat", late-message cache keys "broadcast:chat/<channel>".
// Instances sharing a namespace (multi-node deployments) see each other;
// different namespaces (and legacy NewBroadcast users) are isolated.
broadcast := redis.NewNamedBroadcast("chat", 30,
    redis.WithSubscriberBuffer(64), // per-subscriber buffer, default 64
    redis.WithCheckOrigin(func(r *http.Request) bool { // default: allow all
        return r.Header.Get("Origin") == "https://example.com"
    }),
)

ctx, cancel := context.WithCancel(context.Background())
go broadcast.RunContext(ctx) // returns ctx.Err() once ctx is cancelled
defer cancel()

broadcast.SubscriberCount("room-42") // subscribers on THIS instance only
```

Delivery rules (both constructors):

- Each subscriber has its own buffered queue and the `Run` loop never blocks.
  A subscriber whose queue is full is a slow consumer: it is unsubscribed, its
  WebSocket is closed (close code 1013 when possible) and `messages_dropped`
  is incremented. Clients are expected to reconnect and resync.
- `WsSubChannel` reads from the client (frames are discarded) so a closed or
  dead peer is detected: pings go out every 30s, and a connection that sends
  no pong/frame for 70s is dropped. Writes time out after 10s.
- `Delete(channel)` closes that channel's WebSockets with a normal close frame.

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
    RunContext(ctx context.Context) error
    SubscriberCount(channel string) int
    GetMetrics(c *gin.Context)
    Delete(channel string)
}
```

## Testing

Broadcast tests (`broadcast_test.go`) run against an in-process
[miniredis](https://github.com/alicebob/miniredis) and never skip. The other
tests need a Redis server on `localhost:6379` and skip without one:

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
| `namespace` (`NewNamedBroadcast`) | string | Pub/sub key suffix; must be non-empty | - |
| `WithSubscriberBuffer(n)` | int | Per-subscriber queue size before a slow consumer is kicked | `64` |
| `WithCheckOrigin(fn)` | func | WebSocket origin check | allow all |

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