# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
qtoolkit stands for "Quality Toolkit"
This is a Go toolkit library for the WordGate platform, organized as a **modular monorepo** with independent service modules for optimal compilation and dependency management.

### v1.0 Architecture (Target)
qtoolkit v1.0 uses a **modular architecture** where each service is an independent Go module:
- **Compile on demand**: Only compile modules that are actually used
- **Independent dependencies**: Each module has its own go.mod with minimal dependencies
- **Go Workspace**: Uses go.work for unified development experience
- **Configuration-driven**: Modules can be enabled/disabled through configuration

### 🚨 Parallel Development Strategy (v0.x + v1.0)
**Current status**: v0.x and v1.0 developed in parallel, until v1 covers all existing functionality
- ✅ **New features first**: All new features are developed using the v1 modular architecture first
- ✅ **Progressive migration**: Existing functionality is gradually migrated to the v1 architecture
- ✅ **Compatibility maintenance**: Keep v0.x functionality working normally
- ✅ **Dual testing**: Ensure feature parity between v0.x and v1.0

## Go Version Requirement

**Mandatory requirement: Go 1.24.0**

All modules' `go.mod` must use `go 1.24.0`.

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
Single module structure, all functionality in the root directory:
```
qtoolkit/
├── go.mod                    # Contains all dependencies
├── aws.go, aws_*.go         # AWS functionality
├── aliyun_*.go              # Aliyun functionality
├── slack.go                 # Slack functionality
├── wordgate.go              # WordGate functionality
├── config.go                # Configuration management
├── event.go                 # Event system
├── util/, log/              # Utilities and logging
└── *.go                     # Other functional files
```

### v1.0 Architecture (Completed/Modular)
Modular architecture, independent per service - **24 independent modules**:
```
qtoolkit/
├── go.work                  # Workspace configuration (includes all 24 modules)
├── go.mod                   # Root module
├── core/                    # Core module
│   ├── go.mod
│   ├── config.go           # Configuration management
│   ├── event.go            # Event system
│   ├── util/               # Utility library
│   ├── exchange_rate_api.go # Exchange rate API
│   ├── http_cache.go       # HTTP cache
│   ├── name_generator.go   # Name generator
│   ├── number_encode.go    # Number encoding
│   └── short_url.go        # Short URL service
├── ai/                      # AI module
│   └── go.mod
├── aws/                     # AWS services (independent sub-modules)
│   ├── aws_config.yml      # Unified AWS configuration template
│   ├── cloudwatch/         # CloudWatch module
│   │   └── go.mod
│   ├── ec2/                # EC2 module
│   │   ├── go.mod
│   │   └── ec2_config.yml
│   ├── s3/                 # S3 module
│   │   ├── go.mod
│   │   └── s3_config.yml
│   ├── ses/                # SES module
│   │   ├── go.mod
│   │   └── ses_config.yml
│   ├── sqs/                # SQS module
│   │   ├── go.mod
│   │   └── sqs_config.yml
│   └── ssm/                # SSM Parameter Store module
│       └── go.mod
├── aliyun/                  # Aliyun module
│   ├── go.mod
│   ├── aliyun_cms.go       # Cloud monitoring
│   ├── aliyun_ecs.go       # ECS
│   └── aliyun_log.go       # Log service
├── asynq/                   # Async task queue (hibiken/asynq)
│   └── go.mod
├── chatwoot/                # Chatwoot integration
│   └── go.mod
├── db/                      # Database module (GORM+MySQL)
│   ├── go.mod
│   └── db_config.yml
├── deepl/                   # DeepL translation
│   └── go.mod
├── exchange/                # Exchange rate module
│   └── go.mod
├── fastgpt/                 # FastGPT integration
│   └── go.mod
├── github/                  # GitHub services
│   └── issue/              # GitHub Issues module
│       └── go.mod
├── godaddy/                 # GoDaddy domain management
│   ├── go.mod
│   └── godaddy_config.yml
├── log/                     # Logging module
│   └── go.mod
├── mail/                    # Mail module
│   └── go.mod
├── nextpay/                 # NextPay payment module
│   └── go.mod
├── redis/                   # Redis module
│   ├── go.mod
│   ├── redis.go            # Client
│   ├── broadcast.go        # Broadcast
│   └── cache.go            # Cache
├── slack/                   # Slack module
│   ├── go.mod
│   └── slack_config.yml
├── appstore/                # App Store integration
│   └── go.mod
├── unred/                   # Anti-flagging short URL service
│   └── go.mod
└── util/                    # Utility module
    └── go.mod
```

