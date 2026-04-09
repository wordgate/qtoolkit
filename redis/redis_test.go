package redis

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func setupTestRedis() {
	// 重置单例客户端
	clientOnce = sync.Once{}
	defaultClient = nil

	// 使用 viper 设置测试配置
	viper.Set("redis.addr", "localhost:6379")
	viper.Set("redis.password", "")
	viper.Set("redis.db", 0)
}

func TestRedisConnection(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	if client == nil {
		t.Fatal("Failed to get Redis client")
	}

	ctx := context.Background()
	err := client.Ping(ctx).Err()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}
}

func TestCacheOperations(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// 清理测试数据
	defer func() {
		CacheDel("test_key")
		CacheDel("test_hash")
	}()

	// 测试基本缓存操作
	testData := map[string]interface{}{
		"name": "test",
		"age":  25,
	}

	// 设置缓存
	err := CacheSet("test_key", testData, 60)
	if err != nil {
		t.Fatalf("CacheSet failed: %v", err)
	}

	// 获取缓存
	var result map[string]interface{}
	exists, err := CacheGet("test_key", &result)
	if err != nil {
		t.Fatalf("CacheGet failed: %v", err)
	}

	if !exists {
		t.Fatal("Cache data should exist")
	}

	if result["name"] != "test" {
		t.Errorf("Expected name=test, got %v", result["name"])
	}
}

func TestCacheHashOperations(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// 清理测试数据
	defer CacheDel("test_hash")

	// 测试Hash操作
	err := CacheHSet("test_hash", "field1", "value1")
	if err != nil {
		t.Fatalf("CacheHSet failed: %v", err)
	}

	var value string
	exists, err := CacheHGet("test_hash", "field1", &value)
	if err != nil {
		t.Fatalf("CacheHGet failed: %v", err)
	}

	if !exists {
		t.Fatal("Hash field should exist")
	}

	if value != "value1" {
		t.Errorf("Expected value1, got %v", value)
	}

	// 测试获取Hash键列表
	keys, err := CacheHKeys("test_hash")
	if err != nil {
		t.Fatalf("CacheHKeys failed: %v", err)
	}

	if len(keys) != 1 || keys[0] != "field1" {
		t.Errorf("Expected [field1], got %v", keys)
	}
}

func TestCacheGetDel(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	defer CacheDel("test_getdel")

	// Set a value first
	if err := CacheSet("test_getdel", "hello", 60); err != nil {
		t.Fatalf("CacheSet failed: %v", err)
	}

	// GetDel should return the value and delete it
	var val string
	exists, err := CacheGetDel("test_getdel", &val)
	if err != nil {
		t.Fatalf("CacheGetDel failed: %v", err)
	}
	if !exists {
		t.Fatal("CacheGetDel: key should exist")
	}
	if val != "hello" {
		t.Errorf("CacheGetDel: expected 'hello', got %q", val)
	}

	// Key should be gone now
	exists, err = CacheGet("test_getdel", &val)
	if err != nil {
		t.Fatalf("CacheGet after GetDel failed: %v", err)
	}
	if exists {
		t.Fatal("Key should not exist after CacheGetDel")
	}

	// GetDel on missing key
	exists, err = CacheGetDel("test_getdel_missing", &val)
	if err != nil {
		t.Fatalf("CacheGetDel on missing key failed: %v", err)
	}
	if exists {
		t.Fatal("CacheGetDel on missing key should return false")
	}
}

func TestCacheGetEx(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	defer CacheDel("test_getex")

	// Set a value with short TTL
	if err := CacheSet("test_getex", "world", 2); err != nil {
		t.Fatalf("CacheSet failed: %v", err)
	}

	// GetEx should return value and refresh TTL to 60s
	var val string
	exists, err := CacheGetEx("test_getex", &val, 60)
	if err != nil {
		t.Fatalf("CacheGetEx failed: %v", err)
	}
	if !exists {
		t.Fatal("CacheGetEx: key should exist")
	}
	if val != "world" {
		t.Errorf("CacheGetEx: expected 'world', got %q", val)
	}

	// Verify TTL was refreshed (should be > 2s now)
	ttl := client.TTL(ctx, "test_getex").Val()
	if ttl < 10*time.Second {
		t.Errorf("TTL should have been refreshed to ~60s, got %v", ttl)
	}

	// GetEx on missing key
	exists, err = CacheGetEx("test_getex_missing", &val, 60)
	if err != nil {
		t.Fatalf("CacheGetEx on missing key failed: %v", err)
	}
	if exists {
		t.Fatal("CacheGetEx on missing key should return false")
	}
}

