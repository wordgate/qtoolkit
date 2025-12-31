package ai

import (
	"context"
	"fmt"
	"strings"
)

// Request is a fluent builder for AI text processing tasks
// Supports chaining multiple operations: translate, polish, optimize, etc.
//
// Example:
//
//	result, err := ai.NewRequest("Hello World").
//	    Translate("zh").
//	    Polish().
//	    WithStyle(ai.StyleFormal).
//	    Execute(ctx)
type Request struct {
	input    string
	tasks    []task
	options  requestOptions
	provider string
}

// task represents a single processing task
type task struct {
	taskType taskType
	params   map[string]string
}

type taskType int

const (
	taskTranslate taskType = iota
	taskPolish
	taskOptimize
	taskSummarize
	taskExpand
	taskRewrite
	taskProofread
	taskSimplify
)

// Style constants for consistent API
type Style string

const (
	StyleFormal     Style = "formal"
	StyleCasual     Style = "casual"
	StyleTechnical  Style = "technical"
	StyleMarketing  Style = "marketing"
	StyleAcademic   Style = "academic"
	StyleCreative   Style = "creative"
	StyleConcise    Style = "concise"
	StyleFriendly   Style = "friendly"
	StyleProfessional Style = "professional"
)

// Tone constants for content tone
type Tone string

const (
	ToneNeutral      Tone = "neutral"
	ToneEnthusiastic Tone = "enthusiastic"
	ToneEmpathetic   Tone = "empathetic"
	ToneUrgent       Tone = "urgent"
	ToneConfident    Tone = "confident"
	ToneApologetic   Tone = "apologetic"
	TonePersuasive   Tone = "persuasive"
)

// Purpose constants for optimization
type Purpose string

const (
	PurposeEmail      Purpose = "email"
	PurposeMarketing  Purpose = "marketing"
	PurposeSEO        Purpose = "seo"
	PurposeSocial     Purpose = "social_media"
	PurposePresentation Purpose = "presentation"
	PurposeDocumentation Purpose = "documentation"
)

// requestOptions holds all configuration for the request
type requestOptions struct {
	style       Style
	tone        Tone
	purpose     Purpose
	context     string
	glossary    map[string]string
	constraints []string
	temperature float64
	maxLength   int
	isTemplate  bool
	format      string // output format hint
}

// NewRequest creates a new request builder with the input text
func NewRequest(input string) *Request {
	return &Request{
		input:   input,
		tasks:   make([]task, 0),
		options: requestOptions{temperature: 0.3},
	}
}

// ============================================
// Task Methods (What to do)
// ============================================

// Translate adds a translation task to the target language
// Language codes: "zh", "ja", "ko", "en", "es", "fr", "de", etc.
func (r *Request) Translate(targetLang string) *Request {
	r.tasks = append(r.tasks, task{
		taskType: taskTranslate,
		params:   map[string]string{"target_lang": targetLang},
	})
	return r
}

// Polish adds a polishing task to improve expression while preserving meaning
// Fixes grammar, improves word choice, enhances readability
func (r *Request) Polish() *Request {
	r.tasks = append(r.tasks, task{taskType: taskPolish})
	return r
}

// Optimize adds an optimization task for a specific purpose
// Rewrites content to better achieve its goal
func (r *Request) Optimize() *Request {
	r.tasks = append(r.tasks, task{taskType: taskOptimize})
	return r
}

// Summarize adds a summarization task
// Condenses content to key points
func (r *Request) Summarize() *Request {
	r.tasks = append(r.tasks, task{taskType: taskSummarize})
	return r
}

// Expand adds an expansion task
// Elaborates and adds more detail to the content
func (r *Request) Expand() *Request {
	r.tasks = append(r.tasks, task{taskType: taskExpand})
	return r
}

// Rewrite adds a rewrite task with a different style/approach
func (r *Request) Rewrite() *Request {
	r.tasks = append(r.tasks, task{taskType: taskRewrite})
	return r
}