## v1.0 Modular Development Standards

### 🚫 No Backward Compatibility Principle

The v1.0 architecture **firmly does not maintain backward compatibility**. This is a design decision, not an oversight.

#### Why No Backward Compatibility

1. **Zero technical debt** - Old design mistakes should not be carried forever
2. **Clean API** - No legacy code paths, no deprecated markers
3. **Simple configuration** - No support for multiple configuration formats, only one correct way
4. **Readable code** - No special handling "for compatibility with old versions"

#### Practical Requirements

- ❌ Do not retain old configuration paths
- ❌ Do not add deprecated functions
- ❌ Do not write migration code
- ❌ Do not support multiple configuration formats
- ✅ Delete old code directly
- ✅ Users must update configuration when upgrading
- ✅ Document breaking changes in CHANGELOG

#### Example

```go
// ❌ Wrong: retaining old interface
func SetWebhookURL(url string) { /* deprecated */ }
func SetConfig(cfg *Config) { /* new way */ }

// ✅ Correct: only the new interface
func SetConfig(cfg *Config) { /* the only way */ }
```

```yaml
# ❌ Wrong: supporting multiple configuration formats
slack:
  webhook_url: "..."  # old format, still supported
  webhooks:           # new format
    alert: "..."

# ✅ Correct: only one format
slack:
  webhooks:
    alert: "..."
```

### 🎯 Less is More Design Philosophy

The core principle of v1.0 architecture is **minimalism**. Every line of code, every configuration item, every API must justify its existence.

#### API Design Principles

1. **Only expose necessary APIs**
   - Do not provide "potentially useful" convenience methods
   - Users can achieve advanced functionality by combining basic APIs
   - Removing is harder than adding — expose public interfaces cautiously

2. **Minimize configuration items**
   - Only keep options that cannot be configured through other means
   - What can be configured server-side should not be in the SDK config (e.g., Slack bot name set in Webhook dashboard)
   - Configuration items with reasonable defaults should be optional

3. **Avoid over-abstraction**
   - Do not reserve interfaces for "possibly needed in the future"
   - Do not encapsulate logic used only once
   - Three lines of repeated code is better than one premature abstraction

#### Code Review Checkpoints

Ask yourself at every code review:
- [ ] Can this API/configuration item be removed?
- [ ] Is this feature "must have" or "nice to have"?
- [ ] Can users achieve this by combining existing APIs?
- [ ] Would removing it make the module harder to use?

#### Example

```go
// ❌ Over-engineered
slack.SetDefaultChannel("alert")
slack.SetUsername("Bot")
slack.SetIconEmoji(":robot:")
slack.Alert("message")           // convenience method with preset channel
slack.AlertWithColor("msg", "red")

// ✅ Minimal design
slack.Send("alert", "message")
slack.To("alert").Text("msg").Color("red").Send()
```

```yaml
# ❌ Over-configured
slack:
  default_channel: "alert"    # user should specify explicitly
  username: "Bot"             # configurable in Slack dashboard
  icon_emoji: ":robot:"       # configurable in Slack dashboard
  retry_count: 3              # default value is sufficient in most cases
  retry_delay: "1s"

# ✅ Minimal configuration
slack:
  webhooks:
    alert: "https://hooks.slack.com/..."
    notify: "https://hooks.slack.com/..."
```

