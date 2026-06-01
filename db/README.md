# Database Module

数据库连接管理模块，使用 `sync.Once` 实现 lazy load 模式。

## 特性

- ✅ **Lazy Loading**: 第一次调用 `Get()` 时自动初始化
- ✅ **Thread-Safe**: 使用 `sync.Once` 确保只初始化一次
- ✅ **配置驱动**: 独立的配置结构，不依赖全局配置库
- ✅ **错误处理**: 提供 `Get()` 和 `MustGet()` 两种获取方式

## 使用方法

### 基本用法

```go
import "github.com/wordgate/qtoolkit/db"

// 1. 设置配置（在使用前）
db.SetConfig(&db.Config{
    DSN:   "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local",
    Debug: true, // 开发环境设置为 true
})

// 2. 使用数据库（自动初始化）
db.Get().Create(&user)
db.Get().Where("id = ?", 1).First(&user)
```

### 使用 MustGet（推荐）

如果初始化失败需要 panic 中断程序：

```go
// 设置配置
db.SetConfig(&db.Config{
    DSN:   "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4",
    Debug: false,
})

// 使用 MustGet，初始化失败会 panic
db.MustGet().AutoMigrate(&User{}, &Order{})
db.MustGet().Create(&user)
```

### 使用 Get（安全模式）

如果需要处理初始化错误：

```go
db.SetConfig(&db.Config{
    DSN: "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4",
})

// Get 返回 nil 如果初始化失败
if db.Get() == nil {
    log.Fatalf("Database initialization failed: %v", db.GetError())
}

db.Get().Create(&user)
```

### 完整示例

```go
package main

import (
    "log"
    "github.com/wordgate/qtoolkit/db"
)

type User struct {
    ID   uint   `gorm:"primaryKey"`
    Name string `gorm:"size:100"`
}

func main() {
    // 配置数据库
    db.SetConfig(&db.Config{
        DSN:   "root:password@tcp(127.0.0.1:3306)/myapp?charset=utf8mb4&parseTime=True",
        Debug: true,
    })

    // 自动迁移（第一次调用时会自动初始化数据库）
    db.MustGet().AutoMigrate(&User{})

    // 创建记录
    user := User{Name: "Alice"}
    db.Get().Create(&user)

    // 查询记录
    var users []User
    db.Get().Find(&users)

    log.Printf("Found %d users", len(users))

    // 程序退出时关闭连接
    defer db.Close()
}
```

## API 说明

### SetConfig(cfg *Config)
设置数据库配置，必须在第一次 `Get()` 调用前设置。

### GetConfig() *Config
获取当前的数据库配置。

### Get() *gorm.DB
获取数据库实例，使用 lazy load 模式：
- 第一次调用时自动初始化
- 如果初始化失败返回 `nil`
- 多次调用返回相同实例

### MustGet() *gorm.DB
获取数据库实例，如果初始化失败则 panic：
- 第一次调用时自动初始化
- 初始化失败会 panic，适合在启动时使用

### GetError() error
获取初始化错误（如果有）。

### Close() error
关闭数据库连接。

### Reset()
重置数据库状态，主要用于测试。

## 配置结构

```go
type Config struct {
    DSN   string  // MySQL DSN 连接字符串
    Debug bool    // 是否开启调试模式

    // 连接池调优（均可选，<= 0 时使用默认值）
    MaxOpenConns    int  // 最大并发连接数；默认 0 = 不限制
    MaxIdleConns    int  // 保活的空闲连接数；默认 5（标准库默认仅 2）
    ConnMaxLifetime int  // 连接最长存活秒数；默认 3600（1h，标准库默认 0 = 永不回收）
    ConnMaxIdleTime int  // 空闲连接最长存活秒数；默认 300（5m，标准库默认 0 = 不超时）
}
```

### 连接池默认值

通过 `database/sql` 标准库的 `SetMaxOpenConns / SetMaxIdleConns / SetConnMaxLifetime /
SetConnMaxIdleTime` 应用（GORM 官方推荐做法，GORM 自身不做池管理）。默认值针对**低流量服务**
调优：低流量下的主要风险不是连接耗尽，而是连接长时间空闲被服务端（MySQL `wait_timeout`/代理）
掐断后被复用，导致多秒卡顿或 `invalid connection`。

| Knob | 标准库默认 | qtoolkit 默认 | 原因 |
|------|-----------|--------------|------|
| `MaxOpenConns` | 0（无限） | 0（无限） | 低流量不会突刺，上限保持 opt-in |
| `MaxIdleConns` | 2 | **5** | 低并发用不着大空闲池，少占 MySQL 连接槽 |
| `ConnMaxLifetime` | 0（永不） | **1h** | 兜底：按存活时长回收连接 |
| `ConnMaxIdleTime` | 0（不超时） | **5m** | 关键修复：低流量长空闲间隙里主动回收空闲连接，绝不复用已被服务端掐断的陈旧连接 |

> 高并发场景可显式调大 `max_idle_conns`；多实例部署可设 `max_open_conns`（按 `实例数 × 该值 < MySQL max_connections` 估算）。
> 若 MySQL `wait_timeout` 低于 300s，应把 `conn_max_idle_time_seconds` 设得更小。

### DSN 格式示例

```
user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
```

## 注意事项

1. **必须先设置配置**: 在第一次调用 `Get()` 或 `MustGet()` 之前必须调用 `SetConfig()`
2. **线程安全**: 所有操作都是线程安全的
3. **只初始化一次**: 使用 `sync.Once` 确保数据库只初始化一次
4. **错误处理**: 使用 `Get()` 需要检查返回值，使用 `MustGet()` 会在失败时 panic

## 测试

```bash
go test ./db -v
```
