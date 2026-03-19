# OpenAI File Search Module Design

## Background

The `openai/filesearch` module provides knowledge base Q&A using OpenAI's Responses API + file_search tool. It serves the same purpose as the `fastgpt` module (RAG-based document Q&A with confidence scoring) but uses OpenAI's managed vector store instead of a self-hosted FastGPT instance.

Additionally, `chatwoot` module gains `GetMessages()` to fetch conversation history, enabling context-aware Q&A.

## Architecture

```
User (Chatwoot widget) → Chatwoot webhook → Backend
    ↓
chatwoot.GetMessages(ctx, conversationID, 20)  → fetch history (text + image URLs)
    ↓
filesearch.Ask(ctx, question, WithHistory(...)) → OpenAI Responses API + file_search
    ↓
chatwoot.Reply(ctx, conversationID, answer)     → reply to user
```

## Module Design

### 1. `openai/filesearch/` Module

#### Core API: Knowledge Base Q&A

```go
// Simple query (no context)
result, err := filesearch.Ask(ctx, "怎么安装app")

// With conversation history
msgs, _ := chatwoot.GetMessages(ctx, event.ConversationID, 20)
history := make([]filesearch.Message, len(msgs))
for i, m := range msgs {
    history[i] = filesearch.Message{Role: m.Role, Content: m.Content, Images: m.Images}
}
result, err := filesearch.Ask(ctx, event.Content,
    filesearch.WithHistory(history),
)

// With image on current question
result, err := filesearch.Ask(ctx, "这是什么界面",
    filesearch.WithImage("https://s3.../screenshot.png"),
)

// Check confidence for human handoff
if result.Score < 0.7 {
    return // transfer to human
}
```

#### Types

```go
// Message represents a conversation message (history).
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
```

#### Ask() Signature

```go
// Ask queries the knowledge base.
// question is always the current user question (explicit first parameter).
// Options provide conversation history, images, etc.
func Ask(ctx context.Context, question string, opts ...Option) (Result, error)

// WithHistory provides conversation context (preceding messages).
func WithHistory(messages []Message) Option

// WithImage attaches an image to the current question.
// OpenAI downloads the image directly from the URL.
func WithImage(url string) Option
```

#### Image Handling

- **OpenAI downloads images directly from URLs** — no upload/proxy needed
- Images in `Message.Images` are sent as `{"type": "input_image", "image_url": "..."}` in the multimodal content array
- `WithImage()` attaches an image to the current question
- All images use `detail: "low"` (fixed 85 tokens per image) — sufficient for UI screenshots in installation guides
- Chatwoot's `data_url` (ActiveStorage signed URLs) are publicly accessible, OpenAI can fetch them directly

#### Management API (Optional)

```go
// CreateStore creates a new vector store. Returns store ID.
func CreateStore(ctx context.Context, name string) (string, error)

// UploadFile uploads a file to a vector store.
func UploadFile(ctx context.Context, storeID string, filename string, reader io.Reader) error

// Search performs direct vector search without LLM (for debugging/testing).
func Search(ctx context.Context, query string) ([]Citation, error)
```

#### Configuration (viper)

```yaml
openai:
  filesearch:
    api_key: "YOUR_OPENAI_API_KEY"
    vector_store_id: "vs_xxxxx"      # Pre-created vector store
    model: "gpt-4o-mini"             # Model for generating answers
    score_threshold: 0.0             # Minimum score filter (default: 0, no filter)
    max_results: 5                   # Max retrieved chunks (default: 5)
```

Cascading fallback: `openai.filesearch.api_key` → `ai.providers.openai.api_key`

#### Internal Implementation (Verified with real API 2026-03-19)

- `POST https://api.openai.com/v1/responses`
- Header: `Authorization: Bearer {api_key}`
- Body:
  ```json
  {
    "model": "gpt-4o-mini",
    "input": [
      {"role": "user", "content": "怎么安装app"}
    ],
    "tools": [{
      "type": "file_search",
      "vector_store_ids": ["vs_xxxxx"],
      "max_num_results": 5
    }],
    "tool_choice": "required",
    "include": ["file_search_call.results"]
  }
  ```
- **`include: ["file_search_call.results"]` is REQUIRED** to get search results with scores. Without it, `file_search_call.results` is `null`.
- Multimodal message content (when images present):
  ```json
  {
    "role": "user",
    "content": [
      {"type": "input_text", "text": "这个界面怎么操作"},
      {"type": "input_image", "image_url": "https://...", "detail": "low"}
    ]
  }
  ```
- Multi-turn: `input` contains full message history + current question as last entry
- **Response `output` is an array** with two items:
  1. `{"type": "file_search_call", "results": [...]}` — search results with scores
  2. `{"type": "message", "content": [...]}` — AI answer text
- **Extract `Score` and `Citations` from `file_search_call.results[]`**, each result has:
  - `filename`: string (source document name)
  - `score`: float64 (0-1, e.g., 0.8513 for match, 0.0047 for no match)
  - `text`: string (retrieved text chunk)
  - `file_id`, `vector_store_id`: identifiers
- **`message.content[].annotations[]`** only has `file_citation` with `file_id`, `filename`, `index` — **NO score or text**
- Even unrelated queries return results with very low scores (e.g., 0.0047), not empty results
- Lazy-load config (sync.Once + viper), shared `*http.Client` with 60s timeout (file_search can be slow)

