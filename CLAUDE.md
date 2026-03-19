# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
qtoolkit stands for "Quality Toolkit"
This is a Go toolkit library for the WordGate platform, organized as a **modular monorepo** with independent service modules for optimal compilation and dependency management.

### v1.0 Architecture (Target)
qtoolkit v1.0 uses a **modular architecture** where each service is an independent Go module:
- **按需编译**: Only compile modules that are actually used
- **独立依赖**: Each module has its own go.mod with minimal dependencies  
- **Go Workspace**: Uses go.work for unified development experience
- **配置驱动**: Modules can be enabled/disabled through configuration

### 🚨 并行开发策略 (v0.x + v1.0)
**当前状态**: v0.x和v1.0并行开发，直到v1覆盖所有现有功能
- ✅ **新功能优先**: 所有新feature优先按v1模块化架构开发
- ✅ **渐进迁移**: 现有功能逐步迁移到v1架构
- ✅ **兼容性维护**: 保持v0.x功能正常运行
- ✅ **双重测试**: 确保v0.x和v1.0功能对等 

## Go Version Requirement

**强制要求: Go 1.24.0**

所有模块的 `go.mod` 必须使用 `go 1.24.0`。

## Development Commands

### v1.0 Module Development
```bash
# Workspace-based development (recommended)
go work sync                    # Sync all modules in workspace
go test ./...                   # Test all modules
go build                        # Build with workspace dependencies

# Production build (disable workspace)
GOWORK=off go build            # Build with published module versions

# Individual module development
cd core && go mod tidy         # Update core module
cd aws && go test ./...        # Test specific module
cd slack && go build           # Build specific module
```

### Testing
- **Workspace testing**: `go test ./...` (tests all modules)
- **Module testing**: `cd <module> && go test ./...`
- **Integration testing**: Use workspace for cross-module tests

### Module Management
- **Add new module**: Create directory with `go.mod`, add to `go.work`
- **Update dependencies**: `go work sync` after module changes
- **Version modules**: Each module has independent versioning

## Architecture

### v0.x Architecture (Current/Legacy)
单一模块结构，所有功能在根目录：
```
qtoolkit/
├── go.mod                    # 包含所有依赖
├── aws.go, aws_*.go         # AWS功能
├── aliyun_*.go              # 阿里云功能  
├── slack.go                 # Slack功能
├── wordgate.go              # WordGate功能
├── config.go                # 配置管理
├── event.go                 # 事件系统
├── util/, log/              # 工具和日志
└── *.go                     # 其他功能文件
```

### v1.0 Architecture (Completed/Modular)
模块化架构，按服务独立 - **16个独立模块**：
```
qtoolkit/
├── go.work                  # Workspace配置（包含全部16个模块）
├── go.mod                   # 根模块
├── core/                    # 核心模块
│   ├── go.mod
│   ├── config.go           # 配置管理
│   ├── event.go            # 事件系统
│   ├── util/               # 工具库
│   ├── exchange_rate_api.go # 汇率API
│   ├── http_cache.go       # HTTP缓存
│   ├── name_generator.go   # 名称生成器
│   ├── number_encode.go    # 数字编码
│   └── short_url.go        # 短链接服务
├── aws/                     # AWS服务（独立子模块）
│   ├── aws_config.yml      # 统一AWS配置模板
│   ├── ec2/                # EC2模块
│   │   ├── go.mod
│   │   └── ec2_config.yml
│   ├── s3/                 # S3模块
│   │   ├── go.mod
│   │   └── s3_config.yml
│   ├── ses/                # SES模块
│   │   ├── go.mod
│   │   └── ses_config.yml
│   └── sqs/                # SQS模块
│       ├── go.mod
│       └── sqs_config.yml
├── aliyun/                  # 阿里云模块
│   ├── go.mod
│   ├── aliyun_cms.go       # 云监控
│   ├── aliyun_ecs.go       # ECS
│   └── aliyun_log.go       # 日志服务
├── db/                      # 数据库模块（GORM+MySQL）
│   ├── go.mod
│   └── db_config.yml
├── redis/                   # Redis模块
│   ├── go.mod
│   ├── redis.go            # 客户端
│   ├── broadcast.go        # 广播
│   └── cache.go            # 缓存
├── mail/                    # 邮件模块
│   └── go.mod
├── slack/                   # Slack模块
│   ├── go.mod
│   └── slack_config.yml
├── godaddy/                 # GoDaddy域名管理
│   ├── go.mod
│   └── godaddy_config.yml
├── deepl/                   # DeepL翻译
│   └── go.mod
├── appstore/                # App Store集成
│   └── go.mod
├── log/                     # 日志模块
│   └── go.mod
└── unred/                   # 防标红短链接
    └── go.mod
```

