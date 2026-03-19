# FastGPT + Chatwoot Modules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create two qtoolkit modules — `fastgpt/` for FastGPT Chat API and `chatwoot/` for Chatwoot webhook + reply — enabling a Chatwoot → FastGPT knowledge-base chatbot pipeline.

**Architecture:** Two independent Go modules following qtoolkit v1.0 patterns (sync.Once lazy init, viper config, shared http.Client). `fastgpt/` wraps a single `POST /api/v1/chat/completions` call. `chatwoot/` provides `Mount()` for async webhook handling and `Reply()` for sending messages.

**Tech Stack:** Go 1.24.0, viper, gin, net/http, httptest

**Spec:** `docs/superpowers/specs/2026-03-19-fastgpt-chatwoot-design.md`

---

## File Structure

### `fastgpt/` module
| File | Responsibility |
|------|---------------|
| `fastgpt/go.mod` | Module definition: `github.com/wordgate/qtoolkit/fastgpt`, deps: viper |
| `fastgpt/fastgpt.go` | Config struct, loadConfigFromViper(), lazy init (sync.Once), shared http.Client, Result struct (Content + Similarity), `Chat(ctx, chatID, message) (Result, error)` |
| `fastgpt/fastgpt_test.go` | Tests using httptest mock server: success, error, empty response, config loading |
| `fastgpt/fastgpt_config.yml` | Config template with comments |

### `chatwoot/` module
| File | Responsibility |
|------|---------------|
| `chatwoot/go.mod` | Module definition: `github.com/wordgate/qtoolkit/chatwoot`, deps: viper, gin |
| `chatwoot/chatwoot.go` | Config struct, loadConfigFromViper(), lazy init, shared http.Client, Event/Sender/Conversation types, `Mount(r, path, handler)`, `Reply(ctx, conversationID, text) error`, webhook parsing, HMAC verification |
| `chatwoot/chatwoot_test.go` | Tests using httptest: Reply success/error, webhook parsing, HMAC verification, Mount async behavior |
| `chatwoot/chatwoot_config.yml` | Config template with comments |

---

### Task 1: fastgpt module — scaffold and config

**Files:**
- Create: `fastgpt/go.mod`
- Create: `fastgpt/fastgpt.go`
- Create: `fastgpt/fastgpt_config.yml`

- [ ] **Step 1: Create go.mod**

```
module github.com/wordgate/qtoolkit/fastgpt

go 1.24.0

require github.com/spf13/viper v1.21.0
```

Run: `cd /Users/david/projects/wordgate/qtoolkit/fastgpt && go mod init github.com/wordgate/qtoolkit/fastgpt`

- [ ] **Step 2: Write fastgpt.go with config + Chat()**

Create `fastgpt/fastgpt.go` with:
- Package doc comment with usage example (follow slack pattern)
- `Config` struct: `APIKey string`, `BaseURL string`
- `loadConfigFromViper()` reading `fastgpt.api_key`, `fastgpt.base_url`
- `initialize()` with shared `*http.Client` (30s timeout), trim trailing slash from BaseURL
- `ensureInitialized()` via `sync.Once`
- `SetConfig(cfg *Config)` for testing
- `resetState()` unexported for tests
- `Part` struct + constructors: `Text(string)`, `ImageURL(string)`, `FileURL(name, url string)`
- `Result` struct: `Content string`, `Similarity float64`
- `Chat(ctx context.Context, chatID string, parts ...Part) (Result, error)`:
  - POST `{base_url}/api/v1/chat/completions`
  - Header: `Authorization: Bearer {api_key}`, `Content-Type: application/json`
  - Body: `{"chatId": chatID, "stream": false, "detail": true, "messages": [{"role": "user", "content": ...}]}`
    - 单个 Text Part → content 为 string
    - 多个 Part 或非 Text → content 为数组 `[{"type":"text","text":"..."}, {"type":"image_url",...}, ...]`
  - Parse response JSON: `choices[0].message.content` → Result.Content
  - Parse `responseData` array: find element with `moduleType=datasetSearchNode`, extract max `similarity` → Result.Similarity
  - Return Result and error

