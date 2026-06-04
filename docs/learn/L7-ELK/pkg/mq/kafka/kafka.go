package mq

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
)

// 生产者结构体
type Producer struct {
	writer *kafka.Writer
	topics map[string]string
	mu     sync.RWMutex
}

// 消费者结构体
type Consumer struct {
	reader *kafka.Reader
	topic  string // 记录消费的主题
	group  string // 记录消费者组
}

// 消息结构体
type Message struct {
	Topic     string    `json:"topic"`     // 主题
	Key       string    `json:"key"`       // 消息键
	Value     []byte    `json:"value"`     // 消息体
	Partition int       `json:"partition"` // 分区
	Offset    int64     `json:"offset"`    // 偏移量
	Time      time.Time `json:"time"`      // 时间戳
	Headers   []Header  `json:"headers"`   // 消息头
}

// 消息头结构
type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// 生产者单例实例
var (
	producerInstance *Producer
	producerOnce     sync.Once
	initErr          error // 记录初始化错误
)

type KafkaCfg struct {
	Brokers []string
	Topics  Topic
	Version string
}

type Topic struct {
	user string
	post string
}

// 获取生产者单例
func NewProducer() (*Producer, error) {
	producerOnce.Do(func() {
		kafkaConfig := &KafkaCfg{
			Brokers: []string{"localhost:9092"},
			Topics:  Topic{user: "user-events", post: "post-events"},
			Version: "2.8.0",
		}

		// 验证配置
		if len(kafkaConfig.Brokers) == 0 {
			initErr = fmt.Errorf("KafkaCfg brokers配置为空")
			return
		}

		// 初始化生产者
		producerInstance, initErr = initProducer(kafkaConfig.Brokers, kafkaConfig.Version)
		if initErr != nil {
			log.Printf("初始化Kafka生产者失败: %v", initErr)
			return
		}

		// 缓存主题映射
		producerInstance.topics = map[string]string{
			"user": kafkaConfig.Topics.user,
			"post": kafkaConfig.Topics.post,
		}

		// 异步创建主题（不阻塞启动）
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := createTopics(ctx, *kafkaConfig); err != nil {
				log.Printf("异步创建Kafka主题失败: %v", err)
			} else {
				log.Println("Kafka主题创建/验证完成")
			}
		}()
	})

	return producerInstance, initErr
}

// 初始化生产者
func initProducer(brokers []string, version string) (*Producer, error) {
	// 创建Kafka Writer配置
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),  // 连接地址
		Balancer:     &kafka.Hash{},          // 使用哈希分区，确保不同消费者获取自己的消息
		RequiredAcks: kafka.RequireAll,       // 需要所有副本确认
		MaxAttempts:  3,                      // 最大重试次数
		WriteTimeout: 10 * time.Second,       // 写超时
		ReadTimeout:  10 * time.Second,       // 读超时
		Async:        false,                  // 同步发送（保证顺序）
		Compression:  compress.Snappy,        // 压缩，减少网络流量
		BatchSize:    100,                    // 批量大小
		BatchBytes:   1048576,                // 1MB，控制内存
		BatchTimeout: 100 * time.Millisecond, // 批次超时
	}

	return &Producer{
		writer: writer,
		topics: make(map[string]string),
	}, nil
}

// 创建消费者
func NewConsumer(topic string, groupID string) (*Consumer, error) {
	kafkaConfig := &KafkaCfg{
		Brokers: []string{"localhost:9092"},
		Topics:  Topic{user: "user-events", post: "post-events"},
		Version: "2.8.0",
	}

	// 验证配置
	if len(kafkaConfig.Brokers) == 0 {
		return nil, fmt.Errorf("KafkaCfg brokers配置为空")
	}

	// 消费者组名处理
	if groupID == "" {
		groupID = fmt.Sprintf("consumer-group-%d", time.Now().UnixNano())
	}

	// 创建Kafka Reader配置
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:         kafkaConfig.Brokers,
		Topic:           topic,
		GroupID:         groupID,
		MinBytes:        10e3,              // 10KB，减少小消息的网络开销
		MaxBytes:        10e6,              // 10MB，避免大消息内存溢出
		MaxWait:         1 * time.Second,   // 调整：减少等待时间
		ReadLagInterval: -1,                // 不检查消费延迟
		StartOffset:     kafka.FirstOffset, // 从最早开始消费
		CommitInterval:  1 * time.Second,   // 提交偏移量间隔
		QueueCapacity:   100,               // 内部队列容量
	})

	return &Consumer{
		reader: reader,
		topic:  topic,
		group:  groupID,
	}, nil
}