## v1.0 模块化开发规范

### 🚫 不向后兼容原则

v1.0 架构**坚决不向后兼容**。这是设计决策，不是疏忽。

#### 为什么不向后兼容

1. **技术债务清零** - 旧的设计错误不应该永远背负
2. **API 纯净** - 没有 legacy 代码路径，没有 deprecated 标记
3. **配置简洁** - 不支持多种配置格式，只有一种正确方式
4. **代码可读** - 没有"为了兼容旧版本"的特殊处理

#### 实践要求

- ❌ 不保留旧的配置路径
- ❌ 不添加 deprecated 函数
- ❌ 不写 migration 代码
- ❌ 不支持多种配置格式
- ✅ 直接删除旧代码
- ✅ 用户升级时必须更新配置
- ✅ 在 CHANGELOG 中说明 breaking changes

#### 示例

```go
// ❌ 错误：保留旧接口
func SetWebhookURL(url string) { /* deprecated */ }
func SetConfig(cfg *Config) { /* 新方式 */ }

// ✅ 正确：只有新接口
func SetConfig(cfg *Config) { /* 唯一方式 */ }
```

```yaml
# ❌ 错误：支持多种配置格式
slack:
  webhook_url: "..."  # 旧格式，仍然支持
  webhooks:           # 新格式
    alert: "..."

# ✅ 正确：只有一种格式
slack:
  webhooks:
    alert: "..."
```

### 🎯 Less is More 设计哲学

v1.0 架构的核心原则是**极简主义**。每一行代码、每一个配置项、每一个 API 都必须证明其存在的必要性。

#### API 设计原则

1. **只暴露必需的 API**
   - 不提供"可能有用"的便捷方法
   - 用户可以通过组合基础 API 实现高级功能
   - 删除比添加更难，谨慎暴露公开接口

2. **配置项最小化**
   - 只保留无法通过其他途径配置的选项
   - 能在服务端配置的不在 SDK 配置（如 Slack bot 名称在 Webhook 后台设置）
   - 有合理默认值的配置项应设为可选

3. **不做过度抽象**
   - 不为"未来可能的需求"预留接口
   - 不封装只用一次的逻辑
   - 三行重复代码优于一个过早抽象

#### 代码审查检查点

每次代码审查时问自己：
- [ ] 这个 API/配置项能删掉吗？
- [ ] 这个功能是"必须有"还是"最好有"？
- [ ] 用户能用现有 API 组合实现吗？
- [ ] 删除它会让模块更难用吗？

#### 示例

```go
// ❌ 过度设计
slack.SetDefaultChannel("alert")
slack.SetUsername("Bot")
slack.SetIconEmoji(":robot:")
slack.Alert("message")           // 预设 channel 的便捷方法
slack.AlertWithColor("msg", "red")

// ✅ 极简设计
slack.Send("alert", "message")
slack.To("alert").Text("msg").Color("red").Send()
```

```yaml
# ❌ 过度配置
slack:
  default_channel: "alert"    # 用户应该明确指定
  username: "Bot"             # Slack 后台可配置
  icon_emoji: ":robot:"       # Slack 后台可配置
  retry_count: 3              # 大多数情况默认值足够
  retry_delay: "1s"

# ✅ 极简配置
slack:
  webhooks:
    alert: "https://hooks.slack.com/..."
    notify: "https://hooks.slack.com/..."
```