// Proofread adds a proofreading task
// Checks and fixes grammar, spelling, punctuation
func (r *Request) Proofread() *Request {
	r.tasks = append(r.tasks, task{taskType: taskProofread})
	return r
}

// Simplify adds a simplification task
// Makes content easier to understand
func (r *Request) Simplify() *Request {
	r.tasks = append(r.tasks, task{taskType: taskSimplify})
	return r
}

// ============================================
// Option Methods (How to do it)
// ============================================

// WithStyle sets the writing style
func (r *Request) WithStyle(style Style) *Request {
	r.options.style = style
	return r
}

// WithTone sets the emotional tone
func (r *Request) WithTone(tone Tone) *Request {
	r.options.tone = tone
	return r
}

// ForPurpose sets the content purpose (for optimization)
func (r *Request) ForPurpose(purpose Purpose) *Request {
	r.options.purpose = purpose
	return r
}

// WithContext provides background information
func (r *Request) WithContext(ctx string) *Request {
	r.options.context = ctx
	return r
}

// WithGlossary provides term translations/definitions
func (r *Request) WithGlossary(glossary map[string]string) *Request {
	r.options.glossary = glossary
	return r
}

// WithConstraint adds a specific constraint or requirement
func (r *Request) WithConstraint(constraint string) *Request {
	r.options.constraints = append(r.options.constraints, constraint)
	return r
}

// WithTemperature sets the AI creativity level (0.0-1.0)
// Lower = more consistent, Higher = more creative
func (r *Request) WithTemperature(temp float64) *Request {
	r.options.temperature = temp
	return r
}

// WithMaxLength sets the maximum output length (approximate)
func (r *Request) WithMaxLength(length int) *Request {
	r.options.maxLength = length
	return r
}

// AsTemplate marks the input as a template with variables to preserve
// Preserves: {{.Var}}, ${var}, {var}, HTML tags, URLs, etc.
func (r *Request) AsTemplate() *Request {
	r.options.isTemplate = true
	return r
}

// WithFormat specifies the output format
// e.g., "bullet_points", "numbered_list", "paragraph", "html"
func (r *Request) WithFormat(format string) *Request {
	r.options.format = format
	return r
}

// UseProvider specifies which AI provider to use
func (r *Request) UseProvider(provider string) *Request {
	r.provider = provider
	return r
}

// ============================================
// Execution Methods
// ============================================

// Execute runs the request and returns the result
func (r *Request) Execute(ctx context.Context) (string, error) {
	if len(r.tasks) == 0 {
		return "", fmt.Errorf("no tasks specified, use Translate(), Polish(), etc.")
	}

	client := Get(r.provider)
	messages := r.buildPrompt()

	opts := []ChatOption{WithTemperature(r.options.temperature)}

	return client.Chat(ctx, messages, opts...)
}

// ExecuteStream runs the request and returns a streaming response
func (r *Request) ExecuteStream(ctx context.Context) (*Stream, error) {
	if len(r.tasks) == 0 {
		return nil, fmt.Errorf("no tasks specified")
	}

	client := Get(r.provider)
	messages := r.buildPrompt()

	opts := []ChatOption{WithTemperature(r.options.temperature)}

	return client.ChatStream(ctx, messages, opts...), nil
}

// ============================================
// Prompt Building
// ============================================

