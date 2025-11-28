// Package cloudwatch provides CloudWatch Logs integration for logrus
package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Hook implements logrus.Hook for CloudWatch Logs
type Hook struct {
	client        *cloudwatchlogs.Client
	logGroupName  string
	logStreamName string
	buffer        []types.InputLogEvent
	bufferSize    int
	flushInterval time.Duration
	async         bool
	mutex         sync.Mutex
	timer         *time.Timer
}

// NewHook creates a new CloudWatch Logs hook
// Configuration path: aws.cloudwatch.*
func NewHook() (*Hook, error) {
	// Get region (aws.cloudwatch.region -> aws.region)
	region := viper.GetString("aws.cloudwatch.region")
	if region == "" {
		region = viper.GetString("aws.region")
	}
	if region == "" {
		return nil, fmt.Errorf("aws.cloudwatch.region or aws.region is required")
	}

	logGroupName := viper.GetString("aws.cloudwatch.log_group")
	if logGroupName == "" {
		return nil, fmt.Errorf("aws.cloudwatch.log_group is required")
	}

	logStreamName := viper.GetString("aws.cloudwatch.log_stream")
	if logStreamName == "" {
		return nil, fmt.Errorf("aws.cloudwatch.log_stream is required")
	}

	bufferSize := viper.GetInt("aws.cloudwatch.buffer_size")
	if bufferSize <= 0 {
		bufferSize = 100
	}

	flushInterval := viper.GetInt("aws.cloudwatch.flush_interval")
	if flushInterval <= 0 {
		flushInterval = 5
	}

	async := viper.GetBool("aws.cloudwatch.async")

	// Create AWS config
	ctx := context.Background()
	var cfg aws.Config
	var err error

	accessKey := viper.GetString("aws.access_key")
	secretKey := viper.GetString("aws.secret")

	if accessKey != "" && secretKey != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	client := cloudwatchlogs.NewFromConfig(cfg)

	// Create log group (ignore if exists)
	_, err = client.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
	if err != nil {
		var alreadyExists *types.ResourceAlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			return nil, fmt.Errorf("failed to create log group: %v", err)
		}
	}

	// Set retention policy
	retentionDays := int32(viper.GetInt("aws.cloudwatch.retention_days"))
	if retentionDays > 0 {
		_, _ = client.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{
			LogGroupName:    aws.String(logGroupName),
			RetentionInDays: aws.Int32(retentionDays),
		})
	}

	// Create log stream (ignore if exists)
	_, err = client.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(logStreamName),
	})
	if err != nil {
		var alreadyExists *types.ResourceAlreadyExistsException
		if !errors.As(err, &alreadyExists) {
			return nil, fmt.Errorf("failed to create log stream: %v", err)
		}
	}

	hook := &Hook{
		client:        client,
		logGroupName:  logGroupName,
		logStreamName: logStreamName,
		bufferSize:    bufferSize,
		flushInterval: time.Duration(flushInterval) * time.Second,
		async:         async,
		buffer:        make([]types.InputLogEvent, 0, bufferSize),
	}

	hook.timer = time.AfterFunc(hook.flushInterval, func() {
		hook.flush()
	})

	return hook, nil
}

// Fire implements logrus.Hook
func (h *Hook) Fire(entry *logrus.Entry) error {
	line, err := entry.String()
	if err != nil {
		return err
	}

	event := types.InputLogEvent{
		Message:   aws.String(line),
		Timestamp: aws.Int64(time.Now().UnixMilli()),
	}

	h.mutex.Lock()
	h.buffer = append(h.buffer, event)
	shouldFlush := len(h.buffer) >= h.bufferSize
	h.mutex.Unlock()

	if shouldFlush {
		if h.async {
			go h.flush()
		} else {
			h.flush()
		}
	}

	return nil
}

// flush sends buffered logs to CloudWatch
func (h *Hook) flush() {
	h.mutex.Lock()
	if len(h.buffer) == 0 {
		h.mutex.Unlock()
		h.timer.Reset(h.flushInterval)
		return
	}

	events := h.buffer
	h.buffer = make([]types.InputLogEvent, 0, h.bufferSize)
	h.mutex.Unlock()

	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return *events[i].Timestamp < *events[j].Timestamp
	})

	_, err := h.client.PutLogEvents(context.Background(), &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(h.logGroupName),
		LogStreamName: aws.String(h.logStreamName),
		LogEvents:     events,
	})
	if err != nil {
		fmt.Printf("cloudwatch: failed to send logs: %v\n", err)
	}

	h.timer.Reset(h.flushInterval)
}

// Levels implements logrus.Hook
func (h *Hook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Close flushes remaining logs and stops the timer
func (h *Hook) Close() error {
	if h.timer != nil {
		h.timer.Stop()
	}
	h.flush()
	return nil
}
