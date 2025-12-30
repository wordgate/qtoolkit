package ai

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/spf13/viper"
)

// Client wraps an OpenAI-compatible client with provider configuration
type Client struct {
	*openai.Client
	provider string
	model    string
}

// ProviderConfig holds configuration for a single AI provider
type ProviderConfig struct {
	APIKey  string `yaml:"api_key" json:"api_key"`
	BaseURL string `yaml:"base_url" json:"base_url"`
	Model   string `yaml:"model" json:"model"`
}

var (
	clients    = make(map[string]*Client)
	clientsMux sync.RWMutex
	initOnce   = make(map[string]*sync.Once)
	initErrors = make(map[string]error)
)

// getOnce returns the sync.Once for a provider, creating if needed
func getOnce(provider string) *sync.Once {
	clientsMux.Lock()
	defer clientsMux.Unlock()
	if initOnce[provider] == nil {
		initOnce[provider] = &sync.Once{}
	}
	return initOnce[provider]
}

// loadProviderConfig loads configuration for a specific provider
// Configuration path priority (cascading fallback):
// 1. ai.providers.<provider>.* - Provider-specific config
// 2. Environment variables: AI_<PROVIDER>_API_KEY, AI_<PROVIDER>_BASE_URL
func loadProviderConfig(provider string) (*ProviderConfig, error) {
	cfg := &ProviderConfig{}

	providerPath := fmt.Sprintf("ai.providers.%s", provider)

	// Load from viper
	cfg.APIKey = viper.GetString(providerPath + ".api_key")
	cfg.BaseURL = viper.GetString(providerPath + ".base_url")
	cfg.Model = viper.GetString(providerPath + ".model")

	// Environment variable fallback (e.g., AI_OPENAI_API_KEY)
	envPrefix := fmt.Sprintf("AI_%s_", toEnvKey(provider))
	if apiKey := os.Getenv(envPrefix + "API_KEY"); apiKey != "" {
		cfg.APIKey = apiKey
	}
	if baseURL := os.Getenv(envPrefix + "BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}

	// Validate: API key is required unless it's a local provider (like Ollama)
	if cfg.APIKey == "" && !isLocalProvider(cfg.BaseURL) {
		return nil, fmt.Errorf("ai.providers.%s.api_key is required", provider)
	}

	// Validate: Model is required
	if cfg.Model == "" {
		return nil, fmt.Errorf("ai.providers.%s.model is required", provider)
	}

	return cfg, nil
}

// isLocalProvider checks if the base URL indicates a local provider (no API key needed)
func isLocalProvider(baseURL string) bool {
	return baseURL != "" && (contains(baseURL, "localhost") || contains(baseURL, "127.0.0.1"))
}

// contains is a simple string contains check
func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// toEnvKey converts provider name to environment variable format
func toEnvKey(provider string) string {
	result := make([]byte, 0, len(provider))
	for i := 0; i < len(provider); i++ {
		c := provider[i]
		if c >= 'a' && c <= 'z' {
			result = append(result, c-32) // to uppercase
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c)
		} else if c >= '0' && c <= '9' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}

// initProvider initializes a provider client
func initProvider(provider string) (*Client, error) {
	cfg, err := loadProviderConfig(provider)
	if err != nil {
		return nil, err
	}

	opts := []option.RequestOption{}

	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	return &Client{
		Client:   client,
		provider: provider,
		model:    cfg.Model,
	}, nil
}

// Get returns an AI client for the specified provider
// If no provider is specified, returns the default provider
// Usage:
//
//	ai.Get()           // returns default provider
//	ai.Get("deepseek") // returns deepseek provider
func Get(provider ...string) *Client {
	p := getDefaultProvider()
	if len(provider) > 0 && provider[0] != "" {
		p = provider[0]
	}

	once := getOnce(p)
	once.Do(func() {
		client, err := initProvider(p)
		if err != nil {
			initErrors[p] = err
			return
		}
		clientsMux.Lock()
		clients[p] = client
		clientsMux.Unlock()
	})

	clientsMux.RLock()
	client := clients[p]
	clientsMux.RUnlock()

	if client == nil {
		if err := initErrors[p]; err != nil {
			panic(fmt.Sprintf("ai provider %q initialization failed: %v", p, err))
		}
		panic(fmt.Sprintf("ai provider %q not configured", p))
	}

	return client
}

// GetError returns the initialization error for a provider, if any
func GetError(provider ...string) error {
	p := getDefaultProvider()
	if len(provider) > 0 && provider[0] != "" {
		p = provider[0]
	}
	return initErrors[p]
}

// getDefaultProvider returns the default provider name
func getDefaultProvider() string {
	defaultProvider := viper.GetString("ai.default")
	if defaultProvider == "" {
		defaultProvider = "openai"
	}
	return defaultProvider
}

// Provider returns the provider name for this client
func (c *Client) Provider() string {
	return c.provider
}

// Model returns the configured model for this client
func (c *Client) Model() string {
	return c.model
}

// Chat sends a chat completion request and returns the response content
func (c *Client) Chat(ctx context.Context, messages []Message, opts ...ChatOption) (string, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.F(openai.ChatModel(c.model)),
		Messages: openai.F(toOpenAIMessages(messages)),
	}

	// Apply options
	for _, opt := range opts {
		opt(&params)
	}

	resp, err := c.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("chat completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return resp.Choices[0].Message.Content, nil
}

// ChatStream sends a streaming chat completion request
func (c *Client) ChatStream(ctx context.Context, messages []Message, opts ...ChatOption) *Stream {
	params := openai.ChatCompletionNewParams{
		Model:    openai.F(openai.ChatModel(c.model)),
		Messages: openai.F(toOpenAIMessages(messages)),
	}

	// Apply options
	for _, opt := range opts {
		opt(&params)
	}

	stream := c.Client.Chat.Completions.NewStreaming(ctx, params)

	return &Stream{stream: stream}
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Stream wraps the streaming response
type Stream struct {
	stream *ssestream.Stream[openai.ChatCompletionChunk]
}

// Next returns the next chunk of the stream
func (s *Stream) Next() (string, error) {
	if !s.stream.Next() {
		if err := s.stream.Err(); err != nil {
			return "", err
		}
		return "", nil
	}

	chunk := s.stream.Current()
	if len(chunk.Choices) > 0 {
		return chunk.Choices[0].Delta.Content, nil
	}
	return "", nil
}

// Close closes the stream
func (s *Stream) Close() error {
	return s.stream.Close()
}

// Err returns any error that occurred during streaming
func (s *Stream) Err() error {
	return s.stream.Err()
}

// toOpenAIMessages converts Messages to OpenAI format
func toOpenAIMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			result[i] = openai.SystemMessage(msg.Content)
		case "assistant":
			result[i] = openai.AssistantMessage(msg.Content)
		case "user":
			result[i] = openai.UserMessage(msg.Content)
		default:
			result[i] = openai.UserMessage(msg.Content)
		}
	}
	return result
}

