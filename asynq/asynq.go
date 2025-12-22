// Package asynq provides a simple async task queue built on top of hibiken/asynq.
// It supports automatic worker lifecycle management, graceful shutdown, and
// configuration-driven setup via viper.
//
// Usage:
//
//	// Register handlers
//	asynq.Handle("email:send", handleEmailSend)
//
//	// Register cron tasks (optional)
//	asynq.Cron("@daily", "report:daily", nil)
//
//	// Enqueue tasks
//	asynq.Enqueue("email:send", payload)
//
//	// Mount monitoring UI (auto-starts worker)
//	asynq.Mount(r, "/asynq")
package asynq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/spf13/viper"
)

// HandlerFunc is the function signature for task handlers.
type HandlerFunc func(ctx context.Context, payload []byte) error

// TaskInfo contains information about an enqueued task.
type TaskInfo = asynq.TaskInfo

// Option is an alias for asynq.Option for task enqueue options.
type Option = asynq.Option

// Re-export commonly used options for convenience
var (
	Queue      = asynq.Queue
	MaxRetry   = asynq.MaxRetry
	Timeout    = asynq.Timeout
	Deadline   = asynq.Deadline
	Unique     = asynq.Unique
	ProcessIn  = asynq.ProcessIn
	ProcessAt  = asynq.ProcessAt
	Retention  = asynq.Retention
	TaskID     = asynq.TaskID
)

// Config holds the asynq configuration.
type Config struct {
	// Redis connection (falls back to redis.* if not set)
	RedisAddr     string `mapstructure:"redis_addr"`
	RedisPassword string `mapstructure:"redis_password"`
	RedisDB       int    `mapstructure:"redis_db"`

	// Worker configuration
	Concurrency    int            `mapstructure:"concurrency"`
	Queues         map[string]int `mapstructure:"queues"`
	StrictPriority bool           `mapstructure:"strict_priority"`

	// Task defaults
	DefaultMaxRetry int           `mapstructure:"default_max_retry"`
	DefaultTimeout  time.Duration `mapstructure:"default_timeout"`

	// Monitor configuration
	Monitor MonitorConfig `mapstructure:"monitor"`
}

// MonitorConfig holds the asynqmon UI configuration.
type MonitorConfig struct {
	ReadOnly bool `mapstructure:"readonly"`
}

var (
	globalConfig *Config
	configOnce   sync.Once

	client     *asynq.Client
	clientOnce sync.Once

	server       *asynq.Server
	mux          *asynq.ServeMux
	serverOnce   sync.Once
	serverMux    sync.Mutex
	workerOnce   sync.Once
	workerActive bool

	scheduler     *asynq.Scheduler
	schedulerOnce sync.Once
	cronTasks     []cronTask

	handlers    = make(map[string]HandlerFunc)
	handlersMux sync.RWMutex

	shutdownOnce sync.Once
)

type cronTask struct {
	cronspec string
	taskType string
	payload  any
	opts     []asynq.Option
}

// loadConfig loads configuration from viper with cascading fallback.
// Priority: asynq.* -> redis.* (for connection settings)
// FATAL: Crashes if redis.addr is not configured.
func loadConfig() *Config {
	configOnce.Do(func() {
		globalConfig = &Config{
			Concurrency:     10,
			Queues:          map[string]int{"default": 1},
			DefaultMaxRetry: 3,
			DefaultTimeout:  30 * time.Minute,
		}

		// Load asynq specific config
		if err := viper.UnmarshalKey("asynq", globalConfig); err != nil {
			// Use defaults on error
		}

		// Fallback to redis.* for connection settings
		if globalConfig.RedisAddr == "" {
			globalConfig.RedisAddr = viper.GetString("redis.addr")
		}
		if globalConfig.RedisPassword == "" {
			globalConfig.RedisPassword = viper.GetString("redis.password")
		}
		if globalConfig.RedisDB == 0 && viper.IsSet("redis.db") {
			globalConfig.RedisDB = viper.GetInt("redis.db")
		}

		// FATAL: Redis address is required
		if globalConfig.RedisAddr == "" {
			log.Fatal("asynq: redis.addr is required but not configured")
		}

		// Ensure defaults
		if globalConfig.Concurrency <= 0 {
			globalConfig.Concurrency = 10
		}
		if len(globalConfig.Queues) == 0 {
			globalConfig.Queues = map[string]int{"default": 1}
		}
	})
	return globalConfig
}

