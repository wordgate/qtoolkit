package redis

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

// 全局单例客户端
var (
	defaultClient *redis.Client
	clientOnce    sync.Once
	clientErr     error
)

// ConfigWrapper 外层配置结构
type ConfigWrapper struct {
	Redis Config `yaml:"redis"`
}

// Config Redis配置结构
type Config struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// getClient 获取或创建单例客户端
func getClient() (*redis.Client, error) {
	clientOnce.Do(func() {
		var wrapper ConfigWrapper

		// 优先从环境变量获取
		addr := os.Getenv("REDIS_ADDR")
		password := os.Getenv("REDIS_PASSWORD")
		db := 0

		if addr != "" {
			wrapper.Redis.Addr = addr
			wrapper.Redis.Password = password
			wrapper.Redis.DB = db
		} else {
			// 尝试从配置文件加载
			configPaths := []string{
				"redis_config.yml",
				"redis/redis_config.yml",
				"config/redis.yml",
			}

			for _, path := range configPaths {
				if data, err := os.ReadFile(path); err == nil {
					yaml.Unmarshal(data, &wrapper)
					if wrapper.Redis.Addr != "" && wrapper.Redis.Addr != "YOUR_REDIS_ADDR" {
						break
					}
				}
			}
		}

		if wrapper.Redis.Addr == "" || wrapper.Redis.Addr == "YOUR_REDIS_ADDR" {
			clientErr = fmt.Errorf("redis addr not configured")
			return
		}

		defaultClient = redis.NewClient(&redis.Options{
			Addr:     wrapper.Redis.Addr,
			Password: wrapper.Redis.Password,
			DB:       wrapper.Redis.DB,
		})
	})
	return defaultClient, clientErr
}

// Client 获取Redis客户端
func Client() *redis.Client {
	client, err := getClient()
	if err != nil {
		panic(err)
	}
	return client
}

// Subscribe 订阅Redis频道
func Subscribe(channel string) chan string {
	rds := Client()
	ctx := context.Background()
	pubsub := rds.Subscribe(ctx, channel)
	payloadCH := make(chan string)
	go func() {
		defer close(payloadCH)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				continue
			}
			payloadCH <- msg.Payload
		}
	}()
	return payloadCH
}

// Publish 发布消息到Redis频道
func Publish(channel string, payload string) error {
	rds := Client()
	ctx := context.Background()
	return rds.Publish(ctx, channel, payload).Err()
}

// Close 关闭Redis连接
func Close() error {
	if defaultClient != nil {
		return defaultClient.Close()
	}
	return nil
}