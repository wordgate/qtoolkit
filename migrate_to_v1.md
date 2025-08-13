# qtoolkit v1.0 迁移指南

本文档指导您从 qtoolkit v0.x 迁移到全新的 v1.0 模块化架构。

## 🚨 重大变更概述

qtoolkit v1.0 采用了**完全重构的模块化架构**，主要变更包括：

- ✅ **按依赖分离**: AWS、Aliyun、Slack 等服务拆分为独立模块
- ✅ **编译体积优化**: 只编译实际使用的模块
- ✅ **WordGate 分离**: WordGate 相关功能移至独立仓库
- ✅ **Go Workspace**: 使用 go.work 管理多模块开发
- ✅ **配置驱动**: 通过配置文件控制模块启用

## 📁 新架构对比

### v0.x 架构 (单一模块)
```
qtoolkit/
├── go.mod                    # 包含所有依赖
├── aws.go, aws_*.go         # AWS功能
├── aliyun_*.go              # 阿里云功能  
├── slack.go                 # Slack功能
├── wordgate.go              # WordGate功能
├── config.go                # 配置管理
└── ...                      # 其他功能文件
```

### v1.0 架构 (多模块)
```
qtoolkit/
├── go.work                  # Workspace配置
├── core/                    # 核心模块
│   ├── go.mod
│   ├── config.go
│   ├── event.go
│   ├── util/
│   └── log/
├── aws/                     # AWS模块
│   ├── go.mod              # 仅AWS SDK依赖
│   ├── aws.go
│   ├── aws_ec2.go
│   └── aws_config.yml
├── aliyun/                  # 阿里云模块
│   ├── go.mod              # 仅阿里云SDK依赖
│   ├── aliyun_cms.go
│   └── aliyun_config.yml
├── slack/                   # Slack模块
│   ├── go.mod
│   ├── slack.go
│   └── slack_config.yml
├── database/                # 数据库模块
│   ├── go.mod
│   └── db.go
├── email/                   # 邮件模块
├── redis/                   # Redis模块
├── godaddy/                 # GoDaddy模块
└── integration/             # 其他集成
    ├── appstore/
    ├── listmonk.go
    └── sms.go
```

## 🔄 迁移步骤

### 1. 更新导入路径

**v0.x 导入方式:**
```go
import "github.com/wordgate/qtoolkit"

// 使用所有功能
qtoolkit.SendSlackMessage("hello")
qtoolkit.EC2Info{}
qtoolkit.SetConfigFile("config.yml")
```

**v1.0 导入方式:**
```go
import (
    "github.com/wordgate/qtoolkit/core"    // 核心功能
    "github.com/wordgate/qtoolkit/aws"     // 仅在需要AWS时
    "github.com/wordgate/qtoolkit/slack"   // 仅在需要Slack时
)

// 按模块使用功能
core.SetConfigFile("config.yml")
slack.SendSlackMessage("hello")
aws.EC2Info{}
```

### 2. 配置文件迁移

**v0.x 配置 (单文件):**
```yaml
# config.yml
is_dev: true
aws:
  access_key: "xxx"
slack:
  webhook_url: "xxx"
```

**v1.0 配置 (模块化):**
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
    aliyun:
      enabled: false  # 不启用则不会编译
```

```yaml
# aws/aws_config.yml
aws:
  access_key: "xxx"
  secret_key: "xxx"
```

```yaml
# slack/slack_config.yml
slack:
  webhook_url: "xxx"
  token: "xxx"
```

### 3. 代码重构示例

**v0.x 代码:**
```go
package main

import "github.com/wordgate/qtoolkit"

func main() {
    qtoolkit.SetConfigFile("config.yml")
    
    // 所有功能都可用，即使不需要
    qtoolkit.SendSlackMessage("hello")
    ec2 := qtoolkit.EC2Info{}
    qtoolkit.CreateOrder(order)
}
```

**v1.0 代码:**
```go
package main

import (
    "github.com/wordgate/qtoolkit/core"
    "github.com/wordgate/qtoolkit/slack"
    "github.com/wordgate/qtoolkit/aws"
    // WordGate功能需要单独导入
    "github.com/wordgate/wordgate-client"
)

func main() {
    core.SetConfigFile("main_config.yml")
    
    // 检查模块是否启用
    if core.GetModuleConfig("slack").Enabled {
        slack.SendSlackMessage("hello")
    }
    
    if core.GetModuleConfig("aws").Enabled {
        ec2 := aws.EC2Info{}
    }
    
    // WordGate功能使用独立客户端
    client := wordgate.NewClient("app_code", "app_secret")
    client.CreateOrder(order)
}
```

### 4. WordGate 功能迁移

**重要变更**: WordGate 相关功能已移至独立仓库

**v0.x:**
```go
import "github.com/wordgate/qtoolkit"

