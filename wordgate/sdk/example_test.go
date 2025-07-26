package sdk_test

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/allnationconnect/mods/wordgate/sdk"
)

// Example 展示了如何使用SDK执行基本的同步操作
func Example() {
	// 获取配置文件路径和目录
	configPath := "config.yaml"
	absConfigPath, _ := filepath.Abs(configPath)
	configDir := filepath.Dir(absConfigPath)

	// 加载配置
	config, err := sdk.LoadConfig(absConfigPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建客户端
	client := sdk.NewClient(config, configDir)

	// 执行干运行查看将要同步的数据
	dryRunResult, err := client.DryRun()
	if err != nil {
		log.Fatalf("干运行失败: %v", err)
	}

	// 输出产品信息
	fmt.Printf("产品数量: %d\n", len(dryRunResult.Products))
	for _, product := range dryRunResult.Products {
		fmt.Printf("产品: %s (%s) 价格: %.2f\n",
			product.Name,
			product.Code,
			float64(product.Price)/100)
	}

	// 执行同步
	// 注意：这里不实际执行，只是示例
	// result, err := client.SyncAll()
	// if err != nil {
	//     log.Fatalf("同步失败: %v", err)
	// }
	// fmt.Printf("同步状态: %v\n", result.Success)
}

// ExampleDryRun 展示如何执行干运行模式
func ExampleClient_DryRun() {
	// 创建客户端（实际使用时需要加载配置）
	client := &.Client{} // 简化示例，实际使用时应使用NewClient创建

	// 执行干运行
	result, err := client.DryRun()
	if err != nil {
		log.Fatalf("干运行失败: %v", err)
	}

	// 处理结果
	fmt.Printf("应用名称: %s\n", result.AppConfig.Name)
	fmt.Printf("会员等级数量: %d\n", len(result.Memberships))
	fmt.Printf("产品数量: %d\n", len(result.Products))

	// Output:
	// 应用名称:
	// 会员等级数量: 0
	// 产品数量: 0
}

// ExampleLoadConfig 展示如何加载配置文件
func ExampleLoadConfig() {
	// 加载配置文件
	// config, err := .LoadConfig("config.yaml")
	//
	// if err != nil {
	//     log.Fatalf("加载配置失败: %v", err)
	// }
	//
	// fmt.Printf("应用名称: %s\n", config.App.Name)
	// fmt.Printf("API基础URL: %s\n", config.BaseURL)
}
