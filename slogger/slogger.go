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
	root := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(contextHandler{root: root, cur: root})
}

// contextHandler adds a top-level request_id to records logged with a context
// that carries one (see Middleware), even on loggers derived with WithGroup.
//
// root is the bare JSON handler; cur is root with the logger's With/WithGroup
// calls applied (ops replays them). Records without a request ID go straight to
// cur. For records with one, request_id is added to root first and the calls
// are replayed on top, so it stays outside any group.
type contextHandler struct {
	root slog.Handler
	cur  slog.Handler
	ops  []func(slog.Handler) slog.Handler
}

func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.cur.Enabled(ctx, level)
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	id := RequestID(ctx)
	if id == "" {
		return h.cur.Handle(ctx, r)
	}
	next := h.root.WithAttrs([]slog.Attr{slog.String("request_id", id)})
	for _, op := range h.ops {
		next = op(next)
	}
	return next.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.with(func(next slog.Handler) slog.Handler { return next.WithAttrs(attrs) })
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return h.with(func(next slog.Handler) slog.Handler { return next.WithGroup(name) })
}

func (h contextHandler) with(op func(slog.Handler) slog.Handler) contextHandler {
	return contextHandler{
		root: h.root,
		cur:  op(h.cur),
		ops:  append(h.ops[:len(h.ops):len(h.ops)], op),
	}
}
