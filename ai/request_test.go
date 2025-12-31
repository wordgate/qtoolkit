package ai

import (
	"strings"
	"testing"
)

func TestRequestBuilder(t *testing.T) {
	t.Run("single translate task", func(t *testing.T) {
		r := NewRequest("Hello").Translate("zh")

		if len(r.tasks) != 1 {
			t.Errorf("expected 1 task, got %d", len(r.tasks))
		}
		if r.tasks[0].taskType != taskTranslate {
			t.Errorf("expected translate task")
		}
		if r.tasks[0].params["target_lang"] != "zh" {
			t.Errorf("expected target_lang zh")
		}
	})

	t.Run("chained tasks", func(t *testing.T) {
		r := NewRequest("Hello").
			Translate("zh").
			Polish()

		if len(r.tasks) != 2 {
			t.Errorf("expected 2 tasks, got %d", len(r.tasks))
		}
		if r.tasks[0].taskType != taskTranslate {
			t.Errorf("first task should be translate")
		}
		if r.tasks[1].taskType != taskPolish {
			t.Errorf("second task should be polish")
		}
	})

	t.Run("all task types", func(t *testing.T) {
		r := NewRequest("text").
			Translate("ja").
			Polish().
			Optimize().
			Summarize().
			Expand().
			Rewrite().
			Proofread().
			Simplify()

		if len(r.tasks) != 8 {
			t.Errorf("expected 8 tasks, got %d", len(r.tasks))
		}
	})

	t.Run("options", func(t *testing.T) {
		r := NewRequest("text").
			Translate("zh").
			WithStyle(StyleFormal).
			WithTone(ToneConfident).
			ForPurpose(PurposeEmail).
			WithContext("test context").
			WithGlossary(map[string]string{"a": "b"}).
			WithConstraint("keep it short").
			WithTemperature(0.5).
			WithMaxLength(100).
			AsTemplate().
			WithFormat("bullet_points").
			UseProvider("deepseek")

		if r.options.style != StyleFormal {
			t.Errorf("style not set")
		}
		if r.options.tone != ToneConfident {
			t.Errorf("tone not set")
		}
		if r.options.purpose != PurposeEmail {
			t.Errorf("purpose not set")
		}
		if r.options.context != "test context" {
			t.Errorf("context not set")
		}
		if len(r.options.glossary) != 1 {
			t.Errorf("glossary not set")
		}
		if len(r.options.constraints) != 1 {
			t.Errorf("constraints not set")
		}
		if r.options.temperature != 0.5 {
			t.Errorf("temperature not set")
		}
		if r.options.maxLength != 100 {
			t.Errorf("maxLength not set")
		}
		if !r.options.isTemplate {
			t.Errorf("isTemplate not set")
		}
		if r.options.format != "bullet_points" {
			t.Errorf("format not set")
		}
		if r.provider != "deepseek" {
			t.Errorf("provider not set")
		}
	})
}

