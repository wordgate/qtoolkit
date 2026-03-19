# FastGPT + Chatwoot 模块设计

## 背景

用户在安装 App 时需要保姆级引导（国产安卓、macOS、Apple ID 等）。使用 FastGPT 知识库存放安装教程，通过 Chatwoot 网页客服接入，自动回答用户问题。

## 架构

```
用户(网页 Chatwoot widget) → Chatwoot webhook → 我们的后端 → FastGPT Chat API
                                                            ↓
用户 ← Chatwoot ← chatwoot.Reply() ←────────────────── AI 回答
```

## 模块设计

### 1. `fastgpt/` 模块

封装 FastGPT Chat API，提供单一函数。

**API：**

```go
// 纯文本
result, err := fastgpt.Chat(ctx, chatID, fastgpt.Text("怎么安装app"))

// 文本 + 图片
result, err := fastgpt.Chat(ctx, chatID,
    fastgpt.Text("这个界面怎么操作"),
    fastgpt.ImageURL("https://s3.../screenshot.png"),
)

// 文本 + 文件
result, err := fastgpt.Chat(ctx, chatID,
    fastgpt.Text("请帮我看看这个文件"),
    fastgpt.FileURL("doc.pdf", "https://s3.../doc.pdf"),
)

result.Content    // AI 回答文本
result.Similarity // 知识库匹配相似度（0-1），用于判断是否转人工
```

```go
// Part 表示消息的一个内容片段（文本、图片或文件）
type Part struct { /* 内部字段 */ }

func Text(text string) Part          // 文本片段
func ImageURL(url string) Part       // 图片 URL 片段
func FileURL(name, url string) Part  // 文件 URL 片段（支持 txt/md/html/word/pdf/ppt/csv/excel）

// Result 是 Chat 的返回结果
type Result struct {
    Content    string  // AI 回答
    Similarity float64 // 知识库最高匹配相似度（0-1），无匹配时为 0
}
```

**配置（viper）：**

```yaml
fastgpt:
  api_key: "fastgpt-xxxxxxxx"   # 应用专属 key（含 AppId）
  base_url: "https://your-fastgpt.com"
```

**内部实现要点：**

- `POST {base_url}/api/v1/chat/completions`
- Header: `Authorization: Bearer {api_key}`
- Body: `{"chatId": chatID, "stream": false, "detail": true, "messages": [{"role": "user", "content": ...}]}`
  - 纯文本时 content 为 string
  - 多模态时 content 为数组：`[{"type":"text","text":"..."}, {"type":"image_url","image_url":{"url":"..."}}, {"type":"file_url","name":"...","url":"..."}]`
- 解析响应：`choices[0].message.content` 提取回答，`responseData` 中 `moduleType=datasetSearchNode` 的直接字段 `similarity` 提取相似度（非 quoteList 内的 score）
- FastGPT 不支持直接上传文件，只接受 URL（图片/文件需先上传到对象存储）
- 懒加载 config + 共享 `*http.Client`（sync.Once + viper），HTTP 超时 30s
- `base_url` 内部自动 trim 尾斜杠

**文件结构：**

```
fastgpt/
├── go.mod
├── fastgpt.go           # Chat() + config
├── fastgpt_test.go
└── fastgpt_config.yml
```

### 2. `chatwoot/` 模块

封装 Chatwoot webhook 接收 + 消息回复。

**API：**

```go
// Reply 向 Chatwoot 会话发送回复
chatwoot.Reply(ctx, conversationID, "请按以下步骤...")

// Mount 挂载 webhook 路由到 gin，自动异步处理
chatwoot.Mount(r, "/chatwoot/webhook", func(ctx context.Context, event chatwoot.Event) {
    // 此 handler 在 goroutine 中异步执行
    // ctx 带 60s 超时，不受 HTTP 请求生命周期影响
})
```

**Mount 内部行为（只做基础设施，不做业务过滤）：**

1. 收到 webhook → 验证 HMAC 签名（如配置了 `webhook_token`）
2. 解析 webhook body 为 Event 结构（包含所有字段供 handler 判断）
3. 立即返回 200
4. goroutine 调用 handler（带 panic recovery + 60s 超时 context）
5. handler 内如果 `Reply` 失败，错误输出到 stderr（与 asynq 模式一致）

**业务过滤由 handler 自行处理**（event type、sender type、message type、空消息等）。

**Event 结构：**

```go
type Event struct {
    EventType      string       // "message_created", "conversation_created", etc.
    Content        string       // 消息文本内容
    ConversationID int          // 会话 ID
    MessageType    string       // "incoming" / "outgoing"
    Sender         Sender
    Conversation   Conversation
    Attachments    []Attachment // 附件列表（图片、语音、视频、文件）
}

type Sender struct {
    ID   int
    Name string
    Type string // "contact" (用户) / "user" (agent) / "agent_bot"
}

type Conversation struct {
    Status string // "open" / "resolved" / "pending"
}

type Attachment struct {
    FileType string // "image" / "audio" / "video" / "file"
    DataURL  string // 文件 URL
    ThumbURL string // 缩略图 URL（仅图片有）
    FileSize int    // 文件大小（字节）
}
```

**配置（viper）：**

```yaml
chatwoot:
  api_token: "YOUR_CHATWOOT_API_TOKEN"
  base_url: "https://your-chatwoot.com"
  account_id: 1
  webhook_token: ""   # 可选，HMAC 签名验证
```

**内部实现要点：**

- Reply: `POST {base_url}/api/v1/accounts/{account_id}/conversations/{conversation_id}/messages`
  - Header: `api_access_token: {api_token}`
  - Body: `{"content": text, "message_type": "outgoing"}`
