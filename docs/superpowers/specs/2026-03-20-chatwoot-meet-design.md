# Chatwoot Meet — 客服视频会议模块设计

## 概述

基于 Cal.com（预约）+ LiveKit Cloud（视频）的客服视频会议功能，集成在 qtoolkit 的 chatwoot 模块下。客户通过 Cal.com 预约客服的可用时段，预约确认后自动生成视频会议链接，通过 Chatwoot 对话发送给客户，通过 Slack 通知客服。

## 目标

- 客户能通过 Cal.com 预约客服视频时段
- 预约确认后自动生成会议链接并分发
- 客户点链接即可进入视频会议，客服收到通知后加入
- 嵌入式 HTML 页面，微信浏览器优先兼容
- 使用 Redis 存储会议状态，无需数据库

## 模块结构

```
chatwoot/
├── chatwoot.go              # 现有 webhook 处理
├── calcom/                  # Cal.com 子模块
│   ├── go.mod               # github.com/wordgate/qtoolkit/chatwoot/calcom
│   ├── calcom.go            # API 客户端、配置、初始化
│   ├── webhook.go           # Webhook 解析（含签名验证）
│   ├── calcom_config.yml
│   └── calcom_test.go
└── meet/                    # 视频会议子模块
    ├── go.mod               # github.com/wordgate/qtoolkit/chatwoot/meet
    ├── meet.go              # Schedule 管理、token 生成/验证、Redis 存储
    ├── livekit.go           # LiveKit 房间管理、participant token 生成
    ├── handler.go           # Gin handlers
    ├── embed/
    │   └── meet.html        # 视频会议页面（go:embed）
    ├── meet_config.yml
    └── meet_test.go
```

## 配置

```yaml
# Cal.com 配置（chatwoot/calcom 模块）
chatwoot:
  calcom:
    api_key: "YOUR_CALCOM_API_KEY"
    base_url: "https://api.cal.com/v2"    # 默认值，自托管可改
    webhook_secret: "YOUR_CALCOM_WEBHOOK_SECRET"

# 视频会议配置（chatwoot/meet 模块）
chatwoot:
  meet:
    livekit:
      url: "wss://your-project.livekit.cloud"
      api_key: "YOUR_LIVEKIT_API_KEY"
      api_secret: "YOUR_LIVEKIT_API_SECRET"
    token_expiry: "24h"          # 会议链接有效期
    room_timeout: "60m"          # 房间最长时长（映射到 LiveKit MaxDuration）
    base_url: "https://your-domain.com"  # 生成链接的域名前缀
```

## 依赖

- `github.com/wordgate/qtoolkit/redis` — 存储 schedule 状态
- `github.com/wordgate/qtoolkit/chatwoot/calcom` — Cal.com webhook 解析
- `github.com/wordgate/qtoolkit/slack` — 通知客服
- `github.com/livekit/server-sdk-go/v2` — LiveKit server SDK
- `github.com/livekit/protocol` — LiveKit protocol（auth、livekit 子包）
- `github.com/gin-gonic/gin` — HTTP handlers
- `github.com/spf13/viper` — 配置

注意：meet 模块**不直接依赖**父模块 `chatwoot`。通过 `Mount()` 的 `ReplyFunc` 回调注入 Chatwoot Reply 能力，避免子模块→父模块的循环依赖。

## 完整流程

### 预约阶段

```
客服在 Chatwoot 对话中发送 Cal.com 预约链接
(带 ?metadata[conversation_id]=123&metadata[inbox_id]=5)
         │
         ▼
客户打开 Cal.com → 选择时段 → 填写信息 → 确认
         │
         ▼
Cal.com Webhook (BOOKING_CREATED) → POST {path}/webhook/calcom
         │
         ▼
calcom.ParseWebhook() 解析 + 验证签名（HMAC-SHA256）
         │
         ▼
提取客户信息 + metadata(conversation_id, inbox_id)
         │
         ▼
生成 schedule_id（nanoid，12 字符 URL-safe）
生成 customer_token / agent_token（crypto/rand，32 字节 URL-safe base64）
         │
         ▼
Redis 存 schedule + token 反查索引
         │
         ├──→ ReplyFunc(conversation_id, 客户会议链接)
         │    「您的视频会议已预约，点击加入：{base_url}/meet/{customer_token}」
         │
         └──→ slack.Send(channel, 客服会议链接 + 预约信息)
              「新预约：客户xxx，时间xxx，加入：{base_url}/meet/{agent_token}」
```

### 取消/改期

```
Cal.com Webhook (BOOKING_CANCELLED) → POST {path}/webhook/calcom
         │
         ▼
删除 Redis 中的 schedule + token 键
         │
         ├──→ ReplyFunc(conversation_id, 「视频会议已取消」)
         └──→ slack.Send(channel, 「预约已取消：客户xxx」)

Cal.com Webhook (BOOKING_RESCHEDULED) → POST {path}/webhook/calcom
         │
         ▼
更新 Redis 中 schedule 的 scheduled_at + 重置 TTL
         │
         ├──→ ReplyFunc(conversation_id, 「会议时间已更新为 xxx」)
         └──→ slack.Send(channel, 「预约已改期：客户xxx，新时间xxx」)
```

### 会议阶段

