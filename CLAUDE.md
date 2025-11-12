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

### v1.0 Architecture (Target/Modular)
模块化架构，按服务独立：
```
qtoolkit/
├── go.work                  # Workspace配置
├── core/                    # 核心模块
│   ├── go.mod
│   ├── config.go           # 配置管理
│   ├── event.go            # 事件系统  
│   ├── util/               # 工具库
│   └── log/                # 日志模块
├── aws/                     # AWS模块
│   ├── go.mod              # 仅AWS SDK依赖
│   ├── aws.go, aws_*.go
│   └── aws_config.yml
├── aliyun/                  # 阿里云模块
├── slack/                   # Slack模块
├── database/                # 数据库模块
├── email/                   # 邮件模块
├── redis/                   # Redis模块
├── godaddy/                 # GoDaddy模块
└── integration/             # 其他集成
```

## v1.0 模块化开发规范

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

## ⏱️ v1.0 迁移时间表

### 📅 并行开发阶段
- **当前状态**: v0.x维护 + v1.0新功能开发
- **新功能**: 100%按v1模块化架构实现
- **Bug修复**: v0.x修复，v1.0同步实现
- **重构**: 优先迁移v0.x功能到v1.0

### 🎯 迁移里程碑
1. **Phase 1**: 核心模块（core/util/log）- ✅ 已完成
2. **Phase 2**: 服务模块（aws/aliyun/slack）- 🚧 进行中
3. **Phase 3**: 集成模块（database/email/redis）- 📅 计划中
4. **Phase 4**: 完全切换，废弃v0.x - 📅 待定

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