#### File Structure

```
openai/
└── filesearch/
    ├── go.mod
    ├── filesearch.go          # Ask() + config + types + options
    ├── manage.go              # CreateStore() + UploadFile() + Search()
    ├── filesearch_test.go
    └── filesearch_config.yml
```

### 2. `chatwoot/` Module Enhancement

#### New API: GetMessages

```go
// GetMessages fetches recent messages from a Chatwoot conversation.
// limit controls max messages returned (most recent first, reversed to chronological order).
// Returns messages with text content and image URLs from attachments.
func GetMessages(ctx context.Context, conversationID int, limit int) ([]Message, error)

// Message represents a Chatwoot conversation message.
type Message struct {
    Role    string   // "user" (from contact) / "assistant" (from agent/bot)
    Content string   // Text content
    Images  []string // Image attachment URLs (from attachments with file_type="image")
}
```

Internal (Verified with real API 2026-03-19): `GET {base_url}/api/v1/accounts/{account_id}/conversations/{conversation_id}/messages`
- Header: `api_access_token: {api_token}`
- Response structure: `{"meta": {...}, "payload": [...]}`
- Payload is **newest first** — must reverse to chronological order, then take last N for limit
- No `limit` query parameter — API returns a batch (~20 messages), use `?before=<message_id>` for pagination
- Maps Chatwoot message types to roles:
  - `message_type: 0 (incoming)` → `Role: "user"`
  - `message_type: 1 (outgoing)` → `Role: "assistant"`
  - `message_type: 3 (template/bot)` → `Role: "assistant"`
  - `message_type: 2 (activity)` → **skipped**
- `content` field can be `null` (e.g., pure image messages) — treat as empty string
- Outgoing (1) and template (3) messages may not have `sender` field
- Extracts `data_url` from attachments with `file_type: "image"` → `Images` field
- Attachment fields include: `id`, `file_type`, `data_url`, `thumb_url`, `file_size`, `width`, `height`
- Messages with no content AND no images are skipped

## Usage: Complete Example

```go
func main() {
    viper.SetConfigFile("config.yml")
    viper.ReadInConfig()

    r := gin.Default()

    chatwoot.Mount(r, "/chatwoot/webhook", func(ctx context.Context, event chatwoot.Event) {
        if event.EventType != "message_created" {
            return
        }
        if event.Sender.Type != "contact" || event.MessageType != "incoming" {
            return
        }
        if strings.TrimSpace(event.Content) == "" && len(event.Attachments) == 0 {
            return
        }

        // Fetch conversation history (text + image URLs)
        cwMsgs, _ := chatwoot.GetMessages(ctx, event.ConversationID, 20)
        history := make([]filesearch.Message, len(cwMsgs))
        for i, m := range cwMsgs {
            history[i] = filesearch.Message{Role: m.Role, Content: m.Content, Images: m.Images}
        }

        // Build options
        opts := []filesearch.Option{filesearch.WithHistory(history)}

        // Attach current image if present
        for _, att := range event.Attachments {
            if att.FileType == "image" {
                opts = append(opts, filesearch.WithImage(att.DataURL))
            }
        }

        // Query knowledge base
        question := event.Content
        if question == "" {
            question = "Please describe this image"
        }
        result, err := filesearch.Ask(ctx, question, opts...)
        if err != nil || result.Score < 0.7 {
            return // Transfer to human
        }

        chatwoot.Reply(ctx, event.ConversationID, result.Content)
    })

    r.Run(":8080")
}
```

```yaml
# config.yml
openai:
  filesearch:
    api_key: "sk-xxxxx"
    vector_store_id: "vs_xxxxx"
    model: "gpt-4o-mini"
    max_results: 5

chatwoot:
  api_token: "YOUR_CHATWOOT_API_TOKEN"
  base_url: "https://your-chatwoot.com"
  account_id: 1
  webhook_token: "YOUR_WEBHOOK_SECRET"
```

## Design Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Module path | `openai/filesearch/` | Follows `aws/s3/` pattern — vendor/service |
| Ask() signature | `Ask(ctx, question string, opts ...Option)` | Current question always explicit; history/image are options |
| Message.Images | `[]string` of URLs | OpenAI downloads images directly from URLs, no upload needed |
| Image detail level | `detail: "low"` (85 tokens) | UI screenshots don't need high resolution |
| History images | Preserved in Message.Images | Avoids losing context when user sends image + follow-up text |
| No shared types | Each module defines own Message type | No cross-module dependency |
| Config fallback | `openai.filesearch.api_key` → `ai.providers.openai.api_key` | Reuse existing OpenAI key |
| Conversation context | Fetch from Chatwoot | Single source of truth, zero extra storage |
| HTTP timeout | 60s (not 30s) | file_search + LLM can take longer than simple chat |
| Separate manage.go | Management APIs in own file | Ask is the core; store/upload is optional |

## What This Module Does NOT Do

- No conversation state management (Chatwoot is the source of truth)
- No file chunking/embedding (OpenAI handles it)
- No streaming (Chatwoot can't use it)
- No image upload/proxy (OpenAI fetches from URL directly)
- No vector store lifecycle management (create via OpenAI dashboard or one-time script)
