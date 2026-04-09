package redis

import (
	"context"
	"encoding/json"
	"fmt"
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

// CacheGetDel atomically gets and deletes a cache entry (Redis GETDEL).
// Returns (exist, error). If the key exists, it is removed and val is populated.
func CacheGetDel(key string, val interface{}) (exist bool, err error) {
	jsonData, err := Client().GetDel(
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

// CacheGetEx atomically gets a cache entry and refreshes its TTL (Redis GETEX).
// Returns (exist, error). If the key exists, its expiration is updated.
// Note: seconds must be > 0. Passing 0 will make the key persistent (removes expiry).
func CacheGetEx(key string, val interface{}, seconds int) (exist bool, err error) {
	jsonData, err := Client().GetEx(
		context.Background(),
		key,
		time.Second*time.Duration(seconds)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(jsonData), val)
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

// CacheHDel deletes one or more fields from a hash.
func CacheHDel(key string, fields ...string) error {
	return Client().HDel(
		context.Background(),
		key, fields...).Err()
}

// CacheHKeys 获取Hash键列表
func CacheHKeys(key string) ([]string, error) {
	return Client().HKeys(
		context.Background(),
		key).Result()
}

// CacheHGetAll retrieves all fields from a hash and JSON-unmarshals each value
// into the provided map. The map must be of type map[string]*T or map[string]T
// where T is JSON-deserializable.
func CacheHGetAll[T any](key string) (map[string]T, error) {
	result, err := Client().HGetAll(
		context.Background(),
		key).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return map[string]T{}, nil
	}
	out := make(map[string]T, len(result))
	for field, jsonData := range result {
		var val T
		if err := json.Unmarshal([]byte(jsonData), &val); err != nil {
			return nil, fmt.Errorf("unmarshal field %q: %w", field, err)
		}
		out[field] = val
	}
	return out, nil
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