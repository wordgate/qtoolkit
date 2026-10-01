package redis

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	goredis "github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// These tests run against an in-process miniredis: no external Redis, never skipped.

// useMiniredis starts a miniredis and points the package singleton client at it.
func useMiniredis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)

	clientOnce = sync.Once{}
	defaultClient = nil
	viper.Set("redis.addr", mr.Addr())
	viper.Set("redis.password", "")
	viper.Set("redis.db", 0)

	t.Cleanup(func() {
		if defaultClient != nil {
			defaultClient.Close()
		}
		clientOnce = sync.Once{}
		defaultClient = nil
	})
	return mr
}

// ownClient gives b its own connection pool, simulating a separate process/machine.
func ownClient(t *testing.T, b *Broadcast, mr *miniredis.Miniredis) {
	t.Helper()
	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { c.Close() })
	b.rds = c
}

func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for: %s", timeout, what)
}

// startRun runs b.RunContext in the background and waits until its Redis
// subscription is live, so that a following Pub cannot be lost.
func startRun(t *testing.T, b *Broadcast) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	numSub := func() int64 {
		m, err := b.rds.PubSubNumSub(context.Background(), b.broadcastKey()).Result()
		if err != nil {
			t.Fatalf("PubSubNumSub: %v", err)
		}
		return m[b.broadcastKey()]
	}
	before := numSub()

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.RunContext(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("RunContext did not return after cancel")
		}
	})
	eventually(t, 5*time.Second, "redis subscription established", func() bool {
		return numSub() > before
	})
}