### 🎯 Feature Development Priority
1. **New features**: Must be developed using the v1 modular architecture
2. **Bug fixes**: Fix in v0.x, implement simultaneously in v1
3. **Refactoring**: Prioritize migrating v0.x functionality to the corresponding v1 module

### 📦 Module Creation Standards
Every new module must include:
```bash
<module_name>/
├── go.mod                  # Independent dependency management
├── <module_name>.go       # Main functionality implementation
├── <module_name>_test.go  # Test file
├── <module_name>_config.yml # Configuration template
└── README.md              # Module documentation
```

### 🔧 Module Development Workflow
```bash
# 1. Create new module
mkdir <module_name>
cd <module_name>
go mod init github.com/wordgate/qtoolkit/<module_name>

# 2. Add to workspace
echo "use ./<module_name>" >> ../go.work

# 3. Develop and test
go test ./...
go build

# 4. Integration testing
cd .. && go test ./...
```

### 🎛️ Configuration-Driven Architecture
Each module supports enabling/disabling via configuration:
```yaml
# main_config.yml
qtoolkit:
  modules:
    aws:
      enabled: true
      config_file: "aws/aws_config.yml"
    slack:
      enabled: false  # disabled means not compiled
```

### 🔑 Single Source of Truth for Configuration

**Mandatory requirement**: The only entry point for module configuration is viper (config.yml). Adding environment variable fallbacks inside modules is prohibited.

#### Why

1. **Single source of truth** — Configuration has only one place to be set; when troubleshooting, there is no need to guess "did this value come from config.yml or an environment variable"
2. **The essence of qtoolkit** — `viper.ReadInConfig()` once, all modules lazy-load and are automatically ready to use out of the box
3. **Environment variables are the user's concern** — If users need environment variable overrides, they can use `viper.AutomaticEnv()` or `viper.BindEnv()` in their own application; each module does not need to implement this repeatedly

#### Practical Requirements

```go
// ❌ Wrong: adding environment variable fallback inside a module
func loadConfigFromViper() *Config {
    cfg := &Config{
        APIKey: viper.GetString("service.api_key"),
    }
    if env := os.Getenv("SERVICE_API_KEY"); env != "" {
        cfg.APIKey = env  // introduces a second configuration source
    }
    return cfg
}

// ✅ Correct: read only from viper
func loadConfigFromViper() *Config {
    return &Config{
        APIKey: viper.GetString("service.api_key"),
    }
}
```

```yaml
# ❌ Wrong: config template mentions environment variables
# Can also be set via SERVICE_API_KEY environment variable

# ✅ Correct: config template only describes viper configuration
# API key for the service (required)
api_key: "YOUR_SERVICE_API_KEY"
```

### 🔄 Dependency Management Rules
- **Core dependencies**: Only in `core/go.mod`
- **Service dependencies**: Each module manages independently
- **Cross-dependencies**: Through `core` module interfaces
- **Version sync**: Use `go work sync`

## v1.0 Independent Feature Development Standards

### 🚀 Feature Development Process
Every new feature must be developed as an independent module:

```bash
# 1. Analyze feature requirements
# - Determine which service category the feature belongs to (AWS/Aliyun/Slack/etc)
# - Evaluate whether a new module is needed or an existing module should be extended

# 2. Create feature branch
git checkout -b feature/<module_name>-<feature_name>

# 3. Modular development
mkdir <module_name> # if it's a new module
cd <module_name>
# Develop according to the module creation standards

# 4. Feature completeness verification
# - Unit test coverage
# - Integration tests passing
# - Configuration file template
# - Usage documentation
```

### 📋 Feature Completion Checklist
Every feature must satisfy:
- [ ] ✅ Implemented using v1 modular architecture
- [ ] ✅ Independent go.mod for dependency management
- [ ] ✅ Configuration-driven, supports enabling/disabling
- [ ] ✅ Complete test coverage (unit + integration)
- [ ] ✅ Configuration template file
- [ ] ✅ README documentation
- [ ] ✅ Backward compatible (if migrating existing functionality)

