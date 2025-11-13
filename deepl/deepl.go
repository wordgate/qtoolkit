package deepl

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/cluttrdev/deepl-go/deepl"
	"github.com/spf13/viper"
)

// 全局单例客户端
var (
	defaultClient *deepl.Translator
	clientOnce    sync.Once
	clientErr     error

	// Go 模板标签正则 - 匹配 {{.xxx}}、{{ .xxx }}、{{xxx}} 等各种格式
	templateTagRegex = regexp.MustCompile(`\{\{[^}]*\}\}`)
)

// Config 配置结构
type Config struct {
	APIKey    string `yaml:"api_key" json:"api_key"`
	ServerURL string `yaml:"server_url" json:"server_url"` // 免费版: https://api-free.deepl.com, 付费版: https://api.deepl.com
}

// loadConfigFromViper loads DeepL configuration from viper
// Configuration path: deepl.api_key, deepl.server_url
// Priority: Environment variable > Viper config
func loadConfigFromViper() (*Config, error) {
	cfg := &Config{}

	// 优先从环境变量获取 API Key
	if apiKey := os.Getenv("DEEPL_API_KEY"); apiKey != "" {
		cfg.APIKey = apiKey
	} else {
		cfg.APIKey = viper.GetString("deepl.api_key")
	}

	// Server URL 从 viper 读取，默认使用免费版
	cfg.ServerURL = viper.GetString("deepl.server_url")
	if cfg.ServerURL == "" {
		cfg.ServerURL = "https://api-free.deepl.com"
	}

	// 验证必需字段
	if cfg.APIKey == "" || cfg.APIKey == "YOUR_DEEPL_API_KEY" {
		return nil, fmt.Errorf("deepl api key not configured (check DEEPL_API_KEY env or deepl.api_key)")
	}

	return cfg, nil
}

// getClient 获取或创建单例客户端
func getClient() (*deepl.Translator, error) {
	clientOnce.Do(func() {
		cfg, err := loadConfigFromViper()
		if err != nil {
			clientErr = fmt.Errorf("failed to load deepl config: %v", err)
			return
		}

		defaultClient, clientErr = deepl.NewTranslator(cfg.APIKey, deepl.WithServerURL(cfg.ServerURL))
	})
	return defaultClient, clientErr
}

// TranslateTpl 翻译单个文本，保护模板标签
func TranslateTpl(ctx context.Context, text, fromLang, targetLang string) (string, error) {
	if text == "" {
		return "", nil
	}

	client, err := getClient()
	if err != nil {
		return "", err
	}

	// 检测是否包含模板标签
	hasTemplate := templateTagRegex.MatchString(text)

	// 准备选项 - 暂时移除 WithSourceLang 因为可能导致 400 错误
	opts := []deepl.TranslateOption{}
	// if fromLang != "" && fromLang != "auto" {
	// 	opts = append(opts, deepl.WithSourceLang(normalizeLanguageCode(fromLang)))
	// }

	// 如果包含模板标签，使用 XML 标签处理
	if hasTemplate {
		// 将模板标签转换为 XML 标签进行保护
		// {{months}} -> <x>{{months}}</x>
		protected := templateTagRegex.ReplaceAllStringFunc(text, func(match string) string {
			return fmt.Sprintf("<x>%s</x>", match)
		})

		// 使用 XML 标签模式
		opts = append(opts,
			deepl.WithTagHandling("xml"),
			deepl.WithIgnoreTags([]string{"x"}), // 忽略 x 标签内容
		)

		// 执行翻译
		results, err := client.TranslateText([]string{protected}, normalizeLanguageCode(targetLang), opts...)
		if err != nil {
			return "", fmt.Errorf("translation failed: %w", err)
		}

		if len(results) == 0 {
			return "", fmt.Errorf("no translation result")
		}

		// 移除保护标签
		result := strings.ReplaceAll(results[0].Text, "<x>", "")
		result = strings.ReplaceAll(result, "</x>", "")

		return result, nil
	}

	// 没有模板标签，直接翻译
	results, err := client.TranslateText([]string{text}, normalizeLanguageCode(targetLang), opts...)
	if err != nil {
		return "", fmt.Errorf("translation failed: %w", err)
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no translation result")
	}

	return results[0].Text, nil
}