// wsServer serves b.WsSub at /ws/:channel and b.HttpSub at /sub/:channel.
func wsServer(t *testing.T, b *Broadcast) (wsURL func(channel string) string, httpURL string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws/:channel", b.WsSub("channel"))
	r.GET("/sub/:channel", b.HttpSub("channel"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return func(channel string) string {
		return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/" + channel
	}, srv.URL
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	resp.Body.Close()
	t.Cleanup(func() { conn.Close() })
	return conn
}

func recvOrTimeout(t *testing.T, ch <-chan *BroadcastMessage, timeout time.Duration) *BroadcastMessage {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(timeout):
		t.Fatalf("no message within %v", timeout)
		return nil
	}
}

func TestBroadcast_WebSocketEndToEnd(t *testing.T) {
	useMiniredis(t)
	b := NewNamedBroadcast("e2e", 10)
	startRun(t, b)
	wsURL, _ := wsServer(t, b)

	conn := dial(t, wsURL("room-1"))
	eventually(t, 2*time.Second, "subscriber registered", func() bool {
		return b.SubscriberCount("room-1") == 1
	})
	if n := b.SubscriberCount("other-room"); n != 0 {
		t.Errorf("SubscriberCount(other-room) = %d, want 0", n)
	}

	if err := b.Pub(context.Background(), "room-1", map[string]any{"text": "hello", "n": 7}); err != nil {
		t.Fatalf("Pub: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var got struct {
		Channel   string         `json:"channel"`
		Timestamp int64          `json:"timestamp"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal %q: %v", data, err)
	}
	if got.Channel != "room-1" {
		t.Errorf("channel = %q, want room-1", got.Channel)
	}
	if got.Payload["text"] != "hello" || got.Payload["n"] != float64(7) {
		t.Errorf("payload = %v", got.Payload)
	}
	if got.Timestamp == 0 {
		t.Error("timestamp not set")
	}
}

// A subscriber that never reads must not stall delivery to anyone else, and
// must be kicked once its buffer fills. Runs for both constructors.
func TestBroadcast_SlowConsumerDoesNotBlock(t *testing.T) {
	cases := []struct {
		name string
		new  func() *Broadcast
	}{
		{"legacy constructor, default buffer", func() *Broadcast { return NewBroadcast(10) }},
		{"named constructor, small buffer", func() *Broadcast {
			return NewNamedBroadcast("slow", 10, WithSubscriberBuffer(4))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useMiniredis(t)
			b := tc.new()
			// Take the write deadline out of the picture: on a loaded machine it
			// could evict the stalled connection first. Only the non-blocking
			// kick path may remove the slow subscriber in this test.
			b.writeWait = time.Hour
			startRun(t, b)
			wsURL, _ := wsServer(t, b)

			slow := dial(t, wsURL("room")) // never reads
			fast := dial(t, wsURL("room"))
			eventually(t, 2*time.Second, "both subscribers registered", func() bool {
				return b.SubscriberCount("room") == 2
			})

			got := make(chan struct{}, 4096)
			go func() {
				for {
					if _, _, err := fast.ReadMessage(); err != nil {
						return
					}
					got <- struct{}{}
				}
			}()

			// Large payloads so the slow connection's TCP buffers fill quickly.
			payload := strings.Repeat("x", 256*1024)
			const maxMessages = 600
			kickedAt := -1
			sent := 0
			for i := 0; i < maxMessages; i++ {
				if err := b.Pub(context.Background(), "room", payload); err != nil {
					t.Fatalf("Pub %d: %v", i, err)
				}
				sent++
				select {
				case <-got:
				case <-time.After(15 * time.Second):
					t.Fatalf("fast subscriber starved at message %d (head-of-line blocking)", i)
				}
				if kickedAt < 0 && b.SubscriberCount("room") == 1 {
					kickedAt = i
				}
				// keep going a little after the kick to prove delivery continues
				if kickedAt >= 0 && i >= kickedAt+5 {
					break
				}
			}
			if kickedAt < 0 {
				t.Fatalf("slow subscriber still subscribed after %d messages", sent)
			}
			t.Logf("slow subscriber kicked after %d messages; fast subscriber received all %d", kickedAt+1, sent)

			if n := b.SubscriberCount("room"); n != 1 {
				t.Errorf("SubscriberCount = %d, want 1 (only the fast subscriber)", n)
			}
			if d := b.metrics.messagesDropped.Load(); d != 1 {
				t.Errorf("messagesDropped = %d, want 1", d)
			}

			// The kicked connection must actually be closed by the server:
			// draining it ends in a non-timeout error.
			slow.SetReadDeadline(time.Now().Add(30 * time.Second))
			for {
				_, _, err := slow.ReadMessage()
				if err == nil {
					continue
				}
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					t.Fatalf("slow connection was not closed by server: %v", err)
				}
				break
			}
		})
	}
}

func TestBroadcast_NamespaceIsolation(t *testing.T) {
	mr := useMiniredis(t)

	a1 := NewNamedBroadcast("svc-a", 10)
	a2 := NewNamedBroadcast("svc-a", 10) // same namespace, "second machine"
	other := NewNamedBroadcast("svc-b", 10)
	legacy := NewBroadcast(10)
	for _, b := range []*Broadcast{a1, a2, other, legacy} {
		ownClient(t, b, mr)
		startRun(t, b)
	}

	subA1 := a1.subscribe("room", nil)
	subA2 := a2.subscribe("room", nil)
	subOther := other.subscribe("room", nil)
	subLegacy := legacy.subscribe("room", nil)

	if err := a1.Pub(context.Background(), "room", "from-a"); err != nil {
		t.Fatalf("Pub: %v", err)
	}

	// same namespace: both instances deliver
	for name, sub := range map[string]*subscriber{"a1": subA1, "a2": subA2} {
		m := recvOrTimeout(t, sub.ch, 3*time.Second)
		if m.Payload != "from-a" {
			t.Errorf("%s payload = %v, want from-a", name, m.Payload)
		}
	}
	// different namespace / legacy key: nothing. a1+a2 already got it, so the
	// publish has fully fanned out; a short grace period is enough.
	for name, sub := range map[string]*subscriber{"svc-b": subOther, "legacy": subLegacy} {
		select {
		case m := <-sub.ch:
			t.Errorf("%s received cross-namespace message: %+v", name, m)
		case <-time.After(200 * time.Millisecond):
		}
	}

	// and the reverse direction: legacy traffic does not leak into a namespace
	if err := legacy.Pub(context.Background(), "room", "from-legacy"); err != nil {
		t.Fatalf("Pub: %v", err)
	}
	if m := recvOrTimeout(t, subLegacy.ch, 3*time.Second); m.Payload != "from-legacy" {
		t.Errorf("legacy payload = %v", m.Payload)
	}
	select {
	case m := <-subA1.ch:
		t.Errorf("svc-a received legacy message: %+v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestBroadcast_Keys(t *testing.T) {
	useMiniredis(t)

	legacy := NewBroadcast(30)
	if got := legacy.broadcastKey(); got != "broadcast" {
		t.Errorf("legacy pubsub key = %q, want %q (must stay compatible)", got, "broadcast")
	}
	if got := legacy.messageCacheKey("room"); got != "broadcast/room" {
		t.Errorf("legacy cache key = %q, want %q (must stay compatible)", got, "broadcast/room")
	}

	named := NewNamedBroadcast("chat", 30)
	if got := named.broadcastKey(); got != "broadcast:chat" {
		t.Errorf("named pubsub key = %q, want %q", got, "broadcast:chat")
	}
	if got := named.messageCacheKey("room"); got != "broadcast:chat/room" {
		t.Errorf("named cache key = %q, want %q", got, "broadcast:chat/room")
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("NewNamedBroadcast(\"\") did not panic")
			}
		}()
		NewNamedBroadcast("", 30)
	}()
}

func TestBroadcast_Options(t *testing.T) {
	useMiniredis(t)

	if b := NewBroadcast(10); cap(b.subscribe("c", nil).ch) != 64 {
		t.Errorf("default subscriber buffer != 64")
	}
	if b := NewNamedBroadcast("o", 10, WithSubscriberBuffer(3)); cap(b.subscribe("c", nil).ch) != 3 {
		t.Errorf("WithSubscriberBuffer(3) not applied")
	}
	if b := NewNamedBroadcast("o", 10, WithSubscriberBuffer(0)); cap(b.subscribe("c", nil).ch) != 64 {
		t.Errorf("WithSubscriberBuffer(0) should keep the default")
	}
}

func TestBroadcast_CheckOrigin(t *testing.T) {
	useMiniredis(t)
	hdr := http.Header{"Origin": []string{"https://evil.example"}}

	// default: allow all (backward compatible)
	open := NewNamedBroadcast("origin-open", 10)
	openURL, _ := wsServer(t, open)
	conn, resp, err := websocket.DefaultDialer.Dial(openURL("room"), hdr)
	if err != nil {
		t.Fatalf("default CheckOrigin rejected a cross-origin handshake: %v", err)
	}
	resp.Body.Close()
	conn.Close()

	strict := NewNamedBroadcast("origin-strict", 10, WithCheckOrigin(func(r *http.Request) bool {
		return r.Header.Get("Origin") == "https://good.example"
	}))
	strictURL, _ := wsServer(t, strict)
	_, resp, err = websocket.DefaultDialer.Dial(strictURL("room"), hdr)
	if err == nil {
		t.Fatal("WithCheckOrigin did not reject a disallowed origin")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("rejected handshake status = %v, want 403", resp)
	}
	if n := strict.SubscriberCount("room"); n != 0 {
		t.Errorf("rejected handshake left %d subscribers", n)
	}
	conn, resp, err = websocket.DefaultDialer.Dial(strictURL("room"),
		http.Header{"Origin": []string{"https://good.example"}})
	if err != nil {
		t.Fatalf("allowed origin rejected: %v", err)
	}
	resp.Body.Close()
	conn.Close()
}

func TestBroadcast_ClientCloseCleansUpSubscription(t *testing.T) {
	useMiniredis(t)
	b := NewNamedBroadcast("close", 10)
	wsURL, _ := wsServer(t, b)

	for name, closeFn := range map[string]func(*websocket.Conn){
		"close frame": func(c *websocket.Conn) {
			c.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		},
		"abrupt tcp close": func(c *websocket.Conn) { c.Close() },
	} {
		t.Run(name, func(t *testing.T) {
			conn := dial(t, wsURL("room"))
			eventually(t, 2*time.Second, "subscriber registered", func() bool {
				return b.SubscriberCount("room") == 1
			})
			closeFn(conn)
			eventually(t, 3*time.Second, "subscription cleaned up after client close", func() bool {
				return b.SubscriberCount("room") == 0
			})
			if n := b.metrics.activeChannels.Load(); n != 0 {
				t.Errorf("activeChannels = %d, want 0", n)
			}
		})
	}
}

// A peer that vanishes without closing (never answers pings) is reaped by the
// read deadline; a healthy peer that answers pings is kept past that deadline.
func TestBroadcast_PongKeepsAliveAndDeadPeerIsReaped(t *testing.T) {
	useMiniredis(t)
	b := NewNamedBroadcast("ping", 10)
	b.pingInterval = 50 * time.Millisecond
	b.pongWait = 400 * time.Millisecond
	wsURL, _ := wsServer(t, b)

	healthy := dial(t, wsURL("healthy"))
	go func() { // reading is what makes gorilla answer pings
		for {
			if _, _, err := healthy.ReadMessage(); err != nil {
				return
			}
		}
	}()
	dial(t, wsURL("dead")) // never reads => never pongs

	eventually(t, 2*time.Second, "both registered", func() bool {
		return b.SubscriberCount("healthy") == 1 && b.SubscriberCount("dead") == 1
	})
	eventually(t, 5*time.Second, "dead peer reaped by read deadline", func() bool {
		return b.SubscriberCount("dead") == 0
	})
	time.Sleep(3 * b.pongWait)
	if n := b.SubscriberCount("healthy"); n != 1 {
		t.Errorf("healthy peer dropped despite answering pings: SubscriberCount = %d", n)
	}
}

func TestBroadcast_DeleteClosesSubscribers(t *testing.T) {
	useMiniredis(t)
	b := NewNamedBroadcast("delete", 10)
	wsURL, _ := wsServer(t, b)

	conn := dial(t, wsURL("room"))
	eventually(t, 2*time.Second, "subscriber registered", func() bool {
		return b.SubscriberCount("room") == 1
	})
	if _, ok := b.Load("room"); !ok {
		t.Error("Load(room) = false, want true")
	}

	b.Del("room")

	if n := b.SubscriberCount("room"); n != 0 {
		t.Errorf("SubscriberCount after Delete = %d, want 0", n)
	}
	if _, ok := b.Load("room"); ok {
		t.Error("Load(room) = true after Delete")
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Errorf("client read after Delete = %v, want normal closure", err)
	}
	b.Delete("room") // idempotent
	if n := b.metrics.activeChannels.Load(); n != 0 {
		t.Errorf("activeChannels = %d, want 0", n)
	}
}

func TestBroadcast_RunContextReturnsOnCancel(t *testing.T) {
	useMiniredis(t)
	b := NewNamedBroadcast("run", 10)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- b.RunContext(ctx) }()

	eventually(t, 5*time.Second, "redis subscription established", func() bool {
		m, _ := b.rds.PubSubNumSub(context.Background(), b.broadcastKey()).Result()
		return m[b.broadcastKey()] == 1
	})
	select {
	case err := <-errCh:
		t.Fatalf("RunContext returned before cancel: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("RunContext error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunContext did not return within 3s of cancel")
	}
}

func TestBroadcast_HttpSubLongPoll(t *testing.T) {
	useMiniredis(t)
	b := NewNamedBroadcast("http", 10)
	startRun(t, b)
	_, httpURL := wsServer(t, b)

	type result struct {
		body []byte
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := http.Get(httpURL + "/sub/room?timeout=10000")
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		resCh <- result{body: body, err: err}
	}()

	eventually(t, 3*time.Second, "long-poll subscriber registered", func() bool {
		return b.SubscriberCount("room") == 1
	})
	if err := b.Pub(context.Background(), "room", "ping"); err != nil {
		t.Fatalf("Pub: %v", err)
	}

	select {
	case r := <-resCh:
		if r.err != nil {
			t.Fatalf("long poll: %v", r.err)
		}
		var got struct {
			Code int `json:"code"`
			Data struct {
				Channel string `json:"channel"`
				Payload string `json:"payload"`
			} `json:"data"`
		}
		if err := json.Unmarshal(r.body, &got); err != nil {
			t.Fatalf("unmarshal %q: %v", r.body, err)
		}
		if got.Code != 0 || got.Data.Channel != "room" || got.Data.Payload != "ping" {
			t.Errorf("long poll response = %s", r.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("long poll did not return")
	}
	eventually(t, 2*time.Second, "long-poll subscription cleaned up", func() bool {
		return b.SubscriberCount("room") == 0
	})
}

// Delivery racing with subscribe / unsubscribe / kick / Delete must never
// panic (send on closed channel) or trip the race detector.
func TestBroadcast_ConcurrentDeliverAndUnsubscribe(t *testing.T) {
	useMiniredis(t)
	b := NewNamedBroadcast("race", 10, WithSubscriberBuffer(1))

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ { // deliverers (stand-ins for the Run loop)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.deliver(&BroadcastMessage{Channel: "room", Payload: "x"})
				}
			}
		}()
	}
	for i := 0; i < 8; i++ { // churning subscribers, some of which consume
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				sub := b.subscribe("room", func() {})
				if i%2 == 0 {
					select {
					case <-sub.ch:
					case <-sub.done:
					case <-time.After(time.Millisecond):
					}
				}
				b.unsubscribe("room", sub)
				b.unsubscribe("room", sub) // idempotent
			}
		}(i)
	}
	wg.Add(1)
	go func() { // channel deleter
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				b.Delete("room")
				b.SubscriberCount("room")
				time.Sleep(time.Millisecond)
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()

	if n := b.SubscriberCount("room"); n != 0 {
		t.Errorf("SubscriberCount = %d after all subscribers left, want 0", n)
	}
	if n := b.metrics.activeChannels.Load(); n != 0 {
		t.Errorf("activeChannels = %d after all subscribers left, want 0", n)
	}
}
