# Changelog

## v2.0.0 - 极简重构 (2025-12-22)

### 💥 Breaking Changes

完全重写邮件模块，追求极简设计。**不考虑向后兼容**。

### 🎯 设计理念

- **极简 API**：7 个函数 → 1 个函数
- **清晰结构**：用结构体代替多步骤操作
- **隐藏实现**：完全封装 gomail 细节

### 📦 API 变更

#### 删除的函数

以下函数已全部移除：

```go
// ❌ 已删除
GetMailer()
NewTextMessage()
NewHtmlMessage()
SendMessage()
SendText()
SendHtml()
AttachFile()
AttachBytes()
```

#### 新增的 API

```go
// ✅ 唯一的公共 API
Send(msg *Message) error

// ✅ 新增结构体
type Message struct {
    To          string
    Subject     string
    Body        string
    IsHTML      bool
    ReplyTo     string
    Cc          []string
    Attachments []Attachment
}

type Attachment struct {
    Filename string
    Data     []byte
}
```

### 🔄 迁移指南

#### 1. 纯文本邮件

**旧代码：**
```go
mail.SendText("user@example.com", "标题", "内容")
```

**新代码：**
```go
mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "标题",
    Body:    "内容",
})
```

---

#### 2. HTML 邮件

**旧代码：**
```go
mail.SendHtml("user@example.com", "标题", "<h1>Hello</h1>")
```

**新代码：**
```go
mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "标题",
    Body:    "<h1>Hello</h1>",
    IsHTML:  true,  // 添加此字段
})
```

---

#### 3. 自定义 Header

**旧代码：**
```go
msg := mail.NewTextMessage("user@example.com", "标题", "内容")
msg.SetHeader("Reply-To", "noreply@example.com")
msg.SetHeader("Cc", "cc@example.com")
mail.SendMessage(msg)
```

**新代码：**
```go
mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "标题",
    Body:    "内容",
    ReplyTo: "noreply@example.com",          // 直接设置字段
    Cc:      []string{"cc@example.com"},     // 使用切片
})
```

---

#### 4. 添加文件附件

**旧代码：**
```go
msg := mail.NewTextMessage("user@example.com", "报告", "请查看附件")
if err := mail.AttachFile(msg, "/path/to/report.pdf"); err != nil {
    return err
}
mail.SendMessage(msg)
```

**新代码：**
```go
// 先读取文件
data, err := os.ReadFile("/path/to/report.pdf")
if err != nil {
    return err
}

// 发送邮件
mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "报告",
    Body:    "请查看附件",
    Attachments: []mail.Attachment{
        {Filename: "report.pdf", Data: data},
    },
})
```

---

#### 5. 添加内存附件 (前端上传)

**旧代码：**
```go
file, _ := c.FormFile("upload")
f, _ := file.Open()
defer f.Close()
data, _ := io.ReadAll(f)

msg := mail.NewTextMessage("user@example.com", "文件", "请查收")
if err := mail.AttachBytes(msg, file.Filename, data); err != nil {
    return err
}
mail.SendMessage(msg)
```

**新代码：**
```go
file, _ := c.FormFile("upload")
f, _ := file.Open()
defer f.Close()
data, _ := io.ReadAll(f)

mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "文件",
    Body:    "请查收",
    Attachments: []mail.Attachment{
        {Filename: file.Filename, Data: data},
    },
})
```

---

#### 6. 多个附件

**旧代码：**
```go
msg := mail.NewHtmlMessage("user@example.com", "多个附件", "<p>附件</p>")
mail.AttachFile(msg, "/path/to/doc.pdf")
mail.AttachFile(msg, "/path/to/image.png")
mail.AttachBytes(msg, "data.csv", csvData)
mail.SendMessage(msg)
```

**新代码：**
```go
// 先读取文件
pdfData, _ := os.ReadFile("/path/to/doc.pdf")
imgData, _ := os.ReadFile("/path/to/image.png")

mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "多个附件",
    Body:    "<p>附件</p>",
    IsHTML:  true,
    Attachments: []mail.Attachment{
        {Filename: "doc.pdf", Data: pdfData},
        {Filename: "image.png", Data: imgData},
        {Filename: "data.csv", Data: csvData},
    },
})
```

---

#### 7. 完整示例对比