// 发送消息到指定主题
func (p *Producer) Send(ctx context.Context, topic string, key string, value []byte, headers ...Header) error {
	// 验证参数
	if topic == "" {
		return fmt.Errorf("主题不能为空")
	}

	// 转换消息头格式
	kafkaHeaders := make([]kafka.Header, 0, len(headers))
	for _, h := range headers {
		kafkaHeaders = append(kafkaHeaders, kafka.Header{
			Key:   h.Key,
			Value: []byte(h.Value),
		})
	}

	// 构建Kafka消息
	msg := kafka.Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   value,
		Headers: kafkaHeaders,
		Time:    time.Now(), // 设置消息时间戳
	}

	// 发送消息
	err := p.writer.WriteMessages(ctx, msg)
	if err != nil {
		return fmt.Errorf("发送消息到主题[%s]失败: %w", topic, err)
	}

	// 记录日志
	log.Printf("消息发送成功: topic=%s, key=%s, size=%d bytes",
		topic, key, len(value))
	return nil
}

// 发送带重试的消息
func (p *Producer) SendWithRetry(ctx context.Context, topic string, key string, value []byte, maxRetries int, headers ...Header) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := p.Send(ctx, topic, key, value, headers...)
		if err == nil {
			return nil
		}

		lastErr = err
		log.Printf("发送消息重试 %d/%d: %v", i+1, maxRetries, err)

		// 指数退避策略
		waitTime := time.Duration(1<<i) * 100 * time.Millisecond
		if waitTime > 5*time.Second {
			waitTime = 5 * time.Second
		}

		select {
		case <-time.After(waitTime):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("发送消息失败，重试%d次后放弃: %w", maxRetries, lastErr)
}

// 获取主题名称（带缓存）
func (p *Producer) getTopic(topicType string) (string, error) {
	p.mu.RLock()
	topic, exists := p.topics[topicType]
	p.mu.RUnlock()

	if exists {
		return topic, nil
	}

	// 从配置重新加载
	kafkaConfig := &KafkaCfg{
		Brokers: []string{"localhost:9092"},
		Topics:  Topic{user: "user-events", post: "post-events"},
		Version: "2.8.0",
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// 重新检查（双重检查锁模式）
	if topic, exists = p.topics[topicType]; exists {
		return topic, nil
	}

	// 从配置映射获取
	switch topicType {
	case "user":
		topic = kafkaConfig.Topics.user
	case "post":
		topic = kafkaConfig.Topics.post
	default:
		return "", fmt.Errorf("未知的主题类型: %s", topicType)
	}

	p.topics[topicType] = topic
	return topic, nil
}

// 以下是各种事件类型的便捷发送方法

func (p *Producer) SendUserEvent(ctx context.Context, key string, value []byte, headers ...Header) error {
	topic, err := p.getTopic("user")
	if err != nil {
		return err
	}
	return p.Send(ctx, topic, key, value, headers...)
}

func (p *Producer) SendPostEvent(ctx context.Context, key string, value []byte, headers ...Header) error {
	topic, err := p.getTopic("post")
	if err != nil {
		return err
	}
	return p.Send(ctx, topic, key, value, headers...)
}

// 批量发送消息
func (p *Producer) SendBatch(ctx context.Context, topic string, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}

	kafkaMessages := make([]kafka.Message, 0, len(messages))
	for _, msg := range messages {
		kafkaHeaders := make([]kafka.Header, 0, len(msg.Headers))
		for _, h := range msg.Headers {
			kafkaHeaders = append(kafkaHeaders, kafka.Header{
				Key:   h.Key,
				Value: []byte(h.Value),
			})
		}

		kafkaMessages = append(kafkaMessages, kafka.Message{
			Topic:   topic,
			Key:     []byte(msg.Key),
			Value:   msg.Value,
			Headers: kafkaHeaders,
			Time:    msg.Time,
		})
	}

	err := p.writer.WriteMessages(ctx, kafkaMessages...)
	if err != nil {
		return fmt.Errorf("批量发送消息失败: %w", err)
	}

	log.Printf("批量消息发送成功: topic=%s, count=%d", topic, len(messages))
	return nil
}

