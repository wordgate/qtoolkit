# Asynq - Async Task Queue

基于 [hibiken/asynq](https://github.com/hibiken/asynq) 的异步任务队列模块，提供简化的 API 和自动生命周期管理。

## Features

- **零配置启动**: Worker 自动启动，无需显式调用
- **优雅关闭**: 自动监听信号，确保任务不丢失
- **配置驱动**: 通过 viper 自动加载，复用 redis 连接配置
- **灵活挂载**: Monitor UI 作为 Handler，由使用方控制路径和中间件

## Quick Start

```go
package main

import (
    "context"

    "github.com/gin-gonic/gin"
    "github.com/spf13/viper"
    "github.com/wordgate/qtoolkit/asynq"
)

func main() {
    viper.SetConfigFile("config.yml")
    viper.ReadInConfig()

    // 注册任务处理器
    asynq.Handle("email:send", handleEmailSend)

    r := gin.Default()

    // 挂载监控 UI (Worker 自动启动)
    asynq.Mount(r, "/asynq")

    // 业务接口
    r.POST("/send-email", func(c *gin.Context) {
        info, _ := asynq.Enqueue("email:send", map[string]string{
            "to": c.PostForm("to"),
        })
        c.JSON(200, gin.H{"task_id": info.ID})
    })

    r.Run(":8080")  // 退出时自动优雅关闭
}

func handleEmailSend(ctx context.Context, payload []byte) error {
    var p map[string]string
    asynq.Unmarshal(payload, &p)
    // 发送邮件...
    return nil
}
```

## Configuration

```yaml
# config.yml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0

asynq:
  concurrency: 10
  queues:
    critical: 6
    default: 3
    low: 1
  monitor:
    readonly: false
```

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `asynq.concurrency` | int | 10 | Worker 并发数 |
| `asynq.queues` | map | `{"default": 1}` | 队列优先级 |
| `asynq.strict_priority` | bool | false | 严格优先级模式 |
| `asynq.default_max_retry` | int | 3 | 默认最大重试次数 |
| `asynq.default_timeout` | duration | 30m | 默认任务超时 |
| `asynq.monitor.readonly` | bool | false | Monitor 只读模式 |

## API Reference

### 任务入队

```go
// 立即执行
asynq.Enqueue("task:type", payload)

// 延迟执行
asynq.EnqueueIn("task:type", payload, 5*time.Minute)

// 定时执行
asynq.EnqueueAt("task:type", payload, scheduledTime)

// 唯一任务 (去重)
asynq.EnqueueUnique("task:type", payload, 1*time.Hour)

// 带选项
asynq.Enqueue("task:type", payload,
    asynq.Queue("critical"),
    asynq.MaxRetry(5),
    asynq.Timeout(10*time.Minute),
)
```

### 处理器注册

```go
asynq.Handle("task:type", func(ctx context.Context, payload []byte) error {
    var data MyPayload
    asynq.Unmarshal(payload, &data)
    // 处理逻辑...
    return nil
})
```

### 监控 UI

```go
// 基本用法
asynq.Mount(r, "/asynq")

// 带认证中间件
asynq.Mount(r.Group("/admin", authMiddleware), "/tasks")
```

## Deployment Modes

### Mode 1: API + Worker 混合 (推荐)

```go
func main() {
    viper.ReadInConfig()

    asynq.Handle("email:send", handleEmailSend)

    r := gin.Default()
    asynq.Mount(r, "/asynq")  // 自动启动 Worker
    r.POST("/send", func(c *gin.Context) {
        asynq.Enqueue("email:send", payload)
    })

    r.Run(":8080")
}
```

### Mode 2: 独立 Worker 进程

**API 服务** (只入队):
```go
func main() {
    viper.ReadInConfig()

    r := gin.Default()
    r.POST("/send", func(c *gin.Context) {
        asynq.Enqueue("email:send", payload)
    })
    r.Run(":8080")
}
```

**Worker 服务** (阻塞运行):
```go
func main() {
    viper.ReadInConfig()

    asynq.Handle("email:send", handleEmailSend)

    if err := asynq.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Auto Worker Lifecycle

```
┌─────────────────────────────────────────────────────┐
│  asynq.Handle() 注册 handler                        │
│         ↓                                           │
│  asynq.Mount() 或 Enqueue() 首次调用                │
│         ↓                                           │
│  Worker 自动启动 (sync.Once)                        │
│         ↓                                           │
│  自动注册 SIGINT/SIGTERM 信号监听                   │
│         ↓                                           │
│  进程退出时:                                        │
│    1. 停止接收新任务                                │
│    2. 等待当前任务完成                              │
│    3. 关闭连接                                      │
└─────────────────────────────────────────────────────┘
```

## Advanced

### 手动控制 (一般不需要)

```go
// 阻塞运行 Worker (独立进程场景)
asynq.Run()

// 手动关闭
asynq.Shutdown()
```