**旧代码：**
```go
msg := mail.NewHtmlMessage("user@example.com", "报告", "<h1>月度报告</h1>")
msg.SetHeader("Reply-To", "noreply@example.com")
msg.SetHeader("Cc", "manager@example.com")

if err := mail.AttachFile(msg, "/path/to/report.pdf"); err != nil {
    return err
}

csvData := generateCSV()
if err := mail.AttachBytes(msg, "data.csv", csvData); err != nil {
    return err
}

if err := mail.SendMessage(msg); err != nil {
    return err
}
```

**新代码：**
```go
pdfData, _ := os.ReadFile("/path/to/report.pdf")
csvData := generateCSV()

err := mail.Send(&mail.Message{
    To:      "user@example.com",
    Subject: "报告",
    Body:    "<h1>月度报告</h1>",
    IsHTML:  true,
    ReplyTo: "noreply@example.com",
    Cc:      []string{"manager@example.com"},
    Attachments: []mail.Attachment{
        {Filename: "report.pdf", Data: pdfData},
        {Filename: "data.csv", Data: csvData},
    },
})

if err != nil {
    return err
}
```

### 📋 迁移检查清单

- [ ] 将 `SendText()` 替换为 `Send()` + `Message`
- [ ] 将 `SendHtml()` 替换为 `Send()` + `IsHTML: true`
- [ ] 将 `NewTextMessage/NewHtmlMessage` 替换为 `&Message{}`
- [ ] 将 `msg.SetHeader()` 替换为 `Message` 结构体字段
- [ ] 将 `AttachFile()` 替换为读文件 + `Attachments` 字段
- [ ] 将 `AttachBytes()` 替换为 `Attachments` 字段
- [ ] 删除所有 `SendMessage()` 调用（直接用 `Send()`）

### ✨ 优势

#### 代码可读性

**旧方式**：需要理解 gomail.Message 的工作方式
```go
msg := mail.NewTextMessage(...)
mail.AttachFile(msg, ...)
mail.SendMessage(msg)
// 3 步操作，暴露实现细节
```

**新方式**：一目了然的配置
```go
mail.Send(&mail.Message{
    To:      "...",
    Subject: "...",
    Body:    "...",
    Attachments: []mail.Attachment{...},
})
// 1 次调用，所有配置清晰可见
```

#### 测试友好

**旧方式**：需要 mock gomail.Message
```go
// 难以测试，依赖 gomail 实现
```

**新方式**：只需验证结构体
```go
msg := &mail.Message{
    To:      "test@example.com",
    Subject: "Test",
    Body:    "Test",
}
// 结构体易于断言和 mock
```

#### API 简洁性

| 指标 | 旧版本 | 新版本 | 改进 |
|------|--------|--------|------|
| 公共函数数 | 8 | 1 | ↓ 87.5% |
| 公共类型数 | 0 | 2 | - |
| 学习成本 | 高 | 低 | ↓↓↓ |
| 使用步骤 | 3-5 步 | 1 步 | ↓↓↓ |

### 🔍 常见问题

#### Q: 为什么删除 `AttachFile()`？

A: 新设计中，只支持内存附件（`[]byte`）。这样设计：
- **统一性**：所有附件都是 `Attachment` 结构体
- **明确性**：调用者必须显式读取文件
- **灵活性**：可以轻松修改数据后再发送

```go
// 读取文件由调用者控制
data, err := os.ReadFile("file.pdf")
// 可以在这里处理数据...
mail.Send(&mail.Message{
    Attachments: []mail.Attachment{
        {Filename: "file.pdf", Data: data},
    },
})
```

#### Q: 为什么删除 `GetMailer()`？

A: 完全隐藏实现细节。用户不需要（也不应该）访问底层的 gomail.Dialer。

#### Q: 我需要更多 Header 怎么办？

A: 当前只支持常用的 `ReplyTo` 和 `Cc`。如果需要更多 Header（如 `Bcc`），请提 issue 或 PR。

### 📝 配置不变

配置文件格式保持不变：

```yaml
mail:
  send_from: sender@example.com
  username: smtp-user@example.com
  password: smtp-password
  smtp_host: smtp.example.com
  smtp_port: 587
```

### 🚀 下一步

1. **搜索旧 API**：在项目中搜索 `mail.Send`、`mail.New`、`mail.Attach` 等
2. **逐个替换**：按照上述迁移指南替换
3. **测试验证**：确保邮件发送功能正常
4. **删除导入**：确认不再使用 `gopkg.in/gomail.v2`（如果直接导入）

### 📚 参考文档

- [README.md](./README.md) - 完整使用文档
- [mail.go](./mail.go) - 源代码实现
- [mail_test.go](./mail_test.go) - 测试示例
