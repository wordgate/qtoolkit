package chatwoot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// resetState() is defined in chatwoot.go (unexported, accessible from same package)

func TestReply_Success(t *testing.T) {
	resetState()

	var receivedPath, receivedToken string
	var receivedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedToken = r.Header.Get("api_access_token")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "test-token", BaseURL: server.URL, AccountID: 1})

	err := Reply(context.Background(), 42, "Hello!")
	if err != nil {
		t.Fatalf("Reply error: %v", err)
	}
	if receivedPath != "/api/v1/accounts/1/conversations/42/messages" {
		t.Errorf("path = %q, want /api/v1/accounts/1/conversations/42/messages", receivedPath)
	}
	if receivedToken != "test-token" {
		t.Errorf("token = %q, want %q", receivedToken, "test-token")
	}
	if receivedBody["content"] != "Hello!" {
		t.Errorf("content = %q, want %q", receivedBody["content"], "Hello!")
	}
	if receivedBody["message_type"] != "outgoing" {
		t.Errorf("message_type = %q, want %q", receivedBody["message_type"], "outgoing")
	}
}

func TestReply_ServerError(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("access denied"))
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "bad-token", BaseURL: server.URL, AccountID: 1})

	err := Reply(context.Background(), 42, "Hello!")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, should contain 403", err.Error())
	}
}

func TestReply_NotConfigured(t *testing.T) {
	resetState()
	// Don't call SetConfig — globalConfig stays nil

	err := Reply(context.Background(), 42, "Hello!")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %q, should mention 'not configured'", err.Error())
	}
}

