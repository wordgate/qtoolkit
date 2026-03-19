# OpenAI File Search + Chatwoot GetMessages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create `openai/filesearch/` module for knowledge base Q&A via OpenAI Responses API + file_search, and enhance `chatwoot/` module with `GetMessages()` for conversation history retrieval.

**Architecture:** `openai/filesearch/` is a new independent Go module following qtoolkit v1.0 patterns (sync.Once, viper, shared http.Client). `Ask()` posts to OpenAI Responses API with file_search tool, returns answer + confidence score + citations. `chatwoot/` gains `GetMessages()` to fetch conversation history including image URLs. Modules have no cross-dependency — caller converts between Message types.

**Tech Stack:** Go 1.24.0, viper, net/http, httptest

**Spec:** `docs/superpowers/specs/2026-03-19-openai-filesearch-design.md`

---

## File Structure

### `openai/filesearch/` module (new)
| File | Responsibility |
|------|---------------|
| `openai/filesearch/go.mod` | Module: `github.com/wordgate/qtoolkit/openai/filesearch`, deps: viper |
| `openai/filesearch/filesearch.go` | Config, lazy init, Message/Result/Citation/Option types, `Ask()`, `buildInput()` |
| `openai/filesearch/manage.go` | `CreateStore()`, `UploadFile()`, `Search()` |
| `openai/filesearch/filesearch_test.go` | Tests for Ask(), options, response parsing |
| `openai/filesearch/filesearch_config.yml` | Config template |

### `chatwoot/` module (enhancement)
| File | Responsibility |
|------|---------------|
| `chatwoot/chatwoot.go` | Add `Message` type, `GetMessages()` function |
| `chatwoot/chatwoot_test.go` | Add tests for GetMessages |

---

### Task 1: openai/filesearch module — scaffold, config, types, Ask()

**Files:**
- Create: `openai/filesearch/go.mod`
- Create: `openai/filesearch/filesearch.go`
- Create: `openai/filesearch/filesearch_config.yml`
- Modify: `go.work` (add `./openai/filesearch`)

- [ ] **Step 1: Create directory and go.mod**

```bash
mkdir -p /Users/david/projects/wordgate/qtoolkit/openai/filesearch
cd /Users/david/projects/wordgate/qtoolkit/openai/filesearch
go mod init github.com/wordgate/qtoolkit/openai/filesearch
```

Edit go.mod to ensure `go 1.24.0`. Then `go get github.com/spf13/viper@v1.21.0 && go mod tidy`.

- [ ] **Step 2: Write filesearch.go**

