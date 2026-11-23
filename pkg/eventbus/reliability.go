package eventbus

import (
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/observability"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const defaultConsumerMaxAttempts = 5

// DeliveryState 表示消费者处理某条事件的最终状态。
const (
	DeliveryStateProcessed = "processed"
	DeliveryStateFailed    = "failed"
	DeliveryStateDead      = "dead"
	// DeliveryStateProcessing 表示某个 consumer 实例已抢占该事件。
	// 它用于多实例竞争：抢占成功后其他实例必须跳过，避免同一事件被广播两次。
	DeliveryStateProcessing = "processing"
)

// staleProcessingWindow 是 processing 状态的租约。
// 实例在抢占后崩溃会留下 processing 记录，超过该窗口后允许其他实例重新抢占。
const staleProcessingWindow = 2 * time.Minute

// DeliveryRecord 是 consumer 侧的可靠性事实。
// 同一 consumer_group + event_id 只应成功处理一次；失败达到阈值后进入死信，避免卡住 Kafka 分区。
type DeliveryRecord struct {
	Key          string
	Consumer     string
	EventID      string
	EventType    string
	EventKey     string
	State        string
	Attempts     int
	ErrorMessage string
	Payload      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ReliabilityStore 保存消费者幂等记录和死信记录。
// 生产实现可以落 MySQL；测试和无 DB 的网关可使用内存实现。
type ReliabilityStore interface {
	IsProcessed(ctx context.Context, key string) (bool, error)
	// TryClaim 原子抢占一条事件。返回 true 表示当前实例负责处理；
	// false 表示已被其他实例处理、正在处理或已进入死信。
	TryClaim(ctx context.Context, key string, envelope events.Envelope) (bool, error)
	RecordFailure(ctx context.Context, key string, envelope events.Envelope, err error) (int, error)
	MarkProcessed(ctx context.Context, key string, envelope events.Envelope) error
	MarkDeadLetter(ctx context.Context, key string, envelope events.Envelope, err error) error
}

// ConsumerEventKey 生成 consumer 维度幂等键。
func ConsumerEventKey(consumer string, envelope events.Envelope) string {
	eventID := envelope.EventID
	if eventID == "" {
		eventID = fmt.Sprintf("%s:%s", envelope.Type, envelope.Key)
	}
	return consumer + ":" + eventID
}

// NewReliableHandler 为普通事件处理器增加幂等、失败计数和死信移交。
// 返回 nil 表示 Kafka offset 可以提交；返回错误表示仍希望 Kafka 稍后重投。
func NewReliableHandler(store ReliabilityStore, consumer string, maxAttempts int, next Handler) Handler {
	if maxAttempts <= 0 {
		maxAttempts = defaultConsumerMaxAttempts
	}
	return func(ctx context.Context, envelope events.Envelope) error {
		if next == nil {
			return nil
		}
		if store == nil {
			return next(ctx, envelope)
		}
		key := ConsumerEventKey(consumer, envelope)
		claimed, err := store.TryClaim(ctx, key, envelope)
		if err != nil {
			return err
		}
		if !claimed {
			observability.RecordBusinessEvent("kafka_consumer", consumer, "already_processed")
			return nil
		}
		if err := next(ctx, envelope); err != nil {
			attempts, recordErr := store.RecordFailure(ctx, key, envelope, err)
			if recordErr != nil {
				observability.RecordBusinessEvent("kafka_consumer", consumer, "record_failure_error")
				return errors.Join(err, recordErr)
			}
			if attempts >= maxAttempts {
				if deadErr := store.MarkDeadLetter(ctx, key, envelope, err); deadErr != nil {
					observability.RecordBusinessEvent("kafka_consumer", consumer, "dead_letter_error")
					return errors.Join(err, deadErr)
				}
				observability.RecordBusinessEvent("kafka_consumer", consumer, "dead")
				return nil
			}
			observability.RecordBusinessEvent("kafka_consumer", consumer, "failed")
			return err
		}
		if err := store.MarkProcessed(ctx, key, envelope); err != nil {
			observability.RecordBusinessEvent("kafka_consumer", consumer, "mark_processed_error")
			return err
		}
		observability.RecordBusinessEvent("kafka_consumer", consumer, "success")
		return nil
	}
}

// MemoryReliabilityStore 是进程内可靠性存储，适合测试或无数据库的开发环境。
type MemoryReliabilityStore struct {
	mu      sync.Mutex
	records map[string]DeliveryRecord
}

// NewMemoryReliabilityStore 创建内存可靠性存储。
func NewMemoryReliabilityStore() *MemoryReliabilityStore {
	return &MemoryReliabilityStore{records: map[string]DeliveryRecord{}}
}

func (s *MemoryReliabilityStore) IsProcessed(ctx context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	return ok && record.State == DeliveryStateProcessed, nil
}

// TryClaim 在内存实现中同样保证同一 key 只有一个调用方能拿到处理权。
func (s *MemoryReliabilityStore) TryClaim(ctx context.Context, key string, envelope events.Envelope) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	record, ok := s.records[key]
	if ok {
		switch record.State {
		case DeliveryStateProcessed, DeliveryStateDead:
			return false, nil
		case DeliveryStateFailed:
			record.State = DeliveryStateProcessing
			record.UpdatedAt = now
			s.records[key] = record
			return true, nil
		case DeliveryStateProcessing:
			if now.Sub(record.UpdatedAt) < staleProcessingWindow {
				return false, nil
			}
			record.UpdatedAt = now
			s.records[key] = record
			return true, nil
		}
	}
	record = newDeliveryRecord(key, envelope)
	record.State = DeliveryStateProcessing
	// 抢占本身不算一次处理尝试，Attempts 只由 RecordFailure 累加。
	record.Attempts = 0
	record.UpdatedAt = now
	s.records[key] = record
	return true, nil
}

func (s *MemoryReliabilityStore) RecordFailure(ctx context.Context, key string, envelope events.Envelope, err error) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[key]
	if record.CreatedAt.IsZero() {
		record = newDeliveryRecord(key, envelope)
	}
	record.Attempts++
	record.State = DeliveryStateFailed
	record.ErrorMessage = err.Error()
	record.UpdatedAt = time.Now()
	s.records[key] = record
	return record.Attempts, nil
}

func (s *MemoryReliabilityStore) MarkProcessed(ctx context.Context, key string, envelope events.Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[key]
	if record.CreatedAt.IsZero() {
		record = newDeliveryRecord(key, envelope)
	}
	record.State = DeliveryStateProcessed
	record.ErrorMessage = ""
	record.UpdatedAt = time.Now()
	s.records[key] = record
	return nil
}

func (s *MemoryReliabilityStore) MarkDeadLetter(ctx context.Context, key string, envelope events.Envelope, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[key]
	if record.CreatedAt.IsZero() {
		record = newDeliveryRecord(key, envelope)
	}
	record.State = DeliveryStateDead
	record.ErrorMessage = err.Error()
	record.UpdatedAt = time.Now()
	s.records[key] = record
	return nil
}

// DeadLetter 返回某条死信记录，供测试和本地排查使用。
func (s *MemoryReliabilityStore) DeadLetter(key string) (DeliveryRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	return record, ok && record.State == DeliveryStateDead
}

func newDeliveryRecord(key string, envelope events.Envelope) DeliveryRecord {
	payload, _ := envelope.Bytes()
	now := time.Now()
	return DeliveryRecord{
		Key:       key,
		EventID:   envelope.EventID,
		EventType: envelope.Type,
		EventKey:  envelope.Key,
		Payload:   string(payload),
		CreatedAt: now,
		UpdatedAt: now,
	}
}
