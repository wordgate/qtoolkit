package mods

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

var redisClients map[string]*redis.Client = make(map[string]*redis.Client)
var redisMux sync.RWMutex

func initRedis(app string) *redis.Client {
	addr := viper.GetString("redis.addr")
	password := viper.GetString("redis.password")
	db := viper.GetInt("redis.db")
	if app != "" {
		addr = viper.GetString(fmt.Sprintf("%s.redis.addr", app))
		password = viper.GetString(fmt.Sprintf("%s.redis.password", app))
		db = viper.GetInt(fmt.Sprintf("%s.redis.db", app))
	}
	if addr == "" {
		panic("no redis config for app:" + app)
	}

	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
}

func RedisDefault() *redis.Client {
	return Redis("")
}

func Redis(app string) *redis.Client {
	redisMux.RLock()
	client, ok := redisClients[app]
	redisMux.RUnlock()
	if !ok {
		redisMux.Lock()
		client = initRedis(app)
		redisClients[app] = client
		redisMux.Unlock()
	}
	return client
}

func RedisSubscribe(app string, channel string) chan string {
	rds := Redis(app)
	ctx := context.Background()
	pubsub := rds.Subscribe(ctx, "broadcast")
	payloadCH := make(chan string)
	go func() {
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

func RedisPublish(app string, channel string, payload string) error {
	rds := Redis(app)
	ctx := context.Background()
	return rds.Publish(ctx, channel, payload).Err()
}
