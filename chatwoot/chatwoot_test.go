package chatwoot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func TestParseWebhook_RealPayloadFormat(t *testing.T) {
	// Matches actual Chatwoot webhook format: string message_type, nested inbox, no sender.type
	payload := `{
		"event": "message_created",
		"content": "哈口",
		"message_type": "incoming",
		"inbox": {"id": 5, "name": "K2"},
		"sender": {"id": 1, "name": "User"},
		"conversation": {"id": 269, "status": "open", "inbox_id": 5}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if event.MessageType != "incoming" {
		t.Errorf("MessageType = %q, want %q", event.MessageType, "incoming")
	}
	if event.InboxID != 5 {
		t.Errorf("InboxID = %d, want 5", event.InboxID)
	}
	// Sender type inferred from message_type when missing
	if event.Sender.Type != "contact" {
		t.Errorf("Sender.Type = %q, want %q (inferred from incoming)", event.Sender.Type, "contact")
	}
}

func TestParseWebhook_InboxIDFallback(t *testing.T) {
	// No inbox field, fallback to conversation.inbox_id
	payload := `{
		"event": "message_created",
		"content": "test",
		"message_type": "outgoing",
		"sender": {"id": 1, "name": "Agent"},
		"conversation": {"id": 42, "status": "open", "inbox_id": 7}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if event.InboxID != 7 {
		t.Errorf("InboxID = %d, want 7 (from conversation.inbox_id)", event.InboxID)
	}
	// Sender type inferred as "user" for outgoing
	if event.Sender.Type != "user" {
		t.Errorf("Sender.Type = %q, want %q (inferred from outgoing)", event.Sender.Type, "user")
	}
}

func TestParseWebhook_SenderTypePreserved(t *testing.T) {
	// When sender.type is present, it should NOT be overwritten
	payload := `{
		"event": "message_created",
		"content": "bot reply",
		"message_type": "outgoing",
		"sender": {"id": 1, "name": "Bot", "type": "agent_bot"},
		"conversation": {"id": 42, "status": "open"}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if event.Sender.Type != "agent_bot" {
		t.Errorf("Sender.Type = %q, want %q (preserved from payload)", event.Sender.Type, "agent_bot")
	}
}

func TestParseWebhook_AssigneeID(t *testing.T) {
	// Unassigned: assignee is null → AssigneeID = 0
	payload := `{
		"event": "message_created",
		"content": "hello",
		"message_type": "incoming",
		"sender": {"id": 1, "name": "User"},
		"conversation": {"id": 42, "status": "open", "meta": {"assignee": null}}
	}`
	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if event.AssigneeID != 0 {
		t.Errorf("AssigneeID = %d, want 0 (unassigned)", event.AssigneeID)
	}

	// Assigned: assignee has id → AssigneeID = agent id
	payload = `{
		"event": "message_created",
		"content": "hello",
		"message_type": "incoming",
		"sender": {"id": 1, "name": "User"},
		"conversation": {"id": 42, "status": "open", "meta": {"assignee": {"id": 7, "name": "Agent"}}}
	}`
	event, err = parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if event.AssigneeID != 7 {
		t.Errorf("AssigneeID = %d, want 7", event.AssigneeID)
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
				{"id": 1, "content": "first msg", "message_type": 0},
				{"id": 2, "content": "second msg", "message_type": 0},
				{"id": 3, "content": "third msg", "message_type": 1},
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
	// Already in chronological order (oldest first)
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
				{"id": 1, "content": "msg1", "message_type": 0},
				{"id": 2, "content": "msg2", "message_type": 0},
				{"id": 3, "content": "msg3", "message_type": 0},
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
				{"id": 1, "content": "user question", "message_type": 0},
				{"id": 2, "content": "bot auto-reply", "message_type": 3},
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
	// Chronological: user question first, then bot reply
	if msgs[0].Role != "user" {
		t.Errorf("msgs[0].Role = %q, want user", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("msgs[1].Role = %q, want assistant (template/bot)", msgs[1].Role)
	}
}

func TestGetMessages_Pagination(t *testing.T) {
	resetState()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")

		// Mock returns < 20 items, so pagination stops after first page
		json.NewEncoder(w).Encode(map[string]any{
			"payload": []map[string]any{
				{"id": 30, "content": "msg3", "message_type": 0},
				{"id": 40, "content": "msg4", "message_type": 0},
				{"id": 50, "content": "msg5", "message_type": 0},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	msgs, err := GetMessages(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(msgs) != 3 {
		t.Fatalf("len = %d, want 3", len(msgs))
	}
	// Already chronological (oldest-first from API)
	if msgs[0].Content != "msg3" || msgs[1].Content != "msg4" || msgs[2].Content != "msg5" {
		t.Errorf("wrong order: %v, %v, %v", msgs[0].Content, msgs[1].Content, msgs[2].Content)
	}
	if requestCount != 1 {
		t.Errorf("requestCount = %d, want 1 (single page)", requestCount)
	}
}

func TestGetMessages_PaginationFull(t *testing.T) {
	resetState()

	requestCount := 0
	// Build a page of items in ascending ID order (oldest-first, matching real API)
	makePage := func(startID int, count int) []map[string]any {
		items := make([]map[string]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":           startID + i,
				"content":      fmt.Sprintf("msg-%d", startID+i),
				"message_type": 0,
			}
		}
		return items
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")

		before := r.URL.Query().Get("before")
		var payload []map[string]any

		switch before {
		case "": // page 1: IDs 21..40 (20 items, ascending)
			payload = makePage(21, 20)
		case "21": // page 2: IDs 5..20 (16 items, < 20 = last page)
			payload = makePage(5, 16)
		default:
			t.Errorf("unexpected before=%s", before)
			payload = []map[string]any{}
		}

		json.NewEncoder(w).Encode(map[string]any{"payload": payload})
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	msgs, err := GetMessages(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if requestCount != 2 {
		t.Fatalf("requestCount = %d, want 2", requestCount)
	}
	// 20 + 16 = 36 messages total
	if len(msgs) != 36 {
		t.Fatalf("len = %d, want 36", len(msgs))
	}
	// Chronological order: oldest (ID 5) first, newest (ID 40) last
	if msgs[0].Content != "msg-5" {
		t.Errorf("first msg = %q, want msg-5", msgs[0].Content)
	}
	if msgs[35].Content != "msg-40" {
		t.Errorf("last msg = %q, want msg-40", msgs[35].Content)
	}
}

func TestSendOptions_Success(t *testing.T) {
	resetState()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotPayload)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1}`))
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	err := SendOptions(context.Background(), 42, "请选择：",
		NewOption("技术支持", "support"),
		NewOption("产品咨询", "inquiry"),
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if gotPayload["content_type"] != "input_select" {
		t.Errorf("content_type = %v, want input_select", gotPayload["content_type"])
	}
	if gotPayload["content"] != "请选择：" {
		t.Errorf("content = %v", gotPayload["content"])
	}
	attrs := gotPayload["content_attributes"].(map[string]any)
	items := attrs["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != "技术支持" || first["value"] != "support" {
		t.Errorf("item[0] = %v", first)
	}
}