qtoolkit.CreateOrder(order)
qtoolkit.SyncProducts(products)
```

**v1.0:**
```go
import "github.com/wordgate/wordgate-client"

client := wordgate.NewClient("app_code", "app_secret")
client.CreateOrder(order)
sdk := wordgate.NewSDK(config)
sdk.SyncProducts(products)
```

### 5. 构建和依赖管理

**v0.x 构建:**
```bash
go build  # 编译所有依赖
```

**v1.0 构建:**
```bash
# 本地开发 (使用workspace)
go build

# 生产构建 (禁用workspace)
GOWORK=off go build

# 只构建特定模块
cd aws && go build
```

**依赖管理:**
```bash
# 更新所有模块
go work sync

# 更新特定模块
cd core && go mod tidy
cd aws && go mod tidy
```

## 🔧 迁移工具

我们提供了自动迁移工具帮助您快速迁移：

```bash
# 下载迁移工具
go install github.com/wordgate/qtoolkit-migrate@latest

# 扫描项目并生成迁移报告
qtoolkit-migrate scan ./your-project

# 自动更新导入路径
qtoolkit-migrate update ./your-project
```

## 📦 包管理变更

### go.mod 文件更新

**v0.x:**
```go
require github.com/wordgate/qtoolkit v0.1.1
```

**v0.1.4:**
```go
require (
    github.com/wordgate/qtoolkit/core v0.1.4
    github.com/wordgate/qtoolkit/aws v0.1.4     // 仅在需要时添加
    github.com/wordgate/qtoolkit/slack v0.1.4   // 仅在需要时添加
)
```

### 版本兼容性

- ✅ v1.0+ 版本互相兼容
- ❌ v1.0 与 v0.x 不兼容，需要完整迁移
- ✅ 可以逐步迁移各个模块

## 💡 迁移最佳实践

### 1. 分阶段迁移
```bash
# 第一阶段：迁移核心功能
import "github.com/wordgate/qtoolkit/core"

# 第二阶段：按需添加服务模块
import "github.com/wordgate/qtoolkit/aws"

# 第三阶段：迁移WordGate功能
import "github.com/wordgate/wordgate-client"
```

### 2. 配置管理
- 保持原有配置文件结构
- 使用新的模块配置覆盖
- 逐步拆分配置文件

### 3. 测试策略
```bash
# 测试核心功能
cd core && go test ./...

# 测试各个模块
cd aws && go test ./...
cd slack && go test ./...

# 集成测试
go test ./...
```

## ⚠️ 常见问题

### Q: 为什么要进行这次重构？
A: 主要为了解决编译体积过大的问题。v0.x 即使只使用 Slack 功能，也会编译 AWS、阿里云等所有依赖。v1.0 实现了真正的按需编译。

### Q: 迁移后编译体积能减少多少？
A: 根据使用的模块不同，体积可减少 60-80%。例如只使用 core + slack 的应用，体积从 50MB 减少到 15MB。

### Q: WordGate 功能为什么要分离？
A: WordGate 是平台特定功能，独立维护更合适。qtoolkit 专注于通用工具库。

### Q: 如何处理现有的配置文件？
A: 可以保持原有配置，通过新的 `core.GetModuleConfig()` 读取，也可以逐步拆分为模块化配置。

### Q: 是否支持渐进式迁移？
A: 是的，可以先迁移核心功能，再逐步添加需要的服务模块。

## 🚀 v1.0 新特性

### 1. 编译优化
- 按需编译，大幅减少二进制体积
- 使用 build tags 进一步优化

### 2. 开发体验
- Go Workspace 简化多模块开发
- 统一的配置管理接口
- 更清晰的模块边界

### 3. 配置驱动
- 通过配置控制模块启用
- 运行时检查模块可用性
- 更灵活的服务组合

## 📞 获取帮助

- 📖 查看 [完整文档](https://github.com/wordgate/qtoolkit/wiki)
- 🐛 提交 [Issue](https://github.com/wordgate/qtoolkit/issues)
- 💬 加入 [讨论](https://github.com/wordgate/qtoolkit/discussions)

---

**迁移完成检查清单:**

- [ ] 更新所有导入路径
- [ ] 重构配置文件结构  
- [ ] 迁移 WordGate 功能到独立客户端
- [ ] 更新构建脚本
- [ ] 运行完整测试套件
- [ ] 验证编译体积减少
- [ ] 更新部署配置

祝迁移顺利！🎉