```go
// Package filesearch provides knowledge base Q&A using OpenAI's Responses API + file_search.
//
// Usage:
//
//	result, err := filesearch.Ask(ctx, "怎么安装app")
//	result, err := filesearch.Ask(ctx, "这是什么界面",
//	    filesearch.WithImage("https://s3.../screenshot.png"),
//	    filesearch.WithHistory(history),
//	)
package filesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Config holds OpenAI file search configuration.
type Config struct {
	APIKey       string `yaml:"api_key"`
	VectorStoreID string `yaml:"vector_store_id"`
	Model        string `yaml:"model"`
	ScoreThreshold float64 `yaml:"score_threshold"`
	MaxResults   int    `yaml:"max_results"`
}

// Message represents a conversation message (for history context).
type Message struct {
	Role    string   // "user" / "assistant"
	Content string   // Text content
	Images  []string // Optional image URLs (OpenAI downloads directly)
}

// Result is the return value of Ask().
type Result struct {
	Content   string     // AI answer text
	Score     float64    // Highest retrieval similarity (0-1), 0 if no match
	Citations []Citation // Source references
}

// Citation represents a source reference from the knowledge base.
type Citation struct {
	FileName string  // Source document name
	Score    float64 // Relevance score (0-1)
	Text     string  // Retrieved text chunk
}

// Option configures Ask() behavior.
type Option func(*askConfig)

type askConfig struct {
	history []Message
	images  []string
}

// WithHistory provides conversation context (preceding messages).
func WithHistory(messages []Message) Option {
	return func(c *askConfig) {
		c.history = messages
	}
}

// WithImage attaches an image URL to the current question.
// OpenAI downloads the image directly from the URL.
func WithImage(url string) Option {
	return func(c *askConfig) {
		c.images = append(c.images, url)
	}
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMux    sync.RWMutex
	httpClient   *http.Client
)

func loadConfigFromViper() *Config {
	cfg := &Config{
		APIKey:        viper.GetString("openai.filesearch.api_key"),
		VectorStoreID: viper.GetString("openai.filesearch.vector_store_id"),
		Model:         viper.GetString("openai.filesearch.model"),
		ScoreThreshold: viper.GetFloat64("openai.filesearch.score_threshold"),
		MaxResults:    viper.GetInt("openai.filesearch.max_results"),
	}
	// Cascading fallback for api_key
	if cfg.APIKey == "" {
		cfg.APIKey = viper.GetString("ai.providers.openai.api_key")
	}
	// Defaults
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}
	return cfg
}

func initialize() {
	httpClient = &http.Client{Timeout: 60 * time.Second}

	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil {
		cfg = loadConfigFromViper()
		configMux.Lock()
		globalConfig = cfg
		configMux.Unlock()
	}
}

func ensureInitialized() {
	configOnce.Do(initialize)
}

func getConfig() *Config {
	ensureInitialized()
	configMux.RLock()
	defer configMux.RUnlock()
	return globalConfig
}

// SetConfig sets configuration manually (for testing).
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}
	globalConfig = cfg
	httpClient = &http.Client{Timeout: 60 * time.Second}
}

func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	configOnce = sync.Once{}
	httpClient = nil
}

// buildMessageContent builds the content field for a single message.
// Text-only → string. Text+images → multimodal array.
func buildMessageContent(text string, images []string) any {
	if len(images) == 0 {
		return text
	}
	var parts []map[string]any
	if text != "" {
		parts = append(parts, map[string]any{
			"type": "input_text",
			"text": text,
		})
	}
	for _, img := range images {
		parts = append(parts, map[string]any{
			"type":      "input_image",
			"image_url": img,
			"detail":    "low",
		})
	}
	return parts
}

// buildInput converts question + options into the OpenAI Responses API input array.
func buildInput(question string, cfg *askConfig) []map[string]any {
	var input []map[string]any

	// Add history messages
	for _, m := range cfg.history {
		input = append(input, map[string]any{
			"role":    m.Role,
			"content": buildMessageContent(m.Content, m.Images),
		})
	}

	// Add current question (with optional images from WithImage)
	input = append(input, map[string]any{
		"role":    "user",
		"content": buildMessageContent(question, cfg.images),
	})

	return input
}

// response types for OpenAI Responses API
type apiResponse struct {
	Output []outputItem `json:"output"`
}

type outputItem struct {
	Type    string          `json:"type"`
	Content []contentItem   `json:"content"`
	Role    string          `json:"role"`
	Results []searchResult  `json:"results"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type searchResult struct {
	FileName string  `json:"filename"`
	Score    float64 `json:"score"`
	Text     string  `json:"text"`
	FileID   string  `json:"file_id"`
}

