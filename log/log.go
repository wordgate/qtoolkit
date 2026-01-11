package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logger   *logrus.Logger
	initOnce sync.Once
)

const CTX_REQUEST_ID = "X-Request-ID"

// initialize performs the actual logger initialization
func initialize() {
	level := viper.GetString("log.level")
	logPath := viper.GetString("log.path")
	isDev := viper.GetBool("is_dev")

	logger = logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05.000",
	})
	logger.SetLevel(parseLogLevel(level))
	logger.SetOutput(createLogWriter(logPath, level))

	// Set gin default writers
	gin.DefaultWriter = logger.Writer()
	if isDev {
		gin.DefaultErrorWriter = logger.WriterLevel(logrus.ErrorLevel)
	} else {
		gin.DefaultErrorWriter = io.Discard
	}
}

// getLogger returns the logger with lazy initialization
func getLogger() *logrus.Logger {
	initOnce.Do(initialize)
	return logger
}

// createLogWriter creates the appropriate io.Writer based on config
func createLogWriter(logPath, level string) io.Writer {
	if logPath == "" {
		return os.Stdout
	}

	// Ensure parent directory exists
	dir := path.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("log: failed to create directory '%s': %v, falling back to stdout\n", dir, err)
		return os.Stdout
	}

	writer := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    500,
		MaxBackups: 3,
		MaxAge:     28,
	}

	if parseLogLevel(level) == logrus.DebugLevel {
		return io.MultiWriter(writer, os.Stdout)
	}
	return writer
}

// parseLogLevel parses log level string to logrus.Level
func parseLogLevel(levelS string) logrus.Level {
	level, err := logrus.ParseLevel(levelS)
	if err != nil {
		level = logrus.InfoLevel
	}
	return level
}

// processLog creates log entry with request ID
func processLog(ctx context.Context) *logrus.Entry {
	return getLogger().WithField("reqId", RequestId(ctx))
}

// RequestId returns the request identifier from context
func RequestId(ctx context.Context) string {
	if ctx == nil {
		return "background"
	}

	var ginCtx *gin.Context
	switch v := ctx.(type) {
	case *gin.Context:
		ginCtx = v
	case interface{ GetContext() *gin.Context }:
		ginCtx = v.GetContext()
	}

	if ginCtx != nil {
		if id := ginCtx.GetString(CTX_REQUEST_ID); id != "" {
			return id
		}
		id := xid.New().String()
		ginCtx.Set(CTX_REQUEST_ID, id)
		return id
	}
	return "background"
}

// WithFields creates a log entry with custom fields
func WithFields(ctx context.Context, fields logrus.Fields) *logrus.Entry {
	return getLogger().WithFields(fields).WithField("reqId", RequestId(ctx))
}

// --- Log methods ---

func Tracef(ctx context.Context, format string, args ...any) {
	processLog(ctx).Tracef(format, args...)
}

func Trace(ctx context.Context, args ...any) {
	processLog(ctx).Trace(args...)
}

func Debugf(ctx context.Context, format string, args ...any) {
	processLog(ctx).Debugf(format, args...)
}

func Debug(ctx context.Context, args ...any) {
	processLog(ctx).Debug(args...)
}

func Infof(ctx context.Context, format string, args ...any) {
	processLog(ctx).Infof(format, args...)
}

func Info(ctx context.Context, args ...any) {
	processLog(ctx).Info(args...)
}

func Warnf(ctx context.Context, format string, args ...any) {
	processLog(ctx).Warnf(format, args...)
}

func Warn(ctx context.Context, args ...any) {
	processLog(ctx).Warn(args...)
}

func Errorf(ctx context.Context, format string, args ...any) {
	processLog(ctx).Errorf(format, args...)
}

func Error(ctx context.Context, args ...any) {
	processLog(ctx).Error(args...)
}

func Fatalf(ctx context.Context, format string, args ...any) {
	processLog(ctx).Fatalf(format, args...)
}

func Fatal(ctx context.Context, args ...any) {
	processLog(ctx).Fatal(args...)
}

func Panicf(ctx context.Context, format string, args ...any) {
	processLog(ctx).Panicf(format, args...)
}

func Panic(ctx context.Context, args ...any) {
	processLog(ctx).Panic(args...)
}
