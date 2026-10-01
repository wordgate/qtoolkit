package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// recordedRequest is what the fake Slack server saw.
type recordedRequest struct {
	Method      string
	Path        string
	Query       map[string]string
	Auth        string
	ContentType string
	Body        map[string]any
}

// fakeSlack points slackAPIBase at a test server that records the request and
// answers with the given status and JSON body.
func fakeSlack(t *testing.T, status int, header http.Header, response string) *recordedRequest {
	t.Helper()
	resetState()

	rec := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.Method = r.Method
		rec.Path = r.URL.Path
		rec.Query = map[string]string{}
		for k, v := range r.URL.Query() {
			rec.Query[k] = v[0]
		}
		rec.Auth = r.Header.Get("Authorization")
		rec.ContentType = r.Header.Get("Content-Type")
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &rec.Body); err != nil {
				t.Errorf("request body is not JSON: %q", raw)
			}
		}
		for k, v := range header {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)

	old := slackAPIBase
	slackAPIBase = server.URL
	t.Cleanup(func() { slackAPIBase = old })

	SetConfig(&Config{BotToken: "xoxb-test"})
	return rec
}

func TestPostMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("top-level message", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"channel":"C123","ts":"1700000000.000100"}`)

		ts, err := PostMessage(ctx, "C123", "hello", nil)
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		if ts != "1700000000.000100" {
			t.Errorf("ts = %q, want 1700000000.000100", ts)
		}
		if rec.Method != http.MethodPost || rec.Path != "/chat.postMessage" {
			t.Errorf("request = %s %s, want POST /chat.postMessage", rec.Method, rec.Path)
		}
		if rec.Auth != "Bearer xoxb-test" {
			t.Errorf("Authorization = %q, want %q", rec.Auth, "Bearer xoxb-test")
		}
		if !strings.HasPrefix(rec.ContentType, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", rec.ContentType)
		}
		if rec.Body["channel"] != "C123" || rec.Body["text"] != "hello" {
			t.Errorf("body = %v", rec.Body)
		}
		for _, k := range []string{"thread_ts", "reply_broadcast"} {
			if _, present := rec.Body[k]; present {
				t.Errorf("body unexpectedly contains %q for a top-level message: %v", k, rec.Body)
			}
		}
	})

	t.Run("thread reply", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"ts":"1700000001.000200"}`)

		ts, err := PostMessage(ctx, "C123", "reply", &PostOptions{ThreadTS: "1700000000.000100"})
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		if ts != "1700000001.000200" {
			t.Errorf("ts = %q", ts)
		}
		if rec.Body["thread_ts"] != "1700000000.000100" {
			t.Errorf("thread_ts = %v, want 1700000000.000100", rec.Body["thread_ts"])
		}
		if _, present := rec.Body["reply_broadcast"]; present {
			t.Errorf("reply_broadcast sent although not requested: %v", rec.Body)
		}
	})

	t.Run("thread reply broadcast to channel", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"ts":"1700000002.000300"}`)

		_, err := PostMessage(ctx, "C123", "reply", &PostOptions{ThreadTS: "1700000000.000100", ReplyBroadcast: true})
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		if rec.Body["thread_ts"] != "1700000000.000100" {
			t.Errorf("thread_ts = %v", rec.Body["thread_ts"])
		}
		if rec.Body["reply_broadcast"] != true {
			t.Errorf("reply_broadcast = %v, want true", rec.Body["reply_broadcast"])
		}
	})

	t.Run("long text is truncated", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"ts":"1.2"}`)

		if _, err := PostMessage(ctx, "C123", strings.Repeat("a", MaxTextLength+500), nil); err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		text, _ := rec.Body["text"].(string)
		if n := len([]rune(text)); n != MaxTextLength {
			t.Errorf("sent text length = %d, want %d", n, MaxTextLength)
		}
	})

	t.Run("ok:true without ts is an error", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":true}`)
		if _, err := PostMessage(ctx, "C123", "hello", nil); !errors.Is(err, ErrAPIFailed) {
			t.Errorf("error = %v, want ErrAPIFailed", err)
		}
	})
}

