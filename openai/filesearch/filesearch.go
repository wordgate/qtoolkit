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
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Config holds OpenAI file search configuration.
type Config struct {
	APIKey         string  `yaml:"api_key"`
	VectorStoreID string `yaml:"vector_store_id"`
	Model         string `yaml:"model"`
	MaxResults    int    `yaml:"max_results"`
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
	baseURL      = "https://api.openai.com"
)

func loadConfigFromViper() *Config {
	cfg := &Config{
		APIKey:         viper.GetString("openai.filesearch.api_key"),
		VectorStoreID:  viper.GetString("openai.filesearch.vector_store_id"),
		Model:      viper.GetString("openai.filesearch.model"),
		MaxResults: viper.GetInt("openai.filesearch.max_results"),
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
	Type    string         `json:"type"`
	Content []contentItem  `json:"content"`
	Role    string         `json:"role"`
	Results []searchResult `json:"results"`
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

	url := baseURL + "/v1/responses"
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
func parseResponse(resp apiResponse) Result {
	var result Result

	for _, item := range resp.Output {
		switch item.Type {
		case "file_search_call":
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
			for _, c := range item.Content {
				if c.Type == "output_text" {
					result.Content = c.Text
				}
			}
		}
	}

	return result
}
