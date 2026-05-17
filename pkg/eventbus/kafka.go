package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaPublisher publishes Envelope records to Kafka using envelope.Topic().
type KafkaPublisher struct {
	writer *kafka.Writer
}

// NewKafkaPublisher creates a synchronous Kafka writer. RequiredAcks=1 keeps
// latency low for the development setup while the transactional outbox still
// provides retry after publication failures.
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

// Publish writes one envelope to its derived topic using the envelope key as the
// Kafka partition key.
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

// Close flushes and closes the underlying Kafka writer.
func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

// KafkaConsumer reads one topic as part of a consumer group.
type KafkaConsumer struct {
	reader *kafka.Reader
}

// NewKafkaConsumer creates a Kafka group consumer for one topic.
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

// Run continuously fetches, decodes and handles envelopes.
//
// Offsets are committed only after handler success. Decode failures are
// committed because retrying malformed bytes cannot succeed without operator
// intervention.
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

// Close closes the underlying Kafka reader.
func (c *KafkaConsumer) Close() error {
	if c == nil || c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
