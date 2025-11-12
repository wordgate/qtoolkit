package unred_test

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/unred"
)

// Example_basicUsage demonstrates basic usage of the unred package
func Example_basicUsage() {
	// 配置 viper
	viper.Set("unred.api_endpoint", "api.x.all7.cc")
	viper.Set("unred.secret_key", "your-secret-key")

	// 创建短链接
	expireAt := time.Now().Add(30 * 24 * time.Hour).Unix() // 30天后过期
	resp, err := unred.CreateLink("/s/example", "https://example.com", expireAt)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Created short URL: %s\n", resp.URL)

	// 删除短链接
	deleteResp, err := unred.DeleteLink("/s/example")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Deleted: %v\n", deleteResp.Success)
}

// Example_customClient demonstrates using a custom client
func Example_customClient() {
	// 创建自定义客户端
	client := unred.NewClient("api.x.all7.cc", "your-secret-key")

	// 创建短链接（不设置过期时间）
	resp, err := client.CreateLink("/promo/sale", "https://shop.example.com/sale", 0)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Created: %s\n", resp.URL)

	// 删除短链接
	deleteResp, err := client.DeleteLink("/promo/sale")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Deleted: %v\n", deleteResp.Success)
}

// Example_batchCreate demonstrates batch creation of short links
func Example_batchCreate() {
	client := unred.NewClient("api.x.all7.cc", "your-secret-key")

	links := []struct {
		path      string
		targetURL string
	}{
		{"/product/item1", "https://shop.example.com/item1"},
		{"/product/item2", "https://shop.example.com/item2"},
		{"/product/item3", "https://shop.example.com/item3"},
	}

	expireAt := time.Now().Add(7 * 24 * time.Hour).Unix() // 7天后过期

	for _, link := range links {
		resp, err := client.CreateLink(link.path, link.targetURL, expireAt)
		if err != nil {
			fmt.Printf("Failed to create %s: %v\n", link.path, err)
			continue
		}
		fmt.Printf("✓ Created: %s\n", resp.URL)
	}
}