// GetConfig returns the current configuration.
func GetConfig() *Config {
	return loadConfig()
}

// getRedisOpt returns the asynq redis connection option.
func getRedisOpt() asynq.RedisClientOpt {
	cfg := loadConfig()
	return asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
}

// getClient returns the singleton asynq client (lazy init).
func getClient() *asynq.Client {
	clientOnce.Do(func() {
		client = asynq.NewClient(getRedisOpt())
	})
	return client
}

// initServer initializes the server and mux (lazy init).
func initServer() {
	serverOnce.Do(func() {
		cfg := loadConfig()

		serverCfg := asynq.Config{
			Concurrency:    cfg.Concurrency,
			Queues:         cfg.Queues,
			StrictPriority: cfg.StrictPriority,
		}

		server = asynq.NewServer(getRedisOpt(), serverCfg)
		mux = asynq.NewServeMux()
	})
}

// ensureWorkerStarted starts the worker server if handlers are registered.
// Called automatically on first MonitorHandler() or Enqueue() call.
// This is idempotent and safe to call multiple times.
func ensureWorkerStarted() {
	workerOnce.Do(func() {
		handlersMux.RLock()
		hasHandlers := len(handlers) > 0
		handlersMux.RUnlock()

		if !hasHandlers {
			return
		}

		initServer()

		// Register all handlers to mux
		handlersMux.RLock()
		for taskType, handler := range handlers {
			h := handler // capture
			mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
				return h(ctx, t.Payload())
			})
		}
		handlersMux.RUnlock()

		// Start server in goroutine
		go func() {
			if err := server.Run(mux); err != nil {
				fmt.Fprintf(os.Stderr, "asynq: server error: %v\n", err)
			}
		}()

		workerActive = true

		// Start scheduler if cron tasks are registered
		startScheduler()

		// Register graceful shutdown
		registerShutdown()
	})
}

// registerShutdown registers signal handlers for graceful shutdown.
func registerShutdown() {
	shutdownOnce.Do(func() {
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			// Graceful shutdown
			Shutdown()
		}()
	})
}

// Handle registers a handler for the given task type.
// Worker is automatically started when MonitorHandler() is called or
// on first Enqueue() if handlers are registered.
func Handle(taskType string, handler HandlerFunc) {
	handlersMux.Lock()
	handlers[taskType] = handler
	handlersMux.Unlock()
}

// Cron registers a periodic task with cron expression.
// The task will be enqueued according to the cron schedule.
//
// Cron expressions:
//
//	"*/5 * * * *"     - Every 5 minutes
//	"0 * * * *"       - Every hour
//	"0 9 * * *"       - Every day at 9:00 AM
//	"0 9 * * 1"       - Every Monday at 9:00 AM
//	"@every 1h"       - Every hour
//	"@every 30m"      - Every 30 minutes
//	"@daily"          - Every day at midnight
//
// Example:
//
//	asynq.Cron("@every 5m", "metrics:collect", nil)
//	asynq.Cron("0 9 * * *", "report:daily", ReportPayload{Type: "daily"})
func Cron(cronspec string, taskType string, payload any, opts ...Option) {
	cronTasks = append(cronTasks, cronTask{
		cronspec: cronspec,
		taskType: taskType,
		payload:  payload,
		opts:     opts,
	})
}

