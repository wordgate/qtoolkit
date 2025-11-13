# AWS Module - Lazy Load Architecture

所有 AWS 子模块都已实现 `sync.Once` lazy load 模式，提供简洁的 API 和自动初始化功能。

## 完成的工作总结

### ✅ 已实现 Lazy Load 的模块
- **aws/s3** - S3 文件存储（包含 presign 功能）
- **aws/ses** - SES 邮件服务
- **aws/sqs** - SQS 队列服务 
- **aws/ec2** - EC2 实例管理
- **db** - 数据库连接管理

### ✅ 架构改进
1. **sync.Once lazy load** - 所有模块自动初始化
2. **独立配置** - 每个模块有独立的 Config 结构
3. **presign 迁移** - 从 aws/ 根目录迁移到 aws/s3/
4. **配置示例** - 每个模块都有 xxx_config.yml 示例文件
5. **向后兼容层移除** - 删除 exports.go，确保模块独立

### 使用示例

```go
// S3 文件上传
s3.SetConfig(&s3.Config{Region: "us-east-1", Bucket: "my-bucket", UseIMDS: true})
url, _ := s3.Upload("uploads/image.jpg", fileReader)

// SES 邮件发送
ses.SetConfig(&ses.Config{Region: "us-east-1", DefaultFrom: "no-reply@domain.com", UseIMDS: true})
ses.SendMail("user@example.com", "Welcome", "Thank you!")

// DB 数据库
db.SetConfig(&db.Config{DSN: "user:pass@tcp(localhost:3306)/db", Debug: true})
db.Get().Create(&user)
```

详细文档请查看各模块的 README 和示例文件。