### 🎯 Feature开发优先级
1. **新功能**: 必须按v1模块化架构开发
2. **Bug修复**: v0.x修复，同时在v1中实现
3. **重构**: 优先将v0.x功能迁移到对应v1模块

### 📦 模块创建规范
每个新模块必须包含：
```bash
<module_name>/
├── go.mod                  # 独立依赖管理
├── <module_name>.go       # 主要功能实现
├── <module_name>_test.go  # 测试文件
├── <module_name>_config.yml # 配置模板
└── README.md              # 模块文档
```

### 🔧 模块开发工作流
```bash
# 1. 创建新模块
mkdir <module_name>
cd <module_name>
go mod init github.com/wordgate/qtoolkit/<module_name>

# 2. 添加到workspace
echo "use ./<module_name>" >> ../go.work

# 3. 开发和测试
go test ./...
go build

# 4. 集成测试
cd .. && go test ./...
```

### 🎛️ 配置驱动架构
每个模块支持配置启用/禁用：
```yaml
# main_config.yml
qtoolkit:
  modules:
    aws:
      enabled: true
      config_file: "aws/aws_config.yml"
    slack:
      enabled: false  # 禁用则不编译
```

### 🔑 配置单一真理源原则

**强制要求**: 模块配置的唯一入口是 viper（config.yml），禁止在模块内部添加环境变量回退。

#### 为什么

1. **单一真理源** — 配置只有一个地方可以设置，排查问题时不需要猜"这个值是从 config.yml 来的还是环境变量来的"
2. **qtoolkit 的精髓** — `viper.ReadInConfig()` 一次，所有模块 lazy load 自动就绪，拿来就用
3. **环境变量是使用者的事** — 如果使用者需要环境变量覆盖，他们可以在自己的应用里用 `viper.AutomaticEnv()` 或 `viper.BindEnv()`，不需要每个模块重复实现

#### 实践要求

```go
// ❌ 错误：模块内部添加环境变量回退
func loadConfigFromViper() *Config {
    cfg := &Config{
        APIKey: viper.GetString("service.api_key"),
    }
    if env := os.Getenv("SERVICE_API_KEY"); env != "" {
        cfg.APIKey = env  // 引入第二个配置源
    }
    return cfg
}

// ✅ 正确：只从 viper 读取
func loadConfigFromViper() *Config {
    return &Config{
        APIKey: viper.GetString("service.api_key"),
    }
}
```

```yaml
# ❌ 错误：config template 提及环境变量
# Can also be set via SERVICE_API_KEY environment variable

# ✅ 正确：config template 只描述 viper 配置
# API key for the service (required)
api_key: "YOUR_SERVICE_API_KEY"
```

### 🔄 依赖管理规则
- **核心依赖**: 只在`core/go.mod`中
- **服务依赖**: 各模块独立管理
- **交叉依赖**: 通过`core`模块接口
- **版本同步**: 使用`go work sync`

## v1.0 独立Feature开发规范

### 🚀 Feature开发流程
每个新功能必须作为独立模块开发：

```bash
# 1. 分析功能需求
# - 确定功能属于哪个服务类别 (AWS/Aliyun/Slack/etc)
# - 评估是否需要新模块或扩展现有模块

# 2. 创建Feature分支
git checkout -b feature/<module_name>-<feature_name>

# 3. 模块化开发
mkdir <module_name> # 如果是新模块
cd <module_name>
# 按照模块创建规范开发

# 4. 功能完整性验证
# - 单元测试覆盖
# - 集成测试通过
# - 配置文件模板
# - 使用文档
```

### 📋 Feature完成检查清单
每个Feature必须满足：
- [ ] ✅ 按v1模块化架构实现
- [ ] ✅ 独立go.mod管理依赖
- [ ] ✅ 配置驱动，支持启用/禁用
- [ ] ✅ 完整测试覆盖（单元+集成）
- [ ] ✅ 配置模板文件
- [ ] ✅ README文档
- [ ] ✅ 向后兼容（如果是迁移功能）