### 🔄 Standards for Migrating Existing Functionality
Migrating v0.x functionality to v1 modules:
1. **Maintain compatibility**: v0.x functionality continues to work
2. **Parallel implementation**: Implement in the corresponding v1 module
3. **Test parity**: Ensure complete functional parity
4. **Gradual switch**: Control use of v1 implementation through configuration
5. **Clean up v0.x**: Clean up after functionality is fully migrated

### 🎛️ Configuration Management

#### v0.x Configuration (Legacy)
Single configuration file:
```yaml
# config.yml
is_dev: true
aws:
  access_key: "xxx"
slack:
  webhook_url: "xxx"
```

#### v1.0 Configuration (Modular)
Modular configuration file:
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

### Core Principles

All modules in the v1.0 architecture follow a unified set of configuration auto-loading rules:

1. **Nested YAML structure**: Configuration paths follow the `service.subservice.property` format
2. **Cascading configuration fallback**: Multi-level configuration lookup from specific to general
3. **Lazy-load initialization**: Uses `sync.Once` to auto-load on first use
4. **Thread safety**: Uses `sync.RWMutex` to protect configuration reads and writes
5. **Externally transparent**: Applications only need to load the configuration file at startup; modules handle configuration automatically

### Configuration File Structure

Each module provides a `*_config.yml` configuration template in its own directory. Refer to each module's configuration file when configuring the application:

| Module | Configuration Template |
|--------|----------------------|
| AWS S3 | `aws/s3/s3_config.yml` |
| AWS SES | `aws/ses/ses_config.yml` |
| AWS SQS | `aws/sqs/sqs_config.yml` |
| AWS EC2 | `aws/ec2/ec2_config.yml` |
| AWS CloudWatch | `aws/cloudwatch/cloudwatch_config.yml` |
| AWS SSM | `aws/ssm/ssm_config.yml` |
| Database | `db/db_config.yml` |
| Redis | `redis/redis_config.yml` |
| Slack | `slack/slack_config.yml` |
| Aliyun | `aliyun/aliyun_config.yml` |
| GoDaddy | `godaddy/godaddy_config.yml` |
| DeepL | `deepl/deepl_config.yml` |
| FastGPT | `fastgpt/fastgpt_config.yml` |
| Chatwoot | `chatwoot/chatwoot_config.yml` |
| Asynq | `asynq/asynq_config.yml` |
| Exchange | `exchange/exchange_config.yml` |
| NextPay | `nextpay/nextpay_config.yml` |

### Cascading Configuration Fallback

Configuration read priority from specific to general:

#### Standard Services (2-level fallback)

Applies to: S3, SES, EC2, CloudWatch, SSM, Database, Redis, Slack, Aliyun, GoDaddy, Mail

```
1. Service-specific configuration (aws.s3.region)
2. Global configuration (aws.region)
```

The pattern reads service-specific fields first (e.g., `aws.s3.region`, `aws.s3.access_key`), then falls back to the parent service config (e.g., `aws.region`, `aws.access_key`) for any missing values. Required fields are validated and return an error if absent. See `aws/s3/s3.go` for the canonical implementation.

#### SQS Queues (3-level fallback)

SQS supports per-queue configuration:

```
1. Queue-specific configuration (aws.sqs.queues.my-queue.region)
2. SQS service configuration (aws.sqs.region)
3. Global AWS configuration (aws.region)
```

The pattern reads queue-specific fields first (e.g., `aws.sqs.queues.<name>.region`), then falls back to the SQS service config (e.g., `aws.sqs.region`), and finally to the global AWS config (e.g., `aws.region`). See `aws/sqs/sqs.go` for the canonical implementation.

