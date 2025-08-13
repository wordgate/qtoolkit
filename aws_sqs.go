package qtoolkit

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/spf13/viper"
)

var sqsClients map[string]*SqsClient = make(map[string]*SqsClient)
var sqsMux sync.RWMutex

type SqsMessage struct {
	Action     string      `json:"action"`
	Params     interface{} `json:"params"`
	SendAtMS   int64       `json:"sendAtMS"`
	RetryCount int         `json:"retryCount"`
	MaxRetries int         `json:"maxRetries"`
}

// ParseParams 将消息参数解析到指定的结构体中
// 使用JSON序列化/反序列化来实现类型安全的参数解析
func (msg *SqsMessage) ParseParams(target interface{}) error {
	// 先将Params序列化为JSON
	jsonData, err := json.Marshal(msg.Params)
	if err != nil {
		return fmt.Errorf("marshal params failed: %v", err)
	}
	
	// 再将JSON反序列化到目标结构体
	err = json.Unmarshal(jsonData, target)
	if err != nil {
		return fmt.Errorf("unmarshal to target struct failed: %v", err)
	}
	
	return nil
}

type SqsClient struct {
	sqs      *sqs.SQS
	queueUrl string
	region   string
}

// 内部初始化方法
func initSqs(name string) (*SqsClient, error) {

	region := viper.GetString(fmt.Sprintf("aws.sqs.%s.region", name))
	queueName := viper.GetString(fmt.Sprintf("aws.sqs.%s.queue_name", name))

	// 如果配置中没有指定queue_name，就使用传入的name作为队列名
	if queueName == "" {
		queueName = name
	}

	if region == "" {
		return nil, fmt.Errorf("no sqs config for queue: %s", name)
	}

	sess, err := awsSession(region)
	if err != nil {
		return nil, fmt.Errorf("create aws session error: %v", err)
	}

	sqsClient := sqs.New(sess)

	// 创建或获取队列
	result, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]*string{
			"DelaySeconds":           aws.String("0"),
			"MessageRetentionPeriod": aws.String("345600"), // 4天
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create/get queue error: %v", err)
	}

	return &SqsClient{
		sqs:      sqsClient,
		queueUrl: *result.QueueUrl,
		region:   region,
	}, nil
}

// 获取默认SQS实例
func SqsDefault() *SqsClient {
	return SqsMust("")
}

// 获取指定队列的SQS实例
func Sqs(name string) (*SqsClient, error) {
	sqsMux.RLock()
	client, ok := sqsClients[name]
	sqsMux.RUnlock()

	if !ok {
		sqsMux.Lock()
		defer sqsMux.Unlock()
		if client, ok = sqsClients[name]; !ok {
			var err error
			client, err = initSqs(name)
			if err != nil {
				return nil, err
			}
			sqsClients[name] = client
		}
	}
	return client, nil
}

// Must版本
func SqsMust(name string) *SqsClient {
	client, err := Sqs(name)
	if err != nil {
		panic(err)
	}
	return client
}

// 内部发送消息方法
func (s *SqsClient) sendMessage(msg SqsMessage) error {
	msgBt, _ := json.Marshal(msg)
	_, err := s.sqs.SendMessage(&sqs.SendMessageInput{
		DelaySeconds: aws.Int64(0),
		MessageBody:  aws.String(string(msgBt)),
		QueueUrl:     &s.queueUrl,
	})
	if err != nil {
		return fmt.Errorf("send message error: %v", err)
	}
	return nil
}

// 发送消息
func (s *SqsClient) Send(action string, params interface{}) error {
	msg := SqsMessage{
		Action:     action,
		Params:     params,
		SendAtMS:   time.Now().UnixMicro(),
		RetryCount: 0,
		MaxRetries: 3,
	}
	return s.sendMessage(msg)
}