// TranslateTpls 批量翻译文本，保护模板标签
func TranslateTpls(ctx context.Context, texts []string, fromLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}

	client, err := getClient()
	if err != nil {
		return nil, err
	}

	// 检测哪些文本包含模板
	hasAnyTemplate := false
	protectedTexts := make([]string, len(texts))

	for i, text := range texts {
		if templateTagRegex.MatchString(text) {
			hasAnyTemplate = true
			// 保护模板标签
			protectedTexts[i] = templateTagRegex.ReplaceAllStringFunc(text, func(match string) string {
				return fmt.Sprintf("<x>%s</x>", match)
			})
		} else {
			protectedTexts[i] = text
		}
	}

	// 准备选项 - 暂时移除 WithSourceLang 因为可能导致 400 错误
	opts := []deepl.TranslateOption{}
	// if fromLang != "" && fromLang != "auto" {
	// 	opts = append(opts, deepl.WithSourceLang(normalizeLanguageCode(fromLang)))
	// }

	// 如果有模板，启用 XML 处理
	if hasAnyTemplate {
		opts = append(opts,
			deepl.WithTagHandling("xml"),
			deepl.WithIgnoreTags([]string{"x"}),
		)
	}

	// 执行翻译
	results, err := client.TranslateText(protectedTexts, normalizeLanguageCode(targetLang), opts...)
	if err != nil {
		return nil, fmt.Errorf("translation failed: %w", err)
	}

	// 提取结果并清理保护标签
	translations := make([]string, len(results))
	for i, result := range results {
		if hasAnyTemplate {
			translations[i] = strings.ReplaceAll(result.Text, "<x>", "")
			translations[i] = strings.ReplaceAll(translations[i], "</x>", "")
		} else {
			translations[i] = result.Text
		}
	}

	return translations, nil
}

// normalizeLanguageCode 标准化语言代码处理
func normalizeLanguageCode(code string) string {
	if code == "" {
		return ""
	}

	// 转换为小写进行匹配
	lower := strings.ToLower(code)

	// 完整的语言代码映射
	switch lower {
	// 中文
	case "zh", "zh-cn", "zh-hans", "chinese", "simplified chinese":
		return "ZH"
	case "zh-tw", "zh-hk", "zh-hant", "traditional chinese":
		// DeepL 不区分简繁体，统一使用 ZH
		return "ZH"

	// 英语
	case "en", "english":
		return "EN-US"
	case "en-us", "american english":
		return "EN-US"
	case "en-gb", "british english":
		return "EN-GB"
	case "en-au", "australian english":
		return "EN-GB" // DeepL doesn't support EN-AU, map to EN-GB

	// 日语
	case "ja", "jp", "japanese":
		return "JA"

	// 韩语
	case "ko", "kr", "korean":
		return "KO"

	// 西班牙语
	case "es", "spanish":
		return "ES"

	// 法语
	case "fr", "french":
		return "FR"

	// 德语
	case "de", "german":
		return "DE"

	// 意大利语
	case "it", "italian":
		return "IT"

	// 俄语
	case "ru", "russian":
		return "RU"

	// 葡萄牙语
	case "pt", "portuguese":
		return "PT-PT"
	case "pt-pt", "european portuguese":
		return "PT-PT"
	case "pt-br", "brazilian portuguese":
		return "PT-BR"

	// 荷兰语
	case "nl", "dutch":
		return "NL"

	// 波兰语
	case "pl", "polish":
		return "PL"

	// 瑞典语
	case "sv", "swedish":
		return "SV"

	// 丹麦语
	case "da", "danish":
		return "DA"

	// 挪威语
	case "no", "nb", "norwegian":
		return "NB"

	// 芬兰语
	case "fi", "finnish":
		return "FI"

	// 捷克语
	case "cs", "czech":
		return "CS"

	// 匈牙利语
	case "hu", "hungarian":
		return "HU"

	// 希腊语
	case "el", "greek":
		return "EL"

	// 保加利亚语
	case "bg", "bulgarian":
		return "BG"

	// 罗马尼亚语
	case "ro", "romanian":
		return "RO"

	// 斯洛伐克语
	case "sk", "slovak":
		return "SK"

	// 斯洛文尼亚语
	case "sl", "slovenian":
		return "SL"

	// 爱沙尼亚语
	case "et", "estonian":
		return "ET"

	// 拉脱维亚语
	case "lv", "latvian":
		return "LV"

	// 立陶宛语
	case "lt", "lithuanian":
		return "LT"

	// 土耳其语
	case "tr", "turkish":
		return "TR"

	// 乌克兰语
	case "uk", "ukrainian":
		return "UK"

	// 阿拉伯语
	case "ar", "arabic":
		return "AR"

	// 印尼语
	case "id", "indonesian":
		return "ID"

	default:
		// 如果没有匹配，转换为大写返回（让 DeepL 处理）
		return strings.ToUpper(code)
	}
}