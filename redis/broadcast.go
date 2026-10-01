package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// BroadcastMessage 广播消息结构
type BroadcastMessage struct {
	Channel   string      `json:"channel"`
	Timestamp int64       `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// 默认参数
const (
	defaultSubscriberBuffer = 64               // 每订阅者缓冲的消息数
	defaultPingInterval     = 30 * time.Second // WebSocket 心跳间隔
	defaultPongWait         = 70 * time.Second // 读超时：超过该时长收不到 pong/任何帧即判定对端已死
	defaultWriteWait        = 10 * time.Second // 单次 WebSocket 写超时
)

// subscriber 单个订阅者
//
// ch 永不关闭：广播循环可能与取消订阅并发投递，关闭 ch 会导致 send on closed channel。
// 订阅结束统一通过关闭 done 通知。
type subscriber struct {
	ch     chan *BroadcastMessage
	done   chan struct{}
	once   sync.Once
	kicked atomic.Bool // 因消费过慢被踢
	onKick func()      // 被踢时调用（关闭底层连接），必须非阻塞
}

func (s *subscriber) close() {
	s.once.Do(func() { close(s.done) })
}

// ChannelSubscribers 频道订阅者管理
type ChannelSubscribers struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
}

func (c *ChannelSubscribers) count() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return int64(len(c.subscribers))
}

func (c *ChannelSubscribers) isEmpty() bool {
	return c.count() == 0
}

func (c *ChannelSubscribers) snapshot() []*subscriber {
	c.mu.RLock()
	defer c.mu.RUnlock()
	subs := make([]*subscriber, 0, len(c.subscribers))
	for s := range c.subscribers {
		subs = append(subs, s)
	}
	return subs
}

// BroadcastOption 广播服务可选配置
type BroadcastOption func(*Broadcast)

// WithSubscriberBuffer 设置每个订阅者的消息缓冲大小（默认 64）。
// 缓冲写满的订阅者会被判定为慢消费者并踢掉。n <= 0 时忽略。
func WithSubscriberBuffer(n int) BroadcastOption {
	return func(b *Broadcast) {
		if n > 0 {
			b.subscriberBuffer = n
		}
	}
}

// WithCheckOrigin 设置 WebSocket 握手的 Origin 校验函数（默认全部放行）。
func WithCheckOrigin(fn func(*http.Request) bool) BroadcastOption {
	return func(b *Broadcast) {
		if fn != nil {
			b.checkOrigin = fn
		}
	}
}

// Broadcast 广播服务
type Broadcast struct {
	mu       sync.Mutex                     // 保护 channels 的增删，以及各频道订阅者集合的增删
	channels map[string]*ChannelSubscribers // channel -> subscribers

	rds                  *redis.Client
	cacheSecondsForLated int64
	pubsubKey            string // Redis 发布订阅键
	cachePrefix          string // 迟到消息缓存键前缀
	subscriberBuffer     int
	checkOrigin          func(*http.Request) bool
	pingInterval         time.Duration
	pongWait             time.Duration
	writeWait            time.Duration

	metrics struct {
		activeChannels   atomic.Int64 // 活跃channel数
		messagesSent     atomic.Int64 // 发送消息数
		messagesDropped  atomic.Int64 // 丢弃消息数
		subscribeLatency atomic.Int64 // 订阅延迟(毫秒)
	}
}

func newBroadcast(key string, cacheSecondsForLated int64, opts ...BroadcastOption) *Broadcast {
	if cacheSecondsForLated <= 0 {
		cacheSecondsForLated = 10
	}
	b := &Broadcast{
		channels:             make(map[string]*ChannelSubscribers),
		rds:                  Client(),
		cacheSecondsForLated: cacheSecondsForLated,
		pubsubKey:            key,
		cachePrefix:          key,
		subscriberBuffer:     defaultSubscriberBuffer,
		checkOrigin:          func(*http.Request) bool { return true },
		pingInterval:         defaultPingInterval,
		pongWait:             defaultPongWait,
		writeWait:            defaultWriteWait,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// NewBroadcast 创建新的广播服务实例
//
// Redis 发布订阅键固定为 "broadcast"：同一 Redis 上所有使用本构造函数的服务会互相收到消息
// （Redis pub/sub 不区分 db）。新服务请使用 NewNamedBroadcast。
func NewBroadcast(cacheSecondsForLated int64) *Broadcast {
	return newBroadcast("broadcast", cacheSecondsForLated)
}

// NewNamedBroadcast 创建带命名空间的广播服务实例
//
// Redis 发布订阅键为 "broadcast:"+namespace，迟到消息缓存键为 "broadcast:"+namespace+"/"+channel，
// 不同命名空间之间互不可见；同一命名空间的多个实例（多机部署）互通。
// namespace 为空会 panic（空命名空间等于没有隔离，属于启动期配置错误）。
func NewNamedBroadcast(namespace string, cacheSecondsForLated int64, opts ...BroadcastOption) *Broadcast {
	if namespace == "" {
		panic("redis: NewNamedBroadcast requires a non-empty namespace")
	}
	return newBroadcast("broadcast:"+namespace, cacheSecondsForLated, opts...)
}

func (b *Broadcast) broadcastKey() string {
	return b.pubsubKey
}

func (b *Broadcast) messageCacheKey(channel string) string {
	return fmt.Sprintf("%s/%s", b.cachePrefix, channel)
}

// subscribe 注册一个订阅者
func (b *Broadcast) subscribe(channel string, onKick func()) *subscriber {
	sub := &subscriber{
		ch:     make(chan *BroadcastMessage, b.subscriberBuffer),
		done:   make(chan struct{}),
		onKick: onKick,
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	subscribers, ok := b.channels[channel]
	if !ok {
		subscribers = &ChannelSubscribers{subscribers: make(map[*subscriber]struct{})}
		b.channels[channel] = subscribers
		b.metrics.activeChannels.Add(1)
		log.Printf("new channel created: %s, active channels: %d",
			channel, b.metrics.activeChannels.Load())
	}
	subscribers.mu.Lock()
	subscribers.subscribers[sub] = struct{}{}
	subscribers.mu.Unlock()
	return sub
}

// unsubscribe 移除订阅者（幂等），频道空了则一并清理
func (b *Broadcast) unsubscribe(channel string, sub *subscriber) {
	sub.close()

	b.mu.Lock()
	defer b.mu.Unlock()
	subscribers, ok := b.channels[channel]
	if !ok {
		return
	}
	subscribers.mu.Lock()
	delete(subscribers.subscribers, sub)
	empty := len(subscribers.subscribers) == 0
	subscribers.mu.Unlock()
	if empty {
		delete(b.channels, channel)
		b.metrics.activeChannels.Add(-1)
		log.Printf("channel cleaned: %s, remaining active channels: %d",
			channel, b.metrics.activeChannels.Load())
	}
}

// kick 踢掉慢消费者：取消订阅并关闭其连接。不得阻塞广播循环。
func (b *Broadcast) kick(channel string, sub *subscriber) {
	sub.kicked.Store(true)
	b.unsubscribe(channel, sub)
	if sub.onKick != nil {
		sub.onKick()
	}
}

// deliver 向频道内所有本机订阅者非阻塞投递，返回被踢掉的慢消费者数量
func (b *Broadcast) deliver(message *BroadcastMessage) int {
	chs, ok := b.Load(message.Channel)
	if !ok {
		return 0
	}
	kicked := 0
	for _, sub := range chs.snapshot() {
		select {
		case <-sub.done:
			// 已取消订阅
		case sub.ch <- message:
		default:
			// 缓冲已满 = 慢消费者，踢掉，绝不阻塞循环
			b.metrics.messagesDropped.Add(1)
			b.kick(message.Channel, sub)
			kicked++
			log.Printf("broadcast:slow subscriber kicked, channel:%s total dropped:%d",
				message.Channel, b.metrics.messagesDropped.Load())
		}
	}
	return kicked
}

// SubscriberCount 返回本实例上该频道的订阅者数量（不含其他机器上的订阅者）
func (b *Broadcast) SubscriberCount(channel string) int {
	chs, ok := b.Load(channel)
	if !ok {
		return 0
	}
	return int(chs.count())
}

// WsSubChannel WebSocket订阅频道
//
// 连接结束条件：客户端关闭/读错误、读超时（收不到 pong）、写失败、被判定为慢消费者、
// 频道被 Delete、请求 context 结束。
func (b *Broadcast) WsSubChannel(c *gin.Context, channel string) error {
	log.Printf("new websocket connection for channel: %s", channel)
	upgrader := websocket.Upgrader{
		CheckOrigin: b.checkOrigin,
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return err
	}
	defer ws.Close()

	// 被踢时在广播循环之外关闭连接：先尽力发关闭帧（写协程可能正卡在写上，
	// WriteControl 最多等 1s），再强制关闭底层连接以解除阻塞的写。
	onKick := func() {
		go func() {
			ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "slow consumer"),
				time.Now().Add(time.Second))
			ws.Close()
		}()
	}

	sub := b.subscribe(channel, onKick)
	defer b.unsubscribe(channel, sub)

	// 读循环：客户端帧内容一律丢弃，只用于探测对端关闭并处理 pong。
	// 读超时由 pong（以及任何客户端帧）续期。
	ws.SetReadDeadline(time.Now().Add(b.pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(b.pongWait))
	})
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := ws.NextReader(); err != nil {
				log.Printf("websocket read ended: channel:%s err:%v", channel, err)
				return
			}
			ws.SetReadDeadline(time.Now().Add(b.pongWait))
		}
	}()

	// 写循环：ping 与消息写都在本协程内完成（gorilla/websocket 不允许并发写）
	ticker := time.NewTicker(b.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case msg := <-sub.ch:
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("marshal message failed: %v", err)
				continue
			}
			ws.SetWriteDeadline(time.Now().Add(b.writeWait))
			if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("write message failed: %v", err)
				return err
			}
		case <-ticker.C:
			ws.SetWriteDeadline(time.Now().Add(b.writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("websocket ping failed: %v", err)
				return err
			}
		case <-sub.done:
			if sub.kicked.Load() {
				return fmt.Errorf("redis: slow subscriber kicked from channel %q", channel)
			}
			// 频道被 Delete
			ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(time.Second))
			return nil
		case <-readDone:
			return nil
		case <-c.Done():
			return nil
		}
	}
}

// WsSub WebSocket订阅处理器
func (b *Broadcast) WsSub(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		channel := c.Param(paramName)
		b.WsSubChannel(c, channel)
	}
}

// HttpSub HTTP长轮询订阅处理器
// since 毫秒时间戳
// timeout 客户端请求时设置的超时时间，单位为毫秒
func (b *Broadcast) HttpSub(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		channel := c.Param(paramName)
		if channel == "" {
			log.Printf("http sub failed: empty channel")
			c.JSON(200, map[string]interface{}{
				"code": 400,
				"msg":  "channel is required",
				"data": nil,
			})
			return
		}

		since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
		timeout, _ := strconv.ParseInt(c.Query("timeout"), 10, 64)
		log.Printf("new http subscription: channel:%s since:%d timeout:%d",
			channel, since, timeout)

		if timeout < 10000 || timeout > 120000 {
			// 保持 10-120s之间
			timeout = 60000
		}

		ctx, cancel := context.WithTimeout(c, time.Duration(timeout)*time.Millisecond)
		defer cancel()
		message := &BroadcastMessage{}
		key := b.messageCacheKey(channel)
		val, err := b.rds.Get(ctx, key).Result()
		if err == redis.Nil {
			goto listen
		}
		if err != nil {
			c.JSON(200, map[string]interface{}{
				"code": 500,
				"msg":  "cache error",
				"data": nil,
			})
			return
		}
		json.Unmarshal([]byte(val), message)
		if message.Timestamp >= since {
			c.JSON(200, map[string]interface{}{
				"code": 0,
				"msg":  "",
				"data": message,
			})
			return
		}
	listen:
		log.Printf("start listen channel:%s", channel)
		sub := b.subscribe(channel, nil)
		defer b.unsubscribe(channel, sub)

		select {
		case msg := <-sub.ch:
			log.Printf("http sub message delivered: channel:%s", channel)
			c.JSON(200, map[string]interface{}{
				"code": 0,
				"msg":  "",
				"data": msg,
			})
		case <-sub.done:
			// 频道被 Delete：与旧行为一致，返回空消息
			var msg *BroadcastMessage
			c.JSON(200, map[string]interface{}{
				"code": 0,
				"msg":  "",
				"data": msg,
			})
		case <-ctx.Done():
			log.Printf("http sub timeout: channel:%s duration:%dms",
				channel, timeout)
			c.JSON(200, map[string]interface{}{
				"code": 408,
				"msg":  "timeout",
				"data": map[string]interface{}{
					"timestamp": time.Now().UnixMilli(),
				},
			})
		}
	}
}

// Pub 发布消息到频道
func (b *Broadcast) Pub(ctx context.Context, channel string, payload interface{}) error {
	message := &BroadcastMessage{
		Channel:   channel,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
	data, _ := json.Marshal(message)
	err := b.rds.Publish(ctx, b.broadcastKey(), data).Err()
	if err != nil {
		log.Printf("pub to channel:%s with err:%v", channel, err)
	}
	return err
}

// Del 删除频道（别名）
func (b *Broadcast) Del(channel string) {
	b.Delete(channel)
}

// Run 运行广播服务（阻塞，永不返回）
func (b *Broadcast) Run() {
	_ = b.RunContext(context.Background())
}

// RunContext 运行广播服务，ctx 取消后返回 ctx.Err()
func (b *Broadcast) RunContext(ctx context.Context) error {
	pubsub := b.rds.Subscribe(ctx, b.broadcastKey())
	defer pubsub.Close()
	// ReceiveMessage 阻塞在网络读上时不响应 ctx 取消，靠关闭 pubsub 来打断
	stop := context.AfterFunc(ctx, func() { pubsub.Close() })
	defer stop()

	log.Printf("broadcast service started: key:%s", b.broadcastKey())

	for {
		msg, err := pubsub.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("broadcast service stopped: key:%s", b.broadcastKey())
				return ctx.Err()
			}
			b.metrics.messagesDropped.Add(1)
			log.Printf("receive message error: %v, total dropped: %d",
				err, b.metrics.messagesDropped.Load())
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		startTime := time.Now()
		message := &BroadcastMessage{}
		json.Unmarshal([]byte(msg.Payload), message)

		b.deliver(message)

		key := b.messageCacheKey(message.Channel)
		b.rds.SetNX(ctx, key, message, time.Duration(b.cacheSecondsForLated)*time.Second)

		latency := time.Since(startTime).Milliseconds()
		b.metrics.subscribeLatency.Store(latency)
		b.metrics.messagesSent.Add(1)
	}
}

// Delete 删除频道：结束该频道在本实例上的所有订阅
func (b *Broadcast) Delete(channel string) {
	b.mu.Lock()
	subscribers, ok := b.channels[channel]
	if ok {
		delete(b.channels, channel)
		b.metrics.activeChannels.Add(-1)
	}
	b.mu.Unlock()
	if !ok {
		return
	}

	subscribers.mu.Lock()
	subs := subscribers.subscribers
	subscribers.subscribers = make(map[*subscriber]struct{})
	subscribers.mu.Unlock()
	for sub := range subs {
		sub.close()
	}
}

// Load 加载频道订阅者
func (b *Broadcast) Load(channel string) (*ChannelSubscribers, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subscribers, ok := b.channels[channel]
	return subscribers, ok
}

// GetMetrics 获取广播服务指标
func (b *Broadcast) GetMetrics(c *gin.Context) {
	c.JSON(200,
		map[string]int64{
			"active_channels":   b.metrics.activeChannels.Load(),
			"messages_sent":     b.metrics.messagesSent.Load(),
			"messages_dropped":  b.metrics.messagesDropped.Load(),
			"subscribe_latency": b.metrics.subscribeLatency.Load(),
		})
}

// ResetMetrics 重置广播服务指标
func (b *Broadcast) ResetMetrics() {
	b.metrics.messagesSent.Store(0)
	b.metrics.messagesDropped.Store(0)
	b.metrics.subscribeLatency.Store(0)
	// 注意：不重置 activeChannels，因为这是实时状态
}