```go
// Package fastgpt provides a minimal client for the FastGPT Chat API.
//
// Usage:
//
//	result, err := fastgpt.Chat(ctx, "conv-123", fastgpt.Text("怎么安装app"))
//	result, err := fastgpt.Chat(ctx, "conv-123",
//	    fastgpt.Text("这个界面怎么操作"),
//	    fastgpt.ImageURL("https://s3.../screenshot.png"),
//	)
package fastgpt

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

// Config holds FastGPT module configuration.
type Config struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMux    sync.RWMutex
	httpClient   *http.Client
)

func loadConfigFromViper() *Config {
	return &Config{
		APIKey:  viper.GetString("fastgpt.api_key"),
		BaseURL: viper.GetString("fastgpt.base_url"),
	}
}

func initialize() {
	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil {
		cfg = loadConfigFromViper()
		configMux.Lock()
		globalConfig = cfg
		configMux.Unlock()
	}

	// Trim trailing slash from BaseURL
	configMux.Lock()
	globalConfig.BaseURL = strings.TrimRight(globalConfig.BaseURL, "/")
	configMux.Unlock()

	httpClient = &http.Client{Timeout: 30 * time.Second}
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
	globalConfig = cfg
	globalConfig.BaseURL = strings.TrimRight(globalConfig.BaseURL, "/")
	httpClient = &http.Client{Timeout: 30 * time.Second}
}

func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	configOnce = sync.Once{}
	httpClient = nil
}

// Part represents a content fragment in a multimodal message.
type Part struct {
	partType string
	text     string
	url      string
	name     string
}

// Text creates a text content part.
func Text(text string) Part {
	return Part{partType: "text", text: text}
}

// ImageURL creates an image URL content part.
func ImageURL(url string) Part {
	return Part{partType: "image_url", url: url}
}

// FileURL creates a file URL content part.
// Supported formats: txt, md, html, word, pdf, ppt, csv, excel.
func FileURL(name, url string) Part {
	return Part{partType: "file_url", name: name, url: url}
}

// buildContent converts Parts to the FastGPT message content format.
// Single text part → string; multiple/non-text parts → array.
func buildContent(parts []Part) any {
	if len(parts) == 1 && parts[0].partType == "text" {
		return parts[0].text
	}
	var content []map[string]any
	for _, p := range parts {
		switch p.partType {
		case "text":
			content = append(content, map[string]any{
				"type": "text",
				"text": p.text,
			})
		case "image_url":
			content = append(content, map[string]any{
				"type":      "image_url",
				"image_url": map[string]string{"url": p.url},
			})
		case "file_url":
			content = append(content, map[string]any{
				"type": "file_url",
				"name": p.name,
				"url":  p.url,
			})
		}
	}
	return content
}

// request is the FastGPT chat completions request body.
type request struct {
	ChatID   string `json:"chatId,omitempty"`
	Stream   bool   `json:"stream"`
	Detail   bool   `json:"detail"`
	Messages []any  `json:"messages"`
}

// Result is the return value of Chat().
type Result struct {
	Content    string  // AI reply text
	Similarity float64 // Highest dataset search similarity (0-1), 0 if no dataset match
}

// response is the FastGPT chat completions response with detail=true.
type response struct {
	Choices      []choice       `json:"choices"`
	ResponseData []responseData `json:"responseData"`
}

type choice struct {
	Message chatMessage `json:"message"`
}

type chatMessage struct {
	Content string `json:"content"`
}

type responseData struct {
	ModuleType string          `json:"moduleType"`
	QuoteList  []quoteItem     `json:"quoteList"`
}

type quoteItem struct {
	Score float64 `json:"score"`
}

// Chat sends a message to FastGPT and returns the result with similarity score.
// chatID is used for multi-turn conversation context (pass Chatwoot conversation ID).
func Chat(ctx context.Context, chatID string, parts ...Part) (Result, error) {
	if len(parts) == 0 {
		return Result{}, fmt.Errorf("fastgpt: at least one part is required")
	}
	cfg := getConfig()
	if cfg.APIKey == "" {
		return Result{}, fmt.Errorf("fastgpt: api_key is required")
	}
	if cfg.BaseURL == "" {
		return Result{}, fmt.Errorf("fastgpt: base_url is required")
	}

	reqBody := request{
		ChatID: chatID,
		Stream: false,
		Detail: true,
		Messages: []any{
			map[string]any{
				"role":    "user",
				"content": buildContent(parts),
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return Result{}, fmt.Errorf("fastgpt: marshal error: %w", err)
	}

	url := cfg.BaseURL + "/api/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("fastgpt: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("fastgpt: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("fastgpt: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("fastgpt: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var chatResp response
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return Result{}, fmt.Errorf("fastgpt: unmarshal error: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return Result{}, fmt.Errorf("fastgpt: empty response (no choices)")
	}

	result := Result{
		Content: chatResp.Choices[0].Message.Content,
	}

	// Extract max similarity from dataset search nodes
	for _, rd := range chatResp.ResponseData {
		if rd.ModuleType == "datasetSearchNode" {
			for _, q := range rd.QuoteList {
				if q.Score > result.Similarity {
					result.Similarity = q.Score
				}
			}
		}
	}

	return result, nil
}
```

