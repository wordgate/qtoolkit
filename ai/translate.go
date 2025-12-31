package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================
// Translate Options (for backward compatibility)
// ============================================

// TranslateOption configures translation behavior
type TranslateOption func(*Request)

// TranslateWithProvider specifies which AI provider to use
func TranslateWithProvider(provider string) TranslateOption {
	return func(r *Request) { r.UseProvider(provider) }
}

// TranslateWithStyle sets the translation style
func TranslateWithStyle(style string) TranslateOption {
	return func(r *Request) { r.WithStyle(Style(style)) }
}

// TranslateWithContext provides additional context
func TranslateWithContext(ctx string) TranslateOption {
	return func(r *Request) { r.WithContext(ctx) }
}

// TranslateWithGlossary provides a term glossary
func TranslateWithGlossary(glossary map[string]string) TranslateOption {
	return func(r *Request) { r.WithGlossary(glossary) }
}

// TranslateWithTemperature sets the sampling temperature
func TranslateWithTemperature(temp float64) TranslateOption {
	return func(r *Request) { r.WithTemperature(temp) }
}

// ============================================
// Language Mapping
// ============================================

// languageNames maps language codes to full names for better prompting
var languageNames = map[string]string{
	"en":    "English",
	"zh":    "Simplified Chinese (简体中文)",
	"zh-TW": "Traditional Chinese (繁體中文)",
	"ja":    "Japanese (日本語)",
	"ko":    "Korean (한국어)",
	"es":    "Spanish (Español)",
	"fr":    "French (Français)",
	"de":    "German (Deutsch)",
	"it":    "Italian (Italiano)",
	"pt":    "Portuguese (Português)",
	"ru":    "Russian (Русский)",
	"ar":    "Arabic (العربية)",
	"th":    "Thai (ไทย)",
	"vi":    "Vietnamese (Tiếng Việt)",
	"id":    "Indonesian (Bahasa Indonesia)",
	"ms":    "Malay (Bahasa Melayu)",
	"nl":    "Dutch (Nederlands)",
	"pl":    "Polish (Polski)",
	"tr":    "Turkish (Türkçe)",
	"uk":    "Ukrainian (Українська)",
	"he":    "Hebrew (עברית)",
	"hi":    "Hindi (हिन्दी)",
}

// getLanguageName returns the full language name for prompting
func getLanguageName(code string) string {
	if name, ok := languageNames[code]; ok {
		return name
	}
	return code
}

// ============================================
// Main Translation Functions
// ============================================

// Translate translates text to the target language
//
// Example:
//
//	result, err := ai.Translate(ctx, "Hello World", "zh")
//	// result: "你好，世界"
//
//	result, err := ai.Translate(ctx, "Thank you", "ja", ai.TranslateWithStyle("formal"))
//	// result: "ありがとうございます"
func Translate(ctx context.Context, text, targetLang string, opts ...TranslateOption) (string, error) {
	r := NewRequest(text).
		Translate(targetLang).
		WithTemperature(0.3)

	for _, opt := range opts {
		opt(r)
	}

	return r.Execute(ctx)
}

// TranslateTemplate translates an HTML/text template while preserving:
// - HTML tags (<div>, <p>, <a href="...">, etc.)
// - Template variables ({{.Name}}, ${variable}, {name}, etc.)
// - URLs and email addresses
//
// Example:
//
//	template := `<h1>Hello {{.Name}}</h1><p>Your order #{{.OrderID}} is confirmed.</p>`
//	result, err := ai.TranslateTemplate(ctx, template, "zh")
//	// result: `<h1>您好 {{.Name}}</h1><p>您的订单 #{{.OrderID}} 已确认。</p>`
func TranslateTemplate(ctx context.Context, template, targetLang string, opts ...TranslateOption) (string, error) {
	r := NewRequest(template).
		Translate(targetLang).
		AsTemplate().
		WithTemperature(0.2)

	for _, opt := range opts {
		opt(r)
	}

	return r.Execute(ctx)
}

