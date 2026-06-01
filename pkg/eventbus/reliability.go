package eventbus

import (
	"ClaranAIM/pkg/events"
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
)

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
		processed, err := store.IsProcessed(ctx, key)
		if err != nil {
			return err
		}
		if processed {
			return nil
		}
		if err := next(ctx, envelope); err != nil {
			attempts, recordErr := store.RecordFailure(ctx, key, envelope, err)
			if recordErr != nil {
				return errors.Join(err, recordErr)
			}
			if attempts >= maxAttempts {
				if deadErr := store.MarkDeadLetter(ctx, key, envelope, err); deadErr != nil {
					return errors.Join(err, deadErr)
				}
				return nil
			}
			return err
		}
		return store.MarkProcessed(ctx, key, envelope)
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