- [ ] **Step 3: Create fastgpt_config.yml**

```yaml
# FastGPT Configuration
# Add to your main config.yml

fastgpt:
  # Application-specific API key (includes AppId)
  # Create at: FastGPT Dashboard → App → API Access
  api_key: "fastgpt-xxxxxxxx"

  # FastGPT server base URL (self-hosted or cloud)
  base_url: "https://your-fastgpt.com"

# Usage:
#   reply, err := fastgpt.Chat(ctx, "conversation-id", "你好")
```

- [ ] **Step 4: Add to go.work and run go mod tidy**

Run:
```bash
cd /Users/david/projects/wordgate/qtoolkit
# Add ./fastgpt to go.work
cd fastgpt && go mod tidy
```

Add `./fastgpt` to go.work `use` block.

- [ ] **Step 5: Verify build**

Run: `cd /Users/david/projects/wordgate/qtoolkit/fastgpt && go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add fastgpt/ go.work
git commit -m "feat(fastgpt): add FastGPT Chat API module with viper config"
```

---

### Task 2: fastgpt module — tests

**Files:**
- Create: `fastgpt/fastgpt_test.go`

- [ ] **Step 1: Write tests**

Create `fastgpt/fastgpt_test.go` with tests using `httptest.NewServer` to mock FastGPT API. Follow the slack test pattern (resetState, SetConfig, httptest).

Tests to write:
1. `TestChat_Success` — mock returns valid response with responseData, verify Result.Content and Result.Similarity
2. `TestChat_WithChatID` — verify chatId is included in request body, detail=true
3. `TestChat_ServerError` — mock returns 500, verify error message includes status and body
4. `TestChat_EmptyChoices` — mock returns `{"choices":[]}`, verify "empty response" error
5. `TestChat_NoAPIKey` — SetConfig with empty api_key, verify error
6. `TestChat_NoBaseURL` — SetConfig with empty base_url, verify error
7. `TestChat_TrailingSlash` — SetConfig with trailing slash in base_url, verify URL is correct
8. `TestChat_NoDatasetMatch` — mock returns response without datasetSearchNode, verify Similarity=0
9. `TestChat_Multimodal` — send Text + ImageURL parts, verify request body has content array format
10. `TestChat_NoParts` — call with no parts, verify error

```go
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

	var receivedBody request
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

	if receivedBody.ChatID != "conv-123" {
		t.Errorf("chatId = %q, want %q", receivedBody.ChatID, "conv-123")
	}
	if receivedBody.Stream != false {
		t.Errorf("stream = %v, want false", receivedBody.Stream)
	}
	if receivedBody.Detail != true {
		t.Errorf("detail = %v, want true", receivedBody.Detail)
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
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit/fastgpt && go test -v ./...`
Expected: all 10 tests PASS

