package slogger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func init() { gin.SetMode(gin.TestMode) }

func reset(t *testing.T) {
	clear := func() {
		globalLogger, initOnce = nil, sync.Once{}
		viper.Reset()
	}
	clear()
	t.Cleanup(clear)
}

// useBuffer makes Get return a logger that writes to the returned buffer.
func useBuffer(t *testing.T, level slog.Level) *bytes.Buffer {
	reset(t)
	var buf bytes.Buffer
	initOnce.Do(func() { globalLogger = newLogger(&buf, level) })
	return &buf
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %q", l)
		}
		out = append(out, m)
	}
	return out
}

func TestLevelFromViper(t *testing.T) {
	cases := map[string]slog.Level{"": slog.LevelInfo, "debug": slog.LevelDebug, "WARN": slog.LevelWarn, "error": slog.LevelError}
	for in, want := range cases {
		reset(t)
		viper.Set("slog.level", in)
		got, err := levelFromViper()
		if err != nil || got != want {
			t.Fatalf("%q: got %v, %v; want %v", in, got, err, want)
		}
	}

	reset(t)
	viper.Set("slog.level", "verbose")
	defer func() {
		if recover() == nil {
			t.Fatal("want panic for invalid level")
		}
	}()
	Get()
}

func TestGetIsShared(t *testing.T) {
	reset(t)
	if Get() == nil || Get() != Get() {
		t.Fatal("Get must return one shared logger")
	}
}

func TestLevelFiltering(t *testing.T) {
	buf := useBuffer(t, slog.LevelWarn)
	Get().Info("hidden")
	Get().Warn("shown")
	got := lines(t, buf)
	if len(got) != 1 || got[0]["msg"] != "shown" {
		t.Fatalf("unexpected output: %v", got)
	}
}

