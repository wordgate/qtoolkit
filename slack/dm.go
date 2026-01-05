package slack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// DM-related errors
var (
	ErrNoBotToken   = errors.New("slack: bot_token not configured")
	ErrUserNotFound = errors.New("slack: user not found")
	ErrAPIFailed    = errors.New("slack: API call failed")
)

var slackAPIBase = "https://slack.com/api"

// lookupUserByEmail finds a Slack user ID by email address.
func lookupUserByEmail(email string) (string, error) {
	cfg := getConfig()
	if cfg.BotToken == "" {
		return "", ErrNoBotToken
	}

	reqURL := fmt.Sprintf("%s/users.lookupByEmail?email=%s", slackAPIBase, url.QueryEscape(email))
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.BotToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}

	if !result.OK {
		if result.Error == "users_not_found" {
			return "", fmt.Errorf("%w: %s", ErrUserNotFound, email)
		}
		return "", fmt.Errorf("%w: %s", ErrAPIFailed, result.Error)
	}

	return result.User.ID, nil
}

// SendDM sends a direct message to a user by email.
// Requires bot_token with users:read.email and chat:write scopes.
func SendDM(email, message string) error {
	return DM(email).Text(message).Send()
}

// DMBuilder builds direct messages.
type DMBuilder struct {
	email       string
	text        string
	attachments []Attachment
	current     *Attachment
}

// DM creates a DMBuilder for the specified email.
func DM(email string) *DMBuilder {
	return &DMBuilder{email: email}
}

// Text sets the message text.
func (b *DMBuilder) Text(text string) *DMBuilder {
	b.text = text
	return b
}

// Textf sets formatted message text.
func (b *DMBuilder) Textf(format string, args ...any) *DMBuilder {
	b.text = fmt.Sprintf(format, args...)
	return b
}

func (b *DMBuilder) ensure() {
	if b.current == nil {
		b.current = &Attachment{}
	}
}

func (b *DMBuilder) flush() {
	if b.current != nil {
		b.attachments = append(b.attachments, *b.current)
		b.current = nil
	}
}

// Color sets attachment color.
func (b *DMBuilder) Color(color string) *DMBuilder {
	b.ensure()
	b.current.Color = color
	return b
}

// Title sets attachment title.
func (b *DMBuilder) Title(title string) *DMBuilder {
	b.ensure()
	b.current.Title = title
	return b
}

// Field adds a field to the attachment.
func (b *DMBuilder) Field(title, value string, short bool) *DMBuilder {
	b.ensure()
	b.current.Fields = append(b.current.Fields, Field{Title: title, Value: value, Short: short})
	return b
}

// Send sends the direct message.
func (b *DMBuilder) Send() error {
	b.flush()

	if b.text == "" && len(b.attachments) == 0 {
		return ErrEmptyMessage
	}

	cfg := getConfig()
	if cfg.BotToken == "" {
		return ErrNoBotToken
	}

	userID, err := lookupUserByEmail(b.email)
	if err != nil {
		return err
	}

	return postMessage(cfg.BotToken, userID, b.text, b.attachments)
}

func postMessage(token, channel, text string, attachments []Attachment) error {
	payload := map[string]any{
		"channel": channel,
		"text":    text,
	}
	if len(attachments) > 0 {
		payload["attachments"] = attachments
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}

	req, err := http.NewRequest(http.MethodPost, slackAPIBase+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("%w: %v", ErrAPIFailed, err)
	}

	if !result.OK {
		return fmt.Errorf("%w: %s", ErrAPIFailed, result.Error)
	}

	return nil
}