- [ ] **Step 3: Commit**

```bash
git add fastgpt/fastgpt_test.go
git commit -m "test(fastgpt): add Chat() tests with httptest mocks"
```

---

### Task 3: chatwoot module — scaffold, config, and Reply()

**Files:**
- Create: `chatwoot/go.mod`
- Create: `chatwoot/chatwoot.go`
- Create: `chatwoot/chatwoot_config.yml`

- [ ] **Step 1: Create go.mod**

Run: `mkdir -p /Users/david/projects/wordgate/qtoolkit/chatwoot && cd /Users/david/projects/wordgate/qtoolkit/chatwoot && go mod init github.com/wordgate/qtoolkit/chatwoot`

Dependencies: `github.com/spf13/viper`, `github.com/gin-gonic/gin`

- [ ] **Step 2: Write chatwoot.go**

Create `chatwoot/chatwoot.go` with:
- Package doc comment with usage example
- `Config` struct: `APIToken`, `BaseURL`, `AccountID int`, `WebhookToken` (optional)
- `loadConfigFromViper()` reading `chatwoot.*`, validate `account_id != 0`
- `initialize()` with shared `*http.Client` (30s timeout), trim trailing slash
- `ensureInitialized()` via `sync.Once`
- `SetConfig(cfg *Config)` for testing
- `resetState()` unexported for tests
- Event types: `Event`, `Sender`, `Conversation`
- `EventHandler` type: `func(ctx context.Context, event Event)`
- `Reply(ctx context.Context, conversationID int, text string) error`
- `Mount(r gin.IRouter, path string, handler EventHandler)`
- `parseWebhook(body []byte) (Event, error)` — parse Chatwoot webhook JSON into Event
- `verifySignature(body []byte, signature string) bool` — HMAC-SHA256 verification

```go
// Package chatwoot provides Chatwoot webhook handling and message reply.
//
// Usage:
//
//	chatwoot.Mount(r, "/webhook", func(ctx context.Context, event chatwoot.Event) {
//	    if event.EventType != "message_created" {
//	        return
//	    }
//	    chatwoot.Reply(ctx, event.ConversationID, "Hello!")
//	})
package chatwoot

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// Config holds Chatwoot module configuration.
type Config struct {
	APIToken     string `yaml:"api_token"`
	BaseURL      string `yaml:"base_url"`
	AccountID    int    `yaml:"account_id"`
	WebhookToken string `yaml:"webhook_token"`
}

// Event represents a Chatwoot webhook event.
type Event struct {
	EventType      string
	Content        string
	ConversationID int
	MessageType    string // "incoming" / "outgoing" / "activity"
	Sender         Sender
	Conversation   Conversation
	Attachments    []Attachment
}

// Sender represents the message sender.
type Sender struct {
	ID   int
	Name string
	Type string // "contact" / "user" / "agent_bot"
}

// Conversation represents conversation metadata.
type Conversation struct {
	Status string // "open" / "resolved" / "pending"
}

// Attachment represents a file attached to a message.
type Attachment struct {
	FileType string // "image" / "audio" / "video" / "file"
	DataURL  string // File URL
	ThumbURL string // Thumbnail URL (images only)
	FileSize int    // File size in bytes
}

// EventHandler is called for each webhook event.
type EventHandler func(ctx context.Context, event Event)

var (
	globalConfig *Config
	configOnce   sync.Once
	configMux    sync.RWMutex
	httpClient   *http.Client
)

func loadConfigFromViper() (*Config, error) {
	cfg := &Config{
		APIToken:     viper.GetString("chatwoot.api_token"),
		BaseURL:      viper.GetString("chatwoot.base_url"),
		AccountID:    viper.GetInt("chatwoot.account_id"),
		WebhookToken: viper.GetString("chatwoot.webhook_token"),
	}
	if cfg.AccountID == 0 {
		return nil, fmt.Errorf("chatwoot: account_id is required")
	}
	return cfg, nil
}

func initialize() {
	// Always init httpClient regardless of config success
	httpClient = &http.Client{Timeout: 30 * time.Second}

	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil {
		loaded, err := loadConfigFromViper()
		if err != nil {
			fmt.Fprintf(os.Stderr, "chatwoot: config error: %v\n", err)
			return
		}
		cfg = loaded
		configMux.Lock()
		globalConfig = cfg
		configMux.Unlock()
	}

	configMux.Lock()
	globalConfig.BaseURL = strings.TrimRight(globalConfig.BaseURL, "/")
	configMux.Unlock()
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
	globalConfig = cfg
	globalConfig.BaseURL = strings.TrimRight(globalConfig.BaseURL, "/")
	httpClient = &http.Client{Timeout: 30 * time.Second}
}

func resetState() {
	configMux.Lock()
	globalConfig = nil
	configMux.Unlock()
	configOnce = sync.Once{}
	httpClient = nil
}

// Reply sends a text message to a Chatwoot conversation.
func Reply(ctx context.Context, conversationID int, text string) error {
	cfg := getConfig()
	if cfg == nil {
		return fmt.Errorf("chatwoot: not configured")
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages",
		cfg.BaseURL, cfg.AccountID, conversationID)

	payload := map[string]string{
		"content":      text,
		"message_type": "outgoing",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("chatwoot: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("chatwoot: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", cfg.APIToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("chatwoot: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chatwoot: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// webhookPayload is the raw Chatwoot webhook JSON structure.
type webhookPayload struct {
	Event       string `json:"event"`
	Content     string `json:"content"`
	MessageType int    `json:"message_type"`
	Sender      struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"sender"`
	Conversation struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
	} `json:"conversation"`
	Attachments []struct {
		FileType string `json:"file_type"`
		DataURL  string `json:"data_url"`
		ThumbURL string `json:"thumb_url"`
		FileSize int    `json:"file_size"`
	} `json:"attachments"`
}

