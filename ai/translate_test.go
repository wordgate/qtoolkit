package ai

import (
	"strings"
	"testing"
)

func TestGetLanguageName(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"en", "English"},
		{"zh", "Simplified Chinese (简体中文)"},
		{"ja", "Japanese (日本語)"},
		{"unknown", "unknown"}, // fallback to code
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := getLanguageName(tt.code)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseBatchResult(t *testing.T) {
	tests := []struct {
		name          string
		result        string
		expectedCount int
		expectError   bool
	}{
		{
			name:          "valid JSON array",
			result:        `["こんにちは", "ありがとう", "さようなら"]`,
			expectedCount: 3,
			expectError:   false,
		},
		{
			name:          "with markdown code block",
			result:        "```json\n[\"test1\", \"test2\"]\n```",
			expectedCount: 2,
			expectError:   false,
		},
		{
			name:          "wrong count",
			result:        `["only one"]`,
			expectedCount: 2,
			expectError:   true,
		},
		{
			name:          "invalid JSON",
			result:        "not json",
			expectedCount: 1,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := parseBatchResult(tt.result, tt.expectedCount)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(results) != tt.expectedCount {
					t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
				}
			}
		})
	}
}

func TestTranslateUsingRequest(t *testing.T) {
	// Test that Translate functions build correct requests

	t.Run("Translate builds correct request", func(t *testing.T) {
		// We can't easily test without a real provider, but we can verify
		// the request builder pattern works correctly
		r := NewRequest("Hello").
			Translate("zh").
			WithTemperature(0.3)

		if len(r.tasks) != 1 {
			t.Errorf("expected 1 task, got %d", len(r.tasks))
		}
		if r.tasks[0].taskType != taskTranslate {
			t.Errorf("expected translate task")
		}
	})

	t.Run("TranslateTemplate marks as template", func(t *testing.T) {
		r := NewRequest("<p>{{.Name}}</p>").
			Translate("ja").
			AsTemplate()

		if !r.options.isTemplate {
			t.Errorf("should be marked as template")
		}

		messages := r.buildPrompt()
		system := messages[0].Content

		if !strings.Contains(system, "TEMPLATE PRESERVATION") {
			t.Errorf("should include template preservation rules")
		}
	})

	t.Run("TranslateWithOptions applies correctly", func(t *testing.T) {
		r := NewRequest("Hello")

		// Apply translate options
		TranslateWithStyle("formal")(r)
		TranslateWithContext("test context")(r)
		TranslateWithGlossary(map[string]string{"a": "b"})(r)
		TranslateWithTemperature(0.5)(r)
		TranslateWithProvider("deepseek")(r)

		if r.options.style != StyleFormal {
			t.Errorf("style not applied")
		}
		if r.options.context != "test context" {
			t.Errorf("context not applied")
		}
		if len(r.options.glossary) != 1 {
			t.Errorf("glossary not applied")
		}
		if r.options.temperature != 0.5 {
			t.Errorf("temperature not applied")
		}
		if r.provider != "deepseek" {
			t.Errorf("provider not applied")
		}
	})
}

func TestBatchTranslatePrompt(t *testing.T) {
	texts := []string{"Hello", "Thank you", "Goodbye"}
	r := NewRequest("").WithTemperature(0.3)

	messages := buildBatchTranslatePrompt(texts, "ja", r)

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	user := messages[1].Content

	// Should contain all texts
	for _, text := range texts {
		if !strings.Contains(user, text) {
			t.Errorf("user message should contain %q", text)
		}
	}

	// Should ask for JSON array
	if !strings.Contains(user, "JSON array") {
		t.Errorf("should ask for JSON array response")
	}
}
