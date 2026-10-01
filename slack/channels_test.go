package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeReply is one canned answer of the scripted fake Slack server.
type fakeReply struct {
	Status int
	Header http.Header
	Body   string
}

// requestLog collects every request the scripted fake Slack server received.
type requestLog struct {
	mu   sync.Mutex
	reqs []recordedRequest
}

func (l *requestLog) all() []recordedRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]recordedRequest(nil), l.reqs...)
}

// fakeSlackScript points slackAPIBase at a test server that records every
// request and answers the n-th request with replies[n]. Requests beyond the
// script are answered with the last reply, so a test asserting an exact
// request count still sees the surplus requests in the log.
func fakeSlackScript(t *testing.T, replies ...fakeReply) *requestLog {
	t.Helper()
	resetState()

	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       map[string]string{},
			Auth:        r.Header.Get("Authorization"),
			ContentType: r.Header.Get("Content-Type"),
		}
		for k, v := range r.URL.Query() {
			rec.Query[k] = v[0]
		}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &rec.Body); err != nil {
				t.Errorf("request body is not JSON: %q", raw)
			}
		}

		log.mu.Lock()
		n := len(log.reqs)
		log.reqs = append(log.reqs, rec)
		log.mu.Unlock()

		reply := replies[min(n, len(replies)-1)]
		for k, v := range reply.Header {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.Status)
		io.WriteString(w, reply.Body)
	}))
	t.Cleanup(server.Close)

	old := slackAPIBase
	slackAPIBase = server.URL
	t.Cleanup(func() { slackAPIBase = old })

	SetConfig(&Config{BotToken: "xoxb-test"})
	return log
}

func ok200(body string) fakeReply { return fakeReply{Status: 200, Body: body} }

// assertPost checks method, path, bearer token and JSON content type.
func assertPost(t *testing.T, rec recordedRequest, path string) {
	t.Helper()
	if rec.Method != http.MethodPost || rec.Path != path {
		t.Errorf("request = %s %s, want POST %s", rec.Method, rec.Path, path)
	}
	if rec.Auth != "Bearer xoxb-test" {
		t.Errorf("Authorization = %q, want %q", rec.Auth, "Bearer xoxb-test")
	}
	if !strings.HasPrefix(rec.ContentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", rec.ContentType)
	}
}

func TestAPIError(t *testing.T) {
	fakeSlack(t, 200, nil, `{"ok":false,"error":"channel_not_found"}`)
	err := PinMessage(context.Background(), "C1", "1.2")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v (%T), want *APIError", err, err)
	}
	if apiErr.Method != "pins.add" || apiErr.Code != "channel_not_found" {
		t.Errorf("APIError = %+v, want Method=pins.add Code=channel_not_found", apiErr)
	}
	if !errors.Is(err, ErrAPIFailed) {
		t.Errorf("error %v does not wrap ErrAPIFailed", err)
	}
	// the message format predates APIError and must not change
	if want := "slack: API call failed: pins.add: channel_not_found"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}

	t.Run("non-Slack failures carry no error code", func(t *testing.T) {
		fakeSlack(t, http.StatusInternalServerError, nil, `oops`)
		err := PinMessage(context.Background(), "C1", "1.2")
		if errors.As(err, &apiErr) {
			t.Errorf("HTTP 500 produced an *APIError: %v", err)
		}
		if code := slackErrorCode(err); code != "" {
			t.Errorf("slackErrorCode = %q, want empty", code)
		}
	})
}

func TestCreateChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("public", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"channel":{"id":"C0NEW","name":"support-42"}}`)

		id, err := CreateChannel(ctx, "support-42", false)
		if err != nil {
			t.Fatalf("CreateChannel: %v", err)
		}
		if id != "C0NEW" {
			t.Errorf("channelID = %q, want C0NEW", id)
		}
		assertPost(t, *rec, "/conversations.create")
		if rec.Body["name"] != "support-42" {
			t.Errorf("name = %v, want support-42", rec.Body["name"])
		}
		if rec.Body["is_private"] != false {
			t.Errorf("is_private = %v, want false", rec.Body["is_private"])
		}
	})

	t.Run("private", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"channel":{"id":"G0NEW"}}`)
		if _, err := CreateChannel(ctx, "secret", true); err != nil {
			t.Fatalf("CreateChannel: %v", err)
		}
		if rec.Body["is_private"] != true {
			t.Errorf("is_private = %v, want true", rec.Body["is_private"])
		}
	})

	t.Run("name is sent verbatim", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true,"channel":{"id":"C1"}}`)
		if _, err := CreateChannel(ctx, "Not A Valid Name", false); err != nil {
			t.Fatalf("CreateChannel: %v", err)
		}
		if rec.Body["name"] != "Not A Valid Name" {
			t.Errorf("name = %v, want it unmodified", rec.Body["name"])
		}
	})

	t.Run("name_taken", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":false,"error":"name_taken"}`)
		id, err := CreateChannel(ctx, "support-42", false)
		if !errors.Is(err, ErrChannelNameTaken) {
			t.Errorf("error = %v, want ErrChannelNameTaken", err)
		}
		if !errors.Is(err, ErrAPIFailed) {
			t.Errorf("error = %v, want it to also wrap ErrAPIFailed", err)
		}
		if id != "" {
			t.Errorf("channelID = %q on error, want empty", id)
		}
	})

	t.Run("other errors are not ErrChannelNameTaken", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":false,"error":"invalid_name_specials"}`)
		_, err := CreateChannel(ctx, "bad name", false)
		if !errors.Is(err, ErrAPIFailed) || errors.Is(err, ErrChannelNameTaken) {
			t.Errorf("error = %v, want ErrAPIFailed and not ErrChannelNameTaken", err)
		}
	})

	t.Run("ok:true without channel id is an error", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":true}`)
		if _, err := CreateChannel(ctx, "x", false); !errors.Is(err, ErrAPIFailed) {
			t.Errorf("error = %v, want ErrAPIFailed", err)
		}
	})
}

func TestInviteToChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("users are comma-joined", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true,"channel":{"id":"C1"}}`))

		if err := InviteToChannel(ctx, "C1", []string{"U1", "U2", "U3"}); err != nil {
			t.Fatalf("InviteToChannel: %v", err)
		}
		reqs := log.all()
		if len(reqs) != 1 {
			t.Fatalf("requests = %d, want 1", len(reqs))
		}
		assertPost(t, reqs[0], "/conversations.invite")
		if reqs[0].Body["channel"] != "C1" || reqs[0].Body["users"] != "U1,U2,U3" {
			t.Errorf("body = %v, want channel=C1 users=U1,U2,U3", reqs[0].Body)
		}
	})

	t.Run("already_in_channel is success", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":false,"error":"already_in_channel"}`)
		if err := InviteToChannel(ctx, "C1", []string{"U1"}); err != nil {
			t.Errorf("InviteToChannel = %v, want nil", err)
		}
	})

	t.Run("no users sends no request", func(t *testing.T) {
		for _, ids := range [][]string{nil, {}} {
			log := fakeSlackScript(t, ok200(`{"ok":true}`))
			if err := InviteToChannel(ctx, "C1", ids); err != nil {
				t.Errorf("InviteToChannel(%#v) = %v, want nil", ids, err)
			}
			if n := len(log.all()); n != 0 {
				t.Errorf("InviteToChannel(%#v) sent %d requests, want 0", ids, n)
			}
		}
	})

	t.Run("1001 users go out as 1000 + 1", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true}`))
		ids := make([]string, 1001)
		for i := range ids {
			ids[i] = fmt.Sprintf("U%04d", i)
		}

		if err := InviteToChannel(ctx, "C1", ids); err != nil {
			t.Fatalf("InviteToChannel: %v", err)
		}
		reqs := log.all()
		if len(reqs) != 2 {
			t.Fatalf("requests = %d, want 2", len(reqs))
		}
		first := strings.Split(reqs[0].Body["users"].(string), ",")
		if !reflect.DeepEqual(first, ids[:1000]) {
			t.Errorf("first batch has %d ids (first %q, last %q), want ids[0:1000]", len(first), first[0], first[len(first)-1])
		}
		if got := reqs[1].Body["users"]; got != "U1000" {
			t.Errorf("second batch users = %v, want U1000", got)
		}
		for i, r := range reqs {
			if r.Body["channel"] != "C1" {
				t.Errorf("request %d channel = %v, want C1", i, r.Body["channel"])
			}
		}
	})

	t.Run("already_in_channel in one batch does not stop the next", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":false,"error":"already_in_channel"}`), ok200(`{"ok":true}`))
		if err := InviteToChannel(ctx, "C1", make([]string, 1001)); err != nil {
			t.Fatalf("InviteToChannel: %v", err)
		}
		if n := len(log.all()); n != 2 {
			t.Errorf("requests = %d, want 2", n)
		}
	})

	t.Run("a failing batch stops the remaining ones", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":false,"error":"channel_not_found"}`))
		err := InviteToChannel(ctx, "C1", make([]string, 1001))
		if !errors.Is(err, ErrAPIFailed) {
			t.Errorf("error = %v, want ErrAPIFailed", err)
		}
		if n := len(log.all()); n != 1 {
			t.Errorf("requests = %d, want 1", n)
		}
	})
}

