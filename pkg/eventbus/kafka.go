package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaPublisher 按 Envelope.Topic() 的映射把统一事件写入 Kafka。
type KafkaPublisher struct {
	writer *kafka.Writer
}

// NewKafkaPublisher 创建同步 Kafka 写入器。
// RequiredAcks=1 能降低开发环境延迟；可靠性主要由事务 Outbox 的失败重试兜底，
// 因此业务 DB 提交和 Kafka 发布之间的崩溃窗口不会直接丢失事件。
func NewKafkaPublisher(brokers []string, clientID string) *KafkaPublisher {
	return &KafkaPublisher{
		writer: &kafka.Writer{
			Addr:            kafka.TCP(brokers...),
			Balancer:        &kafka.Hash{},
			RequiredAcks:    kafka.RequireOne,
			Async:           false,
			BatchTimeout:    20 * time.Millisecond,
			ReadTimeout:     2 * time.Second,
			WriteTimeout:    2 * time.Second,
			MaxAttempts:     2,
			WriteBackoffMin: 50 * time.Millisecond,
			WriteBackoffMax: 200 * time.Millisecond,
			Transport: &kafka.Transport{
				ClientID: clientID,
			},
		},
	}
}

// Publish 将一条 Envelope 写入其事件类型对应的 topic。
// envelope.Key 会作为 Kafka 分区键，保证同一聚合对象的事件尽量落在同一分区内有序处理。
func (p *KafkaPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	if p == nil || p.writer == nil {
		return nil
	}
	topic := envelope.Topic()
	if topic == "" {
		return errors.New("event topic is empty")
	}
	value, err := envelope.Bytes()
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.writer.WriteMessages(writeCtx, kafka.Message{
		Topic: topic,
		Key:   []byte(envelope.Key),
		Value: value,
		Time:  time.Now(),
	})
}

// Close 刷新并关闭底层 Kafka writer。
func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

// KafkaConsumer 以消费者组成员身份读取单个 topic。
type KafkaConsumer struct {
	reader *kafka.Reader
}

// NewKafkaConsumer 创建一个绑定指定 topic 和 groupID 的 Kafka 消费者。
func NewKafkaConsumer(brokers []string, topic, groupID string) *KafkaConsumer {
	return &KafkaConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
		}),
	}
}

// Run 持续拉取、解码并处理 Envelope。
// offset 只在 handler 成功后提交；无法解码的脏消息会直接提交，
// 因为重复消费格式错误的字节也无法恢复，只会卡住整个分区。
func (c *KafkaConsumer) Run(ctx context.Context, handler Handler) {
	if c == nil || c.reader == nil || handler == nil {
		return
	}
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Kafka读取事件失败: %v", err)
			time.Sleep(time.Second)
			continue
		}

		envelope, err := events.DecodeEnvelope(msg.Value)
		if err != nil {
			log.Printf("Kafka事件解析失败: %v", err)
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}
		if err := handler(ctx, envelope); err != nil {
			log.Printf("Kafka事件处理失败 type=%s key=%s err=%v", envelope.Type, envelope.Key, err)
			continue
		}
		if err := c.reader.CommitMessages(ctx, msg); err != nil && ctx.Err() == nil {
			log.Printf("Kafka提交offset失败: %v", err)
		}
	}
}

// Close 关闭底层 Kafka reader。
func (c *KafkaConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