// TranslateBatch translates multiple texts in a single API call
// More efficient than calling Translate multiple times
//
// Example:
//
//	texts := []string{"Hello", "Thank you", "Goodbye"}
//	results, err := ai.TranslateBatch(ctx, texts, "ja")
//	// results: []string{"こんにちは", "ありがとう", "さようなら"}
func TranslateBatch(ctx context.Context, texts []string, targetLang string, opts ...TranslateOption) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}

	// Build request for batch translation
	r := NewRequest("").WithTemperature(0.3)
	for _, opt := range opts {
		opt(r)
	}

	client := Get(r.provider)
	prompt := buildBatchTranslatePrompt(texts, targetLang, r)

	chatOpts := []ChatOption{WithTemperature(r.options.temperature)}

	result, err := client.Chat(ctx, prompt, chatOpts...)
	if err != nil {
		return nil, err
	}

	return parseBatchResult(result, len(texts))
}

// buildBatchTranslatePrompt constructs the prompt for batch translation
func buildBatchTranslatePrompt(texts []string, targetLang string, r *Request) []Message {
	langName := getLanguageName(targetLang)

	var systemPrompt strings.Builder
	systemPrompt.WriteString(`You are a professional translator. Translate each text item accurately.

RULES:
1. Translate each numbered item separately
2. Return results as a JSON array of strings
3. Maintain the same order as input
4. Each translation should be complete and independent`)

	// Add style if set
	if r.options.style != "" {
		systemPrompt.WriteString(fmt.Sprintf("\n\nSTYLE: %s", r.buildStyleInstruction()))
	}

	// Add glossary if set
	if len(r.options.glossary) > 0 {
		systemPrompt.WriteString("\n\nTERM GLOSSARY:\n")
		for source, target := range r.options.glossary {
			systemPrompt.WriteString(fmt.Sprintf("• %q → %q\n", source, target))
		}
	}

	// Build user prompt
	var userPrompt strings.Builder
	userPrompt.WriteString(fmt.Sprintf("Translate each item to %s and return as a JSON array:\n\n", langName))
	for i, text := range texts {
		userPrompt.WriteString(fmt.Sprintf("%d. %s\n", i+1, text))
	}
	userPrompt.WriteString("\nRespond with ONLY a JSON array: [\"translation1\", \"translation2\", ...]")

	return []Message{
		SystemMessage(systemPrompt.String()),
		UserMessage(userPrompt.String()),
	}
}

// parseBatchResult parses the JSON array response from batch translation
func parseBatchResult(result string, expectedCount int) ([]string, error) {
	// Clean up - sometimes AI adds markdown code blocks
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var translations []string
	if err := json.Unmarshal([]byte(result), &translations); err != nil {
		return nil, fmt.Errorf("failed to parse batch result: %w\nRaw: %s", err, result)
	}

	if len(translations) != expectedCount {
		return nil, fmt.Errorf("expected %d translations, got %d", expectedCount, len(translations))
	}

	return translations, nil
}

// ============================================
// Email Convenience Functions
// ============================================

// TranslateEmailSubject translates an email subject line
// Uses formal style and concise context by default
func TranslateEmailSubject(ctx context.Context, subject, targetLang string, opts ...TranslateOption) (string, error) {
	r := NewRequest(subject).
		Translate(targetLang).
		WithStyle(StyleFormal).
		WithContext("Email subject line. Keep it concise and professional.").
		WithTemperature(0.2)

	for _, opt := range opts {
		opt(r)
	}

	return r.Execute(ctx)
}

// TranslateEmailBody translates an HTML email body
// Automatically preserves HTML tags and template variables
func TranslateEmailBody(ctx context.Context, body, targetLang string, opts ...TranslateOption) (string, error) {
	r := NewEmailRequest(body).
		Translate(targetLang).
		WithContext("E-commerce email notification. Maintain professional tone.")

	for _, opt := range opts {
		opt(r)
	}

	return r.Execute(ctx)
}