```
客户点击 /meet/{customer_token}
         │
         ▼
handler.go 验证 token（Redis 查 meet:token:{token}）
         │
         ├── 无效/过期 → 错误页面
         │
         ├── 微信 + 不支持 WebRTC → 引导页「请点击右上角 ⋯ → 在浏览器中打开」
         │
         ├── 未到预约时间 → 等候页 + 倒计时
         │
         └── 有效且到时间 →
               │
               ▼
         LiveKit 创建房间（MaxDuration=room_timeout）+ 生成 participant token
               │
               ▼
         返回 meet.html → 客户进入房间
               │
               ▼
         原子更新 status: pending → active（Redis SETNX 去重）
         仅首次转换时 slack.Send() 通知客服「客户已进入，请加入」
               │
               ▼
         客服点击 /meet/{agent_token} → 同样流程 → 进入同一房间
               │
               ▼
         视频通话中
               │
               ▼
         LiveKit MaxDuration 到期 / 参与者全部离开 → 房间自动关闭
         Redis TTL 自动过期清理
```

## 公开 API

### calcom 模块

```go
package calcom

// ParseWebhook 解析并验证 Cal.com webhook 请求
// 内部使用 webhook_secret 进行 HMAC-SHA256 签名验证
// 签名头：X-Cal-Signature-256（hex 编码的 HMAC-SHA256）
func ParseWebhook(r *http.Request) (*Event, error)

// 类型
type Event struct {
    TriggerEvent string            // "BOOKING_CREATED", "BOOKING_CANCELLED", "BOOKING_RESCHEDULED"
    Booking      Booking
    Metadata     map[string]string // 透传的 metadata (conversation_id, inbox_id)
}

type Booking struct {
    ID        int
    Title     string
    StartTime time.Time
    EndTime   time.Time
    Attendees []Attendee
}

type Attendee struct {
    Name  string
    Email string
}

// 配置（标准 qtoolkit lazy-load 模式）
func SetConfig(cfg *Config) // 仅测试用
```

### meet 模块

```go
package meet

// ReplyFunc 是发送 Chatwoot 消息的回调，注入以避免循环依赖
type ReplyFunc func(ctx context.Context, conversationID int, text string) error

// Mount 挂载所有路由
func Mount(r gin.IRouter, path string, reply ReplyFunc)
// 注册的路由：
//   POST {path}/webhook/calcom   — Cal.com webhook 接收
//   GET  {path}/{token}          — 会议页面
```

## ID 与 Token 生成

- **schedule_id**：nanoid，12 字符，URL-safe 字符集（`[A-Za-z0-9_-]`）
- **customer_token / agent_token**：`crypto/rand` 生成 32 字节，URL-safe base64 编码（约 43 字符）
- 两个 token 互相独立，不可从一个推导另一个

## Redis 数据结构

```
Key:   meet:schedule:{schedule_id}
Value: JSON {
    "calcom_booking_id": 12345,
    "agent_email": "agent@example.com",
    "customer_name": "张三",
    "customer_email": "customer@example.com",
    "conversation_id": 123,
    "inbox_id": 5,
    "scheduled_at": "2026-03-20T14:00:00Z",
    "duration": "60m",
    "room_name": "meet-{schedule_id}",
    "customer_token": "xxx...",
    "agent_token": "yyy...",
    "status": "pending"      // pending / active / ended
}
TTL: token_expiry 配置值（默认 24h）

Key:   meet:token:{token} → schedule_id   // token 反查
TTL:   同上
```

## meet.html 嵌入页面

- **livekit-client SDK**：通过 CDN 引入，使用 `LivekitClient.Room` API 手动管理视频连接
- **CDN 引入**：`unpkg.com/livekit-client/dist/livekit-client.umd.min.js`
- **零框架**：纯 HTML + vanilla JS（约 50-80 行 JS 处理视频布局和控制）
- **响应式**：手机优先，CSS flexbox 布局
- **go:embed**：编译进二进制，通过 `html/template` 注入运行时数据

注意：LiveKit 没有可用于 vanilla HTML 的预构建 web component（`@livekit/components-react` 仅限 React）。
需要手动处理：房间连接、本地/远端视频轨道附加、静音/关摄像头/挂断按钮。

### 页面状态

1. **微信引导页**：检测 UserAgent，不支持 WebRTC 时显示「在浏览器中打开」引导
2. **等候页**：未到预约时间，显示倒计时
3. **视频页**：大画面（对方）+ 小画面（自己）+ 三个控制按钮（静音/摄像头/挂断）
4. **结束页**：会议结束或 token 过期

### 核心 HTML 结构

```html
<script src="https://unpkg.com/livekit-client/dist/livekit-client.umd.min.js"></script>

<div id="videos">
  <div id="remote-video"></div>
  <div id="local-video"></div>
</div>
<div id="controls">
  <button id="btn-mic">静音</button>
  <button id="btn-cam">关闭摄像头</button>
  <button id="btn-hangup">挂断</button>
</div>

<script>
  const room = new LivekitClient.Room();
  // 远端轨道订阅 → 附加到 #remote-video
  room.on(LivekitClient.RoomEvent.TrackSubscribed, (track, pub, participant) => {
    document.getElementById('remote-video').appendChild(track.attach());
  });
  // 连接房间 + 开启摄像头麦克风
  room.connect('{{.LiveKitURL}}', '{{.ParticipantToken}}')
    .then(() => room.localParticipant.enableCameraAndMicrophone());
</script>
```

## 微信兼容策略

- 检测 `navigator.userAgent` 包含 `MicroMessenger`
- 检测 `RTCPeerConnection` 是否可用
- 不支持时显示引导页，带图文说明「点击右上角 ⋯ → 在默认浏览器中打开」
- iOS 微信（iOS 14.3+）大概率支持，Android 微信需引导

## 错误处理

- Token 无效/过期：友好错误页，提示联系客服
- LiveKit 连接失败：页面内重试提示
- Cal.com webhook 签名验证失败：返回 401，日志记录
- Redis 不可用：webhook 返回 500，Cal.com 会自动重试
- 预约取消后访问链接：错误页，提示「该会议已取消」