func TestMiddleware(t *testing.T) {
	buf := useBuffer(t, slog.LevelInfo)
	r := gin.New()
	r.Use(Middleware())
	var handlerID string
	r.POST("/users/:id", func(c *gin.Context) {
		handlerID = RequestID(c.Request.Context())
		Get().InfoContext(c.Request.Context(), "inside")
		c.String(http.StatusCreated, "ok")
	})
	r.GET("/fail", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	// Generated ID; raw path, query and body never logged.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/alice@example.com?token=s3cret", strings.NewReader(`{"name":"child"}`))
	req.Header.Set("User-Agent", "secret-agent")
	r.ServeHTTP(w, req)

	id := w.Header().Get(RequestIDHeader)
	if len(id) != 32 || id != handlerID {
		t.Fatalf("request id: header %q, handler %q", id, handlerID)
	}
	out := buf.String()
	for _, leak := range []string{"alice", "s3cret", "child", "secret-agent", "192.0.2"} {
		if strings.Contains(out, leak) {
			t.Fatalf("log contains %q: %s", leak, out)
		}
	}
	got := lines(t, buf)
	if len(got) != 2 {
		t.Fatalf("want 2 lines, got %v", got)
	}
	if got[0]["msg"] != "inside" || got[0]["request_id"] != id {
		t.Fatalf("handler log lacks request_id: %v", got[0])
	}
	l := got[1]
	if l["msg"] != "request" || l["level"] != "INFO" || l["method"] != "POST" || l["route"] != "/users/:id" ||
		l["status"] != float64(201) || l["bytes"] != float64(2) || l["request_id"] != id {
		t.Fatalf("unexpected request line: %v", l)
	}
	if _, ok := l["latency_ms"]; !ok {
		t.Fatalf("missing latency_ms: %v", l)
	}

	// Valid inbound ID is kept; 5xx logs at error.
	buf.Reset()
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/fail", nil)
	req.Header.Set(RequestIDHeader, "edge-123.abc_DEF")
	r.ServeHTTP(w, req)
	if w.Header().Get(RequestIDHeader) != "edge-123.abc_DEF" {
		t.Fatalf("inbound id not kept: %q", w.Header().Get(RequestIDHeader))
	}
	if l := lines(t, buf)[0]; l["level"] != "ERROR" || l["request_id"] != "edge-123.abc_DEF" {
		t.Fatalf("unexpected 5xx line: %v", l)
	}

	// Unmatched route: 404 at warn with an empty route.
	buf.Reset()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope/bob", nil))
	if l := lines(t, buf)[0]; l["level"] != "WARN" || l["route"] != "" || strings.Contains(buf.String(), "bob") {
		t.Fatalf("unexpected 404 line: %v", l)
	}
}

func TestPanicLoggedWithRecoveryAfter(t *testing.T) {
	buf := useBuffer(t, slog.LevelInfo)
	r := gin.New()
	r.Use(Middleware(), gin.RecoveryWithWriter(io.Discard))
	r.GET("/boom", func(c *gin.Context) { panic("boom") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))
	got := lines(t, buf)
	if w.Code != 500 || len(got) != 1 || got[0]["level"] != "ERROR" || got[0]["status"] != float64(500) {
		t.Fatalf("want one ERROR request line, got %d %v", w.Code, got)
	}
}

func TestInvalidInboundRequestID(t *testing.T) {
	for _, bad := range []string{"has space", "new\nline", `quote"`, strings.Repeat("a", 65)} {
		if validRequestID(bad) {
			t.Fatalf("%q must be rejected", bad)
		}
	}
	if !validRequestID(strings.Repeat("a", 64)) {
		t.Fatal("64 chars must be accepted")
	}

	buf := useBuffer(t, slog.LevelInfo)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/", func(c *gin.Context) {})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "evil value")
	r.ServeHTTP(w, req)
	if got := w.Header().Get(RequestIDHeader); got == "evil value" || len(got) != 32 {
		t.Fatalf("invalid inbound id not replaced: %q", got)
	}
	if strings.Contains(buf.String(), "evil") {
		t.Fatalf("invalid id logged: %s", buf.String())
	}
}

func TestRequestIDWithoutMiddleware(t *testing.T) {
	if RequestID(context.Background()) != "" {
		t.Fatal("want empty id")
	}
}

func TestRequestIDStaysTopLevel(t *testing.T) {
	buf := useBuffer(t, slog.LevelInfo)
	ctx := context.WithValue(context.Background(), requestIDKey{}, "rid")

	base := Get().With("k", "v")
	grouped := base.WithGroup("g").With("inner", 1)
	grouped.InfoContext(ctx, "m", "x", 1)
	base.InfoContext(ctx, "plain")
	grouped.Info("no ctx")

	got := lines(t, buf)
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %v", got)
	}
	g, _ := got[0]["g"].(map[string]any)
	if got[0]["request_id"] != "rid" || got[0]["k"] != "v" || g["inner"] != float64(1) || g["x"] != float64(1) {
		t.Fatalf("grouped line: %v", got[0])
	}
	if _, nested := g["request_id"]; nested {
		t.Fatalf("request_id nested in group: %v", got[0])
	}
	if got[1]["request_id"] != "rid" || got[1]["k"] != "v" {
		t.Fatalf("plain line: %v", got[1])
	}
	if _, ok := got[2]["request_id"]; ok {
		t.Fatalf("request_id without context: %v", got[2])
	}
	// Derived loggers must not share op slices.
	a, b := base.WithGroup("a"), base.WithGroup("b")
	buf.Reset()
	a.InfoContext(ctx, "A", "x", 1)
	b.InfoContext(ctx, "B", "x", 1)
	got = lines(t, buf)
	if _, ok := got[0]["a"]; !ok {
		t.Fatalf("logger a lost its group: %v", got[0])
	}
	if _, ok := got[1]["b"]; !ok {
		t.Fatalf("logger b lost its group: %v", got[1])
	}
}