// messageTypeMap maps Chatwoot integer message types to strings.
var messageTypeMap = map[int]string{
	0: "incoming",
	1: "outgoing",
	2: "activity",
}

func parseWebhook(body []byte) (Event, error) {
	var wp webhookPayload
	if err := json.Unmarshal(body, &wp); err != nil {
		return Event{}, fmt.Errorf("chatwoot: parse webhook error: %w", err)
	}

	msgType := messageTypeMap[wp.MessageType]
	if msgType == "" {
		msgType = fmt.Sprintf("unknown(%d)", wp.MessageType)
	}

	var attachments []Attachment
	for _, a := range wp.Attachments {
		attachments = append(attachments, Attachment{
			FileType: a.FileType,
			DataURL:  a.DataURL,
			ThumbURL: a.ThumbURL,
			FileSize: a.FileSize,
		})
	}

	return Event{
		EventType:      wp.Event,
		Content:        wp.Content,
		ConversationID: wp.Conversation.ID,
		MessageType:    msgType,
		Sender: Sender{
			ID:   wp.Sender.ID,
			Name: wp.Sender.Name,
			Type: wp.Sender.Type,
		},
		Conversation: Conversation{
			Status: wp.Conversation.Status,
		},
		Attachments: attachments,
	}, nil
}

func verifySignature(body []byte, signature string, token string) bool {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// Mount mounts a Chatwoot webhook handler to the gin router.
// The handler is called asynchronously in a goroutine with a 60s timeout context.
// Mount only handles infrastructure (parse, verify, async dispatch).
// Business filtering (event type, sender type, etc.) is the handler's responsibility.
func Mount(r gin.IRouter, path string, handler EventHandler) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	r.POST(path, func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		// Verify HMAC signature if webhook_token is configured
		cfg := getConfig()
		if cfg != nil && cfg.WebhookToken != "" {
			signature := c.GetHeader("X-Chatwoot-Signature")
			if !verifySignature(body, signature, cfg.WebhookToken) {
				c.Status(http.StatusUnauthorized)
				return
			}
		}

		event, err := parseWebhook(body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		// Return 200 immediately, process async
		c.Status(http.StatusOK)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "chatwoot: handler panic: %v\n", r)
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			handler(ctx, event)
		}()
	})
}
```

- [ ] **Step 3: Create chatwoot_config.yml**

```yaml
# Chatwoot Configuration
# Add to your main config.yml