func TestArchiveChannel(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true}`)
		if err := ArchiveChannel(ctx, "C1"); err != nil {
			t.Fatalf("ArchiveChannel: %v", err)
		}
		assertPost(t, *rec, "/conversations.archive")
		if rec.Body["channel"] != "C1" {
			t.Errorf("channel = %v, want C1", rec.Body["channel"])
		}
	})

	t.Run("already_archived is success", func(t *testing.T) {
		fakeSlack(t, 200, nil, `{"ok":false,"error":"already_archived"}`)
		if err := ArchiveChannel(ctx, "C1"); err != nil {
			t.Errorf("ArchiveChannel = %v, want nil", err)
		}
	})
}

func TestChannelMembers(t *testing.T) {
	ctx := context.Background()

	t.Run("two pages", func(t *testing.T) {
		log := fakeSlackScript(t,
			ok200(`{"ok":true,"members":["U1","U2"],"response_metadata":{"next_cursor":"dGVhbTpDMQ=="}}`),
			ok200(`{"ok":true,"members":["U3"],"response_metadata":{"next_cursor":""}}`),
		)

		members, err := ChannelMembers(ctx, "C1")
		if err != nil {
			t.Fatalf("ChannelMembers: %v", err)
		}
		if want := []string{"U1", "U2", "U3"}; !reflect.DeepEqual(members, want) {
			t.Errorf("members = %v, want %v", members, want)
		}

		reqs := log.all()
		if len(reqs) != 2 {
			t.Fatalf("requests = %d, want 2", len(reqs))
		}
		for i, r := range reqs {
			if r.Path != "/conversations.members" {
				t.Errorf("request %d path = %q, want /conversations.members", i, r.Path)
			}
			if r.Auth != "Bearer xoxb-test" {
				t.Errorf("request %d Authorization = %q", i, r.Auth)
			}
			if r.Query["channel"] != "C1" || r.Query["limit"] != "200" {
				t.Errorf("request %d query = %v, want channel=C1 limit=200", i, r.Query)
			}
		}
		if c, present := reqs[0].Query["cursor"]; present {
			t.Errorf("first request sent cursor %q", c)
		}
		if got := reqs[1].Query["cursor"]; got != "dGVhbTpDMQ==" {
			t.Errorf("second request cursor = %q, want dGVhbTpDMQ==", got)
		}
	})

	t.Run("single page without response_metadata", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true,"members":["U1"]}`))
		members, err := ChannelMembers(ctx, "C1")
		if err != nil {
			t.Fatalf("ChannelMembers: %v", err)
		}
		if !reflect.DeepEqual(members, []string{"U1"}) || len(log.all()) != 1 {
			t.Errorf("members = %v after %d requests, want [U1] after 1", members, len(log.all()))
		}
	})

	t.Run("error on a later page discards the partial result", func(t *testing.T) {
		fakeSlackScript(t,
			ok200(`{"ok":true,"members":["U1"],"response_metadata":{"next_cursor":"c2"}}`),
			ok200(`{"ok":false,"error":"invalid_cursor"}`),
		)
		members, err := ChannelMembers(ctx, "C1")
		if !errors.Is(err, ErrAPIFailed) {
			t.Errorf("error = %v, want ErrAPIFailed", err)
		}
		if members != nil {
			t.Errorf("members = %v on error, want nil", members)
		}
	})

	t.Run("a cursor that never advances is an error, not an endless loop", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true,"members":["U1"],"response_metadata":{"next_cursor":"same"}}`))
		_, err := ChannelMembers(ctx, "C1")
		if !errors.Is(err, ErrAPIFailed) {
			t.Errorf("error = %v, want ErrAPIFailed", err)
		}
		if n := len(log.all()); n != 2 {
			t.Errorf("requests = %d, want 2", n)
		}
	})
}

func TestSetChannelTopic(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true}`)
		if err := SetChannelTopic(ctx, "C1", "访客 #42 · 待接入"); err != nil {
			t.Fatalf("SetChannelTopic: %v", err)
		}
		assertPost(t, *rec, "/conversations.setTopic")
		if rec.Body["channel"] != "C1" || rec.Body["topic"] != "访客 #42 · 待接入" {
			t.Errorf("body = %v", rec.Body)
		}
	})

	t.Run("300 CJK characters are cut to 250 runes", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true}`)
		long := strings.Repeat("途", 300)
		if err := SetChannelTopic(ctx, "C1", long); err != nil {
			t.Fatalf("SetChannelTopic: %v", err)
		}
		topic, _ := rec.Body["topic"].(string)
		if n := len([]rune(topic)); n != 250 {
			t.Errorf("sent topic has %d runes, want 250", n)
		}
		if topic != strings.Repeat("途", 250) {
			t.Errorf("sent topic is not the first 250 runes of the input")
		}
	})

	t.Run("exactly 250 runes are sent unchanged", func(t *testing.T) {
		rec := fakeSlack(t, 200, nil, `{"ok":true}`)
		exact := strings.Repeat("途", 250)
		if err := SetChannelTopic(ctx, "C1", exact); err != nil {
			t.Fatalf("SetChannelTopic: %v", err)
		}
		if rec.Body["topic"] != exact {
			t.Errorf("a 250-rune topic was modified")
		}
	})
}

func TestBotUserID(t *testing.T) {
	ctx := context.Background()

	t.Run("second call is served from the cache", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true,"user_id":"U0BOT","bot_id":"B1","team_id":"T1"}`))

		for i := range 2 {
			id, err := BotUserID(ctx)
			if err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
			if id != "U0BOT" {
				t.Errorf("call %d: id = %q, want U0BOT", i, id)
			}
		}
		reqs := log.all()
		if len(reqs) != 1 {
			t.Fatalf("auth.test requests = %d, want 1", len(reqs))
		}
		if reqs[0].Path != "/auth.test" {
			t.Errorf("path = %q, want /auth.test", reqs[0].Path)
		}
		if reqs[0].Auth != "Bearer xoxb-test" {
			t.Errorf("Authorization = %q", reqs[0].Auth)
		}
	})

	t.Run("a failure is not cached", func(t *testing.T) {
		log := fakeSlackScript(t,
			ok200(`{"ok":false,"error":"invalid_auth"}`),
			ok200(`{"ok":true,"user_id":"U0BOT"}`),
		)

		id, err := BotUserID(ctx)
		if !errors.Is(err, ErrAPIFailed) || id != "" {
			t.Fatalf("first call = %q, %v; want \"\", ErrAPIFailed", id, err)
		}
		id, err = BotUserID(ctx)
		if err != nil || id != "U0BOT" {
			t.Fatalf("second call = %q, %v; want U0BOT, nil", id, err)
		}
		if n := len(log.all()); n != 2 {
			t.Errorf("auth.test requests = %d, want 2", n)
		}
		// and now it is cached
		if _, err := BotUserID(ctx); err != nil || len(log.all()) != 2 {
			t.Errorf("third call: err = %v, requests = %d; want nil, 2", err, len(log.all()))
		}
	})

	t.Run("ok:true without user_id is an error and is not cached", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true}`), ok200(`{"ok":true,"user_id":"U0BOT"}`))
		if _, err := BotUserID(ctx); !errors.Is(err, ErrAPIFailed) {
			t.Fatalf("error = %v, want ErrAPIFailed", err)
		}
		if id, err := BotUserID(ctx); err != nil || id != "U0BOT" {
			t.Fatalf("retry = %q, %v; want U0BOT, nil", id, err)
		}
		if n := len(log.all()); n != 2 {
			t.Errorf("requests = %d, want 2", n)
		}
	})

	t.Run("a different bot token does not reuse the cached id", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true,"user_id":"U0BOT"}`), ok200(`{"ok":true,"user_id":"U0OTHER"}`))
		if id, _ := BotUserID(ctx); id != "U0BOT" {
			t.Fatalf("id = %q, want U0BOT", id)
		}
		SetConfig(&Config{BotToken: "xoxb-other"})
		id, err := BotUserID(ctx)
		if err != nil || id != "U0OTHER" {
			t.Fatalf("after token change = %q, %v; want U0OTHER, nil", id, err)
		}
		if reqs := log.all(); len(reqs) != 2 || reqs[1].Auth != "Bearer xoxb-other" {
			t.Errorf("requests = %+v, want a second auth.test with the new token", reqs)
		}
	})

	t.Run("concurrent callers", func(t *testing.T) {
		log := fakeSlackScript(t, ok200(`{"ok":true,"user_id":"U0BOT"}`))
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if id, err := BotUserID(ctx); err != nil || id != "U0BOT" {
					t.Errorf("BotUserID = %q, %v", id, err)
				}
			}()
		}
		wg.Wait()
		before := len(log.all())
		if _, err := BotUserID(ctx); err != nil || len(log.all()) != before {
			t.Errorf("call after the concurrent burst hit the server again (err = %v)", err)
		}
	})
}

