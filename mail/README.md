# Mail Module

极简 SMTP 邮件发送模块。

## 设计理念

- **单一 API**：只有一个 `Send()` 函数
- **简单结构**：用 `Message` 结构体配置所有选项
- **零配置**：自动从 viper 加载配置
- **内部隐藏**：gomail 实现细节完全透明

## 配置

```yaml
mail:
  send_from: sender@example.com
  username: smtp-user@example.com
  password: smtp-password
  smtp_host: smtp.example.com
  smtp_port: 587
```

## 使用示例

### 纯文本邮件

```go
import "github.com/wordgate/qtoolkit/mail"

mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "Hello",
    Body:    "Hello World",
})
```

### HTML 邮件

```go
mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "Newsletter",
    Body:    "<h1>Welcome</h1><p>This is HTML email</p>",
    IsHTML:  true,
})
```

### 带附件的邮件

```go
// 读取文件
csvData, _ := os.ReadFile("report.csv")

mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "Monthly Report",
    Body:    "Please see the attached report",
    Attachments: []mail.Attachment{
        {Filename: "report.csv", Data: csvData},
    },
})
```

### 前端上传文件发送邮件 (Gin)

```go
func uploadHandler(c *gin.Context) {
    // 获取上传文件
    file, _ := c.FormFile("upload")
    f, _ := file.Open()
    defer f.Close()
    data, _ := io.ReadAll(f)

    // 发送邮件
    err := mail.Send(&mail.Message{
        To:      "user@example.com",
        Subject: "Your File",
        Body:    "File received",
        Attachments: []mail.Attachment{
            {Filename: file.Filename, Data: data},
        },
    })

    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, gin.H{"status": "sent"})
}
```

### 完整功能示例

```go
mail.Send(&mail.Message{
    To:      "recipient@example.com",
    Subject: "Complete Example",
    Body:    "<h1>Report</h1><p>See attachments</p>",
    IsHTML:  true,
    ReplyTo: "noreply@example.com",
    Cc:      []string{"manager@example.com", "team@example.com"},
    Attachments: []mail.Attachment{
        {Filename: "report.pdf", Data: pdfData},
        {Filename: "data.csv", Data: csvData},
    },
})
```

## API

### Message 结构体

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `To` | `string` | ✅ | 收件人邮箱 |
| `Subject` | `string` | ✅ | 邮件主题 |
| `Body` | `string` | | 邮件正文 |
| `IsHTML` | `bool` | | 是否 HTML 格式（默认 false） |
| `ReplyTo` | `string` | | 回复地址（可选） |
| `Cc` | `[]string` | | 抄送列表（可选） |
| `Attachments` | `[]Attachment` | | 附件列表（可选） |

### Attachment 结构体

| 字段 | 类型 | 说明 |
|------|------|------|
| `Filename` | `string` | 附件文件名 |
| `Data` | `[]byte` | 附件数据 |

### 函数

| 函数 | 说明 |
|------|------|
| `Send(msg *Message) error` | 发送邮件（唯一的公共 API） |

## 特性

- ✅ 纯文本邮件
- ✅ HTML 邮件
- ✅ 附件支持（内存数据）
- ✅ 回复地址（Reply-To）
- ✅ 抄送（Cc）
- ✅ 字段验证
- ✅ 懒加载配置（sync.Once）
- ✅ Viper 自动配置

## 错误处理

```go
err := mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "Test",
    Body:    "Test content",
})

if err != nil {
    log.Printf("Failed to send email: %v", err)
    return err
}
```

## 设计对比

### ❌ 旧设计（复杂）

```go
// 多个函数，暴露实现细节
msg := mail.NewTextMessage("to", "subject", "body")
mail.AttachFile(msg, "file.pdf")
mail.SendMessage(msg)
```

### ✅ 新设计（简洁）

```go
// 单一函数，结构化配置
mail.Send(&mail.Message{
    To:      "to",
    Subject: "subject",
    Body:    "body",
    Attachments: []mail.Attachment{
        {Filename: "file.pdf", Data: data},
    },
})
```

## 优势

1. **极简 API**：只需学习一个函数
2. **清晰结构**：所有选项一目了然
3. **类型安全**：编译时检查
4. **易于测试**：结构体易于 mock
5. **无需清理**：无 gomail 细节泄漏
