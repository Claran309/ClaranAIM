//go:build integration

package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

// TestIntegrationKafkaBrokerDuplicateDeliveryIsIdempotent 覆盖 A1 的“真实 Kafka broker 重复投递”：
// 同一个 event_id 被写入两次，消费侧通过幂等表只执行一次。
func TestIntegrationKafkaBrokerDuplicateDeliveryIsIdempotent(t *testing.T) {
	brokers := kafkaBrokers()
	if len(brokers) == 0 {
		t.Skip("CLARAN_TEST_KAFKA_BROKERS 未设置，跳过真实 Kafka 集成测试")
	}
	topic := fmt.Sprintf("claran.it.%d", time.Now().UnixNano())
	groupID := fmt.Sprintf("claran-it-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := createKafkaTopic(ctx, brokers, topic); err != nil {
		t.Fatalf("create temp topic error = %v", err)
	}

	publisher := NewKafkaPublisher(brokers, "claran-it-publisher")
	defer publisher.Close()
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "conversation-it", map[string]string{"content": "kafka it"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	envelope.EventID = "kafka-it-event-" + fmt.Sprintf("%d", time.Now().UnixNano())
	// 覆盖 topic 映射：使用临时 topic，避免污染业务 topic。
	if err := publisher.writeRaw(ctx, topic, envelope.Key, mustEnvelopeBytes(t, envelope)); err != nil {
		t.Fatalf("publish first error = %v", err)
	}
	if err := publisher.writeRaw(ctx, topic, envelope.Key, mustEnvelopeBytes(t, envelope)); err != nil {
		t.Fatalf("publish duplicate error = %v", err)
	}

	consumer := newKafkaConsumerWithTopic(brokers, topic, groupID)
	defer consumer.Close()
	var handled int64
	store := NewMemoryReliabilityStore()
	handler := NewReliableHandler(store, "it-consumer", 3, func(context.Context, events.Envelope) error {
		atomic.AddInt64(&handled, 1)
		return nil
	})
	runCtx, runCancel := context.WithCancel(ctx)
	go consumer.Run(runCtx, handler)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&handled) >= 1 {
			// 再等一会儿，确认重复消息不会触发第二次处理。
			time.Sleep(3 * time.Second)
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	runCancel()
	if got := atomic.LoadInt64(&handled); got != 1 {
		t.Fatalf("handled = %d, want 1 (duplicate kafka delivery must be idempotent)", got)
	}
}

func kafkaBrokers() []string {
	raw := strings.TrimSpace(os.Getenv("CLARAN_TEST_KAFKA_BROKERS"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func mustEnvelopeBytes(t *testing.T, envelope events.Envelope) []byte {
	t.Helper()
	data, err := envelope.Bytes()
	if err != nil {
		t.Fatalf("envelope.Bytes() error = %v", err)
	}
	return data
}

// createKafkaTopic 在 broker 上创建临时 topic；本地 broker 关闭了自动建 topic。
func createKafkaTopic(ctx context.Context, brokers []string, topic string) error {
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerConn, err := kafka.DialContext(ctx, "tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()
	return controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
}