func TestUpdateMessage(t *testing.T) {
	rec := fakeSlack(t, 200, nil, `{"ok":true,"ts":"1700000000.000100"}`)

	if err := UpdateMessage(context.Background(), "C123", "1700000000.000100", "edited"); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}
	if rec.Method != http.MethodPost || rec.Path != "/chat.update" {
		t.Errorf("request = %s %s, want POST /chat.update", rec.Method, rec.Path)
	}
	if rec.Auth != "Bearer xoxb-test" {
		t.Errorf("Authorization = %q", rec.Auth)
	}
	want := map[string]any{"channel": "C123", "ts": "1700000000.000100", "text": "edited"}
	for k, v := range want {
		if rec.Body[k] != v {
			t.Errorf("body[%q] = %v, want %v", k, rec.Body[k], v)
		}
	}
}

func TestGetPermalink(t *testing.T) {
	// The JSON escape & must decode back to a literal '&'.
	rec := fakeSlack(t, 200, nil,
		`{"ok":true,"permalink":"https://x.slack.com/archives/C123/p1700000000000100?thread_ts=1&cid=C123"}`)

	link, err := GetPermalink(context.Background(), "C123", "1700000000.000100")
	if err != nil {
		t.Fatalf("GetPermalink: %v", err)
	}
	if want := "https://x.slack.com/archives/C123/p1700000000000100?thread_ts=1&cid=C123"; link != want {
		t.Errorf("permalink = %q, want %q", link, want)
	}
	if rec.Path != "/chat.getPermalink" {
		t.Errorf("path = %q, want /chat.getPermalink", rec.Path)
	}
	if rec.Auth != "Bearer xoxb-test" {
		t.Errorf("Authorization = %q", rec.Auth)
	}
	if rec.Query["channel"] != "C123" || rec.Query["message_ts"] != "1700000000.000100" {
		t.Errorf("query = %v, want channel=C123 message_ts=1700000000.000100", rec.Query)
	}
}

func TestPinMessage(t *testing.T) {
	rec := fakeSlack(t, 200, nil, `{"ok":true}`)

	if err := PinMessage(context.Background(), "C123", "1700000000.000100"); err != nil {
		t.Fatalf("PinMessage: %v", err)
	}
	if rec.Method != http.MethodPost || rec.Path != "/pins.add" {
		t.Errorf("request = %s %s, want POST /pins.add", rec.Method, rec.Path)
	}
	if rec.Auth != "Bearer xoxb-test" {
		t.Errorf("Authorization = %q", rec.Auth)
	}
	if rec.Body["channel"] != "C123" || rec.Body["timestamp"] != "1700000000.000100" {
		t.Errorf("body = %v, want channel=C123 timestamp=1700000000.000100", rec.Body)
	}
}

func TestUserEmail(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"user":{"id":"U1","profile":{"email":"agent@example.com"}}}`)

		email, err := UserEmail(context.Background(), "U1")
		if err != nil {
			t.Fatalf("UserEmail: %v", err)
		}
		if email != "agent@example.com" {
			t.Errorf("email = %q", email)
		}
		if rec.Path != "/users.info" || rec.Query["user"] != "U1" {
			t.Errorf("request = %s ?%v, want /users.info ?user=U1", rec.Path, rec.Query)
		}
		if rec.Auth != "Bearer xoxb-test" {
			t.Errorf("Authorization = %q", rec.Auth)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":false,"error":"user_not_found"}`)
		_, err := UserEmail(context.Background(), "U404")
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("profile without email is an error, not an empty string", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":true,"user":{"id":"B1","profile":{}}}`)
		email, err := UserEmail(context.Background(), "B1")
		if err == nil {
			t.Fatalf("UserEmail = %q, nil; want error", email)
		}
	})
}

// call invokes each new bot API function once, for cross-cutting error tests.
var botAPICalls = map[string]func(context.Context) error{
	"PostMessage":   func(ctx context.Context) error { _, err := PostMessage(ctx, "C1", "hi", nil); return err },
	"UpdateMessage": func(ctx context.Context) error { return UpdateMessage(ctx, "C1", "1.2", "hi") },
	"GetPermalink":  func(ctx context.Context) error { _, err := GetPermalink(ctx, "C1", "1.2"); return err },
	"PinMessage":    func(ctx context.Context) error { return PinMessage(ctx, "C1", "1.2") },
	"UserEmail":     func(ctx context.Context) error { _, err := UserEmail(ctx, "U1"); return err },
}