func TestBuildPrompt(t *testing.T) {
	t.Run("single translate", func(t *testing.T) {
		r := NewRequest("Hello World").Translate("zh")
		messages := r.buildPrompt()

		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}

		system := messages[0].Content
		user := messages[1].Content

		if !strings.Contains(system, "translate") {
			t.Errorf("system should mention translate")
		}
		if !strings.Contains(system, "Chinese") {
			t.Errorf("system should mention target language")
		}
		if !strings.Contains(user, "Hello World") {
			t.Errorf("user should contain input")
		}
	})

	t.Run("translate then polish", func(t *testing.T) {
		r := NewRequest("Hello").Translate("ja").Polish()
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "→") {
			t.Errorf("should show task sequence with arrow")
		}
		if !strings.Contains(system, "translate") {
			t.Errorf("should mention translate")
		}
		if !strings.Contains(system, "polish") {
			t.Errorf("should mention polish")
		}
	})

	t.Run("with template flag", func(t *testing.T) {
		r := NewRequest("<p>{{.Name}}</p>").Translate("zh").AsTemplate()
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "TEMPLATE PRESERVATION") {
			t.Errorf("should include template preservation rules")
		}
		if !strings.Contains(system, "{{.Name}}") {
			t.Errorf("should mention template variable format")
		}
	})

	t.Run("with style", func(t *testing.T) {
		r := NewRequest("text").Polish().WithStyle(StyleFormal)
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "formal") {
			t.Errorf("should mention formal style")
		}
	})

	t.Run("with tone", func(t *testing.T) {
		r := NewRequest("text").Rewrite().WithTone(ToneEnthusiastic)
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "enthusiastic") {
			t.Errorf("should mention enthusiastic tone")
		}
	})

	t.Run("with glossary", func(t *testing.T) {
		r := NewRequest("Order shipped").
			Translate("zh").
			WithGlossary(map[string]string{
				"Order": "订单",
			})
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "GLOSSARY") {
			t.Errorf("should include glossary section")
		}
		if !strings.Contains(system, "订单") {
			t.Errorf("should include glossary terms")
		}
	})

	t.Run("with constraints", func(t *testing.T) {
		r := NewRequest("text").
			Optimize().
			WithConstraint("max 50 words").
			WithConstraint("no jargon")
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "CONSTRAINTS") {
			t.Errorf("should include constraints section")
		}
		if !strings.Contains(system, "max 50 words") {
			t.Errorf("should include first constraint")
		}
		if !strings.Contains(system, "no jargon") {
			t.Errorf("should include second constraint")
		}
	})

	t.Run("with context", func(t *testing.T) {
		r := NewRequest("text").
			Polish().
			WithContext("e-commerce email notification")
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "CONTEXT") {
			t.Errorf("should include context section")
		}
		if !strings.Contains(system, "e-commerce") {
			t.Errorf("should include context text")
		}
	})

	t.Run("with max length", func(t *testing.T) {
		r := NewRequest("text").Summarize().WithMaxLength(200)
		messages := r.buildPrompt()

		system := messages[0].Content

		if !strings.Contains(system, "200") {
			t.Errorf("should include max length")
		}
	})
}

func TestConvenienceConstructors(t *testing.T) {
	t.Run("NewEmailRequest", func(t *testing.T) {
		r := NewEmailRequest("<p>{{.Name}}</p>")

		if !r.options.isTemplate {
			t.Errorf("should be template")
		}
		if r.options.style != StyleProfessional {
			t.Errorf("should use professional style")
		}
		if r.options.purpose != PurposeEmail {
			t.Errorf("should have email purpose")
		}
	})

	t.Run("NewMarketingRequest", func(t *testing.T) {
		r := NewMarketingRequest("Buy now!")

		if r.options.style != StyleMarketing {
			t.Errorf("should use marketing style")
		}
		if r.options.tone != TonePersuasive {
			t.Errorf("should use persuasive tone")
		}
		if r.options.purpose != PurposeMarketing {
			t.Errorf("should have marketing purpose")
		}
	})

	t.Run("NewTechnicalRequest", func(t *testing.T) {
		r := NewTechnicalRequest("API documentation")

		if r.options.style != StyleTechnical {
			t.Errorf("should use technical style")
		}
		if r.options.purpose != PurposeDocumentation {
			t.Errorf("should have documentation purpose")
		}
	})
}

func TestTaskInstructions(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *Request
		expected []string
	}{
		{
			name:     "polish",
			setup:    func() *Request { return NewRequest("x").Polish() },
			expected: []string{"polish", "improve"},
		},
		{
			name:     "optimize",
			setup:    func() *Request { return NewRequest("x").Optimize() },
			expected: []string{"optimize", "effectiveness"},
		},
		{
			name:     "summarize",
			setup:    func() *Request { return NewRequest("x").Summarize() },
			expected: []string{"summarize"},
		},
		{
			name:     "expand",
			setup:    func() *Request { return NewRequest("x").Expand() },
			expected: []string{"expand", "detail"},
		},
		{
			name:     "rewrite",
			setup:    func() *Request { return NewRequest("x").Rewrite() },
			expected: []string{"rewrite"},
		},
		{
			name:     "proofread",
			setup:    func() *Request { return NewRequest("x").Proofread() },
			expected: []string{"proofread", "correct"},
		},
		{
			name:     "simplify",
			setup:    func() *Request { return NewRequest("x").Simplify() },
			expected: []string{"simplify"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.setup()
			instructions := r.buildTaskInstructions()

			for _, exp := range tt.expected {
				if !strings.Contains(strings.ToLower(instructions), exp) {
					t.Errorf("instructions should contain %q, got: %s", exp, instructions)
				}
			}
		})
	}
}
