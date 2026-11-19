# slogger

Returns a native `*slog.Logger` (standard library `log/slog`, JSON to stdout,
level from config), plus a gin middleware that logs one line per request and
carries a request ID in the request context.

The package is not named `slog` so it can be imported next to `log/slog`
without an alias.

```bash
go get github.com/wordgate/qtoolkit/slogger
```

## Configuration

See [`slogger_config.yml`](slogger_config.yml).

| Key | Default | |
|---|---|---|
| `slog.level` | `info` | `debug`, `info`, `warn` or `error`; anything else panics in `Get` |

## Usage

```go
logger := slogger.Get()
slog.SetDefault(logger) // optional

r := gin.New()
r.Use(gin.Recovery(), slogger.Middleware())

r.GET("/users/:id", func(c *gin.Context) {
	ctx := c.Request.Context()
	logger.InfoContext(ctx, "loading user") // gets "request_id" automatically
	id := slogger.RequestID(ctx)           // e.g. to pass to a river job
	_ = id
})
```

Every record logged through the logger with a context from the middleware
(`InfoContext`, `LogAttrs`, ...) carries `request_id`.

## Request log

One `"request"` line per request, after the handler:

```json
{"time":"...","level":"INFO","msg":"request","method":"POST","route":"/users/:id","status":201,"bytes":2,"latency_ms":0.41,"request_id":"9f86d081884c7d659a2feaa0c55ad015"}
```

- Level: `ERROR` for 5xx, `WARN` for 4xx, `INFO` otherwise.
- `route` is the gin route template, empty for unmatched routes.
- The request ID is taken from an inbound `X-Request-ID` when it is 1–64
  characters of `[A-Za-z0-9._-]`, otherwise generated (128-bit random hex). It
  is always echoed in the `X-Request-ID` response header.

**Never logged:** the raw path and query string, request and response bodies,
headers, client IP, user agent. They can carry personal data or tokens. Log
what you need yourself, deliberately, with `InfoContext`.
