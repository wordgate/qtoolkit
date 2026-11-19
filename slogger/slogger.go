// Package slogger returns a native *slog.Logger (standard library log/slog)
// configured from viper (slog.*), plus a gin middleware that writes one log
// line per request and carries a request ID in the request context.
//
// Usage:
//
//	logger := slogger.Get() // JSON to stdout, level from slog.level
//	slog.SetDefault(logger) // optional
//
//	r := gin.New()
//	r.Use(slogger.Middleware())
//
//	r.GET("/x", func(c *gin.Context) {
//		// Any *Context call on the logger gets request_id from the context.
//		logger.InfoContext(c.Request.Context(), "did x")
//	})
//
// The logger writes no personal data on its own. The middleware logs only the
// method, the route template (never the raw URL or query), status, response
// size and latency: no bodies, headers, client IP or user agent.
package slogger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/spf13/viper"
)

var (
	globalLogger *slog.Logger
	initOnce     sync.Once
)

// Get returns the shared logger, created from viper config on the first call.
// It writes JSON to stdout at slog.level (debug, info, warn or error; default
// info). It panics if slog.level is set to anything else, so a typo is caught
// at startup instead of silently changing what gets logged.
func Get() *slog.Logger {
	initOnce.Do(func() {
		level, err := levelFromViper()
		if err != nil {
			panic("slogger: " + err.Error())
		}
		globalLogger = newLogger(os.Stdout, level)
	})
	return globalLogger
}

func levelFromViper() (slog.Level, error) {
	var level slog.Level
	s := viper.GetString("slog.level")
	if s == "" {
		return slog.LevelInfo, nil
	}
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("invalid slog.level %q (want debug, info, warn or error)", s)
	}
	return level, nil
}

func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(contextHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

// contextHandler adds request_id to records logged with a context that carries
// one (see Middleware).
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
