package deepl

import (
	"context"
	"strings"
	"testing"
)

func TestTranslateTpl(t *testing.T) {
	// 需要配置文件中有正确的 API Key

	tests := []struct {
		name       string
		text       string
		fromLang   string
		targetLang string
		wantErr    bool
		expectTags bool // 期望结果中包含模板标签
	}{
		{
			name:       "Template tags protection",
			text:       "Pro Authorization {{months}} months",
			fromLang:   "en",
			targetLang: "zh",
			wantErr:    false,
			expectTags: true,
		},
		{
			name:       "Multiple template tags",
			text:       "Hello {{.Name}}, welcome to {{.Site}}!",
			fromLang:   "en",
			targetLang: "zh",
			wantErr:    false,
			expectTags: true,
		},
		{
			name:       "No template tags",
			text:       "Hello, World!",
			fromLang:   "en",
			targetLang: "zh",
			wantErr:    false,
			expectTags: false,
		},
		{
			name:       "Empty text",
			text:       "",
			fromLang:   "en",
			targetLang: "zh",
			wantErr:    false,
			expectTags: false,
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranslateTpl(ctx, tt.text, tt.fromLang, tt.targetLang)
			if (err != nil) != tt.wantErr {
				t.Errorf("TranslateTpl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.text != "" && result == "" && !tt.wantErr {
				t.Errorf("TranslateTpl() got empty result for non-empty text")
				return
			}

			// 检查模板标签是否被保护
			if tt.expectTags {
				if !strings.Contains(result, "{{") {
					t.Errorf("TranslateTpl() expected template tags in result, got: %s", result)
				}
				t.Logf("Input: %s", tt.text)
				t.Logf("Output: %s", result)
			}
		})
	}
}

func TestTranslateTpls(t *testing.T) {
	// 需要配置文件中有正确的 API Key

	tests := []struct {
		name       string
		texts      []string
		fromLang   string
		targetLang string
		wantErr    bool
	}{
		{
			name: "Mixed texts with and without templates",
			texts: []string{
				"Pro Authorization {{months}} months",
				"Hello World",
				"Welcome {{.User}}!",
			},
			fromLang:   "en",
			targetLang: "zh",
			wantErr:    false,
		},
		{
			name:       "Empty array",
			texts:      []string{},
			fromLang:   "en",
			targetLang: "zh",
			wantErr:    false,
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := TranslateTpls(ctx, tt.texts, tt.fromLang, tt.targetLang)
			if (err != nil) != tt.wantErr {
				t.Errorf("TranslateTpls() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(results) != len(tt.texts) && !tt.wantErr {
				t.Errorf("TranslateTpls() got %d results, want %d", len(results), len(tt.texts))
				return
			}

			// 检查结果
			for i, result := range results {
				if i < len(tt.texts) {
					t.Logf("Input[%d]: %s", i, tt.texts[i])
					t.Logf("Output[%d]: %s", i, result)

					// 检查模板标签是否被保护
					if strings.Contains(tt.texts[i], "{{") {
						if !strings.Contains(result, "{{") {
							t.Errorf("TranslateTpls() expected template tags in result[%d], got: %s", i, result)
						}
					}
				}
			}
		})
	}
}

func TestNormalizeLanguageCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// 中文 - 大小写测试
		{"zh", "ZH"},
		{"ZH", "ZH"},
		{"zh-CN", "ZH"},
		{"zh-cn", "ZH"},
		{"ZH-CN", "ZH"},
		{"zh-tw", "ZH"},
		{"ZH-TW", "ZH"},
		{"zh-hk", "ZH"},
		{"chinese", "ZH"},
		{"Chinese", "ZH"},

		// 英语
		{"en", "EN-US"},
		{"EN", "EN-US"},
		{"en-GB", "EN-GB"},
		{"En-Gb", "EN-GB"},
		{"en-AU", "EN-GB"},
		{"en-au", "EN-GB"},
		{"australian english", "EN-GB"},

		// 日语
		{"ja", "JA"},
		{"JA", "JA"},
		{"jp", "JA"},
		{"japanese", "JA"},

		// 未知代码 - 转换为大写
		{"xyz", "XYZ"},
		{"unknown", "UNKNOWN"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeLanguageCode(tt.input); got != tt.want {
				t.Errorf("normalizeLanguageCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Example 使用示例
func ExampleTranslateTpl() {
	// 通过配置文件或环境变量设置API密钥
	// export DEEPL_API_KEY=your-api-key

	// 翻译带模板标签的文本
	ctx := context.Background()
	result, err := TranslateTpl(ctx, "Pro Authorization {{months}} months", "en", "zh")
	if err != nil {
		// 处理错误
		return
	}
	println(result) // 应该输出类似: "专业授权 {{months}} 个月"
}

func ExampleTranslateTpls() {
	// 批量翻译
	ctx := context.Background()
	texts := []string{
		"Hello {{.Name}}",
		"Welcome to {{.Site}}",
		"Normal text",
	}
	results, err := TranslateTpls(ctx, texts, "en", "zh")
	if err != nil {
		// 处理错误
		return
	}
	for i, result := range results {
		println(texts[i], "->", result)
	}
}