### 🔄 迁移现有功能规范
将v0.x功能迁移到v1模块：
1. **保持兼容**: v0.x功能继续工作
2. **并行实现**: 在对应v1模块中实现
3. **测试对等**: 确保功能完全对等
4. **逐步切换**: 通过配置控制使用v1实现
5. **清理v0.x**: 功能完全迁移后清理

### 🎛️ Configuration Management

#### v0.x Configuration (Legacy)
单一配置文件：
```yaml
# config.yml
is_dev: true
aws:
  access_key: "xxx"
slack:
  webhook_url: "xxx"
```

#### v1.0 Configuration (Modular)
模块化配置文件：
```yaml
# main_config.yml
qtoolkit:
  is_dev: true
  modules:
    aws:
      enabled: true
      config_file: "aws/aws_config.yml"
    slack:
      enabled: true  
      config_file: "slack/slack_config.yml"
```

```yaml
# aws/aws_config.yml
aws:
  access_key: "YOUR_AWS_ACCESS_KEY"
  secret_key: "YOUR_AWS_SECRET_KEY"
```

## v1.0 Configuration Auto-Loading System

### 核心原则

v1.0架构的所有模块遵循统一的配置自动加载规则：

1. **嵌套YAML结构**: 配置路径遵循 `服务.子服务.属性` 格式
2. **级联配置回退**: 从具体到通用的多级配置查找
3. **懒加载初始化**: 使用 `sync.Once` 实现首次使用时自动加载
4. **线程安全**: 使用 `sync.RWMutex` 保护配置读写
5. **外部透明**: 应用只需在启动时加载配置文件，模块自动完成配置

### 配置文件结构

每个模块在自己的目录下提供 `*_config.yml` 配置模板。应用配置时参考各模块的配置文件：

| 模块 | 配置模板 |
|------|---------|
| AWS S3 | `aws/s3/s3_config.yml` |
| AWS SES | `aws/ses/ses_config.yml` |
| AWS SQS | `aws/sqs/sqs_config.yml` |
| AWS EC2 | `aws/ec2/ec2_config.yml` |
| Database | `db/db_config.yml` |
| Redis | `redis/redis_config.yml` |
| Slack | `slack/slack_config.yml` |
| Aliyun | `aliyun/aliyun_config.yml` |
| GoDaddy | `godaddy/godaddy_config.yml` |
| DeepL | `deepl/deepl_config.yml` |

### 级联配置回退 (Cascading Fallback)

配置读取优先级从具体到通用：

#### 标准服务 (2级回退)

适用于: S3, SES, EC2, Database, Redis, Slack, Aliyun, GoDaddy, Mail

```
1. 服务特定配置 (aws.s3.region)
2. 全局配置 (aws.region)
```

**实现示例** (aws/s3/s3.go):
```go
func loadConfigFromViper() (*Config, error) {
    cfg := &Config{}

    // 1. 服务特定配置
    cfg.Region = viper.GetString("aws.s3.region")
    cfg.AccessKey = viper.GetString("aws.s3.access_key")
    cfg.SecretKey = viper.GetString("aws.s3.secret_key")
    cfg.Bucket = viper.GetString("aws.s3.bucket")
    cfg.URLPrefix = viper.GetString("aws.s3.url_prefix")

    // 2. 全局AWS配置回退
    if cfg.Region == "" {
        cfg.Region = viper.GetString("aws.region")
    }
    if cfg.AccessKey == "" {
        cfg.AccessKey = viper.GetString("aws.access_key")
    }
    if cfg.SecretKey == "" {
        cfg.SecretKey = viper.GetString("aws.secret_key")
    }
    cfg.UseIMDS = viper.GetBool("aws.use_imds")

    // 验证必需字段
    if cfg.Bucket == "" {
        return nil, fmt.Errorf("aws.s3.bucket is required")
    }

    return cfg, nil
}
```

