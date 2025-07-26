package qtoolkit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

func CacheGet(key string, val interface{}) (exist bool, err error) {
	jsonData, err := RedisDefault().Get(
		context.Background(),
		key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, json.Unmarshal([]byte(jsonData), val)
}

func CacheSet(key string, value interface{}, seconds int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return RedisDefault().Set(
		context.Background(),
		key, data,
		time.Second*time.Duration(seconds)).Err()
}

func CacheDel(key string) error {
	return RedisDefault().Del(
		context.Background(),
		key).Err()
}

func CacheHSet(key, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return RedisDefault().HSet(
		context.Background(),
		key, field, data).Err()
}

func CacheHGet(key, field string, val interface{}) (exist bool, err error) {
	jsonData, err := RedisDefault().HGet(
		context.Background(),
		key, field).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, json.Unmarshal([]byte(jsonData), val)
}

func CacheHKeys(key string) ([]string, error) {
	return RedisDefault().HKeys(
		context.Background(),
		key).Result()
}

// TryLock 尝试获取分布式锁
// key: 锁的键名
// expireSeconds: 锁的过期时间（秒）
// 返回值：(是否获取到锁, 错误)
func TryLock(key string, expireSeconds int) (bool, error) {
	// 使用 SetNX 命令，确保原子性
	success, err := RedisDefault().SetNX(
		context.Background(),
		key,
		"1",
		time.Duration(expireSeconds)*time.Second,
	).Result()

	if err != nil {
		return false, err
	}

	return success, nil
}