chatwoot:
  # API access token (from Chatwoot profile settings)
  api_token: "YOUR_CHATWOOT_API_TOKEN"

  # Chatwoot server base URL
  base_url: "https://your-chatwoot.com"

  # Account ID (required)
  account_id: 1

  # Webhook token for HMAC signature verification (optional)
  # Set in Chatwoot: Settings → Integrations → Webhooks
  # webhook_token: "YOUR_WEBHOOK_SECRET"

# Usage:
#   chatwoot.Mount(r, "/webhook", func(ctx context.Context, event chatwoot.Event) {
#       chatwoot.Reply(ctx, event.ConversationID, "Hello!")
#   })
```

- [ ] **Step 4: Add to go.work and run go mod tidy**

Add `./chatwoot` to go.work `use` block.

Run:
```bash
cd /Users/david/projects/wordgate/qtoolkit/chatwoot && go get github.com/spf13/viper@v1.21.0 && go get github.com/gin-gonic/gin@v1.11.0 && go mod tidy
```

- [ ] **Step 5: Verify build**

Run: `cd /Users/david/projects/wordgate/qtoolkit/chatwoot && go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add chatwoot/ go.work
git commit -m "feat(chatwoot): add Chatwoot webhook + reply module with HMAC verification"
```

---

### Task 4: chatwoot module — tests

**Files:**
- Create: `chatwoot/chatwoot_test.go`

- [ ] **Step 1: Write tests**

Tests to write:
1. `TestReply_Success` — mock Chatwoot API, verify correct URL/headers/body
2. `TestReply_ServerError` — mock returns 403, verify error
3. `TestReply_NotConfigured` — no config set, verify error
4. `TestParseWebhook_MessageCreated` — valid message_created payload, verify all Event fields
5. `TestParseWebhook_InvalidJSON` — invalid JSON, verify error
6. `TestParseWebhook_MessageTypes` — verify 0=incoming, 1=outgoing, 2=activity mapping
7. `TestVerifySignature_Valid` — compute correct HMAC, verify true
8. `TestVerifySignature_Invalid` — wrong signature, verify false
9. `TestParseWebhook_WithAttachments` — payload with image + audio attachments, verify Attachment fields
10. `TestMount_AsyncHandler` — mount to gin, POST webhook, verify handler is called and 200 returned immediately
11. `TestMount_HMACReject` — mount with webhook_token, POST without signature, verify 401

```go
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
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit/chatwoot && go test -v ./...`
Expected: all 12 tests PASS

- [ ] **Step 3: Commit**

```bash
git add chatwoot/chatwoot_test.go
git commit -m "test(chatwoot): add Reply, webhook parsing, HMAC, and Mount tests"
```

---

### Task 5: Integration verification and cleanup

**Files:**
- Modify: `go.work` (verify both modules listed)
- Modify: `CLAUDE.md` (add config paths for new modules)

- [ ] **Step 1: Verify workspace builds**

Run: `cd /Users/david/projects/wordgate/qtoolkit && go build ./...`
Expected: no errors

- [ ] **Step 2: Run all module tests**

Run: `cd /Users/david/projects/wordgate/qtoolkit && go test ./fastgpt/... ./chatwoot/...`
Expected: all tests PASS

- [ ] **Step 3: Add config paths to CLAUDE.md**

Add to the config path table in CLAUDE.md:

```
| **FastGPT** | `fastgpt.*` | 1级 | `fastgpt.api_key`, `fastgpt.base_url` |
| **Chatwoot** | `chatwoot.*` | 1级 | `chatwoot.api_token`, `chatwoot.base_url`, `chatwoot.account_id` |
```

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add FastGPT and Chatwoot config paths to CLAUDE.md"
```
