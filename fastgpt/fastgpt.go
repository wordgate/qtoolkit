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
	ModuleType string      `json:"moduleType"`
	QuoteList  []quoteItem `json:"quoteList"`
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