func TestParseWebhook_MessageCreated(t *testing.T) {
	payload := `{
		"event": "message_created",
		"content": "怎么安装",
		"message_type": 0,
		"sender": {"id": 1, "name": "User", "type": "contact"},
		"conversation": {"id": 42, "status": "open"}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if event.EventType != "message_created" {
		t.Errorf("EventType = %q, want %q", event.EventType, "message_created")
	}
	if event.Content != "怎么安装" {
		t.Errorf("Content = %q, want %q", event.Content, "怎么安装")
	}
	if event.ConversationID != 42 {
		t.Errorf("ConversationID = %d, want 42", event.ConversationID)
	}
	if event.MessageType != "incoming" {
		t.Errorf("MessageType = %q, want %q", event.MessageType, "incoming")
	}
	if event.Sender.Type != "contact" {
		t.Errorf("Sender.Type = %q, want %q", event.Sender.Type, "contact")
	}
	if event.Conversation.Status != "open" {
		t.Errorf("Conversation.Status = %q, want %q", event.Conversation.Status, "open")
	}
}

func TestParseWebhook_InvalidJSON(t *testing.T) {
	_, err := parseWebhook([]byte("not json"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseWebhook_MessageTypes(t *testing.T) {
	tests := []struct {
		typeInt int
		want    string
	}{
		{0, "incoming"},
		{1, "outgoing"},
		{2, "activity"},
	}
	for _, tt := range tests {
		payload, _ := json.Marshal(map[string]any{
			"event":        "message_created",
			"message_type": tt.typeInt,
			"conversation": map[string]any{"id": 1},
		})
		event, err := parseWebhook(payload)
		if err != nil {
			t.Fatalf("parse error for type %d: %v", tt.typeInt, err)
		}
		if event.MessageType != tt.want {
			t.Errorf("MessageType for %d = %q, want %q", tt.typeInt, event.MessageType, tt.want)
		}
	}
}

func TestVerifySignature_Valid(t *testing.T) {
	body := []byte(`{"event":"message_created"}`)
	token := "secret"

	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	if !verifySignature(body, signature, token) {
		t.Error("valid signature rejected")
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	body := []byte(`{"event":"message_created"}`)
	if verifySignature(body, "wrong-signature", "secret") {
		t.Error("invalid signature accepted")
	}
}

func TestParseWebhook_WithAttachments(t *testing.T) {
	payload := `{
		"event": "message_created",
		"content": "",
		"message_type": 0,
		"sender": {"id": 1, "name": "User", "type": "contact"},
		"conversation": {"id": 42, "status": "open"},
		"attachments": [
			{"file_type": "image", "data_url": "https://s3.example.com/img.png", "thumb_url": "https://s3.example.com/img_thumb.png", "file_size": 102400},
			{"file_type": "audio", "data_url": "https://s3.example.com/voice.mp3", "thumb_url": "", "file_size": 51200}
		]
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(event.Attachments) != 2 {
		t.Fatalf("Attachments = %d, want 2", len(event.Attachments))
	}
	if event.Attachments[0].FileType != "image" {
		t.Errorf("Attachments[0].FileType = %q, want %q", event.Attachments[0].FileType, "image")
	}
	if event.Attachments[0].DataURL != "https://s3.example.com/img.png" {
		t.Errorf("Attachments[0].DataURL = %q", event.Attachments[0].DataURL)
	}
	if event.Attachments[1].FileType != "audio" {
		t.Errorf("Attachments[1].FileType = %q, want %q", event.Attachments[1].FileType, "audio")
	}
	if event.Attachments[1].FileSize != 51200 {
		t.Errorf("Attachments[1].FileSize = %d, want 51200", event.Attachments[1].FileSize)
	}
}

func TestMount_AsyncHandler(t *testing.T) {
	resetState()
	SetConfig(&Config{APIToken: "t", BaseURL: "http://localhost", AccountID: 1})

	called := make(chan Event, 1)

	r := gin.New()
	Mount(r, "/webhook", func(ctx context.Context, event Event) {
		called <- event
	})

	payload := `{
		"event": "message_created",
		"content": "hello",
		"message_type": 0,
		"sender": {"id": 1, "name": "User", "type": "contact"},
		"conversation": {"id": 99, "status": "open"}
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Verify 200 returned immediately
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Wait for async handler
	select {
	case event := <-called:
		if event.ConversationID != 99 {
			t.Errorf("ConversationID = %d, want 99", event.ConversationID)
		}
		if event.Content != "hello" {
			t.Errorf("Content = %q, want %q", event.Content, "hello")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called within 2s")
	}
}

func TestMount_HMACReject(t *testing.T) {
	resetState()
	SetConfig(&Config{APIToken: "t", BaseURL: "http://localhost", AccountID: 1, WebhookToken: "secret"})

	r := gin.New()
	Mount(r, "/webhook", func(ctx context.Context, event Event) {
		t.Error("handler should not be called")
	})

	payload := `{"event": "message_created"}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	// No X-Chatwoot-Signature header
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMount_InvalidJSON(t *testing.T) {
	resetState()
	SetConfig(&Config{APIToken: "t", BaseURL: "http://localhost", AccountID: 1})

	handlerCalled := make(chan struct{}, 1)

	r := gin.New()
	Mount(r, "/webhook", func(ctx context.Context, event Event) {
		handlerCalled <- struct{}{}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// Verify handler was NOT called
	select {
	case <-handlerCalled:
		t.Error("handler should not be called for invalid JSON")
	case <-time.After(200 * time.Millisecond):
		// Expected: handler not called
	}
}

func TestMount_PanicRecovery(t *testing.T) {
	resetState()
	SetConfig(&Config{APIToken: "t", BaseURL: "http://localhost", AccountID: 1})

	r := gin.New()
	Mount(r, "/webhook", func(ctx context.Context, event Event) {
		panic("intentional test panic")
	})

	payload := `{
		"event": "message_created",
		"content": "hello",
		"message_type": 0,
		"sender": {"id": 1, "name": "User", "type": "contact"},
		"conversation": {"id": 10, "status": "open"}
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Verify 200 returned (async, panic happens after response)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Sleep briefly to let goroutine run and recover from panic
	// If the test doesn't crash, panic recovery works correctly
	time.Sleep(100 * time.Millisecond)
}

func TestParseWebhook_NoAttachments(t *testing.T) {
	payload := `{
		"event": "message_created",
		"content": "hello",
		"message_type": 0,
		"sender": {"id": 1, "name": "User", "type": "contact"},
		"conversation": {"id": 42, "status": "open"}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if event.Attachments != nil {
		t.Errorf("Attachments = %v, want nil when no attachments field present", event.Attachments)
	}
}

func TestReply_CorrectURL(t *testing.T) {
	resetState()

	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "test-token", BaseURL: server.URL, AccountID: 5})

	err := Reply(context.Background(), 123, "test message")
	if err != nil {
		t.Fatalf("Reply error: %v", err)
	}

	want := "/api/v1/accounts/5/conversations/123/messages"
	if receivedPath != want {
		t.Errorf("path = %q, want %q", receivedPath, want)
	}
}

func TestGetMessages_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/accounts/1/conversations/42/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("api_access_token") != "test-token" {
			t.Errorf("token = %q", r.Header.Get("api_access_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"payload": []map[string]any{
				{"content": "third msg", "message_type": 1},
				{"content": "second msg", "message_type": 0},
				{"content": "first msg", "message_type": 0},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "test-token", BaseURL: server.URL, AccountID: 1})

	msgs, err := GetMessages(context.Background(), 42, 10)
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("len = %d, want 3", len(msgs))
	}
	// Should be reversed to chronological order
	if msgs[0].Content != "first msg" || msgs[0].Role != "user" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[2].Content != "third msg" || msgs[2].Role != "assistant" {
		t.Errorf("msgs[2] = %+v", msgs[2])
	}
}

func TestGetMessages_WithImages(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"payload": []map[string]any{
				{
					"content": "看这个", "message_type": 0,
					"attachments": []map[string]any{
						{"file_type": "image", "data_url": "https://example.com/img.png"},
					},
				},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	msgs, err := GetMessages(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len = %d, want 1", len(msgs))
	}
	if len(msgs[0].Images) != 1 || msgs[0].Images[0] != "https://example.com/img.png" {
		t.Errorf("Images = %v", msgs[0].Images)
	}
}

func TestGetMessages_SkipsActivityAndEmpty(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"payload": []map[string]any{
				{"content": "real msg", "message_type": 0},
				{"content": "activity", "message_type": 2},
				{"content": "", "message_type": 0},
				{"content": nil, "message_type": 0},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	msgs, err := GetMessages(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len = %d, want 1 (activity, empty, and null skipped)", len(msgs))
	}
}

func TestGetMessages_Limit(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"payload": []map[string]any{
				{"content": "msg3", "message_type": 0},
				{"content": "msg2", "message_type": 0},
				{"content": "msg1", "message_type": 0},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	msgs, err := GetMessages(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2 (limited)", len(msgs))
	}
	if msgs[0].Content != "msg2" {
		t.Errorf("msgs[0].Content = %q, want msg2", msgs[0].Content)
	}
	if msgs[1].Content != "msg3" {
		t.Errorf("msgs[1].Content = %q, want msg3", msgs[1].Content)
	}
}

func TestGetMessages_TemplateBotMessages(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"payload": []map[string]any{
				{"content": "bot auto-reply", "message_type": 3},
				{"content": "user question", "message_type": 0},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	msgs, err := GetMessages(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	// Reversed to chronological: user question first, then bot reply
	if msgs[0].Role != "user" {
		t.Errorf("msgs[0].Role = %q, want user", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("msgs[1].Role = %q, want assistant (template/bot)", msgs[1].Role)
	}
}
