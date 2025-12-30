package ai

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
)

func TestToEnvKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"openai", "OPENAI"},
		{"deepseek", "DEEPSEEK"},
		{"azure-openai", "AZURE_OPENAI"},
		{"gpt4", "GPT4"},
		{"my_provider", "MY_PROVIDER"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toEnvKey(tt.input)
			if result != tt.expected {
				t.Errorf("toEnvKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsLocalProvider(t *testing.T) {
	tests := []struct {
		baseURL  string
		expected bool
	}{
		{"http://localhost:11434/v1", true},
		{"http://127.0.0.1:11434/v1", true},
		{"https://api.openai.com/v1", false},
		{"https://api.deepseek.com", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			result := isLocalProvider(tt.baseURL)
			if result != tt.expected {
				t.Errorf("isLocalProvider(%q) = %v, want %v", tt.baseURL, result, tt.expected)
			}
		})
	}
}

func TestLoadProviderConfig(t *testing.T) {
	configYAML := `
ai:
  default: "openai"
  providers:
    openai:
      api_key: "sk-test-key"
      model: "gpt-4o"
    deepseek:
      api_key: "sk-deepseek-key"
      base_url: "https://api.deepseek.com"
      model: "deepseek-chat"
    ollama:
      base_url: "http://localhost:11434/v1"
      model: "llama3"
`
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(bytes.NewBufferString(configYAML)); err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	t.Run("openai config", func(t *testing.T) {
		cfg, err := loadProviderConfig("openai")
		if err != nil {
			t.Fatalf("Failed to load openai config: %v", err)
		}
		if cfg.APIKey != "sk-test-key" {
			t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test-key")
		}
		if cfg.Model != "gpt-4o" {
			t.Errorf("Model = %q, want %q", cfg.Model, "gpt-4o")
		}
	})

	t.Run("deepseek config", func(t *testing.T) {
		cfg, err := loadProviderConfig("deepseek")
		if err != nil {
			t.Fatalf("Failed to load deepseek config: %v", err)
		}
		if cfg.APIKey != "sk-deepseek-key" {
			t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-deepseek-key")
		}
		if cfg.BaseURL != "https://api.deepseek.com" {
			t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.deepseek.com")
		}
		if cfg.Model != "deepseek-chat" {
			t.Errorf("Model = %q, want %q", cfg.Model, "deepseek-chat")
		}
	})

	t.Run("ollama local config", func(t *testing.T) {
		cfg, err := loadProviderConfig("ollama")
		if err != nil {
			t.Fatalf("Failed to load ollama config: %v", err)
		}
		if cfg.APIKey != "" {
			t.Errorf("APIKey = %q, want empty for local provider", cfg.APIKey)
		}
		if cfg.BaseURL != "http://localhost:11434/v1" {
			t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:11434/v1")
		}
	})

	t.Run("missing api key error", func(t *testing.T) {
		_, err := loadProviderConfig("unconfigured")
		if err == nil {
			t.Error("Expected error for unconfigured provider")
		}
	})
}

func TestGetDefaultProvider(t *testing.T) {
	viper.Reset()

	t.Run("default is openai", func(t *testing.T) {
		result := getDefaultProvider()
		if result != "openai" {
			t.Errorf("getDefaultProvider() = %q, want %q", result, "openai")
		}
	})

	t.Run("configured default", func(t *testing.T) {
		viper.Set("ai.default", "deepseek")
		result := getDefaultProvider()
		if result != "deepseek" {
			t.Errorf("getDefaultProvider() = %q, want %q", result, "deepseek")
		}
	})
}

func TestMessageHelpers(t *testing.T) {
	t.Run("SystemMessage", func(t *testing.T) {
		msg := SystemMessage("You are a helpful assistant")
		if msg.Role != "system" {
			t.Errorf("Role = %q, want %q", msg.Role, "system")
		}
		if msg.Content != "You are a helpful assistant" {
			t.Errorf("Content = %q, want %q", msg.Content, "You are a helpful assistant")
		}
	})

	t.Run("UserMessage", func(t *testing.T) {
		msg := UserMessage("Hello")
		if msg.Role != "user" {
			t.Errorf("Role = %q, want %q", msg.Role, "user")
		}
	})

	t.Run("AssistantMessage", func(t *testing.T) {
		msg := AssistantMessage("Hi there!")
		if msg.Role != "assistant" {
			t.Errorf("Role = %q, want %q", msg.Role, "assistant")
		}
	})
}

func TestToOpenAIMessages(t *testing.T) {
	messages := []Message{
		SystemMessage("You are helpful"),
		UserMessage("Hello"),
		AssistantMessage("Hi!"),
	}

	result := toOpenAIMessages(messages)
	if len(result) != 3 {
		t.Errorf("len(result) = %d, want %d", len(result), 3)
	}
}
