package asynq

import (
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// 这一组测试锁住「Task defaults 真的接上线了」。
//
// 背景:Config 早就声明了 DefaultMaxRetry / DefaultTimeout,loadConfig 也给了
// 默认值,但它们从没被传进 asynq.NewTask——实际生效的一直是 hibiken/asynq 的硬
// 默认(MaxRetry=25、Timeout=30m)。下游服务在 config.yml 里写 default_max_retry: 3
// 却在重试 25 次,一条必然失败的任务被重放 25 遍并刷了 25 条告警,直到有人去解码
// Redis 里的任务 protobuf 才发现。
//
// 这类「配置项声明了但没接线」的失效是完全静默的:编译通过、配置读得到、字段有值,
// 只有行为不对。所以这里必须验行为,不能只验字段存在。

// setTestConfig 直接改写包级配置。loadConfig 用 sync.Once 守卫,不 Do 一次的话
// 后续 loadConfig 会拿 viper 里的值把这里设的覆盖掉。
func setTestConfig(t *testing.T, cfg *Config) {
	t.Helper()
	prev := globalConfig
	globalConfig = cfg
	configOnce.Do(func() {})
	t.Cleanup(func() { globalConfig = prev })
}

// findOption 返回 opts 里最后一个匹配该类型的选项值。取「最后一个」是因为
// asynq 的 composeOptions 顺序赋值、后者覆盖前者,最后一个才是真正生效的那个。
func findOption(opts []asynq.Option, typ asynq.OptionType) (any, bool) {
	var val any
	var found bool
	for _, o := range opts {
		if o.Type() == typ {
			val = o.Value()
			found = true
		}
	}
	return val, found
}

func TestWithTaskDefaults_AppliesConfiguredValues(t *testing.T) {
	setTestConfig(t, &Config{DefaultMaxRetry: 3, DefaultTimeout: 12 * time.Minute})

	opts := withTaskDefaults(nil)

	retry, ok := findOption(opts, asynq.MaxRetryOpt)
	if !ok {
		t.Fatal("MaxRetry 没有下发——default_max_retry 又变回死配置了")
	}
	if retry != 3 {
		t.Errorf("MaxRetry = %v, 期望 3", retry)
	}

	timeout, ok := findOption(opts, asynq.TimeoutOpt)
	if !ok {
		t.Fatal("Timeout 没有下发——default_timeout 又变回死配置了")
	}
	if timeout != 12*time.Minute {
		t.Errorf("Timeout = %v, 期望 12m", timeout)
	}
}

// 默认值必须排在调用方的 opts 前面,这样调用方显式传的值才能覆盖它。
func TestWithTaskDefaults_CallerOptionsWin(t *testing.T) {
	setTestConfig(t, &Config{DefaultMaxRetry: 3, DefaultTimeout: 12 * time.Minute})

	opts := withTaskDefaults([]asynq.Option{
		asynq.MaxRetry(7),
		asynq.Timeout(90 * time.Second),
	})

	if retry, _ := findOption(opts, asynq.MaxRetryOpt); retry != 7 {
		t.Errorf("生效的 MaxRetry = %v, 期望调用方的 7 覆盖默认的 3", retry)
	}
	if timeout, _ := findOption(opts, asynq.TimeoutOpt); timeout != 90*time.Second {
		t.Errorf("生效的 Timeout = %v, 期望调用方的 90s 覆盖默认的 12m", timeout)
	}
}

// MaxRetry 的 0 是合法值(明确表示不重试),不能被当成「没配置」而跳过——
// 跳过的后果是静默回退到 hibiken 的硬默认 25,与用户意图正好相反。
func TestWithTaskDefaults_ZeroMaxRetryIsHonored(t *testing.T) {
	setTestConfig(t, &Config{DefaultMaxRetry: 0, DefaultTimeout: 12 * time.Minute})

	retry, ok := findOption(withTaskDefaults(nil), asynq.MaxRetryOpt)
	if !ok {
		t.Fatal("default_max_retry: 0 被吞掉了,任务会退回 hibiken 硬默认的 25 次重试")
	}
	if retry != 0 {
		t.Errorf("MaxRetry = %v, 期望 0", retry)
	}
}

// Timeout 的 0 在 asynq 里是「未设置」哨兵,传它与不传等价,所以不该下发。
func TestWithTaskDefaults_ZeroTimeoutNotEmitted(t *testing.T) {
	setTestConfig(t, &Config{DefaultMaxRetry: 3, DefaultTimeout: 0})

	if _, ok := findOption(withTaskDefaults(nil), asynq.TimeoutOpt); ok {
		t.Error("DefaultTimeout=0 不该下发 Timeout 选项(0 是 asynq 的未设置哨兵)")
	}
}

// 端到端:真的入队一次,把 asynq 记下来的 MaxRetry/Timeout 读回来比对。
//
// 上面几个测试只证明了 opts 列表组装正确;这一个才证明它确实穿过 NewTask 落到了
// 任务上——原来的 bug 恰恰是「配置读到了、字段有值,但没传下去」,只验列表是抓不住
// 同类回归的。需要 Redis,CI 里没有,连不上就跳过。
func TestEnqueue_PersistsConfiguredDefaults(t *testing.T) {
	const (
		wantRetry   = 2
		wantTimeout = 7 * time.Minute
	)

	setTestConfig(t, &Config{
		RedisAddr:       "127.0.0.1:6379",
		RedisPassword:   "dev",
		RedisDB:         15,
		Concurrency:     10,
		Queues:          map[string]int{"default": 1},
		DefaultMaxRetry: wantRetry,
		DefaultTimeout:  wantTimeout,
	})

	redisOpt := getRedisOpt()
	inspector := asynq.NewInspector(redisOpt)
	defer inspector.Close()
	if _, err := inspector.Queues(); err != nil {
		t.Skipf("本地 Redis 不可用,跳过端到端校验: %v", err)
	}

	info, err := Enqueue("test:defaults", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("Enqueue 失败: %v", err)
	}
	t.Cleanup(func() { _ = inspector.DeleteTask(info.Queue, info.ID) })

	if info.MaxRetry != wantRetry {
		t.Errorf("落库的 MaxRetry = %d, 期望 %d(拿到 25 就说明默认值又没传下去)",
			info.MaxRetry, wantRetry)
	}
	if info.Timeout != wantTimeout {
		t.Errorf("落库的 Timeout = %v, 期望 %v", info.Timeout, wantTimeout)
	}
}