### Lazy Load + sync.Once Initialization Pattern

All modules use a unified lazy-loading pattern with the following components:
- `globalConfig *Config` + `globalClient *Client` package-level variables
- `sync.Once` to ensure initialization happens exactly once
- `sync.RWMutex` to protect configuration reads and writes
- `loadConfigFromViper()` to read configuration with cascading fallback
- `Get()` function that triggers lazy initialization via `clientOnce.Do(initialize)`
- `SetConfig()` retained as a deprecated fallback for tests and legacy code

See any existing module (e.g., `slack/slack.go`, `aws/s3/s3.go`) for the canonical implementation.

### Usage

Load the configuration file once at application startup, then use modules directly -- configuration is auto-loaded on first access:

```go
func main() {
    viper.SetConfigFile("config.yml")
    viper.ReadInConfig()

    s3.Upload("file.jpg", data)       // config auto-loaded
    sqs.Get("notifications")          // config auto-loaded per queue
    db.Get().Create(&user)            // config auto-loaded
}
```

### Configuration Path Reference Table

| Module | Configuration Path | Fallback Levels | Example Fields |
|--------|--------------------|-----------------|----------------|
| **Database** | `database.*` | 1 level | `database.dsn`, `database.debug` |
| **AWS S3** | `aws.s3.*` → `aws.*` | 2 levels | `aws.s3.bucket`, `aws.s3.region` → `aws.region` |
| **AWS SES** | `aws.ses.*` → `aws.*` | 2 levels | `aws.ses.default_from`, `aws.ses.region` → `aws.region` |
| **AWS SQS** | `aws.sqs.queues.<name>.*` → `aws.sqs.*` → `aws.*` | 3 levels | `aws.sqs.queues.my-queue.region` → `aws.sqs.region` → `aws.region` |
| **AWS EC2** | `aws.ec2.*` → `aws.*` | 2 levels | `aws.ec2.region` → `aws.region` |
| **AWS CloudWatch** | `aws.cloudwatch.*` → `aws.*` | 2 levels | `aws.cloudwatch.region` → `aws.region` |
| **AWS SSM** | `aws.ssm.*` → `aws.*` | 2 levels | `aws.ssm.region` → `aws.region` |
| **Redis** | `redis.*` | 1 level | `redis.addr`, `redis.password`, `redis.db` |
| **Slack** | `slack.*` | 1 level | `slack.webhooks.*`, `slack.bot_token` |
| **Aliyun** | `aliyun.*` | 1 level | `aliyun.access_key`, `aliyun.region` |
| **GoDaddy** | `godaddy.*` | 1 level | `godaddy.api_key`, `godaddy.api_secret` |
| **Mail** | `mail.*` | 1 level | `mail.smtp_host`, `mail.smtp_port` |
| **Core** | `exchange_rate.*` | 1 level | `exchange_rate.api_key` |
| **DeepL** | `deepl.*` | 1 level | `deepl.api_key`, `deepl.api_url` |
| **Log** | `log.*` | 1 level | `log.level`, `log.format` |
| **Unred** | `unred.*` | 1 level | `unred.api_url`, `unred.api_key` |
| **Asynq** | `asynq.*` → `redis.*` | 2 levels | `asynq.concurrency`, `asynq.queues` → `redis.addr` |
| **FastGPT** | `fastgpt.*` | 1 level | `fastgpt.api_key`, `fastgpt.base_url` |
| **Chatwoot** | `chatwoot.*` | 1 level | `chatwoot.api_token`, `chatwoot.base_url`, `chatwoot.account_id` |
| **Exchange** | `exchange.*` | 1 level | `exchange.api_key`, `exchange.base_url` |
| **NextPay** | `nextpay.*` | 1 level | `nextpay.api_key`, `nextpay.api_secret` |

## Asynq Async Task Module

### Overview

