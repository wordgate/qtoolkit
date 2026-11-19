package slogger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestIDHeader is read from the request (when valid) and always set on the
// response.
const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// RequestID returns the request ID stored in ctx by Middleware, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// Middleware returns a gin middleware that:
//
//   - takes the request ID from the X-Request-ID header when it is 1–64
//     characters of [A-Za-z0-9._-] (so a proxy's ID is kept), and otherwise
//     generates a random one;
//   - stores it in c.Request.Context() (read it with RequestID) and echoes it
//     in the X-Request-ID response header;
//   - after the handler, logs one "request" line with the shared logger
//     (see Get): method, route, status, bytes, latency_ms and request_id.
//     Status 5xx logs at error, 4xx at warn, anything else at info.
//
// route is the gin route template (c.FullPath(), e.g. /users/:id), or empty
// for unmatched routes. The raw path, query string, bodies, headers, client
// IP and user agent are never logged: they can carry personal data or tokens.
func Middleware() gin.HandlerFunc {
	logger := Get()
	return func(c *gin.Context) {
		start := time.Now()

		id := c.GetHeader(RequestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
		}
		ctx := context.WithValue(c.Request.Context(), requestIDKey{}, id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(RequestIDHeader, id)

		c.Next()

		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}
		logger.LogAttrs(ctx, level, "request",
			slog.String("method", c.Request.Method),
			slog.String("route", c.FullPath()),
			slog.Int("status", status),
			slog.Int("bytes", max(c.Writer.Size(), 0)),
			slog.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
		)
	}
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		b := id[i]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '.' || b == '_' || b == '-') {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return hex.EncodeToString(b[:])
}