// channelAPICalls invokes each channel management function once, for
// cross-cutting error tests.
var channelAPICalls = map[string]func(context.Context) error{
	"CreateChannel":   func(ctx context.Context) error { _, err := CreateChannel(ctx, "n", false); return err },
	"InviteToChannel": func(ctx context.Context) error { return InviteToChannel(ctx, "C1", []string{"U1"}) },
	"ArchiveChannel":  func(ctx context.Context) error { return ArchiveChannel(ctx, "C1") },
	"ChannelMembers":  func(ctx context.Context) error { _, err := ChannelMembers(ctx, "C1"); return err },
	"SetChannelTopic": func(ctx context.Context) error { return SetChannelTopic(ctx, "C1", "t") },
	"BotUserID":       func(ctx context.Context) error { _, err := BotUserID(ctx); return err },
}

func TestChannelAPI_OkFalseIsError(t *testing.T) {
	for name, call := range channelAPICalls {
		t.Run(name, func(t *testing.T) {
			fakeSlack(t, 200, nil, `{"ok":false,"error":"missing_scope"}`)
			err := call(context.Background())
			if !errors.Is(err, ErrAPIFailed) {
				t.Fatalf("error = %v, want ErrAPIFailed", err)
			}
			if code := slackErrorCode(err); code != "missing_scope" {
				t.Errorf("slackErrorCode = %q, want missing_scope (error: %v)", code, err)
			}
		})
	}
}

func TestChannelAPI_RateLimited(t *testing.T) {
	for name, call := range channelAPICalls {
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
		})
	}
}

func TestChannelAPI_NoBotToken(t *testing.T) {
	for name, call := range channelAPICalls {
		t.Run(name, func(t *testing.T) {
			rec := fakeSlack(t, 200, nil, `{"ok":true}`)
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
