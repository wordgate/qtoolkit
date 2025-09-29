package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheGet 从缓存获取数据
func CacheGet(key string, val interface{}) (exist bool, err error) {
	jsonData, err := Client().Get(
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

// CacheSet 设置缓存数据
func CacheSet(key string, value interface{}, seconds int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return Client().Set(
		context.Background(),
		key, data,
		time.Second*time.Duration(seconds)).Err()
}

// CacheDel 删除缓存数据
func CacheDel(key string) error {
	return Client().Del(
		context.Background(),
		key).Err()
}

// CacheHSet 设置Hash缓存数据
func CacheHSet(key, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return Client().HSet(
		context.Background(),
		key, field, data).Err()
}

// CacheHGet 获取Hash缓存数据
func CacheHGet(key, field string, val interface{}) (exist bool, err error) {
	jsonData, err := Client().HGet(
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

// CacheHKeys 获取Hash键列表
func CacheHKeys(key string) ([]string, error) {
	return Client().HKeys(
		context.Background(),
		key).Result()
}

// TryLock 尝试获取分布式锁
// key: 锁的键名
// expireSeconds: 锁的过期时间（秒）
// 返回值：(是否获取到锁, 错误)
func TryLock(key string, expireSeconds int) (bool, error) {
	// 使用 SetNX 命令，确保原子性
	success, err := Client().SetNX(
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

// ReleaseLock 释放分布式锁
func ReleaseLock(key string) error {
	return Client().Del(context.Background(), key).Err()
}