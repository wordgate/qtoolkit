package redis

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// 全局单例客户端
var (
	defaultClient *redis.Client
	clientOnce    sync.Once
)

// initClient initializes the singleton client from viper configuration (lazy load)
func initClient() *redis.Client {
	clientOnce.Do(func() {
		addr := viper.GetString("redis.addr")
		password := viper.GetString("redis.password")
		db := viper.GetInt("redis.db")

		if addr == "" {
			return
		}

		defaultClient = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		})
	})
	return defaultClient
}

// Client 获取Redis客户端
// Configuration is automatically loaded from viper on first use
func Client() *redis.Client {
	client := initClient()
	if client == nil {
		panic("redis client not configured")
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