func TestSendOptions_NoOptions(t *testing.T) {
	resetState()
	SetConfig(&Config{APIToken: "t", BaseURL: "http://localhost", AccountID: 1})

	err := SendOptions(context.Background(), 42, "text")
	if err == nil || err.Error() != "chatwoot: at least one option is required" {
		t.Errorf("err = %v, want 'at least one option is required'", err)
	}
}

func TestSendCards_Success(t *testing.T) {
	resetState()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotPayload)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1}`))
	}))
	defer server.Close()

	SetConfig(&Config{APIToken: "t", BaseURL: server.URL, AccountID: 1})

	err := SendCards(context.Background(), 42, "推荐：",
		*NewCard("产品A").Desc("描述A").Image("https://img.com/a.jpg").Link("查看", "https://example.com/a"),
		*NewCard("产品B").Link("了解更多", "https://example.com/b"),
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if gotPayload["content_type"] != "cards" {
		t.Errorf("content_type = %v, want cards", gotPayload["content_type"])
	}
	attrs := gotPayload["content_attributes"].(map[string]any)
	items := attrs["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}

	card1 := items[0].(map[string]any)
	if card1["title"] != "产品A" {
		t.Errorf("card1 title = %v", card1["title"])
	}
	if card1["description"] != "描述A" {
		t.Errorf("card1 description = %v", card1["description"])
	}
	if card1["media_url"] != "https://img.com/a.jpg" {
		t.Errorf("card1 media_url = %v", card1["media_url"])
	}
	actions := card1["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("card1 actions len = %d", len(actions))
	}
	action := actions[0].(map[string]any)
	if action["type"] != "link" || action["text"] != "查看" || action["uri"] != "https://example.com/a" {
		t.Errorf("action = %v", action)
	}
}

func TestSendCards_NoActions(t *testing.T) {
	resetState()
	SetConfig(&Config{APIToken: "t", BaseURL: "http://localhost", AccountID: 1})

	err := SendCards(context.Background(), 42, "text", Card{Title: "No Actions"})
	if err == nil {
		t.Fatal("expected error for card without actions")
	}
}

func TestParseWebhook_MessageUpdated(t *testing.T) {
	// Real payload structure from Chatwoot message_updated webhook
	payload := `{
		"event": "message_updated",
		"id": 3002,
		"content": "测试按钮回调",
		"content_type": "input_select",
		"message_type": "outgoing",
		"content_attributes": {
			"items": [
				{"title": "选项A", "value": "option_a"},
				{"title": "选项B", "value": "option_b"}
			],
			"submitted_values": [
				{"title": "选项A", "value": "option_a"}
			]
		},
		"inbox": {"id": 5},
		"sender": {"id": 1, "name": "Ruth", "type": "user"},
		"conversation": {
			"id": 269,
			"status": "open",
			"meta": {"assignee": null}
		}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if event.EventType != "message_updated" {
		t.Errorf("EventType = %q, want message_updated", event.EventType)
	}
	if event.MessageID != 3002 {
		t.Errorf("MessageID = %d, want 3002", event.MessageID)
	}
	if event.ContentType != "input_select" {
		t.Errorf("ContentType = %q, want input_select", event.ContentType)
	}
	if event.ConversationID != 269 {
		t.Errorf("ConversationID = %d, want 269", event.ConversationID)
	}
	if len(event.SubmittedValues) != 1 {
		t.Fatalf("SubmittedValues len = %d, want 1", len(event.SubmittedValues))
	}
	if event.SubmittedValues[0].Title != "选项A" || event.SubmittedValues[0].Value != "option_a" {
		t.Errorf("SubmittedValues[0] = %+v", event.SubmittedValues[0])
	}
}