- 懒加载 config + 共享 `*http.Client`（sync.Once + viper）
- `base_url` 内部自动 trim 尾斜杠
- `account_id` 必需字段验证（不能为 0）

**文件结构：**

```
chatwoot/
├── go.mod
├── chatwoot.go           # Mount() + Reply() + config
├── chatwoot_test.go
└── chatwoot_config.yml
```

## 使用者完整代码

```go
func main() {
    viper.SetConfigFile("config.yml")
    viper.ReadInConfig()

    r := gin.Default()

    chatwoot.Mount(r, "/chatwoot/webhook", func(ctx context.Context, event chatwoot.Event) {
        if event.EventType != "message_created" {
            return
        }
        if event.Sender.Type != "contact" || event.MessageType != "incoming" {
            return
        }

        // 构建多模态消息
        var parts []fastgpt.Part
        if strings.TrimSpace(event.Content) != "" {
            parts = append(parts, fastgpt.Text(event.Content))
        }
        for _, att := range event.Attachments {
            switch att.FileType {
            case "image":
                parts = append(parts, fastgpt.ImageURL(att.DataURL))
            case "audio":
                // 语音先 STT 转文字（使用 OpenAI Whisper 等）
                text, _ := transcribe(ctx, att.DataURL)
                if text != "" {
                    parts = append(parts, fastgpt.Text(text))
                }
            }
        }
        if len(parts) == 0 {
            return
        }

        chatID := fmt.Sprint(event.ConversationID)
        result, err := fastgpt.Chat(ctx, chatID, parts...)
        if err != nil {
            chatwoot.Reply(ctx, event.ConversationID, "抱歉，系统暂时无法回答，请稍后再试")
            return
        }
        // 相似度低于阈值 → 不回复，留给人工处理
        if result.Similarity < 0.7 {
            return
        }
        chatwoot.Reply(ctx, event.ConversationID, result.Content)
    })

    r.Run(":8080")
}
```

```yaml
# config.yml
fastgpt:
  api_key: "fastgpt-xxxxxxxx"
  base_url: "https://your-fastgpt.com"

chatwoot:
  api_token: "YOUR_CHATWOOT_API_TOKEN"
  base_url: "https://your-chatwoot.com"
  account_id: 1
  webhook_token: "YOUR_WEBHOOK_SECRET"
```

## 多模态支持

### 图片
- 用户发图片 → Chatwoot webhook 包含 `attachments[].file_type="image"` + `data_url`
- `data_url` 直接传给 FastGPT（`ImageURL` Part），FastGPT 支持视觉模型理解图片
- FastGPT 回复只有文本（但文本中可包含 markdown 图片链接 `![](url)`）

### 语音
- 用户发语音 → Chatwoot webhook 包含 `attachments[].file_type="audio"` + `data_url`
- **STT 不在 fastgpt/chatwoot 模块中处理** — 是业务逻辑，使用者在 handler 中调用
- 推荐方案：OpenAI Whisper API（gpt-4o-mini-transcribe），中文识别好、价格低（$0.003/分钟）、现有 `ai/` 模块可复用
- 语音转文字后作为 `Text` Part 发给 FastGPT

### FastGPT 限制
- 不支持直接上传文件，只接受 URL
- 回复只有文本，不返回图片/文件

## 转人工策略

FastGPT 在 `detail=true` 时，`responseData` 中的 `datasetSearchNode` 返回知识库匹配的 `similarity` 分数（0-1）。`Chat()` 提取该分数放入 `Result.Similarity`，使用者自行设定阈值判断是否转人工。

**低于阈值时的行为**：不发送 AI 回复，消息留在 Chatwoot 队列中等待人工处理。无需额外 API 调用。

## 设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 独立模块 vs 扩展 ai/ | 独立模块 | FastGPT 的 chatId 概念与通用 AI provider 不兼容，且 HTTP 调用极简不值得引入依赖 |
| Streaming vs 非 Streaming | 非 Streaming | Chatwoot 不支持逐字推送，用 streaming 增加复杂度无收益 |
| chatId 策略 | 透传 Chatwoot conversation ID | 安装引导是多轮对话，零成本获得上下文能力 |
| Webhook 处理 | 异步（goroutine） | FastGPT 响应可能 3-10 秒，不能阻塞 webhook 返回 |
| goroutine context | context.WithTimeout(Background(), 60s) | HTTP 返回后 request context 被取消，goroutine 需要独立 context + 超时保护 |
| Webhook 签名 | 可选 HMAC 验证 | 防止外部伪造调用，通过 webhook_token 配置启用 |
| 事件/消息过滤 | handler 自行处理 | Mount 只做基础设施（解析、验签、异步），业务过滤交给使用者，保持灵活性 |
| HTTP Client | 共享 *http.Client + 30s 超时 | 复用连接，符合项目其他模块模式 |
| Panic recovery | goroutine 内 defer recover | 保护进程不因用户 handler 代码崩溃 |
| Chat() 输入格式 | variadic Part（Text/ImageURL/FileURL） | 支持纯文本和多模态，API 简洁 |
| STT 服务 | 不封装，使用者在 handler 调用 | STT 是业务逻辑，推荐 OpenAI Whisper，现有 ai/ 模块可复用 |

## 不做的事情

- 不封装 FastGPT 知识库管理 API（在 FastGPT 后台维护）
- 不封装 Chatwoot 会话/联系人管理
- 不做 streaming
- 不做消息类型（cards、form 等）— 回复纯文本足够
- 不做 webhook 注册 — 在 Chatwoot 后台配置
- 不做并发限制 — 当前用户规模不需要，未来按需添加
