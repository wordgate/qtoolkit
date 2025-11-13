package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/spf13/viper"
)

var sqsClients map[string]*Client = make(map[string]*Client)
var sqsMux sync.RWMutex

// Message represents a message in SQS queue
type Message struct {
	Action     string      `json:"action"`
	Params     interface{} `json:"params"`
	SendAtMS   int64       `json:"sendAtMS"`
	RetryCount int         `json:"retryCount"`
	MaxRetries int         `json:"maxRetries"`
}

// ParseParams parses message parameters to specified struct
// Uses JSON serialization/deserialization for type-safe parameter parsing
func (msg *Message) ParseParams(target interface{}) error {
	// Marshal Params to JSON first
	jsonData, err := json.Marshal(msg.Params)
	if err != nil {
		return fmt.Errorf("marshal params failed: %v", err)
	}

	// Unmarshal JSON to target struct
	err = json.Unmarshal(jsonData, target)
	if err != nil {
		return fmt.Errorf("unmarshal to target struct failed: %v", err)
	}

	return nil
}

// Client represents an SQS client instance
type Client struct {
	sqs      *sqs.Client
	queueUrl string
	region   string
}

// Config represents SQS configuration for a specific queue
type Config struct {
	AccessKey string `yaml:"access_key" json:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key"`
	UseIMDS   bool   `yaml:"use_imds" json:"use_imds"`
	Region    string `yaml:"region" json:"region"`
	QueueName string `yaml:"queue_name" json:"queue_name"`
}

// loadConfig loads AWS configuration for SQS
func loadConfig(region string, cfg *Config) (awsv2.Config, error) {
	ctx := context.Background()

	// If UseIMDS is explicitly set to false, use static credentials
	if cfg != nil && !cfg.UseIMDS {
		if cfg.AccessKey != "" && cfg.SecretKey != "" {
			return awsconfig.LoadDefaultConfig(ctx,
				awsconfig.WithRegion(region),
				awsconfig.WithCredentialsProvider(awscredentials.NewStaticCredentialsProvider(
					cfg.AccessKey,
					cfg.SecretKey,
					"",
				)),
			)
		}
		return awsv2.Config{}, fmt.Errorf("UseIMDS is false but AccessKey/SecretKey are not configured")
	}

	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
}

// loadConfigFromViper loads SQS configuration from viper for a specific queue
// Configuration path priority (cascading fallback):
// 1. aws.sqs.queues.<queueName> - Queue-specific config
// 2. aws.sqs - SQS service config
// 3. aws - Global AWS config
func loadConfigFromViper(queueName string) (*Config, error) {
	cfg := &Config{
		QueueName: queueName,
	}

	// Try queue-specific config first
	queueConfigPath := fmt.Sprintf("aws.sqs.queues.%s", queueName)
	if viper.IsSet(queueConfigPath) {
		// Queue-specific config exists
		cfg.Region = viper.GetString(queueConfigPath + ".region")
		cfg.AccessKey = viper.GetString(queueConfigPath + ".access_key")
		cfg.SecretKey = viper.GetString(queueConfigPath + ".secret_key")
		cfg.UseIMDS = viper.GetBool(queueConfigPath + ".use_imds")
	}

	// Fall back to SQS service config for missing values
	if cfg.Region == "" {
		cfg.Region = viper.GetString("aws.sqs.region")
	}
	if cfg.AccessKey == "" {
		cfg.AccessKey = viper.GetString("aws.sqs.access_key")
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = viper.GetString("aws.sqs.secret_key")
	}
	if !viper.IsSet(queueConfigPath+".use_imds") && viper.IsSet("aws.sqs.use_imds") {
		cfg.UseIMDS = viper.GetBool("aws.sqs.use_imds")
	}

	// Fall back to global AWS config for missing values
	if cfg.Region == "" {
		cfg.Region = viper.GetString("aws.region")
	}
	if cfg.AccessKey == "" {
		cfg.AccessKey = viper.GetString("aws.access_key")
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = viper.GetString("aws.secret_key")
	}
	if !viper.IsSet(queueConfigPath+".use_imds") && !viper.IsSet("aws.sqs.use_imds") && viper.IsSet("aws.use_imds") {
		cfg.UseIMDS = viper.GetBool("aws.use_imds")
	}

	// Validate required fields
	if cfg.Region == "" {
		return nil, fmt.Errorf("sqs region not configured for queue: %s (check aws.region, aws.sqs.region, or aws.sqs.queues.%s.region)", queueName, queueName)
	}

	return cfg, nil
}

// initSqs initializes SQS client for a specific queue
// The queueName parameter is used as the queue name and config lookup key
func initSqs(queueName string) (*Client, error) {
	// Load config from viper
	cfg, err := loadConfigFromViper(queueName)
	if err != nil {
		return nil, err
	}

	awsCfg, err := loadConfig(cfg.Region, cfg)
	if err != nil {
		return nil, fmt.Errorf("create aws session error: %v", err)
	}

	sqsClient := sqs.NewFromConfig(awsCfg)
	ctx := context.Background()

	// Create or get queue
	result, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: awsv2.String(queueName),
		Attributes: map[string]string{
			string(sqstypes.QueueAttributeNameDelaySeconds):           "0",
			string(sqstypes.QueueAttributeNameMessageRetentionPeriod): "345600", // 4 days
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create/get queue error: %v", err)
	}

	return &Client{
		sqs:      sqsClient,
		queueUrl: *result.QueueUrl,
		region:   cfg.Region,
	}, nil
}

