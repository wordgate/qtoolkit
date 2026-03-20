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
	EventType       string
	Content         string
	ContentType     string // "text", "input_select", "cards", etc.
	ConversationID  int
	MessageID       int
	InboxID         int
	AssigneeID      int    // 0 = unassigned (bot handles), >0 = human agent assigned
	MessageType     string // "incoming" / "outgoing" / "activity"
	Sender          Sender
	Conversation    Conversation
	Attachments     []Attachment
	SubmittedValues []SubmittedValue // populated on message_updated for interactive messages
}

// SubmittedValue represents a user's selection from an interactive message.
type SubmittedValue struct {
	Title string
	Value string
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

// Option represents a selectable option in an input_select message.
type Option struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

// NewOption creates a selectable option with display title and callback value.
func NewOption(title, value string) Option {
	return Option{Title: title, Value: value}
}

// SendOptions sends an interactive option-button message.
// Users see clickable buttons; clicking triggers a message_updated webhook
// with the selected value in Event.SubmittedValues.
func SendOptions(ctx context.Context, conversationID int, text string, options ...Option) error {
	if len(options) == 0 {
		return fmt.Errorf("chatwoot: at least one option is required")
	}
	return sendInteractive(ctx, conversationID, text, "input_select", map[string]any{
		"items": options,
	})
}

// CardAction represents a link button on a card.
type CardAction struct {
	Type string `json:"type"`
	Text string `json:"text"`
	URI  string `json:"uri"`
}

// Card represents a rich card with optional image, description, and link actions.
type Card struct {
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	MediaURL    string       `json:"media_url,omitempty"`
	Actions     []CardAction `json:"actions"`
}

// NewCard creates a card builder. Chain Desc(), Image(), and Link() to configure it.
func NewCard(title string) *Card {
	return &Card{Title: title}
}

// Desc sets the card description.
func (c *Card) Desc(description string) *Card {
	c.Description = description
	return c
}

// Image sets the card image URL.
func (c *Card) Image(url string) *Card {
	c.MediaURL = url
	return c
}

// Link adds a link button to the card.
func (c *Card) Link(text, url string) *Card {
	c.Actions = append(c.Actions, CardAction{Type: "link", Text: text, URI: url})
	return c
}

// SendCards sends a rich card message with images, descriptions, and link buttons.
// Each card must have at least one Link action (enforced by Chatwoot).
func SendCards(ctx context.Context, conversationID int, text string, cards ...Card) error {
	if len(cards) == 0 {
		return fmt.Errorf("chatwoot: at least one card is required")
	}
	for i, card := range cards {
		if len(card.Actions) == 0 {
			return fmt.Errorf("chatwoot: card %d (%q) must have at least one Link action", i, card.Title)
		}
	}
	return sendInteractive(ctx, conversationID, text, "cards", map[string]any{
		"items": cards,
	})
}

func sendInteractive(ctx context.Context, conversationID int, text, contentType string, contentAttrs map[string]any) error {
	cfg := getConfig()
	if cfg == nil {
		return fmt.Errorf("chatwoot: not configured")
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages",
		cfg.BaseURL, cfg.AccountID, conversationID)

	payload := map[string]any{
		"content":             text,
		"content_type":        contentType,
		"content_attributes":  contentAttrs,
		"message_type":        "outgoing",
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
	ID          int     `json:"id"`
	Content     *string `json:"content"` // pointer: can be null for image-only messages
	MessageType int     `json:"message_type"`
	Attachments []struct {
		FileType string `json:"file_type"`
		DataURL  string `json:"data_url"`
	} `json:"attachments"`
}

// GetMessages fetches messages from a Chatwoot conversation with automatic pagination.
// It paginates backwards through history using the `before` cursor until all messages
// are retrieved. limit controls max messages returned (0 = all). Messages are returned
// in chronological order.
func GetMessages(ctx context.Context, conversationID int, limit int) ([]Message, error) {
	cfg := getConfig()
	if cfg == nil {
		return nil, fmt.Errorf("chatwoot: not configured")
	}

	baseURL := fmt.Sprintf("%s/api/v1/accounts/%d/conversations/%d/messages",
		cfg.BaseURL, cfg.AccountID, conversationID)

	const chatwootPageSize = 20 // hardcoded in Chatwoot backend
	var allItems []messageItem
	beforeID := 0 // 0 means no cursor (first request)

	for {
		reqURL := baseURL
		if beforeID > 0 {
			reqURL = fmt.Sprintf("%s?before=%d", baseURL, beforeID)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("chatwoot: request error: %w", err)
		}
		req.Header.Set("api_access_token", cfg.APIToken)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("chatwoot: request failed: %w", err)
		}

		if resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("chatwoot: status %d, body: %s", resp.StatusCode, string(respBody))
		}

		var msgResp messagesPayload
		if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("chatwoot: decode error: %w", err)
		}
		resp.Body.Close()

		if len(msgResp.Payload) == 0 {
			break
		}

		// Payload is oldest-first (ascending ID); prepend older pages
		allItems = append(msgResp.Payload, allItems...)

		// If fewer than page size, we've reached the beginning
		if len(msgResp.Payload) < chatwootPageSize {
			break
		}

		// First element has the smallest ID; use it as the next cursor
		beforeID = msgResp.Payload[0].ID
	}

	// Convert to Message structs
	var messages []Message
	for _, item := range allItems {
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

		content := ""
		if item.Content != nil {
			content = *item.Content
		}

		if content == "" && len(images) == 0 {
			continue
		}

		messages = append(messages, Message{
			Role:    role,
			Content: content,
			Images:  images,
		})
	}

	// allItems is already in chronological order (oldest first); keep most recent N
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	return messages, nil
}

