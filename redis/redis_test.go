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