func TestBotAPI_OkFalseIsError(t *testing.T) {
	for name, call := range botAPICalls {
		t.Run(name, func(t *testing.T) {
			fakeSlack(t, 200, nil, `{"ok":false,"error":"channel_not_found"}`)
			err := call(context.Background())
			if !errors.Is(err, ErrAPIFailed) {
				t.Fatalf("error = %v, want ErrAPIFailed", err)
			}
			if !strings.Contains(err.Error(), "channel_not_found") {
				t.Errorf("error %q does not carry Slack's error code", err)
			}
		})
	}
}

func TestBotAPI_RateLimited(t *testing.T) {
	for name, call := range botAPICalls {
		t.Run(name, func(t *testing.T) {
			fakeSlack(t, http.StatusTooManyRequests, http.Header{"Retry-After": []string{"30"}},
				`{"ok":false,"error":"ratelimited"}`)
			err := call(context.Background())
			var rl *RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("error = %v (%T), want *RateLimitError", err, err)
			}
			if rl.RetryAfter != 30*time.Second {
				t.Errorf("RetryAfter = %v, want 30s", rl.RetryAfter)
			}
			if !strings.Contains(rl.Error(), "30s") {
				t.Errorf("Error() = %q, want it to mention the retry delay", rl.Error())
			}
		})
	}

	t.Run("missing Retry-After header", func(t *testing.T) {
		fakeSlack(t, http.StatusTooManyRequests, nil, ``)
		_, err := PostMessage(context.Background(), "C1", "hi", nil)
		var rl *RateLimitError
		if !errors.As(err, &rl) {
			t.Fatalf("error = %v, want *RateLimitError", err)
		}
		if rl.RetryAfter != 0 {
			t.Errorf("RetryAfter = %v, want 0", rl.RetryAfter)
		}
	})
}

func TestBotAPI_NoBotToken(t *testing.T) {
	for name, call := range botAPICalls {
		t.Run(name, func(t *testing.T) {
			rec := fakeSlack(t, 200, nil, `{"ok":true,"ts":"1.2"}`)
			SetConfig(&Config{})
			if err := call(context.Background()); !errors.Is(err, ErrNoBotToken) {
				t.Errorf("error = %v, want ErrNoBotToken", err)
			}
			if rec.Path != "" {
				t.Errorf("request was sent to %s without a token", rec.Path)
			}
		})
	}
}

func TestBotAPI_ServerErrorAndBadJSON(t *testing.T) {
	t.Run("http 500", func(t *testing.T) {
		fakeSlack(t, http.StatusInternalServerError, nil, `oops`)
		_, err := PostMessage(context.Background(), "C1", "hi", nil)
		if !errors.Is(err, ErrAPIFailed) || !strings.Contains(err.Error(), "500") {
			t.Errorf("error = %v, want ErrAPIFailed mentioning status 500", err)
		}
	})
	t.Run("non-JSON 200", func(t *testing.T) {
		fakeSlack(t, 200, nil, `<html>`)
		if err := PinMessage(context.Background(), "C1", "1.2"); !errors.Is(err, ErrAPIFailed) {
			t.Errorf("error = %v, want ErrAPIFailed", err)
		}
	})
}

