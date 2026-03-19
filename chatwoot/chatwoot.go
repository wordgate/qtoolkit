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
// limit controls max messages returned. Messages are returned in chronological order.
// Maps incoming messages to Role:"user", outgoing/template to Role:"assistant".
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

	// Payload is newest first — reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	// Apply limit (keep most recent N)
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	return messages, nil
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