// Ask queries the knowledge base.
// question is the current user question (explicit first parameter).
// Options provide conversation history, images, etc.
func Ask(ctx context.Context, question string, opts ...Option) (Result, error) {
	if question == "" {
		return Result{}, fmt.Errorf("filesearch: question is required")
	}

	cfg := getConfig()
	if cfg.APIKey == "" {
		return Result{}, fmt.Errorf("filesearch: api_key is required")
	}
	if cfg.VectorStoreID == "" {
		return Result{}, fmt.Errorf("filesearch: vector_store_id is required")
	}

	ac := &askConfig{}
	for _, opt := range opts {
		opt(ac)
	}

	reqBody := map[string]any{
		"model": cfg.Model,
		"input": buildInput(question, ac),
		"tools": []map[string]any{
			{
				"type":             "file_search",
				"vector_store_ids": []string{cfg.VectorStoreID},
				"max_num_results":  cfg.MaxResults,
			},
		},
		"tool_choice": "required",
		"include":     []string{"file_search_call.results"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return Result{}, fmt.Errorf("filesearch: marshal error: %w", err)
	}

	url := "https://api.openai.com/v1/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("filesearch: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("filesearch: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("filesearch: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return Result{}, fmt.Errorf("filesearch: unmarshal error: %w", err)
	}

	return parseResponse(apiResp), nil
}

// parseResponse extracts Result from the OpenAI Responses API output.
// Score and Citations come from file_search_call.results[] (NOT from message annotations).
// Message annotations only have file_id/filename/index, no score or text.
func parseResponse(resp apiResponse) Result {
	var result Result

	for _, item := range resp.Output {
		switch item.Type {
		case "file_search_call":
			// Extract citations and max score from search results
			for _, sr := range item.Results {
				citation := Citation{
					FileName: sr.FileName,
					Score:    sr.Score,
					Text:     sr.Text,
				}
				result.Citations = append(result.Citations, citation)
				if sr.Score > result.Score {
					result.Score = sr.Score
				}
			}
		case "message":
			// Extract assistant message content
			for _, c := range item.Content {
				if c.Type == "output_text" {
					result.Content = c.Text
				}
			}
		}
	}

	return result
}

// baseURL allows overriding the OpenAI API base URL for testing.
var baseURL = "https://api.openai.com"
```

Note: Replace the hardcoded URL in Ask() with `baseURL + "/v1/responses"` to enable test mocking. Update the line:
```go
url := baseURL + "/v1/responses"
```

- [ ] **Step 3: Create filesearch_config.yml**

```yaml
# OpenAI File Search Configuration Template
# Add this to your main config.yml file

openai:
  filesearch:
    # OpenAI API key (required)
    # Falls back to ai.providers.openai.api_key if not set
    api_key: "YOUR_OPENAI_API_KEY"

    # Vector store ID (required)
    # Create at: OpenAI Dashboard → Storage → Vector Stores
    vector_store_id: "vs_xxxxx"

    # Model for generating answers (default: gpt-4o-mini)
    model: "gpt-4o-mini"

    # Minimum score filter (default: 0.0, no filter)
    # score_threshold: 0.0

    # Max retrieved chunks (default: 5)
    # max_results: 5

# Security Notes:
# - Never commit real API keys to version control
# - Rotate API keys regularly

# Usage:
#   result, err := filesearch.Ask(ctx, "怎么安装app")
#   result, err := filesearch.Ask(ctx, "这是什么",
#       filesearch.WithImage("https://example.com/screenshot.png"),
#       filesearch.WithHistory(history),
#   )
```

- [ ] **Step 4: Add to go.work and verify build**

Add `./openai/filesearch` to go.work `use` block.

```bash
cd /Users/david/projects/wordgate/qtoolkit && go work sync
cd openai/filesearch && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add openai/ go.work
git commit -m "feat(openai/filesearch): add knowledge base Q&A module with file_search"
```

---

### Task 2: openai/filesearch module — tests

**Files:**
- Create: `openai/filesearch/filesearch_test.go`

- [ ] **Step 1: Write tests**

Tests to write:
1. `TestAsk_Success` — mock returns valid response with annotations, verify Content + Score + Citations
2. `TestAsk_WithHistory` — verify request body includes history messages in input array
3. `TestAsk_WithImage` — verify request body has multimodal content with input_image
4. `TestAsk_WithHistoryImages` — verify history messages with Images produce multimodal content
5. `TestAsk_ServerError` — mock returns 500, verify error
6. `TestAsk_EmptyQuestion` — empty question, verify error
7. `TestAsk_NoAPIKey` — no api key configured, verify error
8. `TestAsk_NoVectorStoreID` — no vector store ID, verify error
9. `TestBuildMessageContent_TextOnly` — verify plain string output
10. `TestBuildMessageContent_WithImages` — verify multimodal array output
11. `TestParseResponse_NoCitations` — response with no annotations, verify Score=0

```go
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

// resetState() is defined in filesearch.go

func TestAsk_Success(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", r.URL.Path)
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
	// First message is history user
	msg0 := input[0].(map[string]any)
	if msg0["role"] != "user" {
		t.Errorf("input[0].role = %v, want user", msg0["role"])
	}
	// Last message is current question
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
	// First message should have multimodal content (text + image)
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
}
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit/openai/filesearch && go test -v -cover ./...`
Expected: all 11 tests PASS

- [ ] **Step 3: Commit**

```bash
git add openai/filesearch/filesearch_test.go
git commit -m "test(openai/filesearch): add Ask() tests with httptest mocks"
```

---

### Task 3: openai/filesearch — management API

**Files:**
- Create: `openai/filesearch/manage.go`

- [ ] **Step 1: Write manage.go**

```go
package filesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// CreateStore creates a new OpenAI vector store.
// Returns the store ID.
func CreateStore(ctx context.Context, name string) (string, error) {
	cfg := getConfig()
	if cfg.APIKey == "" {
		return "", fmt.Errorf("filesearch: api_key is required")
	}

	reqBody, _ := json.Marshal(map[string]string{"name": name})

	url := baseURL + "/v1/vector_stores"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("filesearch: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("filesearch: create store failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("filesearch: unmarshal error: %w", err)
	}

	return result.ID, nil
}

// UploadFile uploads a file to an OpenAI vector store.
// The file is first uploaded via the Files API, then attached to the vector store.
func UploadFile(ctx context.Context, storeID string, filename string, reader io.Reader) error {
	cfg := getConfig()
	if cfg.APIKey == "" {
		return fmt.Errorf("filesearch: api_key is required")
	}

	// Step 1: Upload file via Files API
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("purpose", "assistants")
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("filesearch: create form file error: %w", err)
	}
	if _, err := io.Copy(part, reader); err != nil {
		return fmt.Errorf("filesearch: copy file error: %w", err)
	}
	writer.Close()

	url := baseURL + "/v1/files"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("filesearch: upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("filesearch: upload failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var fileResult struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &fileResult); err != nil {
		return fmt.Errorf("filesearch: unmarshal error: %w", err)
	}

	// Step 2: Attach file to vector store
	attachBody, _ := json.Marshal(map[string]string{"file_id": fileResult.ID})
	url = fmt.Sprintf("%s/v1/vector_stores/%s/files", baseURL, storeID)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(attachBody))
	if err != nil {
		return fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err = httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("filesearch: attach file failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ = io.ReadAll(resp.Body)
		return fmt.Errorf("filesearch: attach file failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Search performs a direct vector search without LLM generation.
// Useful for debugging and testing retrieval quality.
func Search(ctx context.Context, query string) ([]Citation, error) {
	cfg := getConfig()
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("filesearch: api_key is required")
	}
	if cfg.VectorStoreID == "" {
		return nil, fmt.Errorf("filesearch: vector_store_id is required")
	}

	reqBody, _ := json.Marshal(map[string]any{
		"query":       query,
		"max_results": cfg.MaxResults,
	})

	url := fmt.Sprintf("%s/v1/vector_stores/%s/search", baseURL, cfg.VectorStoreID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("filesearch: search failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("filesearch: search failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var searchResp struct {
		Data []struct {
			FileName string  `json:"filename"`
			Score    float64 `json:"score"`
			Content  []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return nil, fmt.Errorf("filesearch: unmarshal error: %w", err)
	}

	var citations []Citation
	for _, d := range searchResp.Data {
		text := ""
		if len(d.Content) > 0 {
			text = d.Content[0].Text
		}
		citations = append(citations, Citation{
			FileName: d.FileName,
			Score:    d.Score,
			Text:     text,
		})
	}

	return citations, nil
}
```

- [ ] **Step 2: Verify build**

Run: `cd /Users/david/projects/wordgate/qtoolkit/openai/filesearch && go build ./...`

- [ ] **Step 3: Commit**

```bash
git add openai/filesearch/manage.go
git commit -m "feat(openai/filesearch): add CreateStore, UploadFile, Search management APIs"
```

---

### Task 4: chatwoot module — add GetMessages + Message type

**Files:**
- Modify: `chatwoot/chatwoot.go`
- Modify: `chatwoot/chatwoot_test.go`

- [ ] **Step 1: Add Message type and GetMessages to chatwoot.go**

Add after the `Reply()` function (before `webhookPayload`):

```go
// Message represents a conversation message for history retrieval.
type Message struct {
	Role    string   // "user" (from contact) / "assistant" (from agent/bot)
	Content string   // Text content
	Images  []string // Image attachment URLs
}

// messageRoleMap maps Chatwoot message_type integers to role strings.
var messageRoleMap = map[int]string{
	0: "user",      // incoming (from contact)
	1: "assistant", // outgoing (from agent/bot)
	3: "assistant", // template/bot auto-response
}

// messagesPayload is the Chatwoot messages API response.
type messagesPayload struct {
	Payload []messageItem `json:"payload"`
}

type messageItem struct {
	Content     *string `json:"content"` // pointer: can be null for image-only messages
	MessageType int     `json:"message_type"`
	Attachments []struct {
		FileType string `json:"file_type"`
		DataURL  string `json:"data_url"`
	} `json:"attachments"`
}

// GetMessages fetches recent messages from a Chatwoot conversation.
// limit controls max messages to fetch. Messages are returned in chronological order.
// Maps incoming messages to Role:"user", outgoing to Role:"assistant".
// Extracts image attachment URLs into the Images field.
// Skips activity messages and messages with no content and no images.
func GetMessages(ctx context.Context, conversationID int, limit int) ([]Message, error) {
	cfg := getConfig()
	if cfg == nil {
		return nil, fmt.Errorf("chatwoot: not configured")
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages",
		cfg.BaseURL, cfg.AccountID, conversationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: request error: %w", err)
	}
	req.Header.Set("api_access_token", cfg.APIToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatwoot: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chatwoot: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var msgResp messagesPayload
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return nil, fmt.Errorf("chatwoot: decode error: %w", err)
	}

	var messages []Message
	for _, item := range msgResp.Payload {
		role, ok := messageRoleMap[item.MessageType]
		if !ok {
			continue // skip activity and unknown types
		}

		var images []string
		for _, att := range item.Attachments {
			if att.FileType == "image" {
				images = append(images, att.DataURL)
			}
		}

		// Handle null content (image-only messages)
		content := ""
		if item.Content != nil {
			content = *item.Content
		}

		// Skip messages with no content and no images
		if content == "" && len(images) == 0 {
			continue
		}

		messages = append(messages, Message{
			Role:    role,
			Content: content,
			Images:  images,
		})
	}

	// Apply limit (payload is newest first, we want chronological order)
	// Reverse to chronological, then take last N
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	return messages, nil
}
```

- [ ] **Step 2: Add GetMessages tests to chatwoot_test.go**

Append these tests:

```go
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
				{"content": "activity", "message_type": 2},     // activity — skipped
				{"content": "", "message_type": 0},              // empty — skipped
				{"content": nil, "message_type": 0},             // null content — skipped
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
	// Should be the most recent 2 in chronological order
	if msgs[0].Content != "msg2" {
		t.Errorf("msgs[0].Content = %q, want msg2", msgs[0].Content)
	}
	if msgs[1].Content != "msg3" {
		t.Errorf("msgs[1].Content = %q, want msg3", msgs[1].Content)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit/chatwoot && go test -v -cover ./...`
Expected: all tests PASS (previous 15 + 5 new = 20)

- [ ] **Step 4: Commit**

```bash
git add chatwoot/chatwoot.go chatwoot/chatwoot_test.go
git commit -m "feat(chatwoot): add GetMessages() for conversation history with image URLs"
```

---

### Task 5: Integration verification, CLAUDE.md, and config paths

**Files:**
- Modify: `CLAUDE.md` (add openai/filesearch config path)
- Verify: all modules build and test

- [ ] **Step 1: Verify all builds**

```bash
cd /Users/david/projects/wordgate/qtoolkit/openai/filesearch && go build ./... && go test -cover ./...
cd /Users/david/projects/wordgate/qtoolkit/chatwoot && go build ./... && go test -cover ./...
```

- [ ] **Step 2: Add config path to CLAUDE.md**

Add to the Configuration Path Reference Table:

```
| **OpenAI File Search** | `openai.filesearch.*` → `ai.providers.openai.*` | 2 levels | `openai.filesearch.api_key` → `ai.providers.openai.api_key` |
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add openai/filesearch config path to CLAUDE.md"
```