// Get returns SQS client for specified queue
// Config is automatically loaded from viper configuration file
// Configuration should be under aws.sqs.queues.<queueName> or aws.sqs (global)
func Get(queueName string) (*Client, error) {
	sqsMux.RLock()
	client, ok := sqsClients[queueName]
	sqsMux.RUnlock()

	if !ok {
		sqsMux.Lock()
		defer sqsMux.Unlock()
		if client, ok = sqsClients[queueName]; !ok {
			var err error
			client, err = initSqs(queueName)
			if err != nil {
				return nil, err
			}
			sqsClients[queueName] = client
		}
	}
	return client, nil
}

// sendMessage sends a message to the queue (internal method)
func (c *Client) sendMessage(msg Message) error {
	msgBt, _ := json.Marshal(msg)
	ctx := context.Background()

	_, err := c.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		DelaySeconds: 0,
		MessageBody:  awsv2.String(string(msgBt)),
		QueueUrl:     &c.queueUrl,
	})
	if err != nil {
		return fmt.Errorf("send message error: %v", err)
	}
	return nil
}

// Send sends a message to the queue
func (c *Client) Send(action string, params interface{}) error {
	msg := Message{
		Action:     action,
		Params:     params,
		SendAtMS:   time.Now().UnixMicro(),
		RetryCount: 0,
		MaxRetries: 3,
	}
	return c.sendMessage(msg)
}

// SendWithRetry sends a message with custom max retry count
func (c *Client) SendWithRetry(action string, params interface{}, maxRetries int) error {
	msg := Message{
		Action:     action,
		Params:     params,
		SendAtMS:   time.Now().UnixMicro(),
		RetryCount: 0,
		MaxRetries: maxRetries,
	}
	return c.sendMessage(msg)
}

// retry retries a failed message (internal method)
func (c *Client) retry(msg Message) error {
	if msg.RetryCount >= msg.MaxRetries {
		return fmt.Errorf("message has reached max retries: %d", msg.MaxRetries)
	}

	msg.RetryCount++
	delaySeconds := int32(math.Pow(2, float64(msg.RetryCount-1))) * 60

	msgBt, _ := json.Marshal(msg)
	ctx := context.Background()

	_, err := c.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		DelaySeconds: delaySeconds,
		MessageBody:  awsv2.String(string(msgBt)),
		QueueUrl:     &c.queueUrl,
	})
	if err != nil {
		return fmt.Errorf("retry message error: %v", err)
	}
	return nil
}

// MessageHandler is the function type for processing messages
type MessageHandler func(msg Message) error

// Consume consumes messages from the queue
func (c *Client) Consume(handler MessageHandler) {
	ctx := context.Background()

	for {
		result, err := c.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &c.queueUrl,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     20,
			AttributeNames: []sqstypes.QueueAttributeName{
				sqstypes.QueueAttributeNameAll,
			},
		})

		if err != nil {
			fmt.Printf("receive message error: %v\n", err)
			time.Sleep(time.Second)
			continue
		}

		for _, message := range result.Messages {
			var msg Message
			if err := json.Unmarshal([]byte(*message.Body), &msg); err != nil {
				fmt.Printf("unmarshal message error: %v\n", err)
				continue
			}

			// Process message
			if err := handler(msg); err != nil {
				// If processing failed, retry
				retryErr := c.retry(msg)
				if retryErr != nil {
					fmt.Printf("retry message failed: %v\n", retryErr)
				}
			}

			// Delete processed message
			_, err := c.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      &c.queueUrl,
				ReceiptHandle: message.ReceiptHandle,
			})
			if err != nil {
				fmt.Printf("delete message error: %v\n", err)
			}
		}
	}
}

// CreateQueue creates a new SQS queue and returns its URL
func (c *Client) CreateQueue(queueName string) (string, error) {
	ctx := context.Background()

	result, err := c.sqs.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: &queueName,
		Attributes: map[string]string{
			string(sqstypes.QueueAttributeNameDelaySeconds): "0",
		},
	})
	if err != nil {
		return "", err
	}
	return *result.QueueUrl, nil
}

// DeleteQueue deletes a queue by its URL
func (c *Client) DeleteQueue(queueUrl string) error {
	ctx := context.Background()

	_, err := c.sqs.DeleteQueue(ctx, &sqs.DeleteQueueInput{
		QueueUrl: &queueUrl,
	})
	return err
}

// ConsumeQueue consumes messages from a specific queue URL and sends them to a channel
func (c *Client) ConsumeQueue(queueUrl string, msgCh chan *Message) {
	ctx := context.Background()

	for {
		msgResult, err := c.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{
				sqstypes.MessageSystemAttributeNameSentTimestamp,
			},
			MessageAttributeNames: []string{
				string(sqstypes.QueueAttributeNameAll),
			},
			QueueUrl:            &queueUrl,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			fmt.Printf("consume sqs err:%v\n", err)
			time.Sleep(time.Second)
			continue
		}

		if len(msgResult.Messages) == 0 {
			fmt.Println("no message")
			continue
		}

		msg := &Message{}
		msgStr := *msgResult.Messages[0].Body

		fmt.Printf("get queue message:%s", msgStr)

		err = json.Unmarshal([]byte(msgStr), msg)
		if err != nil {
			fmt.Printf("error queue message: %v", err)
			continue
		}
		msgCh <- msg
	}
}