#### SQS队列 (3级回退)

SQS支持按队列配置：

```
1. 队列特定配置 (aws.sqs.queues.my-queue.region)
2. SQS服务配置 (aws.sqs.region)
3. 全局AWS配置 (aws.region)
```

**实现示例** (aws/sqs/sqs.go):
```go
func loadConfigFromViper(queueName string) (*Config, error) {
    cfg := &Config{QueueName: queueName}

    // 1. 队列特定配置
    queuePath := fmt.Sprintf("aws.sqs.queues.%s", queueName)
    if viper.IsSet(queuePath) {
        cfg.Region = viper.GetString(queuePath + ".region")
        cfg.AccessKey = viper.GetString(queuePath + ".access_key")
        cfg.SecretKey = viper.GetString(queuePath + ".secret_key")
    }

    // 2. SQS服务级别回退
    if cfg.Region == "" {
        cfg.Region = viper.GetString("aws.sqs.region")
    }
    if cfg.AccessKey == "" {
        cfg.AccessKey = viper.GetString("aws.sqs.access_key")
    }
    if cfg.SecretKey == "" {
        cfg.SecretKey = viper.GetString("aws.sqs.secret_key")
    }

    // 3. 全局AWS配置回退
    if cfg.Region == "" {
        cfg.Region = viper.GetString("aws.region")
    }
    if cfg.AccessKey == "" {
        cfg.AccessKey = viper.GetString("aws.access_key")
    }
    if cfg.SecretKey == "" {
        cfg.SecretKey = viper.GetString("aws.secret_key")
    }
    cfg.UseIMDS = viper.GetBool("aws.use_imds")

    return cfg, nil
}
```

### Lazy Load + sync.Once 初始化模式

所有模块使用统一的懒加载模式：

```go
package mymodule

import (
    "fmt"
    "sync"
    "github.com/spf13/viper"
)

var (
    globalConfig *Config       // 全局配置
    globalClient *Client       // 全局客户端
    clientOnce   sync.Once     // 确保只初始化一次
    initErr      error         // 初始化错误
    configMux    sync.RWMutex  // 配置读写锁
)

// Config represents module configuration
type Config struct {
    Field1 string `yaml:"field1"`
    Field2 string `yaml:"field2"`
}

// loadConfigFromViper loads configuration from viper
// Configuration path priority (cascading fallback):
// 1. service.subservice.field - Service-specific config
// 2. service.field - Global service config (if applicable)
func loadConfigFromViper() (*Config, error) {
    cfg := &Config{}

    // Load service-specific config
    cfg.Field1 = viper.GetString("service.subservice.field1")
    cfg.Field2 = viper.GetString("service.subservice.field2")

    // Fall back to global config for missing values
    if cfg.Field1 == "" {
        cfg.Field1 = viper.GetString("service.field1")
    }

    // Validate required fields
    if cfg.Field1 == "" {
        return nil, fmt.Errorf("service.subservice.field1 is required")
    }

    return cfg, nil
}

// initialize performs the actual initialization
// Called once via sync.Once
func initialize() {
    // Try to load from viper first
    cfg, err := loadConfigFromViper()
    if err != nil {
        // Fall back to SetConfig if viper config not available
        configMux.RLock()
        cfg = globalConfig
        configMux.RUnlock()

        if cfg == nil {
            initErr = fmt.Errorf("config not available: %v", err)
            return
        }
    } else {
        // Store loaded config
        configMux.Lock()
        globalConfig = cfg
        configMux.Unlock()
    }

    // Initialize client with config
    globalClient, initErr = createClient(cfg)
}

// Get returns the client with lazy initialization
func Get() *Client {
    clientOnce.Do(initialize)
    return globalClient
}

// GetError returns the initialization error if any
func GetError() error {
    return initErr
}

// SetConfig sets the configuration for lazy loading (deprecated)
// Use viper configuration instead
func SetConfig(cfg *Config) {
    configMux.Lock()
    defer configMux.Unlock()
    globalConfig = cfg
}
```

