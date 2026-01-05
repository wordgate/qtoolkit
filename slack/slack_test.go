package slack

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	configOnce = sync.Once{}
	httpClient = nil
}

func TestMessageBuilder_Text(t *testing.T) {
	resetState()

	p, err := To("test").Text("Hello").Build()
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if p.Text != "Hello" {
		t.Errorf("Text = %q, want %q", p.Text, "Hello")
	}
}

func TestMessageBuilder_Textf(t *testing.T) {
	resetState()

	p, err := To("test").Textf("Hello %s", "World").Build()
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if p.Text != "Hello World" {
		t.Errorf("Text = %q, want %q", p.Text, "Hello World")
	}
}

func TestMessageBuilder_Empty(t *testing.T) {
	resetState()

	_, err := To("test").Build()
	if !errors.Is(err, ErrEmptyMessage) {
		t.Errorf("error = %v, want %v", err, ErrEmptyMessage)
	}
}

func TestMessageBuilder_Attachment(t *testing.T) {
	resetState()

	p, err := To("test").
		Text("Deploy").
		Color(ColorGood).
		Field("Env", "prod", true).
		Build()

	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("Attachments = %d, want 1", len(p.Attachments))
	}
	if p.Attachments[0].Color != ColorGood {
		t.Errorf("Color = %q, want %q", p.Attachments[0].Color, ColorGood)
	}
}

func TestMessageBuilder_MultipleAttachments(t *testing.T) {
	resetState()

	p, err := To("test").
		Text("Report").
		BeginAttachment().Color(ColorGood).Title("OK").EndAttachment().
		BeginAttachment().Color(ColorDanger).Title("Fail").EndAttachment().
		Build()

	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if len(p.Attachments) != 2 {
		t.Fatalf("Attachments = %d, want 2", len(p.Attachments))
	}
}

func TestMessageBuilder_Timestamp(t *testing.T) {
	resetState()

	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	p, err := To("test").Text("Event").Timestamp(ts).Build()

	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if p.Attachments[0].Timestamp != ts.Unix() {
		t.Errorf("Timestamp = %d, want %d", p.Attachments[0].Timestamp, ts.Unix())
	}
}

func TestSend_NoWebhook(t *testing.T) {
	resetState()
	SetConfig(&Config{Channels: map[string]string{}, Timeout: 10 * time.Second})

	err := Send("nonexistent", "Hello")
	if !errors.Is(err, ErrNoWebhookURL) {
		t.Errorf("error = %v, want %v", err, ErrNoWebhookURL)
	}
}

func TestSend_Success(t *testing.T) {
	resetState()

	var received payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	SetConfig(&Config{
		Channels: map[string]string{"test": server.URL},
		Timeout:  10 * time.Second,
	})

	if err := Send("test", "Hello"); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if received.Text != "Hello" {
		t.Errorf("Text = %q, want %q", received.Text, "Hello")
	}
}

func TestSend_RichMessage(t *testing.T) {
	resetState()

	var received payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	SetConfig(&Config{
		Channels: map[string]string{"deploy": server.URL},
		Timeout:  10 * time.Second,
	})

	err := To("deploy").
		Text("Deployed").
		Color(ColorGood).
		Field("Env", "prod", true).
		Field("Ver", "v1.0", true).
		Send()

	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if len(received.Attachments) != 1 {
		t.Fatalf("Attachments = %d, want 1", len(received.Attachments))
	}
	if len(received.Attachments[0].Fields) != 2 {
		t.Errorf("Fields = %d, want 2", len(received.Attachments[0].Fields))
	}
}

func TestSend_ServerError(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	SetConfig(&Config{
		Channels: map[string]string{"test": server.URL},
		Timeout:  10 * time.Second,
	})

	err := Send("test", "Hello")
	if !errors.Is(err, ErrSendFailed) {
		t.Errorf("error = %v, want %v", err, ErrSendFailed)
	}
}

func TestIsConfigured(t *testing.T) {
	resetState()
	SetConfig(&Config{
		Channels: map[string]string{"alert": "https://example.com"},
		Timeout:  10 * time.Second,
	})

	if !IsConfigured("alert") {
		t.Error("IsConfigured(alert) = false, want true")
	}
	if IsConfigured("nonexistent") {
		t.Error("IsConfigured(nonexistent) = true, want false")
	}
}

func TestSendDM_NoBotToken(t *testing.T) {
	resetState()
	SetConfig(&Config{Timeout: 10 * time.Second})

	err := SendDM("user@example.com", "Hello")
	if !errors.Is(err, ErrNoBotToken) {
		t.Errorf("error = %v, want %v", err, ErrNoBotToken)
	}
}

func TestSendDM_EmptyMessage(t *testing.T) {
	resetState()
	SetConfig(&Config{BotToken: "xoxb-test", Timeout: 10 * time.Second})

	err := DM("user@example.com").Send()
	if !errors.Is(err, ErrEmptyMessage) {
		t.Errorf("error = %v, want %v", err, ErrEmptyMessage)
	}
}

func TestSendDM_UserNotFound(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users.lookupByEmail" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": "users_not_found",
			})
		}
	}))
	defer server.Close()

	slackAPIBase = server.URL
	defer func() { slackAPIBase = "https://slack.com/api" }()

	SetConfig(&Config{BotToken: "xoxb-test", Timeout: 10 * time.Second})

	err := SendDM("unknown@example.com", "Hello")
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestSendDM_Success(t *testing.T) {
	resetState()

	var receivedChannel, receivedText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users.lookupByEmail" {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"user": map[string]string{"id": "U12345"},
			})
			return
		}
		if r.URL.Path == "/chat.postMessage" {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			json.Unmarshal(body, &payload)
			receivedChannel = payload["channel"].(string)
			receivedText = payload["text"].(string)
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
	}))
	defer server.Close()

	slackAPIBase = server.URL
	defer func() { slackAPIBase = "https://slack.com/api" }()

	SetConfig(&Config{BotToken: "xoxb-test", Timeout: 10 * time.Second})

	if err := SendDM("user@example.com", "Hello!"); err != nil {
		t.Fatalf("SendDM error: %v", err)
	}
	if receivedChannel != "U12345" {
		t.Errorf("channel = %q, want %q", receivedChannel, "U12345")
	}
	if receivedText != "Hello!" {
		t.Errorf("text = %q, want %q", receivedText, "Hello!")
	}
}

func TestDMBuilder_RichMessage(t *testing.T) {
	resetState()

	var receivedPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users.lookupByEmail" {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"user": map[string]string{"id": "U12345"},
			})
			return
		}
		if r.URL.Path == "/chat.postMessage" {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedPayload)
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
	}))
	defer server.Close()

	slackAPIBase = server.URL
	defer func() { slackAPIBase = "https://slack.com/api" }()

	SetConfig(&Config{BotToken: "xoxb-test", Timeout: 10 * time.Second})

	err := DM("user@example.com").
		Text("Deploy done").
		Color(ColorGood).
		Field("Env", "prod", true).
		Send()

	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if receivedPayload["text"] != "Deploy done" {
		t.Errorf("text = %v, want %q", receivedPayload["text"], "Deploy done")
	}
	attachments := receivedPayload["attachments"].([]any)
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
}
