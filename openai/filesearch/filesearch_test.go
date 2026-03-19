package filesearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAsk_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", r.URL.Path)
		}

		// Verify include parameter in request
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]any
		json.Unmarshal(body, &reqBody)
		include, _ := reqBody["include"].([]any)
		if len(include) != 1 || include[0] != "file_search_call.results" {
			t.Errorf("include = %v, want [file_search_call.results]", include)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{
					"type": "file_search_call",
					"results": []map[string]any{
						{"filename": "guide.pdf", "score": 0.92, "text": "安装步骤...", "file_id": "file-1"},
						{"filename": "faq.md", "score": 0.78, "text": "常见问题...", "file_id": "file-2"},
					},
				},
				{
					"type": "message",
					"role": "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": "请按以下步骤安装"},
					},
				},
			},
		})
	}))
	defer server.Close()

	baseURL = server.URL
	defer func() { baseURL = "https://api.openai.com" }()

	SetConfig(&Config{APIKey: "test-key", VectorStoreID: "vs_test"})

	result, err := Ask(context.Background(), "怎么安装")
	if err != nil {
		t.Fatalf("Ask error: %v", err)
	}
	if result.Content != "请按以下步骤安装" {
		t.Errorf("Content = %q, want %q", result.Content, "请按以下步骤安装")
	}
	if result.Score != 0.92 {
		t.Errorf("Score = %f, want 0.92", result.Score)
	}
	if len(result.Citations) != 2 {
		t.Fatalf("Citations = %d, want 2", len(result.Citations))
	}
	if result.Citations[0].FileName != "guide.pdf" {
		t.Errorf("Citations[0].FileName = %q", result.Citations[0].FileName)
	}
	if result.Citations[0].Text != "安装步骤..." {
		t.Errorf("Citations[0].Text = %q", result.Citations[0].Text)
	}
}

func TestAsk_WithHistory(t *testing.T) {
	resetState()

	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{"type": "message", "content": []map[string]any{{"type": "output_text", "text": "ok"}}},
			},
		})
	}))
	defer server.Close()

	baseURL = server.URL
	defer func() { baseURL = "https://api.openai.com" }()

	SetConfig(&Config{APIKey: "test-key", VectorStoreID: "vs_test"})

	Ask(context.Background(), "第三步看不懂",
		WithHistory([]Message{
			{Role: "user", Content: "怎么安装"},
			{Role: "assistant", Content: "请按以下步骤..."},
		}),
	)

	input := receivedBody["input"].([]any)
	if len(input) != 3 {
		t.Fatalf("input length = %d, want 3 (2 history + 1 current)", len(input))
	}
	msg0 := input[0].(map[string]any)
	if msg0["role"] != "user" {
		t.Errorf("input[0].role = %v, want user", msg0["role"])
	}
	msg2 := input[2].(map[string]any)
	if msg2["role"] != "user" {
		t.Errorf("input[2].role = %v, want user", msg2["role"])
	}
	if msg2["content"] != "第三步看不懂" {
		t.Errorf("input[2].content = %v, want 第三步看不懂", msg2["content"])
	}
}

func TestAsk_WithImage(t *testing.T) {
	resetState()

	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{"type": "message", "content": []map[string]any{{"type": "output_text", "text": "ok"}}},
			},
		})
	}))
	defer server.Close()

	baseURL = server.URL
	defer func() { baseURL = "https://api.openai.com" }()

	SetConfig(&Config{APIKey: "test-key", VectorStoreID: "vs_test"})

	Ask(context.Background(), "这是什么界面",
		WithImage("https://example.com/screenshot.png"),
	)

	input := receivedBody["input"].([]any)
	msg := input[0].(map[string]any)
	content, ok := msg["content"].([]any)
	if !ok {
		t.Fatal("content should be an array for multimodal message")
	}
	if len(content) != 2 {
		t.Fatalf("content length = %d, want 2", len(content))
	}
	imgPart := content[1].(map[string]any)
	if imgPart["type"] != "input_image" {
		t.Errorf("content[1].type = %v, want input_image", imgPart["type"])
	}
	if imgPart["detail"] != "low" {
		t.Errorf("content[1].detail = %v, want low", imgPart["detail"])
	}
	if imgPart["image_url"] != "https://example.com/screenshot.png" {
		t.Errorf("content[1].image_url = %v", imgPart["image_url"])
	}
}