// webhookPayload is the raw Chatwoot webhook JSON structure.
type webhookPayload struct {
	Event       string          `json:"event"`
	ID          int             `json:"id"`
	Content     string          `json:"content"`
	ContentType string          `json:"content_type"`
	MessageType json.RawMessage `json:"message_type"` // string ("incoming") or int (0)
	ContentAttributes struct {
		SubmittedValues []struct {
			Title string `json:"title"`
			Value string `json:"value"`
		} `json:"submitted_values"`
	} `json:"content_attributes"`
	Inbox struct {
		ID int `json:"id"`
	} `json:"inbox"`
	Sender struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"sender"`
	Conversation struct {
		ID      int    `json:"id"`
		Status  string `json:"status"`
		InboxID int    `json:"inbox_id"`
		Meta    struct {
			Assignee *struct {
				ID int `json:"id"`
			} `json:"assignee"`
		} `json:"meta"`
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

// parseMessageType handles message_type as string or int.
func parseMessageType(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "unknown"
	}
	// Try string first (webhook sends "incoming"/"outgoing")
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Try int (Messages API sends 0/1/2)
	var i int
	if json.Unmarshal(raw, &i) == nil {
		if name, ok := messageTypeMap[i]; ok {
			return name
		}
		return fmt.Sprintf("unknown(%d)", i)
	}
	return "unknown"
}

func parseWebhook(body []byte) (Event, error) {
	var wp webhookPayload
	if err := json.Unmarshal(body, &wp); err != nil {
		return Event{}, fmt.Errorf("chatwoot: parse webhook error: %w", err)
	}

	msgType := parseMessageType(wp.MessageType)

	// InboxID: prefer inbox.id, fallback to conversation.inbox_id
	inboxID := wp.Inbox.ID
	if inboxID == 0 {
		inboxID = wp.Conversation.InboxID
	}

	// AssigneeID: from conversation.meta.assignee.id (0 if unassigned/null)
	var assigneeID int
	if wp.Conversation.Meta.Assignee != nil {
		assigneeID = wp.Conversation.Meta.Assignee.ID
	}

	// Sender type: if missing, infer from message type
	senderType := wp.Sender.Type
	if senderType == "" {
		switch msgType {
		case "incoming":
			senderType = "contact"
		case "outgoing":
			senderType = "user"
		}
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

	var submittedValues []SubmittedValue
	for _, sv := range wp.ContentAttributes.SubmittedValues {
		submittedValues = append(submittedValues, SubmittedValue{
			Title: sv.Title,
			Value: sv.Value,
		})
	}

	return Event{
		EventType:       wp.Event,
		Content:         wp.Content,
		ContentType:     wp.ContentType,
		ConversationID:  wp.Conversation.ID,
		MessageID:       wp.ID,
		InboxID:         inboxID,
		AssigneeID:      assigneeID,
		MessageType:     msgType,
		Sender: Sender{
			ID:   wp.Sender.ID,
			Name: wp.Sender.Name,
			Type: senderType,
		},
		Conversation: Conversation{
			Status: wp.Conversation.Status,
		},
		Attachments:     attachments,
		SubmittedValues: submittedValues,
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