// ChatOption configures chat completion parameters
type ChatOption func(*openai.ChatCompletionNewParams)

// WithModel overrides the default model for this request
func WithModel(model string) ChatOption {
	return func(p *openai.ChatCompletionNewParams) {
		p.Model = openai.F(openai.ChatModel(model))
	}
}

// WithTemperature sets the sampling temperature
func WithTemperature(temp float64) ChatOption {
	return func(p *openai.ChatCompletionNewParams) {
		p.Temperature = openai.F(temp)
	}
}

// WithMaxTokens sets the maximum number of tokens to generate
func WithMaxTokens(tokens int64) ChatOption {
	return func(p *openai.ChatCompletionNewParams) {
		p.MaxTokens = openai.F(tokens)
	}
}

// WithTopP sets the nucleus sampling parameter
func WithTopP(topP float64) ChatOption {
	return func(p *openai.ChatCompletionNewParams) {
		p.TopP = openai.F(topP)
	}
}

// WithStop sets the stop sequences
func WithStop(stop ...string) ChatOption {
	return func(p *openai.ChatCompletionNewParams) {
		p.Stop = openai.F[openai.ChatCompletionNewParamsStopUnion](openai.ChatCompletionNewParamsStopArray(stop))
	}
}

// Helper functions for creating messages
func SystemMessage(content string) Message {
	return Message{Role: "system", Content: content}
}

func UserMessage(content string) Message {
	return Message{Role: "user", Content: content}
}

func AssistantMessage(content string) Message {
	return Message{Role: "assistant", Content: content}
}