func TestBotAPI_ContextCancelled(t *testing.T) {
	fakeSlack(t, 200, nil, `{"ok":true,"ts":"1.2"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PostMessage(ctx, "C1", "hi", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

// sign computes a v0 Slack signature independently of the code under test.
func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	const secret = "test-signing-secret"
	now := time.Unix(1700000000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"event_callback","event":{"type":"message","text":"hi"}}`)

	headers := func(timestamp, signature string) http.Header {
		h := http.Header{}
		if timestamp != "" {
			h.Set("X-Slack-Request-Timestamp", timestamp)
		}
		if signature != "" {
			h.Set("X-Slack-Signature", signature)
		}
		return h
	}
	tsAt := func(d time.Duration) string { return strconv.FormatInt(now.Add(d).Unix(), 10) }

	valid := sign(secret, ts, body)
	lastHex := valid[len(valid)-1]
	flipped := valid[:len(valid)-1] + map[bool]string{true: "1", false: "0"}[lastHex == '0']

	tests := []struct {
		name    string
		secret  string
		header  http.Header
		body    []byte
		wantErr bool
	}{
		{"valid", secret, headers(ts, valid), body, false},
		{"valid, 4m59s old", secret, headers(tsAt(-299*time.Second), sign(secret, tsAt(-299*time.Second), body)), body, false},
		{"valid, exactly 5m old", secret, headers(tsAt(-5*time.Minute), sign(secret, tsAt(-5*time.Minute), body)), body, false},
		{"valid, 4m59s in the future (clock skew)", secret, headers(tsAt(299*time.Second), sign(secret, tsAt(299*time.Second), body)), body, false},
		{"correctly signed but 5m01s old", secret, headers(tsAt(-301*time.Second), sign(secret, tsAt(-301*time.Second), body)), body, true},
		{"correctly signed but 5m01s in the future", secret, headers(tsAt(301*time.Second), sign(secret, tsAt(301*time.Second), body)), body, true},
		{"signature one hex digit off", secret, headers(ts, flipped), body, true},
		{"signed with another secret", secret, headers(ts, sign("other-secret", ts, body)), body, true},
		{"body tampered", secret, headers(ts, valid), append([]byte("x"), body...), true},
		{"timestamp swapped (replay with fresh timestamp)", secret, headers(tsAt(-10*time.Second), valid), body, true},
		{"missing signature header", secret, headers(ts, ""), body, true},
		{"missing timestamp header", secret, headers("", valid), body, true},
		{"no headers", secret, http.Header{}, body, true},
		{"non-numeric timestamp", secret, headers("yesterday", sign(secret, "yesterday", body)), body, true},
		{"missing v0= prefix", secret, headers(ts, strings.TrimPrefix(valid, "v0=")), body, true},
		{"wrong version prefix", secret, headers(ts, "v1="+strings.TrimPrefix(valid, "v0=")), body, true},
		{"non-hex signature", secret, headers(ts, "v0=zzzz"), body, true},
		{"empty signing secret is rejected even with a matching HMAC", "", headers(ts, sign("", ts, body)), body, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySignature(tt.secret, tt.header, tt.body, now)
			if tt.wantErr {
				if err == nil {
					t.Fatal("VerifySignature = nil, want error")
				}
				if !errors.Is(err, ErrInvalidSignature) {
					t.Errorf("error = %v, want it to wrap ErrInvalidSignature", err)
				}
			} else if err != nil {
				t.Fatalf("VerifySignature = %v, want nil", err)
			}
		})
	}
}

// Example vector from Slack's "Verifying requests from Slack" documentation
// (https://docs.slack.dev/authentication/verifying-requests-from-slack),
// copied verbatim: the expected signature is Slack's, not computed here.
func TestVerifySignature_SlackDocsVector(t *testing.T) {
	const (
		secret    = "8f742231b10e8888abcd99yyyzzz85a5"
		timestamp = "1531420618"
		body      = "token=xyzz0WbapA4vBCDEFasx0q6G&team_id=T1DC2JH3J&team_domain=testteamnow&channel_id=G8PSS9T3V&channel_name=foobar&user_id=U2CERLKJA&user_name=roadrunner&command=%2Fwebhook-collect&text=&response_url=https%3A%2F%2Fhooks.slack.com%2Fcommands%2FT1DC2JH3J%2F397700885554%2F96rGlfmibIGlgcZRskXaIFfN&trigger_id=398738663015.47445629121.803a0bc887a14d10d2c447fce8b6703c"
		signature = "v0=a2114d57b48eac39b9ad189dd8316235a7b4a8d21a10bd27519666489c69b503"
	)
	h := http.Header{}
	h.Set("X-Slack-Request-Timestamp", timestamp)
	h.Set("X-Slack-Signature", signature)

	if err := VerifySignature(secret, h, []byte(body), time.Unix(1531420618, 0)); err != nil {
		t.Fatalf("VerifySignature rejected Slack's documented example: %v", err)
	}
	// the same request replayed an hour later must be rejected
	if err := VerifySignature(secret, h, []byte(body), time.Unix(1531420618+3600, 0)); err == nil {
		t.Fatal("stale replay of the documented example was accepted")
	}
}