// startScheduler starts the cron scheduler if cron tasks are registered.
func startScheduler() {
	schedulerOnce.Do(func() {
		if len(cronTasks) == 0 {
			return
		}

		loc, _ := time.LoadLocation("Local")
		scheduler = asynq.NewScheduler(getRedisOpt(), &asynq.SchedulerOpts{
			Location: loc,
		})

		for _, ct := range cronTasks {
			data, _ := marshal(ct.payload)
			task := asynq.NewTask(ct.taskType, data, ct.opts...)
			scheduler.Register(ct.cronspec, task)
		}

		go func() {
			if err := scheduler.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "asynq: scheduler error: %v\n", err)
			}
		}()
	})
}

// Run starts the worker server and blocks until shutdown signal is received.
// Use this for dedicated worker processes that don't serve HTTP.
func Run() error {
	handlersMux.RLock()
	hasHandlers := len(handlers) > 0
	handlersMux.RUnlock()

	if !hasHandlers {
		return fmt.Errorf("asynq: no handlers registered")
	}

	initServer()

	// Register all handlers
	handlersMux.RLock()
	for taskType, handler := range handlers {
		h := handler // capture
		mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
			return h(ctx, t.Payload())
		})
	}
	handlersMux.RUnlock()

	// Start scheduler for cron tasks
	startScheduler()

	// Register graceful shutdown
	registerShutdown()

	// Run blocks until shutdown
	return server.Run(mux)
}

// Enqueue enqueues a task for immediate processing.
// Automatically starts the worker if handlers are registered.
func Enqueue(taskType string, payload any, opts ...Option) (*TaskInfo, error) {
	// Auto-start worker on first enqueue
	ensureWorkerStarted()

	data, err := marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("asynq: failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(taskType, data, opts...)
	return getClient().Enqueue(task)
}

// EnqueueContext enqueues a task with context for immediate processing.
// Automatically starts the worker if handlers are registered.
func EnqueueContext(ctx context.Context, taskType string, payload any, opts ...Option) (*TaskInfo, error) {
	// Auto-start worker on first enqueue
	ensureWorkerStarted()

	data, err := marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("asynq: failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(taskType, data, opts...)
	return getClient().EnqueueContext(ctx, task)
}

// EnqueueIn enqueues a task to be processed after the given delay.
func EnqueueIn(taskType string, payload any, delay time.Duration, opts ...Option) (*TaskInfo, error) {
	opts = append(opts, ProcessIn(delay))
	return Enqueue(taskType, payload, opts...)
}

// EnqueueAt enqueues a task to be processed at the given time.
func EnqueueAt(taskType string, payload any, at time.Time, opts ...Option) (*TaskInfo, error) {
	opts = append(opts, ProcessAt(at))
	return Enqueue(taskType, payload, opts...)
}

// EnqueueUnique enqueues a unique task (deduplication within TTL).
func EnqueueUnique(taskType string, payload any, ttl time.Duration, opts ...Option) (*TaskInfo, error) {
	opts = append(opts, Unique(ttl))
	return Enqueue(taskType, payload, opts...)
}

// Shutdown gracefully shuts down the worker, scheduler and client.
func Shutdown() {
	serverMux.Lock()
	defer serverMux.Unlock()

	if scheduler != nil {
		scheduler.Shutdown()
	}
	if server != nil {
		server.Shutdown()
	}
	if client != nil {
		client.Close()
	}
}

// marshal converts payload to JSON bytes.
func marshal(payload any) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	if b, ok := payload.([]byte); ok {
		return b, nil
	}
	return json.Marshal(payload)
}

// Unmarshal is a helper to unmarshal task payload in handlers.
func Unmarshal(payload []byte, v any) error {
	return json.Unmarshal(payload, v)
}

// GetTaskID extracts a task ID from a context, if any.
// The ID of a task is guaranteed to be unique.
// The ID of a task doesn't change if the task is being retried.
func GetTaskID(ctx context.Context) string {
	id, ok := asynq.GetTaskID(ctx)
	if !ok {
		return ""
	}
	return id
}