### 使用方式 - 外部透明

#### 应用启动时

只需一次性加载配置文件：

```go
package main

import (
    "github.com/spf13/viper"
    "github.com/wordgate/qtoolkit/aws/s3"
    "github.com/wordgate/qtoolkit/aws/sqs"
    "github.com/wordgate/qtoolkit/db"
)

func main() {
    // 1. 加载配置文件（全局，一次性）
    viper.SetConfigFile("config.yml")
    if err := viper.ReadInConfig(); err != nil {
        panic(err)
    }

    // 2. 直接使用各模块，配置自动加载
    // 无需调用 SetConfig()

    // 使用S3
    s3.Upload("file.jpg", data)

    // 使用SQS（按队列名自动加载对应配置）
    sqs.Get("notifications")

    // 使用数据库
    db.Get().Create(&user)
}
```

#### 不再需要的旧方式

```go
// ❌ 旧方式：需要手动配置每个模块
s3.SetConfig(&s3.Config{
    AccessKey: "...",
    SecretKey: "...",
    Bucket: "...",
})

// ✅ 新方式：配置文件 + 自动加载
// 无需任何 SetConfig() 调用
```

### 配置路径规范表

| 模块 | 配置路径 | 回退层级 | 示例字段 |
|------|---------|---------|---------|
| **Database** | `database.*` | 1级 | `database.dsn`, `database.debug` |
| **AWS S3** | `aws.s3.*` → `aws.*` | 2级 | `aws.s3.bucket`, `aws.s3.region` → `aws.region` |
| **AWS SES** | `aws.ses.*` → `aws.*` | 2级 | `aws.ses.default_from`, `aws.ses.region` → `aws.region` |
| **AWS SQS** | `aws.sqs.queues.<name>.*` → `aws.sqs.*` → `aws.*` | 3级 | `aws.sqs.queues.my-queue.region` → `aws.sqs.region` → `aws.region` |
| **AWS EC2** | `aws.ec2.*` → `aws.*` | 2级 | `aws.ec2.region` → `aws.region` |
| **Redis** | `redis.*` | 1级 | `redis.addr`, `redis.password`, `redis.db` |
| **Slack** | `slack.*` | 1级 | `slack.webhooks.*`, `slack.bot_token` |
| **Aliyun** | `aliyun.*` | 1级 | `aliyun.access_key`, `aliyun.region` |
| **GoDaddy** | `godaddy.*` | 1级 | `godaddy.api_key`, `godaddy.api_secret` |
| **Mail** | `mail.*` | 1级 | `mail.smtp_host`, `mail.smtp_port` |
| **Core** | `exchange_rate.*` | 1级 | `exchange_rate.api_key` |
| **DeepL** | `deepl.*` | 1级 | `deepl.api_key`, `deepl.api_url` |
| **Log** | `log.*` | 1级 | `log.level`, `log.format` |
| **Unred** | `unred.*` | 1级 | `unred.api_url`, `unred.api_key` |
| **Asynq** | `asynq.*` → `redis.*` | 2级 | `asynq.concurrency`, `asynq.queues` → `redis.addr` |
| **FastGPT** | `fastgpt.*` | 1级 | `fastgpt.api_key`, `fastgpt.base_url` |
| **Chatwoot** | `chatwoot.*` | 1级 | `chatwoot.api_token`, `chatwoot.base_url`, `chatwoot.account_id` |

## Asynq 异步任务模块

### 概述