func TestCacheHDel(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	defer CacheDel("test_hdel")

	// Set multiple hash fields
	if err := CacheHSet("test_hdel", "f1", "v1"); err != nil {
		t.Fatalf("CacheHSet failed: %v", err)
	}
	if err := CacheHSet("test_hdel", "f2", "v2"); err != nil {
		t.Fatalf("CacheHSet failed: %v", err)
	}
	if err := CacheHSet("test_hdel", "f3", "v3"); err != nil {
		t.Fatalf("CacheHSet failed: %v", err)
	}

	// Delete one field
	if err := CacheHDel("test_hdel", "f1"); err != nil {
		t.Fatalf("CacheHDel failed: %v", err)
	}

	// f1 should be gone
	var val string
	exists, _ := CacheHGet("test_hdel", "f1", &val)
	if exists {
		t.Fatal("f1 should not exist after CacheHDel")
	}

	// Delete multiple fields at once
	if err := CacheHDel("test_hdel", "f2", "f3"); err != nil {
		t.Fatalf("CacheHDel multiple failed: %v", err)
	}

	keys, _ := CacheHKeys("test_hdel")
	if len(keys) != 0 {
		t.Errorf("Expected no remaining keys, got %v", keys)
	}
}

func TestCacheHGetAll(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	defer CacheDel("test_hgetall")

	// Empty hash should return empty map
	result, err := CacheHGetAll[string]("test_hgetall")
	if err != nil {
		t.Fatalf("CacheHGetAll on empty hash failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty map for empty hash, got %v", result)
	}

	// Set some fields
	type Item struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	if err := CacheHSet("test_hgetall", "a", Item{Name: "alpha", Count: 1}); err != nil {
		t.Fatalf("CacheHSet failed: %v", err)
	}
	if err := CacheHSet("test_hgetall", "b", Item{Name: "beta", Count: 2}); err != nil {
		t.Fatalf("CacheHSet failed: %v", err)
	}

	// Get all with generic type
	items, err := CacheHGetAll[Item]("test_hgetall")
	if err != nil {
		t.Fatalf("CacheHGetAll failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}
	if items["a"].Name != "alpha" || items["a"].Count != 1 {
		t.Errorf("Unexpected item a: %+v", items["a"])
	}
	if items["b"].Name != "beta" || items["b"].Count != 2 {
		t.Errorf("Unexpected item b: %+v", items["b"])
	}
}

func TestDistributedLock(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	lockKey := "test_lock"
	defer ReleaseLock(lockKey)

	// 第一次获取锁应该成功
	success, err := TryLock(lockKey, 5)
	if err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}

	if !success {
		t.Fatal("First lock attempt should succeed")
	}

	// 第二次获取锁应该失败
	success, err = TryLock(lockKey, 5)
	if err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}

	if success {
		t.Fatal("Second lock attempt should fail")
	}

	// 释放锁
	err = ReleaseLock(lockKey)
	if err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}

	// 释放后应该能再次获取锁
	success, err = TryLock(lockKey, 5)
	if err != nil {
		t.Fatalf("TryLock after release failed: %v", err)
	}

	if !success {
		t.Fatal("Lock attempt after release should succeed")
	}
}

func TestBroadcastBasic(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// 创建广播实例
	broadcast := NewBroadcast(10)
	if broadcast == nil {
		t.Fatal("Failed to create broadcast instance")
	}

	// 测试发布消息
	err := broadcast.Pub(ctx, "test_channel", "test_message")
	if err != nil {
		t.Fatalf("Broadcast Pub failed: %v", err)
	}
}

func TestPubSub(t *testing.T) {
	if os.Getenv("REDIS_TEST_SKIP") != "" {
		t.Skip("Skipping Redis tests (REDIS_TEST_SKIP is set)")
	}

	setupTestRedis()

	client := Client()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	channel := "test_pubsub_channel"
	message := "test_message"

	// 启动订阅
	ch := Subscribe(channel)
	defer func() {
		// 清理订阅
		close(ch)
	}()

	// 等待订阅建立
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	err := Publish(channel, message)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// 等待接收消息
	select {
	case receivedMsg := <-ch:
		if receivedMsg != message {
			t.Errorf("Expected %s, got %s", message, receivedMsg)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for message")
	}
}