The `asynq` module is based on [hibiken/asynq](https://github.com/hibiken/asynq) and provides async task queue functionality:
- **Zero-configuration startup**: Worker starts automatically, no explicit call needed
- **Graceful shutdown**: Automatically listens for signals, ensuring tasks are not lost
- **Scheduled tasks**: Supports periodic tasks with Cron expressions
- **Monitoring UI**: Built-in Asynqmon web interface

### Configuration

```yaml
# config.yml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0

asynq:
  concurrency: 10              # Worker concurrency (default: 10)
  queues:                      # Queue priority (higher number = higher priority)
    critical: 6
    default: 3
    low: 1
  strict_priority: false       # Strict priority mode (default: false)
  default_max_retry: 3         # Default maximum retry count (default: 3)
  default_timeout: "30m"       # Default task timeout (default: 30m)
```

### API Usage

```go
import "github.com/wordgate/qtoolkit/asynq"

// 1. Register task handler
asynq.Handle("email:send", func(ctx context.Context, payload []byte) error {
    var data EmailPayload
    asynq.Unmarshal(payload, &data)
    // Processing logic...
    return nil
})

// 2. Register scheduled tasks (optional)
asynq.Cron("@every 5m", "metrics:collect", nil)
asynq.Cron("0 9 * * *", "report:daily", nil)

// 3. Mount monitoring UI (auto-starts Worker)
r := gin.Default()
asynq.Mount(r, "/asynq")

// 4. Enqueue tasks
asynq.Enqueue("email:send", payload)                    // Execute immediately
asynq.EnqueueIn("email:send", payload, 5*time.Minute)   // Delayed execution
asynq.EnqueueAt("email:send", payload, scheduledTime)   // Scheduled execution
asynq.EnqueueUnique("user:sync", payload, 1*time.Hour)  // Deduplicated task

// 5. Enqueue with options
asynq.Enqueue("task", payload,
    asynq.Queue("critical"),
    asynq.MaxRetry(5),
    asynq.Timeout(10*time.Minute),
)
```

### Deployment Modes

**Mode 1: API + Worker mixed (recommended)**
```go
func main() {
    viper.ReadInConfig()

    asynq.Handle("email:send", handleEmailSend)
    asynq.Cron("@daily", "report:daily", nil)

    r := gin.Default()
    asynq.Mount(r, "/asynq")  // Auto-starts Worker + Scheduler

    r.POST("/send", func(c *gin.Context) {
        asynq.Enqueue("email:send", payload)
    })

    r.Run(":8080")
}
```

**Mode 2: Standalone Worker process**
```go
// worker/main.go
func main() {
    viper.ReadInConfig()

    asynq.Handle("email:send", handleEmailSend)
    asynq.Cron("@daily", "report:daily", nil)

    asynq.Run()  // Blocking run
}
```

### Cron Expressions

| Expression | Description |
|------------|-------------|
| `*/5 * * * *` | Every 5 minutes |
| `0 * * * *` | Every hour |
| `0 9 * * *` | Every day at 9:00 |
| `0 9 * * 1` | Every Monday at 9:00 |
| `@every 30m` | Every 30 minutes |
| `@hourly` | Every hour |
| `@daily` | Every day at 0:00 |

### Lifecycle

```
Handle() registers handler
       ↓
Cron() registers scheduled tasks (optional)
       ↓
Mount() or Enqueue() first call
       ↓
Worker + Scheduler auto-start
       ↓
SIGINT/SIGTERM signal
       ↓
Graceful shutdown (wait for tasks to complete)
```

### Configuration Template Files

Each module provides a `<module>_config.yml` template file containing:

1. **Configuration path comments**: Explains the nested structure
2. **Field descriptions**: Purpose of each configuration item
3. **Example values**: Using placeholders (e.g., `YOUR_*_KEY`)
4. **Security notes**: Warning not to commit real credentials

**Example** (db/db_config.yml):
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
# - Rotate database passwords regularly
```

### Required Field Validation

Every `loadConfigFromViper()` must validate required fields:

```go
// Validate required fields
if cfg.RequiredField == "" {
    return nil, fmt.Errorf("service.subservice.required_field is required")
}

// Error messages include the full configuration path
if cfg.Bucket == "" {
    return nil, fmt.Errorf("aws.s3.bucket is required")
}
```

### Backward Compatibility

All modules retain the `SetConfig()` function as a deprecated interface:

```go
// SetConfig sets the configuration for lazy loading (deprecated)
// Prefer using viper configuration instead
func SetConfig(cfg *Config) {
    configMux.Lock()
    defer configMux.Unlock()
    globalConfig = cfg
}
```

**Use cases**:
- Test code that needs dynamic configuration
- Legacy code not using viper
- Fallback when configuration file is unavailable

## ⏱️ v1.0 Migration Timeline

### 📅 Parallel Development Phase
- **Current status**: v0.x maintenance + v1.0 new feature development
- **New features**: 100% implemented using v1 modular architecture
- **Bug fixes**: Fix in v0.x, implement simultaneously in v1.0
- **Refactoring**: Prioritize migrating v0.x functionality to v1.0

### 🎯 Migration Milestones
1. **Phase 1**: Core modules (core/util/log) - ✅ Completed
2. **Phase 2**: Service modules (aws/aliyun/slack/godaddy) - ✅ Completed
3. **Phase 3**: Integration modules (database/redis/mail/deepl/appstore/unred) - ✅ Completed
4. **Phase 4**: Unified configuration auto-loading system - ✅ Completed
5. **Phase 5**: Documentation completion and v1.0 official release - ✅ Completed

**v1.0 Migration Completion Status**:
- ✅ All 24 independent modules completed
- ✅ Unified configuration auto-loading architecture implemented
- ✅ Cascading configuration fallback system completed
- ✅ Lazy-load + sync.Once initialization pattern applied to all modules
- ✅ Configuration template files and documentation completed
- ✅ go.work workspace configuration completed
- ✅ All modules compile successfully

### 📊 Feature Coverage Check
Periodically check v1.0 feature coverage:
```bash
# Check feature parity
go test ./... -tags="v0_compat"
# Performance comparison testing
go test ./... -bench=".*" -tags="v1_bench"
```

## 🔒 Security Considerations

### v1.0 Modular Security
- **Module isolation**: Each module has independent configuration, reducing exposure risk
- **Load on demand**: Only load required modules, reducing attack surface
- **Configuration separation**: Sensitive configuration distributed across module files
- **Version control**: Each module has independent versioning, facilitating security updates

### Configuration Security (v0.x + v1.0)
- **Do not commit credentials**: All configuration files use placeholders
- **Least privilege**: API keys use minimum required permissions
- **Regular rotation**: Regularly rotate all keys and credentials

### Placeholder Replacement Rules
Placeholders used in v0.x and v1.0 configuration files:
- `YOUR_AWS_ACCESS_KEY`, `YOUR_AWS_SECRET_KEY`
- `YOUR_ALIYUN_ACCESS_KEY`, `YOUR_ALIYUN_ACCESS_SECRET`
- `YOUR_SLACK_WEBHOOK_URL`, `YOUR_SLACK_TOKEN`
- `YOUR_*_API_KEY` and various other API keys

## 💡 Development Best Practices

### v1.0 Module Development
1. **Single responsibility**: Each module focuses on one service
2. **Interface design**: Provide unified interfaces through the core module
3. **Error handling**: Unified error types and handling approach
4. **Logging standards**: Use core/log for unified log format
5. **Test coverage**: Each module >= 80% test coverage

### Parallel Development Strategy
- **Feature first**: New features must be implemented in v1.0
- **Compatibility maintenance**: Continue fixing critical bugs in v0.x
- **Progressive migration**: Migrate module by module gradually
- **Dual verification**: Ensure v1.0 functionality is completely equivalent
