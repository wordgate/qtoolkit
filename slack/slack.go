// Package slack provides a minimal Slack webhook client.
//
// Usage:
//
//	slack.Send("alert", "Server is down!")
//
//	slack.To("alert").
//	    Text("Deployment completed").
//	    Color(slack.ColorGood).
//	    Field("Environment", "production", true).
//	    Send()
package slack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Errors
var (
	ErrNoWebhookURL = errors.New("slack: webhook URL not configured")
	ErrEmptyMessage = errors.New("slack: message content is empty")
	ErrSendFailed   = errors.New("slack: failed to send message")
)

// Colors
const (
	ColorGood    = "good"    // Green
	ColorWarning = "warning" // Yellow
	ColorDanger  = "danger"  // Red
)

// Config holds Slack module configuration.
type Config struct {
	Channels map[string]string `yaml:"channels"` // channel name -> webhook URL
	Timeout  time.Duration     `yaml:"timeout"`  // HTTP timeout (default: 10s)
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMux    sync.RWMutex
	httpClient   *http.Client
)

func loadConfigFromViper() *Config {
	cfg := &Config{
		Channels: make(map[string]string),
		Timeout:  10 * time.Second,
	}

	if timeout := viper.GetDuration("slack.timeout"); timeout > 0 {
		cfg.Timeout = timeout
	}

	// Load from slack.channels map
	if channels := viper.GetStringMapString("slack.channels"); len(channels) > 0 {
		cfg.Channels = channels
	}

	// Legacy: slack.alert, slack.notify, etc.
	if slackMap := viper.GetStringMap("slack"); len(slackMap) > 0 {
		for key, value := range slackMap {
			if key == "channels" || key == "timeout" {
				continue
			}
			if url, ok := value.(string); ok && url != "" {
				if _, exists := cfg.Channels[key]; !exists {
					cfg.Channels[key] = url
				}
			}
		}
	}

	return cfg
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

	httpClient = &http.Client{Timeout: cfg.Timeout}
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
	if cfg.Timeout > 0 {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
}

// Field in an attachment.
type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short,omitempty"`
}

// Attachment for rich messages.
type Attachment struct {
	Color      string  `json:"color,omitempty"`
	Title      string  `json:"title,omitempty"`
	TitleLink  string  `json:"title_link,omitempty"`
	Text       string  `json:"text,omitempty"`
	Pretext    string  `json:"pretext,omitempty"`
	Fields     []Field `json:"fields,omitempty"`
	Footer     string  `json:"footer,omitempty"`
	FooterIcon string  `json:"footer_icon,omitempty"`
	Timestamp  int64   `json:"ts,omitempty"`
}

type payload struct {
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// MessageBuilder builds Slack messages.
type MessageBuilder struct {
	channel     string
	text        string
	attachments []Attachment
	current     *Attachment
}

// To creates a MessageBuilder for the specified channel.
func To(channel string) *MessageBuilder {
	return &MessageBuilder{channel: channel}
}

// Text sets the message text.
func (b *MessageBuilder) Text(text string) *MessageBuilder {
	b.text = text
	return b
}

// Textf sets formatted message text.
func (b *MessageBuilder) Textf(format string, args ...any) *MessageBuilder {
	b.text = fmt.Sprintf(format, args...)
	return b
}

// Attachment adds a complete attachment.
func (b *MessageBuilder) Attachment(att Attachment) *MessageBuilder {
	b.flush()
	b.attachments = append(b.attachments, att)
	return b
}

// BeginAttachment starts a new attachment.
func (b *MessageBuilder) BeginAttachment() *MessageBuilder {
	b.flush()
	b.current = &Attachment{}
	return b
}

// EndAttachment finishes the current attachment.
func (b *MessageBuilder) EndAttachment() *MessageBuilder {
	b.flush()
	return b
}

func (b *MessageBuilder) ensure() {
	if b.current == nil {
		b.current = &Attachment{}
	}
}

func (b *MessageBuilder) flush() {
	if b.current != nil {
		b.attachments = append(b.attachments, *b.current)
		b.current = nil
	}
}

// Color sets attachment color.
func (b *MessageBuilder) Color(color string) *MessageBuilder {
	b.ensure()
	b.current.Color = color
	return b
}

// Title sets attachment title.
func (b *MessageBuilder) Title(title string) *MessageBuilder {
	b.ensure()
	b.current.Title = title
	return b
}

// TitleLink sets attachment title link.
func (b *MessageBuilder) TitleLink(url string) *MessageBuilder {
	b.ensure()
	b.current.TitleLink = url
	return b
}

// AttachmentText sets attachment text.
func (b *MessageBuilder) AttachmentText(text string) *MessageBuilder {
	b.ensure()
	b.current.Text = text
	return b
}

// Pretext sets attachment pretext.
func (b *MessageBuilder) Pretext(pretext string) *MessageBuilder {
	b.ensure()
	b.current.Pretext = pretext
	return b
}

// Field adds a field to the attachment.
func (b *MessageBuilder) Field(title, value string, short bool) *MessageBuilder {
	b.ensure()
	b.current.Fields = append(b.current.Fields, Field{Title: title, Value: value, Short: short})
	return b
}

// Footer sets attachment footer.
func (b *MessageBuilder) Footer(text, icon string) *MessageBuilder {
	b.ensure()
	b.current.Footer = text
	b.current.FooterIcon = icon
	return b
}

// Timestamp sets attachment timestamp.
func (b *MessageBuilder) Timestamp(t time.Time) *MessageBuilder {
	b.ensure()
	b.current.Timestamp = t.Unix()
	return b
}

// Build constructs the payload.
func (b *MessageBuilder) Build() (*payload, error) {
	b.flush()
	if b.text == "" && len(b.attachments) == 0 {
		return nil, ErrEmptyMessage
	}
	return &payload{Text: b.text, Attachments: b.attachments}, nil
}

// Send sends the message.
func (b *MessageBuilder) Send() error {
	p, err := b.Build()
	if err != nil {
		return err
	}
	return sendPayload(b.channel, p)
}

// Send sends a simple text message.
func Send(channel, text string) error {
	return To(channel).Text(text).Send()
}

// Sendf sends a formatted text message.
func Sendf(channel, format string, args ...any) error {
	return To(channel).Textf(format, args...).Send()
}

func sendPayload(channel string, p *payload) error {
	cfg := getConfig()

	url, ok := cfg.Channels[channel]
	if !ok || url == "" {
		return fmt.Errorf("%w: channel %q", ErrNoWebhookURL, channel)
	}

	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("slack: marshal error: %w", err)
	}

	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSendFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d, body: %s", ErrSendFailed, resp.StatusCode, string(respBody))
	}

	return nil
}

// GetWebhookURL returns the webhook URL for a channel.
func GetWebhookURL(channel string) string {
	return getConfig().Channels[channel]
}

// IsConfigured returns true if the channel has a webhook URL.
func IsConfigured(channel string) bool {
	return GetWebhookURL(channel) != ""
}