func TestAsk_WithHistoryImages(t *testing.T) {
	resetState()

	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{"type": "message", "content": []map[string]any{{"type": "output_text", "text": "ok"}}},
			},
		})
	}))
	defer server.Close()

	baseURL = server.URL
	defer func() { baseURL = "https://api.openai.com" }()

	SetConfig(&Config{APIKey: "test-key", VectorStoreID: "vs_test"})

	Ask(context.Background(), "这个按钮是什么",
		WithHistory([]Message{
			{Role: "user", Content: "看看这个", Images: []string{"https://example.com/img.png"}},
			{Role: "assistant", Content: "这是设置页面"},
		}),
	)

	input := receivedBody["input"].([]any)
	msg0 := input[0].(map[string]any)
	content, ok := msg0["content"].([]any)
	if !ok {
		t.Fatal("history message with images should have array content")
	}
	if len(content) != 2 {
		t.Fatalf("content length = %d, want 2 (text + image)", len(content))
	}
}

func TestAsk_ServerError(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	baseURL = server.URL
	defer func() { baseURL = "https://api.openai.com" }()

	SetConfig(&Config{APIKey: "test-key", VectorStoreID: "vs_test"})

	_, err := Ask(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, should contain 500", err.Error())
	}
}

func TestAsk_EmptyQuestion(t *testing.T) {
	resetState()
	SetConfig(&Config{APIKey: "key", VectorStoreID: "vs_test"})

	_, err := Ask(context.Background(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "question is required") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestAsk_NoAPIKey(t *testing.T) {
	resetState()
	SetConfig(&Config{APIKey: "", VectorStoreID: "vs_test"})

	_, err := Ask(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestAsk_NoVectorStoreID(t *testing.T) {
	resetState()
	SetConfig(&Config{APIKey: "key", VectorStoreID: ""})

	_, err := Ask(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vector_store_id") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestBuildMessageContent_TextOnly(t *testing.T) {
	result := buildMessageContent("hello", nil)
	s, ok := result.(string)
	if !ok {
		t.Fatal("text-only should return string")
	}
	if s != "hello" {
		t.Errorf("result = %q, want %q", s, "hello")
	}
}

func TestBuildMessageContent_WithImages(t *testing.T) {
	result := buildMessageContent("看这个", []string{"https://example.com/img.png"})
	arr, ok := result.([]map[string]any)
	if !ok {
		t.Fatal("with images should return array")
	}
	if len(arr) != 2 {
		t.Fatalf("length = %d, want 2", len(arr))
	}
	if arr[0]["type"] != "input_text" {
		t.Errorf("arr[0].type = %v, want input_text", arr[0]["type"])
	}
	if arr[1]["type"] != "input_image" {
		t.Errorf("arr[1].type = %v, want input_image", arr[1]["type"])
	}
}

func TestParseResponse_NoCitations(t *testing.T) {
	resp := apiResponse{
		Output: []outputItem{
			{
				Type:    "message",
				Content: []contentItem{{Type: "output_text", Text: "I don't know"}},
			},
		},
	}
	result := parseResponse(resp)
	if result.Score != 0 {
		t.Errorf("Score = %f, want 0", result.Score)
	}
	if len(result.Citations) != 0 {
		t.Errorf("Citations = %d, want 0", len(result.Citations))
	}
	if result.Content != "I don't know" {
		t.Errorf("Content = %q", result.Content)
	}
}
