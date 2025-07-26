package qtoolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wordgate/qtoolkit/log"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

type BroadcastMessage struct {
	Channel   string      `json:"channel"`
	Timestamp int64       `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

type ChannelSubscribers struct {
	subscribers sync.Map // chan *BroadcastMessage -> bool
}

func (c *ChannelSubscribers) count() int64 {
	var count int64
	c.subscribers.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func (c *ChannelSubscribers) isEmpty() bool {
	return c.count() == 0
}

type Broadcast struct {
	channels             sync.Map // string -> *ChannelSubscribers
	app                  string
	rds                  *redis.Client
	cacheSecondsForLated int64
	metrics              struct {
		activeChannels   atomic.Int64 // 活跃channel数
		messagesSent     atomic.Int64 // 发送消息数
		messagesDropped  atomic.Int64 // 丢弃消息数
		subscribeLatency atomic.Int64 // 订阅延迟(毫秒)
	}
}

func NewBroadcast(app string, cacheSecondsForLated int64) *Broadcast {
	if cacheSecondsForLated <= 0 {
		cacheSecondsForLated = 10
	}
	return &Broadcast{
		app:                  app,
		rds:                  Redis(app),
		cacheSecondsForLated: cacheSecondsForLated, // 在錯誤消息的訂閱者，在redis中存留多久消息
	}
}

func LoadBroadcast(app string) *Broadcast {
	cacheSecondsForLated := viper.GetInt64(fmt.Sprintf("%s.broadcast.cacheSecondsForLated", app))
	return NewBroadcast(app, cacheSecondsForLated)
}

func (b *Broadcast) broadcastKey() string {
	return fmt.Sprintf("broadcast/%s", b.app)
}

func (b *Broadcast) messageCacheKey(channel string) string {
	return fmt.Sprintf("broadcast/%s/%s", b.app, channel)
}

func (b *Broadcast) unsubscribe(channel string, ch chan *BroadcastMessage, subscribers *ChannelSubscribers) {
	if _, exists := subscribers.subscribers.LoadAndDelete(ch); exists {
		close(ch)
	}
	b.cleanEmptyChannel(channel, subscribers)
}

func (b *Broadcast) WsSubChannel(c *gin.Context, channel string) error {
	log.Infof(c, "new websocket connection for channel: %s", channel)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Warnf(c, "websocket upgrade failed: %v", err)
		return err
	}
	defer ws.Close()

	// 创建消息通道
	ch := make(chan *BroadcastMessage)
	subscribers := b.getOrCreateChannelSubscribers(channel)
	subscribers.subscribers.Store(ch, true)

	// 清理工作
	defer func() {
		b.unsubscribe(channel, ch, subscribers)
	}()

	// 用于协调goroutine退出
	done := make(chan struct{})
	defer close(done)

	// 心跳检测
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Warnf(c, "websocket ping failed: %v", err)
					return
				}
				log.Tracef(c, "websocket ping sent: channel:%s", channel)
			case <-done:
				log.Tracef(c, "websocket ping goroutine exiting: channel:%s", channel)
				return
			case <-c.Done():
				log.Infof(c, "websocket connection closed: channel:%s", channel)
				return
			}
		}
	}()

	// 处理接收到的消息
	for {
		select {
		case msg := <-ch:
			data, err := json.Marshal(msg)
			if err != nil {
				log.Warnf(c, "marshal message failed: %v", err)
				continue
			}

			if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Warnf(c, "write message failed: %v", err)
				return err
			}

			log.Tracef(c, "websocket message sent to channel: %s", channel)
		case <-c.Done():
			return nil
		}
	}
}

func (b *Broadcast) WsSub(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		channel := c.Param(paramName)
		b.WsSubChannel(c, channel)
	}
}

// since 毫秒時間戳
// timeout 客戶端請求時設置的超時時間，單位為毫秒
func (b *Broadcast) HttpSub(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		channel := c.Param(paramName)
		if channel == "" {
			log.Warnf(c, "http sub failed: empty channel")
			c.JSON(200, map[string]interface{}{
				"code": 400,
				"msg":  "channel is required",
				"data": nil,
			})
			return
		}

		since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
		timeout, _ := strconv.ParseInt(c.Query("timeout"), 10, 64)
		log.Infof(c, "new http subscription: channel:%s since:%d timeout:%d",
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
		log.Tracef(ctx, "start listen channel:%s", channel)
		ch := make(chan *BroadcastMessage)
		subscribers := b.getOrCreateChannelSubscribers(channel)
		subscribers.subscribers.Store(ch, true)

		defer func() {
			b.unsubscribe(channel, ch, subscribers)
		}()

		select {
		case msg := <-ch:
			log.Infof(c, "http sub message delivered: channel:%s message:%+v", channel, msg)
			c.JSON(200, map[string]interface{}{
				"code": 0,
				"msg":  "",
				"data": msg,
			})
		case <-ctx.Done():
			log.Infof(c, "http sub timeout: channel:%s duration:%dms",
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

func (b *Broadcast) Pub(ctx context.Context, channel string, payload interface{}) error {
	message := &BroadcastMessage{
		Channel:   channel,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
	data, _ := json.Marshal(message)
	err := b.rds.Publish(ctx, b.broadcastKey(), data).Err()
	if err != nil {
		log.Warnf(ctx, "pub to app:%s channel:%s with err:%v", b.app, channel, err)
	}
	return err
}

func (b *Broadcast) Del(channel string) {
	b.Delete(channel)
}

func (b *Broadcast) Run() {
	ctx := context.Background()
	pubsub := b.rds.Subscribe(ctx, b.broadcastKey())
	defer pubsub.Close()

	log.Infof(ctx, "broadcast service started for app: %s", b.app)

	for {
		msg, err := pubsub.ReceiveMessage(ctx)
		if err != nil {
			b.metrics.messagesDropped.Add(1)
			log.Warnf(ctx, "receive message error: %v, total dropped: %d",
				err, b.metrics.messagesDropped.Load())
			time.Sleep(time.Second)
			continue
		}

		startTime := time.Now()
		message := &BroadcastMessage{}
		json.Unmarshal([]byte(msg.Payload), message)
		log.Tracef(ctx, "broadcast:get message from redis, app:%s message:%s", b.app, msg)

		chs, ok := b.Load(message.Channel)
		if ok {
			log.Tracef(ctx, "broadcast:find subscribers, app:%s channel:%s subscribers count:%d",
				b.app, message.Channel, chs.count())
			chs.subscribers.Range(func(key, _ interface{}) bool {
				ch := key.(chan *BroadcastMessage)
				ch <- message
				log.Tracef(ctx, "broadcast:send to one subscriber done, app:%s channel:%s",
					b.app, message.Channel)
				return true
			})
		} else {
			log.Tracef(ctx, "broadcast:no subscribers for channel:%s", message.Channel)
		}
		log.Tracef(ctx, "broadcast:cache a backup to redis, app:%s message:%s", b.app, msg)
		key := b.messageCacheKey(message.Channel)
		b.rds.SetNX(ctx, key, message, time.Duration(b.cacheSecondsForLated)*time.Second)

		latency := time.Since(startTime).Milliseconds()
		b.metrics.subscribeLatency.Store(latency)
		b.metrics.messagesSent.Add(1)

		log.Tracef(ctx, "broadcast:message processed, app:%s channel:%s latency:%dms sent:%d",
			b.app, message.Channel, latency, b.metrics.messagesSent.Load())
	}
}

func (b *Broadcast) getOrCreateChannelSubscribers(channel string) *ChannelSubscribers {
	value, loaded := b.channels.LoadOrStore(channel, &ChannelSubscribers{})
	if !loaded {
		b.metrics.activeChannels.Add(1)
		log.Infof(context.Background(), "new channel created: %s, active channels: %d",
			channel, b.metrics.activeChannels.Load())
	}
	return value.(*ChannelSubscribers)
}

func (b *Broadcast) cleanEmptyChannel(channel string, subscribers *ChannelSubscribers) {
	if subscribers.isEmpty() {
		b.channels.Delete(channel)
		b.metrics.activeChannels.Add(-1)
		log.Infof(context.Background(), "channel cleaned: %s, remaining active channels: %d",
			channel, b.metrics.activeChannels.Load())
	}
}

func (b *Broadcast) Delete(channel string) {
	if value, ok := b.channels.Load(channel); ok {
		subscribers := value.(*ChannelSubscribers)
		subscribers.subscribers.Range(func(ch, _ interface{}) bool {
			subscribers.subscribers.Delete(ch)
			close(ch.(chan *BroadcastMessage))
			return true
		})
		b.channels.Delete(channel)
	}
}

func (b *Broadcast) Load(channel string) (*ChannelSubscribers, bool) {
	value, ok := b.channels.Load(channel)
	if !ok {
		return nil, false
	}
	return value.(*ChannelSubscribers), ok
}

func (b *Broadcast) GetMetrics(c *gin.Context) {
	c.JSON(200,
		map[string]int64{
			"active_channels":   b.metrics.activeChannels.Load(),
			"messages_sent":     b.metrics.messagesSent.Load(),
			"messages_dropped":  b.metrics.messagesDropped.Load(),
			"subscribe_latency": b.metrics.subscribeLatency.Load(),
		})
}

func (b *Broadcast) ResetMetrics() {
	b.metrics.messagesSent.Store(0)
	b.metrics.messagesDropped.Store(0)
	b.metrics.subscribeLatency.Store(0)
	// 注意：不重置 activeChannels，因为这是实时状态
}
