// CloudWatch日志实现
package log

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// CloudWatchLogsHook 实现了 logrus.Hook 接口，将日志发送到 AWS CloudWatch Logs
type CloudWatchLogsHook struct {
	svc           *cloudwatchlogs.CloudWatchLogs
	logGroupName  string
	logStreamName string
	sequenceToken *string
	buffer        []*cloudwatchlogs.InputLogEvent
	bufferSize    int
	flushInterval time.Duration
	async         bool
	mutex         sync.Mutex
	timer         *time.Timer
}

// SetupCloudWatchLogging 设置CloudWatch日志
func SetupCloudWatchLogging(logger *logrus.Logger, topic string) error {
	hook, err := NewCloudWatchLogsHook(topic)
	if err != nil {
		return err
	}

	logger.AddHook(hook)
	return nil
}

// NewCloudWatchLogsHook 创建一个新的CloudWatch日志钩子
func NewCloudWatchLogsHook(topic string) (*CloudWatchLogsHook, error) {
	// 检查是否启用CloudWatch日志
	if !viper.GetBool("log.watchlog.enabled") {
		return nil, fmt.Errorf("CloudWatch Logs 未启用")
	}

	// 获取AWS配置
	region := viper.GetString("aws.cloudwatch.region")
	if region == "" {
		region = viper.GetString("aws.region")
	}

	// 获取CloudWatch配置
	logGroupName := viper.GetString("aws.cloudwatch.log_group")
	logStreamPrefix := viper.GetString("aws.cloudwatch.log_stream_prefix")
	logStreamName := fmt.Sprintf("%s%s", logStreamPrefix, topic)
	bufferSize := viper.GetInt("aws.cloudwatch.buffer_size")
	if bufferSize <= 0 {
		bufferSize = 100 // 默认缓冲区大小
	}

	flushInterval := viper.GetInt("aws.cloudwatch.flush_interval")
	if flushInterval <= 0 {
		flushInterval = 5 // 默认5秒刷新一次
	}

	async := viper.GetBool("aws.cloudwatch.async")

	// 获取AWS凭证
	awsAccessKey := viper.GetString("aws.access_key")
	awsSecret := viper.GetString("aws.secret")

	// 创建AWS会话
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(awsAccessKey, awsSecret, ""),
	})
	if err != nil {
		return nil, fmt.Errorf("创建AWS会话失败: %v", err)
	}

	// 创建CloudWatch Logs客户端
	svc := cloudwatchlogs.New(sess)

	// 确保日志组存在
	_, err = svc.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
	if err != nil {
		// 忽略已存在错误
		if _, ok := err.(*cloudwatchlogs.ResourceAlreadyExistsException); !ok {
			return nil, fmt.Errorf("创建日志组失败: %v", err)
		}
	}

	// 设置日志保留期限
	retentionDays := int64(viper.GetInt("aws.cloudwatch.retention_days"))
	if retentionDays > 0 {
		_, err = svc.PutRetentionPolicy(&cloudwatchlogs.PutRetentionPolicyInput{
			LogGroupName:    aws.String(logGroupName),
			RetentionInDays: aws.Int64(retentionDays),
		})
		if err != nil {
			fmt.Printf("设置日志保留期限失败: %v\n", err)
		}
	}

	// 创建日志流
	_, err = svc.CreateLogStream(&cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(logStreamName),
	})
	if err != nil {
		// 忽略已存在错误
		if _, ok := err.(*cloudwatchlogs.ResourceAlreadyExistsException); !ok {
			return nil, fmt.Errorf("创建日志流失败: %v", err)
		}
	}

	hook := &CloudWatchLogsHook{
		svc:           svc,
		logGroupName:  logGroupName,
		logStreamName: logStreamName,
		bufferSize:    bufferSize,
		flushInterval: time.Duration(flushInterval) * time.Second,
		async:         async,
		buffer:        make([]*cloudwatchlogs.InputLogEvent, 0, bufferSize),
	}

	// 启动定时刷新
	hook.timer = time.AfterFunc(hook.flushInterval, func() {
		hook.flushBuffer()
	})

	return hook, nil
}

// Fire 实现 logrus.Hook 接口，每当生成一个日志条目时调用
func (hook *CloudWatchLogsHook) Fire(entry *logrus.Entry) error {
	// 将entry转换为JSON
	line, err := entry.String()
	if err != nil {
		return err
	}

	// 创建日志事件
	logEvent := &cloudwatchlogs.InputLogEvent{
		Message:   aws.String(line),
		Timestamp: aws.Int64(time.Now().UnixNano() / 1000000), // 毫秒时间戳
	}

	hook.mutex.Lock()
	defer hook.mutex.Unlock()

	// 将事件添加到缓冲区
	hook.buffer = append(hook.buffer, logEvent)

	// 如果缓冲区达到阈值，刷新缓冲区
	if len(hook.buffer) >= hook.bufferSize {
		if hook.async {
			go hook.flushBuffer()
		} else {
			hook.flushBuffer()
		}
	}

	return nil
}

// flushBuffer 将缓冲区中的日志发送到CloudWatch
func (hook *CloudWatchLogsHook) flushBuffer() {
	hook.mutex.Lock()

	// 如果缓冲区为空，不需要刷新
	if len(hook.buffer) == 0 {
		hook.mutex.Unlock()

		// 重置定时器
		hook.timer.Reset(hook.flushInterval)
		return
	}

	// 提取缓冲区中的日志事件
	events := hook.buffer
	hook.buffer = make([]*cloudwatchlogs.InputLogEvent, 0, hook.bufferSize)
	hook.mutex.Unlock()

	// 按时间戳排序
	sort.Slice(events, func(i, j int) bool {
		return *events[i].Timestamp < *events[j].Timestamp
	})

	// 构建请求参数
	params := &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(hook.logGroupName),
		LogStreamName: aws.String(hook.logStreamName),
		LogEvents:     events,
	}

	// 如果有序列令牌，添加到请求中
	hook.mutex.Lock()
	if hook.sequenceToken != nil {
		params.SequenceToken = hook.sequenceToken
	}
	hook.mutex.Unlock()

	// 发送日志事件
	resp, err := hook.svc.PutLogEvents(params)
	if err != nil {
		fmt.Printf("发送日志到CloudWatch失败: %v\n", err)
	} else if resp.NextSequenceToken != nil {
		// 更新序列令牌
		hook.mutex.Lock()
		hook.sequenceToken = resp.NextSequenceToken
		hook.mutex.Unlock()
	}

	// 重置定时器
	hook.timer.Reset(hook.flushInterval)
}

// Levels 实现 logrus.Hook 接口，定义此钩子适用的日志级别
func (hook *CloudWatchLogsHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Close 关闭钩子并刷新所有待处理的日志
func (hook *CloudWatchLogsHook) Close() error {
	if hook.timer != nil {
		hook.timer.Stop()
	}

	hook.flushBuffer()
	return nil
}
