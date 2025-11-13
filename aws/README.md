# AWS Module

AWS services integration for WordGate qtoolkit with automatic configuration loading.

## 架构升级 (v1.0)

所有 AWS 子模块已升级到自动配置加载模式：
- ✅ **自动配置加载** - 无需调用 `SetConfig()`，自动从 viper 读取
- ✅ **级联配置回退** - 服务配置自动回退到全局 AWS 配置
- ✅ **多区域支持** - 每个服务可使用不同的 AWS 区域
- ✅ **IAM 角色支持** - EC2 上使用 IMDS 获取凭证
- ✅ **按服务凭证** - 每个服务可覆盖全局凭证

## 子模块

- **s3/** - S3 文件存储
- **ses/** - Simple Email Service (邮件服务)
- **sqs/** - Simple Queue Service (消息队列)
- **ec2/** - EC2 实例管理

## 配置结构

所有 AWS 服务使用统一的级联配置结构：

```yaml
aws:
  # 全局 AWS 凭证（所有服务的默认值）
  access_key: "YOUR_AWS_ACCESS_KEY"
  secret_key: "YOUR_AWS_SECRET_KEY"
  region: "us-west-2"
  use_imds: false  # EC2 IAM 角色设为 true

  # S3 配置（可选覆盖）
  s3:
    bucket: "my-bucket"
    url_prefix: "https://my-bucket.s3.us-west-2.amazonaws.com"
    # region: "us-west-2"  # 可选：覆盖全局区域

  # SES 配置
  ses:
    default_from: "noreply@example.com"
    # region: "us-east-1"  # 可选：覆盖全局区域

  # SQS 配置
  sqs:
    # 按队列配置
    queues:
      notifications:
        region: "us-east-1"
      background-jobs:
        region: "us-west-2"
```

### 配置优先级

每个服务遵循以下回退链：

1. **服务特定配置** (如 `aws.s3.region`)
2. **全局 AWS 配置** (如 `aws.region`)

对于 SQS 队列：

1. **队列特定配置** (如 `aws.sqs.queues.notifications.region`)
2. **SQS 服务配置** (如 `aws.sqs.region`)
3. **全局 AWS 配置** (如 `aws.region`)

## 使用示例

### S3 文件上传

```go
import (
    "github.com/wordgate/qtoolkit/aws/s3"
    "github.com/spf13/viper"
)

func main() {
    // 加载配置文件
    viper.SetConfigFile("config.yml")
    viper.ReadInConfig()

    // 直接使用（配置自动加载）
    url, err := s3.UploadBytes("path/to/file.jpg", data)
    if err != nil {
        panic(err)
    }
    fmt.Println("Uploaded:", url)
}
```

### SES 邮件发送

```go
import (
    "github.com/wordgate/qtoolkit/aws/ses"
)

func main() {
    // 发送邮件（配置自动加载）
    resp := ses.SendEmail(ses.EmailRequest{
        To:       []string{"user@example.com"},
        Subject:  "Welcome!",
        BodyHTML: "<h1>Hello!</h1>",
    })
    if resp.Success {
        fmt.Println("Sent:", resp.MessageID)
    }
}
```

### SQS 消息队列

```go
import (
    "github.com/wordgate/qtoolkit/aws/sqs"
)

func main() {
    // 获取队列客户端（配置自动从 aws.sqs.queues.notifications 加载）
    client, err := sqs.Get("notifications")
    if err != nil {
        panic(err)
    }

    // 发送消息
    err = client.Send("user.registered", map[string]interface{}{
        "user_id": 123,
        "email": "user@example.com",
    })

    // 消费消息
    client.Consume(func(msg sqs.Message) error {
        fmt.Printf("Received: %s\n", msg.Action)
        return nil  // 返回 error 会触发重试
    })
}
```

## 从旧版 API 迁移

### 之前 (v0.x) - 需要手动配置

```go
// 旧方式：手动设置配置
s3Config := &s3.Config{
    AccessKey: "...",
    SecretKey: "...",
    Region: "us-west-2",
    Bucket: "my-bucket",
}
s3.SetConfig(s3Config)
s3.Upload("file.jpg", data)
```

### 现在 (v1.0) - 自动加载

```yaml
# config.yml
aws:
  access_key: "..."
  secret_key: "..."
  region: "us-west-2"
  s3:
    bucket: "my-bucket"
```

```go
// 新方式：直接使用！
viper.SetConfigFile("config.yml")
viper.ReadInConfig()
s3.Upload("file.jpg", data)  // 配置自动加载
```

## 安全最佳实践

1. **永不提交凭证** - 配置文件使用占位符
2. **使用环境变量** - 设置 `AWS_ACCESS_KEY_ID` 和 `AWS_SECRET_ACCESS_KEY`
3. **EC2 使用 IAM 角色** - 设置 `use_imds: true`，无需凭证
4. **定期轮换凭证**
5. **使用最小权限 IAM 策略**

## EC2 IMDS 支持

在 EC2 实例上运行时使用 IAM 角色：

```yaml
aws:
  use_imds: true  # 使用实例元数据服务
  region: "us-west-2"  # 仍需指定区域
  # 不需要 access_key 和 secret_key！
```

所有服务将自动使用 EC2 实例的 IAM 角色凭证。

## 开发

```bash
# 编译所有子模块
cd aws
go build ./...

# 运行测试
go test ./...

# 更新依赖
go mod tidy
```

## 模块文档

- [SQS Module README](sqs/README.md) - 消息队列服务
- [AWS Configuration Template](aws_config.yml) - 完整配置示例

## 完成的工作总结

### ✅ v1.0 架构升级
1. **自动配置** - 所有模块支持 viper 自动加载
2. **级联回退** - 服务配置 → 全局配置的三级回退
3. **独立模块** - 每个 AWS 服务独立的 go.mod
4. **向后兼容** - 保留 SetConfig() 作为备用接口
5. **统一配置** - aws_config.yml 完整示例

### ✅ 已实现的模块
- **aws/s3** - S3 文件存储（包含 presign 功能）
- **aws/ses** - SES 邮件服务
- **aws/sqs** - SQS 队列服务
- **aws/ec2** - EC2 实例管理
