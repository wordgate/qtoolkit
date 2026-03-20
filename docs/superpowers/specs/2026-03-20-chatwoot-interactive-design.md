# Chatwoot Interactive Messages & Event Enhancements

**Date:** 2026-03-20
**Module:** `qtoolkit/chatwoot`

## Problem

The chatwoot module only supports plain text replies (`Reply()`). Users need to send interactive messages (option buttons, image cards) and handle user interactions (button clicks via `message_updated` events). Each new use case shouldn't require a new bespoke function — the API should be generic enough to cover Chatwoot's interactive message types while remaining self-documenting.

## Design

### 1. SendOptions — Option Button Messages

Sends `input_select` messages. Users see clickable buttons; clicking triggers a `message_updated` webhook with the selected value.

```go
func SendOptions(ctx context.Context, conversationID int, text string, options ...Option) error

type Option struct {
    Title string // Display text on the button
    Value string // Value sent back via webhook
}

func NewOption(title, value string) Option
```

**Chatwoot API mapping (verified):**
```json
{
  "content": "<text>",
  "content_type": "input_select",
  "message_type": "outgoing",
  "content_attributes": {
    "items": [
      {"title": "<Option.Title>", "value": "<Option.Value>"}
    ]
  }
}
```

**Validation:**
- At least 1 option required
- Title and Value must be non-empty

### 2. SendCards — Image/Link Card Messages

Sends `cards` messages for displaying images, descriptions, and links. Purely presentational — no postback interaction.

```go
func SendCards(ctx context.Context, conversationID int, text string, cards ...Card) error

type Card struct {
    Title       string // Card title (required)
    Description string // Card description
    ImageURL    string // Image URL (optional)
    Actions     []CardAction // At least 1 required (Chatwoot enforces this)
}

type CardAction struct {
    Text string // Button display text
    URL  string // Link URL
}

func NewCard(title string) *Card              // Start building a card
func (c *Card) Desc(description string) *Card // Set description
func (c *Card) Image(url string) *Card        // Set image URL
func (c *Card) Link(text, url string) *Card   // Add a link action (can chain)
```

Usage:
```go
chatwoot.SendCards(ctx, convID, "推荐产品：",
    *chatwoot.NewCard("产品 A").Desc("描述...").Image("https://img.com/a.jpg").Link("查看", "https://example.com/a"),
    *chatwoot.NewCard("产品 B").Desc("描述...").Link("了解更多", "https://example.com/b"),
)
```

**Chatwoot API mapping (verified):**
```json
{
  "content": "<text>",
  "content_type": "cards",
  "message_type": "outgoing",
  "content_attributes": {
    "items": [
      {
        "title": "<Card.Title>",
        "description": "<Card.Description>",
        "media_url": "<Card.ImageURL>",
        "actions": [
          {"type": "link", "text": "<CardAction.Text>", "uri": "<CardAction.URL>"}
        ]
      }
    ]
  }
}
```

**Constraints (verified by testing):**
- Each card MUST have at least 1 action (Chatwoot rejects empty actions)
- `media_url` is optional (verified: cards without images work)
- Only `type: "link"` actions are supported (postback doesn't trigger webhooks)

### 3. Event Enhancements — SubmittedValues

When a user clicks an `input_select` option, Chatwoot fires a `message_updated` webhook. The Event struct needs to carry the submitted values.

```go
type Event struct {
    // ... existing fields ...
    ContentType     string           // "text", "input_select", "cards", etc.
    SubmittedValues []SubmittedValue // Populated on message_updated for interactive messages
}

type SubmittedValue struct {
    Title string // Display text of selected option
    Value string // Programmatic value
}
```

**Chatwoot webhook payload mapping (verified):**
- `content_type` → `Event.ContentType`
- `content_attributes.submitted_values[].title` → `SubmittedValue.Title`
- `content_attributes.submitted_values[].value` → `SubmittedValue.Value`

### 4. Usage Example — Full Flow

```go
// Send welcome options on conversation_created
chatwoot.Mount(r, "/chatwoot/webhook", func(ctx context.Context, event chatwoot.Event) {
    switch event.EventType {
    case "conversation_created":
        chatwoot.SendOptions(ctx, event.ConversationID, "欢迎！请选择：",
            chatwoot.NewOption("技术支持", "support"),
            chatwoot.NewOption("产品咨询", "inquiry"),
            chatwoot.NewOption("转人工客服", "human"),
        )

    case "message_updated":
        if len(event.SubmittedValues) > 0 {
            selected := event.SubmittedValues[0].Value
            // Handle selection...
        }

    case "message_created":
        // Normal AI flow for text messages...
    }
})
```

## Verified Chatwoot API Facts

All verified against real instance `chat.anc.52j.me`:

| Fact | Status |
|------|--------|
| `input_select` items format: `[{title, value}]` | Verified (msg 2981) |
| User click populates `submitted_values: [{title, value}]` | Verified (fetched msg 2981) |
| `message_updated` webhook fires on click | Verified (Chatwoot source + docs) |
| `cards` items require `actions` (non-empty) | Verified (empty → validation error) |
| `cards` `media_url` is optional | Verified (msg 2998) |
| `cards` action format: `{type: "link", text, uri}` | Verified (msg 2996) |
| API endpoint: same `/messages` with `content_type` field | Verified |
| Default page size: 20 (hardcoded in Chatwoot) | Verified |

## Files to Modify

- `chatwoot/chatwoot.go` — Add types, SendOptions, SendCards, update parseWebhook for ContentType/SubmittedValues
- `chatwoot/chatwoot_test.go` — Tests for new functions and event parsing
