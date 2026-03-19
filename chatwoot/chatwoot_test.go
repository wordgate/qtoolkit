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