// 发送带自定义重试次数的消息
func (s *SqsClient) SendWithRetry(action string, params interface{}, maxRetries int) error {
	msg := SqsMessage{
		Action:     action,
		Params:     params,
		SendAtMS:   time.Now().UnixMicro(),
		RetryCount: 0,
		MaxRetries: maxRetries,
	}
	return s.sendMessage(msg)
}

// 内部重试方法
func (s *SqsClient) retry(msg SqsMessage) error {
	if msg.RetryCount >= msg.MaxRetries {
		return fmt.Errorf("message has reached max retries: %d", msg.MaxRetries)
	}

	msg.RetryCount++
	delaySeconds := int64(math.Pow(2, float64(msg.RetryCount-1))) * 60

	msgBt, _ := json.Marshal(msg)
	_, err := s.sqs.SendMessage(&sqs.SendMessageInput{
		DelaySeconds: aws.Int64(delaySeconds),
		MessageBody:  aws.String(string(msgBt)),
		QueueUrl:     &s.queueUrl,
	})
	if err != nil {
		return fmt.Errorf("retry message error: %v", err)
	}
	return nil
}

// 消息处理函数类型
type MessageHandler func(msg SqsMessage) error

// 消费消息
func (s *SqsClient) Consume(handler MessageHandler) {
	for {
		result, err := s.sqs.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueUrl:            &s.queueUrl,
			MaxNumberOfMessages: aws.Int64(1),
			WaitTimeSeconds:     aws.Int64(20),
			AttributeNames: []*string{
				aws.String(sqs.QueueAttributeNameAll),
			},
		})

		if err != nil {
			fmt.Printf("receive message error: %v\n", err)
			time.Sleep(time.Second)
			continue
		}

		for _, message := range result.Messages {
			var msg SqsMessage
			if err := json.Unmarshal([]byte(*message.Body), &msg); err != nil {
				fmt.Printf("unmarshal message error: %v\n", err)
				continue
			}

			// 处理消息
			if err := handler(msg); err != nil {
				// 如果处理失败，尝试重试
				retryErr := s.retry(msg)
				if retryErr != nil {
					fmt.Printf("retry message failed: %v\n", retryErr)
				}
			}

			// 删除已处理的消息
			_, err := s.sqs.DeleteMessage(&sqs.DeleteMessageInput{
				QueueUrl:      &s.queueUrl,
				ReceiptHandle: message.ReceiptHandle,
			})
			if err != nil {
				fmt.Printf("delete message error: %v\n", err)
			}
		}
	}
}

func (s *SqsClient) CreateQueue(queueName string) (string, error) {
	result, err := s.sqs.CreateQueue(&sqs.CreateQueueInput{
		QueueName: &queueName,
		Attributes: map[string]*string{
			"DelaySeconds": aws.String("0"),
			// 控制多久后msg自动drop
			//"MessageRetentionPeriod": aws.String("86400"),
		},
	})
	if err != nil {
		return "", err
	}
	return *result.QueueUrl, nil
}

func (s *SqsClient) DeleteQueue(queueUrl string) {
	s.sqs.DeleteQueue(&sqs.DeleteQueueInput{
		QueueUrl: &queueUrl,
	})
}

func (s *SqsClient) ConsumeQueue(queueUrl string, msgCh chan *SqsMessage) {
	for {
		msgResult, err := s.sqs.ReceiveMessage(&sqs.ReceiveMessageInput{
			AttributeNames: []*string{
				aws.String(sqs.MessageSystemAttributeNameSentTimestamp),
			},
			MessageAttributeNames: []*string{
				aws.String(sqs.QueueAttributeNameAll),
			},
			QueueUrl:            &queueUrl,
			MaxNumberOfMessages: aws.Int64(1),
			WaitTimeSeconds:     aws.Int64(20),
		})
		if err != nil {
			fmt.Printf("consume sqs err:%v\n", err)
			time.Sleep(time.Second)
			continue
		}
		msg := &SqsMessage{}
		if len(msgResult.Messages) == 0 {
			fmt.Println("no message")
			continue
		}
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