func TestParseWebhook_ConversationCreated(t *testing.T) {
	payload := `{
		"event": "conversation_created",
		"content": "",
		"content_type": "text",
		"message_type": "incoming",
		"inbox": {"id": 5},
		"sender": {"id": 1, "name": "User", "type": "contact"},
		"conversation": {
			"id": 300,
			"status": "open",
			"meta": {"assignee": null}
		}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if event.EventType != "conversation_created" {
		t.Errorf("EventType = %q, want conversation_created", event.EventType)
	}
	if event.ConversationID != 300 {
		t.Errorf("ConversationID = %d, want 300", event.ConversationID)
	}
	if len(event.SubmittedValues) != 0 {
		t.Errorf("SubmittedValues should be empty, got %d", len(event.SubmittedValues))
	}
}

func TestParseWebhook_TextMessageHasNoSubmittedValues(t *testing.T) {
	payload := `{
		"event": "message_created",
		"id": 100,
		"content": "hello",
		"content_type": "text",
		"message_type": "incoming",
		"inbox": {"id": 5},
		"sender": {"id": 1, "name": "User", "type": "contact"},
		"conversation": {"id": 42, "status": "open", "meta": {"assignee": null}}
	}`

	event, err := parseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if event.ContentType != "text" {
		t.Errorf("ContentType = %q, want text", event.ContentType)
	}
	if event.MessageID != 100 {
		t.Errorf("MessageID = %d, want 100", event.MessageID)
	}
	if len(event.SubmittedValues) != 0 {
		t.Errorf("SubmittedValues should be empty for text messages")
	}
}
