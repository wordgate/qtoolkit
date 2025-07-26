package sdk

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ContentProcessor 内容处理器，用于从Hugo内容中提取产品信息
type ContentProcessor struct {
	// ConfigDir 配置文件所在目录，用作相对路径的基准
	ConfigDir string
	// Products 从内容文件中提取的产品列表
	Products []Product
	// FilePatterns 要匹配的文件模式列表
	FilePatterns []string
}

// NewContentProcessor 创建新的内容处理器
//
// configDir 参数指定配置文件所在目录，用于解析相对路径
// config 参数包含Wordgate配置信息
// 返回初始化后的ContentProcessor实例
func NewContentProcessor(configDir string, config *WordgateConfig) *ContentProcessor {
	return &ContentProcessor{
		ConfigDir:    configDir,
		Products:     []Product{},
		FilePatterns: config.Products.Files,
	}
}

// Process 处理内容目录，提取产品信息
//
// 遍历所有匹配的文件，提取产品信息，并返回产品列表
// 返回提取的产品列表和可能的错误
func (p *ContentProcessor) Process() ([]Product, error) {
	// 清空产品列表，确保每次都是新的结果
	p.Products = []Product{}

	// 处理每个产品配置文件匹配模式
	for _, pattern := range p.FilePatterns {
		// 获取匹配的文件，相对于配置文件目录
		fullPattern := filepath.Join(p.ConfigDir, pattern)
		files, err := filepath.Glob(fullPattern)
		if err != nil {
			return nil, fmt.Errorf("匹配文件失败: %w", err)
		}

		// 处理每个文件
		for _, file := range files {
			err = p.processContentFile(file)
			if err != nil {
				// 记录错误但继续处理其他文件
				fmt.Printf("处理文件 %s 失败: %v\n", file, err)
				continue
			}
		}
	}

	return p.Products, nil
}

// processContentFile 处理单个内容文件，提取其中的产品信息
//
// filePath 参数指定要处理的文件路径
// 返回可能的错误
func (p *ContentProcessor) processContentFile(filePath string) error {
	// 读取文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 提取前置元数据
	frontMatter, err := extractFrontMatter(string(content))
	if err != nil {
		return fmt.Errorf("提取前置元数据失败: %w", err)
	}

	// 解析前置元数据为结构化数据
	var metadata struct {
		Product struct {
			Code  string `yaml:"code"`
			Name  string `yaml:"name"`
			Price int    `yaml:"price"`
		} `yaml:"product"`
	}

	err = yaml.Unmarshal([]byte(frontMatter), &metadata)
	if err != nil {
		return fmt.Errorf("解析YAML失败: %w", err)
	}

	// 验证必要字段
	if metadata.Product.Code == "" {
		return fmt.Errorf("缺少必要的product.code字段")
	}
	if metadata.Product.Name == "" {
		return fmt.Errorf("缺少必要的product.name字段")
	}
	if metadata.Product.Price <= 0 {
		return fmt.Errorf("product.price字段必须大于0")
	}

	// 创建产品并添加到列表
	product := Product{
		Code:  metadata.Product.Code,
		Name:  metadata.Product.Name,
		Price: metadata.Product.Price,
	}

	// 添加到产品列表
	p.Products = append(p.Products, product)
	return nil
}

// extractFrontMatter 从Markdown内容中提取前置元数据
//
// content 参数包含Markdown文件内容
// 返回提取的前置元数据和可能的错误
func extractFrontMatter(content string) (string, error) {
	// 使用(?s)标志启用单行模式(DOTALL)，使.能匹配包括换行符在内的所有字符
	re := regexp.MustCompile(`(?s)^---\s*(.*?)\s*---`)
	parts := re.FindStringSubmatch(content)
	if len(parts) == 0 {
		// 尝试TOML格式
		re = regexp.MustCompile(`(?s)^\+\+\+\s*(.*?)\s*\+\+\+`)
		parts = re.FindStringSubmatch(content)
		if len(parts) == 0 {
			return "", fmt.Errorf("缺少前置元数据")
		}
	}

	// 提取前置元数据并去除首尾空白
	front := strings.TrimSpace(parts[1])
	return front, nil
}