func (r *Request) buildPrompt() []Message {
	var system strings.Builder
	var user strings.Builder

	// Build role description
	system.WriteString("You are an expert content specialist. ")

	// Add task-specific instructions
	taskInstructions := r.buildTaskInstructions()
	system.WriteString(taskInstructions)

	// Add template preservation rules if needed
	if r.options.isTemplate {
		system.WriteString(templatePreservationRules)
	}

	// Add style instructions
	if r.options.style != "" {
		system.WriteString(r.buildStyleInstruction())
	}

	// Add tone instructions
	if r.options.tone != "" {
		system.WriteString(r.buildToneInstruction())
	}

	// Add purpose instructions
	if r.options.purpose != "" {
		system.WriteString(r.buildPurposeInstruction())
	}

	// Add glossary
	if len(r.options.glossary) > 0 {
		system.WriteString("\n\nTERM GLOSSARY (use these exact translations/terms):\n")
		for source, target := range r.options.glossary {
			system.WriteString(fmt.Sprintf("• %q → %q\n", source, target))
		}
	}

	// Add constraints
	if len(r.options.constraints) > 0 {
		system.WriteString("\n\nCONSTRAINTS:\n")
		for _, c := range r.options.constraints {
			system.WriteString(fmt.Sprintf("• %s\n", c))
		}
	}

	// Add context
	if r.options.context != "" {
		system.WriteString(fmt.Sprintf("\n\nCONTEXT: %s", r.options.context))
	}

	// Add max length
	if r.options.maxLength > 0 {
		system.WriteString(fmt.Sprintf("\n\nKeep output under approximately %d characters.", r.options.maxLength))
	}

	// Add format instruction
	if r.options.format != "" {
		system.WriteString(fmt.Sprintf("\n\nOUTPUT FORMAT: %s", r.options.format))
	}

	// Final instruction
	system.WriteString("\n\nRespond with ONLY the processed text. No explanations, no quotes around the result.")

	// Build user message
	user.WriteString(r.buildUserPrompt())

	return []Message{
		SystemMessage(system.String()),
		UserMessage(user.String()),
	}
}

func (r *Request) buildTaskInstructions() string {
	var parts []string

	for _, t := range r.tasks {
		switch t.taskType {
		case taskTranslate:
			lang := getLanguageName(t.params["target_lang"])
			parts = append(parts, fmt.Sprintf("translate to %s", lang))

		case taskPolish:
			parts = append(parts, "polish and improve the expression (fix grammar, enhance word choice, improve flow)")

		case taskOptimize:
			parts = append(parts, "optimize the content for maximum effectiveness")

		case taskSummarize:
			parts = append(parts, "summarize the key points concisely")

		case taskExpand:
			parts = append(parts, "expand with more detail and elaboration")

		case taskRewrite:
			parts = append(parts, "rewrite in a fresh way while preserving the core meaning")

		case taskProofread:
			parts = append(parts, "proofread and correct any errors")

		case taskSimplify:
			parts = append(parts, "simplify to make it easier to understand")
		}
	}

	if len(parts) == 1 {
		return fmt.Sprintf("Your task is to %s.\n", parts[0])
	}

	// Multiple tasks - process in sequence
	return fmt.Sprintf("Your task is to: %s (process in this order).\n", strings.Join(parts, " → "))
}

func (r *Request) buildStyleInstruction() string {
	instructions := map[Style]string{
		StyleFormal:       "\n\nSTYLE: Use formal, professional language suitable for business communication.",
		StyleCasual:       "\n\nSTYLE: Use casual, conversational language.",
		StyleTechnical:    "\n\nSTYLE: Use precise technical terminology. Be accurate and specific.",
		StyleMarketing:    "\n\nSTYLE: Use engaging, persuasive marketing language. Make it compelling.",
		StyleAcademic:     "\n\nSTYLE: Use academic language with proper structure and citations format.",
		StyleCreative:     "\n\nSTYLE: Use creative, expressive language. Be imaginative.",
		StyleConcise:      "\n\nSTYLE: Be extremely concise. Every word must count.",
		StyleFriendly:     "\n\nSTYLE: Use warm, friendly language that builds rapport.",
		StyleProfessional: "\n\nSTYLE: Use professional language that conveys expertise and reliability.",
	}
	return instructions[r.options.style]
}

