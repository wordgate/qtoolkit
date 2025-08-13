package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/gin-gonic/gin"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *logrus.Logger

const (
	CTX_REQUEST_ID = "X-Request-ID"
	CTX_APP_TYPE   = "X-App-Type"
)

var preprocessors []func(ctx context.Context, e *logrus.Entry) *logrus.Entry

// AddPreprocessor 添加日志预处理器
func AddPreprocessor(p func(ctx context.Context, e *logrus.Entry) *logrus.Entry) {
	preprocessors = append(preprocessors, p)
}

// processLog 处理日志条目
func processLog(ctx context.Context) *logrus.Entry {
	entry := logger.WithFields(RequestIdFields(ctx))
	return entry
}

func CtxWithRequestId(ctx context.Context, reqId string) context.Context {
	return context.WithValue(ctx, CTX_REQUEST_ID, reqId)
}

func CtxWithAppType(ctx context.Context, appType string) context.Context {
	return context.WithValue(ctx, CTX_APP_TYPE, appType)
}

// Get returns the request identifier
func RequestId(ctx context.Context) string {
	id := "background"
	if ctx == nil {
		return id
	}

	var ginCtx *gin.Context
	switch v := ctx.(type) {
	case *gin.Context:
		ginCtx = v
	case interface{ GetContext() *gin.Context }:
		ginCtx = v.GetContext()
	}

	if ginCtx != nil {
		id = ginCtx.GetString(CTX_REQUEST_ID)
		if id != "" {
			return id
		}
		id = xid.New().String()
		ginCtx.Set(CTX_REQUEST_ID, id)
	}
	return id
}

func init() {
	// 在包初始化时创建一个 logrus.Logger 实例
	logger = logrus.StandardLogger()
}

func InitLogger(topic string) {
	logger = LogWithFile(topic)
	gin.DefaultWriter = logger.Writer()

	// 在非开发环境下，禁用Gin的错误日志输出
	if !viper.GetBool("is_dev") {
		gin.DefaultErrorWriter = io.Discard
	} else {
		gin.DefaultErrorWriter = logger.WriterLevel(logrus.ErrorLevel)
	}

	// 如果启用了CloudWatch日志，添加钩子
	if viper.GetBool("log.watchlog.enabled") {

		// 添加CloudWatch日志钩子
		if err := SetupCloudWatchLogging(logger, topic); err != nil {
			fmt.Printf("初始化CloudWatch日志钩子失败: %v\n", err)
		} else {
			fmt.Printf("CloudWatch日志钩子已启用\n")
		}
	}
}

func LogWithFile(topic string) *logrus.Logger {
	logI := logrus.New()
	logI.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	logI.SetLevel(LogLevel())
	logI.SetOutput(Logger(topic))

	return logI
}

func WithFields(ctx context.Context, fields logrus.Fields) *logrus.Entry {
	return logger.WithFields(fields).WithFields(RequestIdFields(ctx))
}

func Logger(topic string) io.Writer {
	var logger io.Writer
	log_path := viper.GetString("log.path")
	//fmt.Printf("prepare log for topic:%s path:%s\n", topic, log_path)
	if stat, err := os.Stat(log_path); err != nil || !stat.IsDir() {
		if log_path != "" {
			// log.path 设置但无效，打印警告
			fmt.Printf("log path: '%s' is not a valid directory, falling back to stdout\n", log_path)
		}
		return os.Stdout
	}
	logger = &lumberjack.Logger{
		// 日志输出文件路径
		Filename:   path.Join(log_path, topic+".log"),
		MaxSize:    500,
		MaxBackups: 3,
		MaxAge:     28, //days
	}

	if LogLevel() == logrus.DebugLevel {
		logger = io.MultiWriter(logger, os.Stdout)
	}
	return logger
}

func LogLevel() logrus.Level {
	levelS := viper.GetString("log.level")
	level, err := logrus.ParseLevel(levelS)
	if err != nil {
		level = logrus.InfoLevel
	}
	return level
}

func GetAppType(ctx context.Context) string {
	if c, ok := ctx.(*gin.Context); ok {
		if c == nil {
			return ""
		}
		if appType := c.GetString(CTX_APP_TYPE); appType != "" {
			return appType
		}
	}
	if appType, ok := ctx.Value(CTX_APP_TYPE).(string); ok && appType != "" {
		return appType
	}
	return ""
}

func RequestIdFields(ctx context.Context) logrus.Fields {
	if ctx == nil {
		ctx = context.Background()
	}
	// 实现从 context 中获取相关字段的逻辑
	reqId := RequestId(ctx)
	appType := GetAppType(ctx)
	if appType == "" {
		return logrus.Fields{
			"reqId": reqId,
		}
	}
	return logrus.Fields{
		"reqId":    reqId,
		"app_type": appType,
	}
}

func Tracef(ctx context.Context, format string, args ...interface{}) {
	processLog(ctx).Tracef(format, args...)
}

func Trace(ctx context.Context, args ...interface{}) {
	processLog(ctx).Trace(args...)
}

func Debugf(ctx context.Context, format string, args ...interface{}) {
	processLog(ctx).Debugf(format, args...)
}

func Debug(ctx context.Context, args ...interface{}) {
	processLog(ctx).Debug(args...)
}

func Infof(ctx context.Context, format string, args ...interface{}) {
	processLog(ctx).Infof(format, args...)
}

func Info(ctx context.Context, args ...interface{}) {
	processLog(ctx).Info(args...)
}

func Warnf(ctx context.Context, format string, args ...interface{}) {
	processLog(ctx).Warnf(format, args...)
}

func Warn(ctx context.Context, args ...interface{}) {
	processLog(ctx).Warn(args...)
}

func Errorf(ctx context.Context, format string, args ...interface{}) {
	processLog(ctx).Errorf(format, args...)
}

func Error(ctx context.Context, args ...interface{}) {
	processLog(ctx).Error(args...)
}

func Fatalf(ctx context.Context, format string, args ...interface{}) {
	processLog(ctx).Fatalf(format, args...)
	logger.Exit(1)
}

func Fatal(ctx context.Context, args ...interface{}) {
	processLog(ctx).Fatal(args...)
	logger.Exit(1)
}

func Panicf(ctx context.Context, format string, args ...interface{}) {
	processLog(ctx).Panicf(format, args...)
}

func Panic(ctx context.Context, args ...interface{}) {
	processLog(ctx).Panic(args...)
}

// MiddlewareAppType 创建一个设置 app_type 的中间件
func MiddlewareAppType(appType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(CTX_APP_TYPE, appType)
		c.Next()
	}
}
