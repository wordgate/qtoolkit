# Mail Module

SMTP 邮件发送模块，支持纯文本和 HTML 邮件。

## 配置

```yaml
mail:
  send_from: sender@example.com
  username: smtp-user@example.com
  password: smtp-password
  smtp_host: smtp.example.com
  smtp_port: 587
```

## 使用

### 简单发送

```go
// 纯文本邮件
mail.SendText("user@example.com", "标题", "内容")

// HTML邮件
mail.SendHtml("user@example.com", "标题", "<h1>Hello</h1>")
```

### 自定义 Header

```go
// 创建消息，添加 Reply-To 等自定义 header
msg := mail.NewTextMessage("user@example.com", "标题", "内容")
msg.SetHeader("Reply-To", "reply@example.com")
msg.SetHeader("Cc", "cc@example.com")
mail.SendMessage(msg)

// HTML 消息同理
msg := mail.NewHtmlMessage("user@example.com", "标题", "<h1>Hello</h1>")
msg.SetHeader("Reply-To", "reply@example.com")
mail.SendMessage(msg)
```

## API

| 函数 | 说明 |
|------|------|
| `SendText(to, subject, content)` | 发送纯文本邮件 |
| `SendHtml(to, subject, html)` | 发送 HTML 邮件 |
| `NewTextMessage(to, subject, text)` | 创建纯文本消息 |
| `NewHtmlMessage(to, subject, html)` | 创建 HTML 消息 |
| `SendMessage(msg)` | 发送自定义消息 |