`asynq` 模块基于 [hibiken/asynq](https://github.com/hibiken/asynq) 提供异步任务队列功能：
- **零配置启动**: Worker 自动启动，无需显式调用
- **优雅关闭**: 自动监听信号，确保任务不丢失
- **定时任务**: 支持 Cron 表达式的周期性任务
- **监控 UI**: 内置 Asynqmon Web 界面

### 配置

```yaml
# config.yml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0

asynq:
  concurrency: 10              # Worker 并发数 (默认: 10)
  queues:                      # 队列优先级 (数字越大优先级越高)
    critical: 6
    default: 3
    low: 1
  strict_priority: false       # 严格优先级模式 (默认: false)
  default_max_retry: 3         # 默认最大重试次数 (默认: 3)
  default_timeout: "30m"       # 默认任务超时 (默认: 30m)
```

### API 使用

```go
import "github.com/wordgate/qtoolkit/asynq"

// 1. 注册任务处理器
asynq.Handle("email:send", func(ctx context.Context, payload []byte) error {
    var data EmailPayload
    asynq.Unmarshal(payload, &data)
    // 处理逻辑...
    return nil
})

// 2. 注册定时任务 (可选)
asynq.Cron("@every 5m", "metrics:collect", nil)
asynq.Cron("0 9 * * *", "report:daily", nil)

// 3. 挂载监控 UI (自动启动 Worker)
r := gin.Default()
asynq.Mount(r, "/asynq")

// 4. 入队任务
asynq.Enqueue("email:send", payload)                    // 立即执行
asynq.EnqueueIn("email:send", payload, 5*time.Minute)   // 延迟执行
asynq.EnqueueAt("email:send", payload, scheduledTime)   // 定时执行
asynq.EnqueueUnique("user:sync", payload, 1*time.Hour)  // 去重任务

// 5. 带选项入队
asynq.Enqueue("task", payload,
    asynq.Queue("critical"),
    asynq.MaxRetry(5),
    asynq.Timeout(10*time.Minute),
)
```

### 部署模式

**模式1: API + Worker 混合 (推荐)**
```go
func main() {
    viper.ReadInConfig()

    asynq.Handle("email:send", handleEmailSend)
    asynq.Cron("@daily", "report:daily", nil)

    r := gin.Default()
    asynq.Mount(r, "/asynq")  // 自动启动 Worker + Scheduler

    r.POST("/send", func(c *gin.Context) {
        asynq.Enqueue("email:send", payload)
    })

    r.Run(":8080")
}
```

**模式2: 独立 Worker 进程**
```go
// worker/main.go
func main() {
    viper.ReadInConfig()

    asynq.Handle("email:send", handleEmailSend)
    asynq.Cron("@daily", "report:daily", nil)

    asynq.Run()  // 阻塞运行
}
```

### Cron 表达式

| 表达式 | 说明 |
|--------|------|
| `*/5 * * * *` | 每5分钟 |
| `0 * * * *` | 每小时 |
| `0 9 * * *` | 每天9:00 |
| `0 9 * * 1` | 每周一9:00 |
| `@every 30m` | 每30分钟 |
| `@hourly` | 每小时 |
| `@daily` | 每天0:00 |

### 生命周期

```
Handle() 注册 handler
       ↓
Cron() 注册定时任务 (可选)
       ↓
Mount() 或 Enqueue() 首次调用
       ↓
Worker + Scheduler 自动启动
       ↓
SIGINT/SIGTERM 信号
       ↓
优雅关闭 (等待任务完成)
```

### 配置模板文件

每个模块提供 `<module>_config.yml` 模板文件，包含：

1. **配置路径注释**: 说明嵌套结构
2. **字段说明**: 每个配置项的用途
3. **示例值**: 使用占位符（如 `YOUR_*_KEY`）
4. **安全提示**: 不提交真实凭证的警告

**示例** (db/db_config.yml):
```yaml
# Database Configuration Template
# Add this to your main config.yml file

database:
  # MySQL DSN (Data Source Name) connection string
  # Format: username:password@tcp(host:port)/database?charset=utf8mb4&parseTime=True&loc=Local
  dsn: "user:password@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"

  # Enable debug mode (prints SQL queries)
  debug: false

# Security Notes:
# - Never commit real credentials to version control
# - Use environment variables for production
# - Rotate database passwords regularly
```

### 必需字段验证

每个 `loadConfigFromViper()` 必须验证必需字段：

```go
// Validate required fields
if cfg.RequiredField == "" {
    return nil, fmt.Errorf("service.subservice.required_field is required")
}

// 错误信息包含完整配置路径
if cfg.Bucket == "" {
    return nil, fmt.Errorf("aws.s3.bucket is required")
}
```

### 向后兼容性

所有模块保留 `SetConfig()` 函数作为废弃接口：

```go
// SetConfig sets the configuration for lazy loading (deprecated)
// Prefer using viper configuration instead
func SetConfig(cfg *Config) {
    configMux.Lock()
    defer configMux.Unlock()
    globalConfig = cfg
}
```

**使用场景**:
- 测试代码需要动态配置
- 不使用viper的遗留代码
- 配置文件不可用时的备用方案

## ⏱️ v1.0 迁移时间表

### 📅 并行开发阶段
- **当前状态**: v0.x维护 + v1.0新功能开发
- **新功能**: 100%按v1模块化架构实现
- **Bug修复**: v0.x修复，v1.0同步实现
- **重构**: 优先迁移v0.x功能到v1.0

### 🎯 迁移里程碑
1. **Phase 1**: 核心模块（core/util/log）- ✅ 已完成
2. **Phase 2**: 服务模块（aws/aliyun/slack/godaddy）- ✅ 已完成
3. **Phase 3**: 集成模块（database/redis/mail/deepl/appstore/unred）- ✅ 已完成
4. **Phase 4**: 统一配置自动加载系统 - ✅ 已完成
5. **Phase 5**: 文档完善和v1.0正式发布 - ✅ 已完成

**v1.0 迁移完成状态**:
- ✅ 16个独立模块全部完成
- ✅ 统一配置自动加载架构实施
- ✅ 级联配置回退系统完成
- ✅ 懒加载 + sync.Once 初始化模式应用到所有模块
- ✅ 配置模板文件和文档完成
- ✅ go.work工作区配置完成
- ✅ 所有模块编译通过

### 📊 功能覆盖检查
定期检查v1.0功能覆盖度：
```bash
# 检查功能对等性
go test ./... -tags="v0_compat"
# 性能对比测试
go test ./... -bench=".*" -tags="v1_bench"
```

## 🔒 Security Considerations

### v1.0 模块化安全
- **模块隔离**: 各模块独立配置，减少泄露风险
- **按需加载**: 只加载需要的模块，减少攻击面
- **配置分离**: 敏感配置分散到各模块文件
- **版本控制**: 每个模块独立版本，便于安全更新

### 配置安全（v0.x + v1.0）
- **不提交凭证**: 所有配置文件使用占位符
- **环境变量**: 生产环境使用环境变量覆盖
- **权限最小**: API密钥使用最小权限
- **定期轮换**: 定期更换所有密钥和凭证

### 占位符替换规则
v0.x和v1.0配置文件中的占位符：
- `YOUR_AWS_ACCESS_KEY`, `YOUR_AWS_SECRET_KEY`
- `YOUR_ALIYUN_ACCESS_KEY`, `YOUR_ALIYUN_ACCESS_SECRET` 
- `YOUR_SLACK_WEBHOOK_URL`, `YOUR_SLACK_TOKEN`
- `YOUR_*_API_KEY` 等各种API密钥

## 💡 开发最佳实践

### v1.0 模块开发
1. **单一职责**: 每个模块专注一个服务
2. **接口设计**: 通过core模块提供统一接口
3. **错误处理**: 统一错误类型和处理方式
4. **日志规范**: 使用core/log统一日志格式
5. **测试覆盖**: 每个模块>=80%测试覆盖率

### 并行开发策略
- **功能优先**: 新功能必须v1.0实现
- **兼容维护**: v0.x关键bug继续修复
- **渐进迁移**: 按模块逐步迁移
- **双重验证**: 确保v1.0功能完全对等
- 我们需要能够调用它部署的api来完成防标红短链接服务