func (r *Request) buildToneInstruction() string {
	instructions := map[Tone]string{
		ToneNeutral:      "\n\nTONE: Maintain a neutral, balanced tone.",
		ToneEnthusiastic: "\n\nTONE: Be enthusiastic and energetic.",
		ToneEmpathetic:   "\n\nTONE: Show empathy and understanding.",
		ToneUrgent:       "\n\nTONE: Convey urgency and importance.",
		ToneConfident:    "\n\nTONE: Be confident and authoritative.",
		ToneApologetic:   "\n\nTONE: Express sincere apology and commitment to resolution.",
		TonePersuasive:   "\n\nTONE: Be persuasive and compelling.",
	}
	return instructions[r.options.tone]
}

func (r *Request) buildPurposeInstruction() string {
	instructions := map[Purpose]string{
		PurposeEmail:         "\n\nPURPOSE: Optimized for email communication. Clear subject matter, scannable content.",
		PurposeMarketing:     "\n\nPURPOSE: Optimized for marketing. Focus on benefits, include call-to-action.",
		PurposeSEO:           "\n\nPURPOSE: Optimized for SEO. Natural keyword usage, engaging meta-friendly content.",
		PurposeSocial:        "\n\nPURPOSE: Optimized for social media. Engaging, shareable, appropriate length.",
		PurposePresentation:  "\n\nPURPOSE: Optimized for presentations. Clear points, impactful phrases.",
		PurposeDocumentation: "\n\nPURPOSE: Optimized for documentation. Clear, complete, well-structured.",
	}
	return instructions[r.options.purpose]
}

func (r *Request) buildUserPrompt() string {
	// Single task - simple prompt
	if len(r.tasks) == 1 {
		switch r.tasks[0].taskType {
		case taskTranslate:
			return fmt.Sprintf("Translate:\n\n%s", r.input)
		case taskPolish:
			return fmt.Sprintf("Polish this text:\n\n%s", r.input)
		case taskOptimize:
			return fmt.Sprintf("Optimize this content:\n\n%s", r.input)
		case taskSummarize:
			return fmt.Sprintf("Summarize:\n\n%s", r.input)
		case taskExpand:
			return fmt.Sprintf("Expand on this:\n\n%s", r.input)
		case taskRewrite:
			return fmt.Sprintf("Rewrite this:\n\n%s", r.input)
		case taskProofread:
			return fmt.Sprintf("Proofread and correct:\n\n%s", r.input)
		case taskSimplify:
			return fmt.Sprintf("Simplify:\n\n%s", r.input)
		}
	}

	// Multiple tasks - generic prompt
	return fmt.Sprintf("Process the following text:\n\n%s", r.input)
}

const templatePreservationRules = `

TEMPLATE PRESERVATION RULES (CRITICAL):
1. PRESERVE EXACTLY as-is (do not translate or modify):
   • HTML tags: <div>, <p>, <span>, <a href="...">, etc.
   • Template variables: {{.Name}}, {{.OrderID}}, ${variable}, {name}, %s, etc.
   • URLs: https://..., http://...
   • Email addresses: user@example.com
   • Code snippets and technical identifiers
2. Only process the human-readable text content
3. Maintain original structure and formatting`

// ============================================
// Convenience Constructors
// ============================================

// NewEmailRequest creates a request pre-configured for email content
func NewEmailRequest(content string) *Request {
	return NewRequest(content).
		AsTemplate().
		WithStyle(StyleProfessional).
		ForPurpose(PurposeEmail)
}

// NewMarketingRequest creates a request pre-configured for marketing content
func NewMarketingRequest(content string) *Request {
	return NewRequest(content).
		WithStyle(StyleMarketing).
		WithTone(TonePersuasive).
		ForPurpose(PurposeMarketing)
}

// NewTechnicalRequest creates a request pre-configured for technical content
func NewTechnicalRequest(content string) *Request {
	return NewRequest(content).
		WithStyle(StyleTechnical).
		ForPurpose(PurposeDocumentation)
}
