package fastgpt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// resetState() is defined in fastgpt.go (unexported, accessible from same package)

func TestChat_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "请按以下步骤安装"}},
			},
			"responseData": []map[string]any{
				{
					"moduleType": "datasetSearchNode",
					"quoteList": []map[string]any{
						{"score": 0.85},
						{"score": 0.72},
					},
				},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIKey: "test-key", BaseURL: server.URL})

	result, err := Chat(context.Background(), "conv-1", Text("怎么安装"))
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if result.Content != "请按以下步骤安装" {
		t.Errorf("Content = %q, want %q", result.Content, "请按以下步骤安装")
	}
	if result.Similarity != 0.85 {
		t.Errorf("Similarity = %f, want 0.85", result.Similarity)
	}
}

func TestChat_WithChatID(t *testing.T) {
	resetState()

	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIKey: "test-key", BaseURL: server.URL})

	Chat(context.Background(), "conv-123", Text("hello"))

	if receivedBody["chatId"] != "conv-123" {
		t.Errorf("chatId = %v, want %q", receivedBody["chatId"], "conv-123")
	}
	if receivedBody["stream"] != false {
		t.Errorf("stream = %v, want false", receivedBody["stream"])
	}
	if receivedBody["detail"] != true {
		t.Errorf("detail = %v, want true", receivedBody["detail"])
	}
}

func TestChat_ServerError(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	SetConfig(&Config{APIKey: "test-key", BaseURL: server.URL})

	_, err := Chat(context.Background(), "", Text("hello"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, should contain status code", err.Error())
	}
}

func TestChat_EmptyChoices(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer server.Close()

	SetConfig(&Config{APIKey: "test-key", BaseURL: server.URL})

	_, err := Chat(context.Background(), "", Text("hello"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("error = %q, should contain 'empty response'", err.Error())
	}
}

func TestChat_NoAPIKey(t *testing.T) {
	resetState()
	SetConfig(&Config{APIKey: "", BaseURL: "http://localhost"})

	_, err := Chat(context.Background(), "", Text("hello"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("error = %q, should mention api_key", err.Error())
	}
}

func TestChat_NoBaseURL(t *testing.T) {
	resetState()
	SetConfig(&Config{APIKey: "key", BaseURL: ""})

	_, err := Chat(context.Background(), "", Text("hello"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Errorf("error = %q, should mention base_url", err.Error())
	}
}

func TestChat_TrailingSlash(t *testing.T) {
	resetState()

	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIKey: "test-key", BaseURL: server.URL + "/"})

	Chat(context.Background(), "", Text("hello"))

	if receivedPath != "/api/v1/chat/completions" {
		t.Errorf("path = %q, want %q", receivedPath, "/api/v1/chat/completions")
	}
}

func TestChat_NoDatasetMatch(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "I don't know"}},
			},
			"responseData": []map[string]any{
				{"moduleType": "chatNode"},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIKey: "test-key", BaseURL: server.URL})

	result, err := Chat(context.Background(), "", Text("random question"))
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if result.Similarity != 0 {
		t.Errorf("Similarity = %f, want 0", result.Similarity)
	}
}

func TestChat_Multimodal(t *testing.T) {
	resetState()

	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "这是设置页面"}},
			},
		})
	}))
	defer server.Close()

	SetConfig(&Config{APIKey: "test-key", BaseURL: server.URL})

	Chat(context.Background(), "conv-1",
		Text("这个界面怎么操作"),
		ImageURL("https://s3.example.com/screenshot.png"),
	)

	// Verify content is an array (multimodal format)
	messages := receivedBody["messages"].([]any)
	msg := messages[0].(map[string]any)
	content, ok := msg["content"].([]any)
	if !ok {
		t.Fatal("content should be an array for multimodal messages")
	}
	if len(content) != 2 {
		t.Fatalf("content length = %d, want 2", len(content))
	}

	textPart := content[0].(map[string]any)
	if textPart["type"] != "text" {
		t.Errorf("content[0].type = %q, want %q", textPart["type"], "text")
	}

	imgPart := content[1].(map[string]any)
	if imgPart["type"] != "image_url" {
		t.Errorf("content[1].type = %q, want %q", imgPart["type"], "image_url")
	}
}

func TestChat_NoParts(t *testing.T) {
	resetState()
	SetConfig(&Config{APIKey: "key", BaseURL: "http://localhost"})

	_, err := Chat(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "at least one part") {
		t.Errorf("error = %q, should mention parts", err.Error())
	}
}
