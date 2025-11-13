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
}
```

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