// 接收消息
func (c *Consumer) Receive(ctx context.Context) (*Message, error) {
	// 设置读取超时
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	msg, err := c.reader.ReadMessage(readCtx)
	if err != nil {
		if err == context.DeadlineExceeded {
			// 读取超时，不是错误
			return nil, nil
		}
		return nil, fmt.Errorf("读取消息失败: %w", err)
	}

	// 转换消息头
	headers := make([]Header, 0, len(msg.Headers))
	for _, h := range msg.Headers {
		headers = append(headers, Header{
			Key:   h.Key,
			Value: string(h.Value),
		})
	}

	return &Message{
		Topic:     msg.Topic,
		Key:       string(msg.Key),
		Value:     msg.Value,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Time:      msg.Time,
		Headers:   headers,
	}, nil
}

// 批量接收消息
func (c *Consumer) ReceiveBatch(ctx context.Context, maxMessages int) ([]*Message, error) {
	if maxMessages <= 0 {
		maxMessages = 100
	}

	messages := make([]*Message, 0, maxMessages)

	for i := 0; i < maxMessages; i++ {
		msg, err := c.Receive(ctx)
		if err != nil {
			return messages, err
		}

		if msg == nil {
			break // 没有消息了
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// 手动提交偏移量
func (c *Consumer) CommitOffset(ctx context.Context) error {
	return c.reader.CommitMessages(ctx)
}

// 获取消费者状态
func (c *Consumer) GetStatus() map[string]interface{} {
	stats := c.reader.Stats()
	return map[string]interface{}{
		"topic":     c.topic,
		"group":     c.group,
		"dials":     stats.Dials,
		"messages":  stats.Messages,
		"bytes":     stats.Bytes,
		"errors":    stats.Errors,
		"timeouts":  stats.Timeouts,
		"lag":       stats.Lag,
		"partition": stats.Partition,
		"offset":    stats.Offset,
	}
}

// 关闭生产者
func (p *Producer) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

// 关闭消费者
func (c *Consumer) Close() error {
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}

// 创建主题
func createTopics(ctx context.Context, kafkaConfig KafkaCfg) error {

	log.Printf("开始创建Kafka主题，brokers: %v", kafkaConfig.Brokers)

	// 连接到Kafka
	conn, err := kafka.Dial("tcp", kafkaConfig.Brokers[0])
	if err != nil {
		log.Printf("连接Kafka失败: %v", err)
		return err
	}
	defer conn.Close()

	// 获取集群控制器
	controller, err := conn.Controller()
	if err != nil {
		log.Printf("获取Kafka控制器失败: %v", err)
		return err
	}

	// 连接到控制器
	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		log.Printf("连接Kafka控制器失败: %v", err)
		return err
	}
	defer controllerConn.Close()

	// 主题配置
	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             kafkaConfig.Topics.user,
			NumPartitions:     3, // 增加分区数
			ReplicationFactor: 1,
		},
		{
			Topic:             kafkaConfig.Topics.post,
			NumPartitions:     6, // 分区更多
			ReplicationFactor: 1,
		},
	}

	// 创建主题
	createdCount := 0
	for _, tc := range topicConfigs {
		// 检查主题是否已存在
		partitions, err := controllerConn.ReadPartitions(tc.Topic)
		if err == nil && len(partitions) > 0 {
			log.Printf("主题已存在: %s (分区数: %d)", tc.Topic, len(partitions))
			continue
		}

		// 创建主题
		err = controllerConn.CreateTopics(tc)
		if err != nil {
			log.Printf("创建主题 %s 失败: %v", tc.Topic, err)
			// 继续创建其他主题
			continue
		}

		log.Printf("主题创建成功: %s (分区数: %d, 副本数: %d)",
			tc.Topic, tc.NumPartitions, tc.ReplicationFactor)
		createdCount++
	}

	log.Printf("主题创建完成: 总数=%d, 成功=%d", len(topicConfigs), createdCount)
	return nil
}
