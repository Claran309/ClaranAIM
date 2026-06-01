// Package outbox 实现事务 Outbox 模式。
// 业务服务在同一个 MySQL 事务里同时写业务表和 event_outbox 表；后台 Worker 之后再把待发布事件投递到 Kafka。
// 这样即使服务在“数据库已提交、Kafka 尚未发送”的间隙崩溃，事件仍留在数据库里，可以由 Worker 重试补发。
package outbox

import (
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/idgen"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Outbox 发布状态机。
// pending/retrying 会被 Worker 扫描并投递到 Kafka，published 表示事件已经成功发布，不能再被普通轮询取走。
const (
	// StatusPending 表示事件还没有发布到 Kafka。
	StatusPending = "pending"
	// StatusPublished 表示发布器已接受该事件。
	StatusPublished = "published"
	// StatusRetrying 表示上次发布失败，等待下一次退避重试。
	StatusRetrying = "retrying"
	// StatusDead 表示事件连续发布失败超过上限，需要人工排查或显式重放。
	StatusDead = "dead"
)

// Publisher 是 Worker 依赖的事件发布接口，通常由 KafkaPublisher 实现。
type Publisher interface {
	Publish(ctx context.Context, envelope events.Envelope) error
}

// Event 对应 event_outbox 表中的一行待发布事件。
// Payload 保存完整 events.Envelope JSON；Status、RetryCount、NextRetryAt 负责异步发布和退避重试；
// AggregateType/AggregateID 保留事件与业务聚合的关联，便于排查、重放和审计。
type Event struct {
	ID            int64      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	AggregateType string     `json:"aggregate_type" gorm:"size:50;not null;index:idx_outbox_aggregate,priority:1"`
	AggregateID   int64      `json:"aggregate_id" gorm:"not null;index:idx_outbox_aggregate,priority:2"`
	EventType     string     `json:"event_type" gorm:"size:100;not null"`
	EventKey      string     `json:"event_key" gorm:"size:100;not null"`
	Payload       string     `json:"payload" gorm:"type:json;not null"`
	Status        string     `json:"status" gorm:"size:20;not null;default:pending;index:idx_outbox_status_next_retry,priority:1"`
	RetryCount    int        `json:"retry_count" gorm:"not null;default:0"`
	LastError     string     `json:"last_error" gorm:"type:text"`
	NextRetryAt   time.Time  `json:"next_retry_at" gorm:"not null;index:idx_outbox_status_next_retry,priority:2"`
	LockedUntil   *time.Time `json:"locked_until" gorm:"index"`
	PublishedAt   *time.Time `json:"published_at"`
	CreatedAt     time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 固定 Outbox 表名，避免 GORM 按结构体名推导出不一致的表名。
func (Event) TableName() string {
	return "event_outbox"
}

// BeforeCreate 在调用方未指定 ID 时补充分布式雪花 ID。
func (e *Event) BeforeCreate(tx *gorm.DB) error {
	if e.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	if err != nil {
		return err
	}
	e.ID = id
	return nil
}

// NewEvent 将统一事件 Envelope 转换成待发布 Outbox 行。
// 如果 Envelope 没有事件 ID，这里会补齐 ID，并把事件初始状态设置为 pending。
func NewEvent(aggregateType string, aggregateID int64, envelope events.Envelope) (Event, error) {
	if aggregateType == "" {
		return Event{}, errors.New("aggregate type is empty")
	}
	if envelope.Type == "" {
		return Event{}, errors.New("event type is empty")
	}
	if envelope.Key == "" {
		return Event{}, errors.New("event key is empty")
	}
	if envelope.EventID == "" {
		id, err := idgen.NextID()
		if err != nil {
			return Event{}, err
		}
		envelope.EventID = strconv.FormatInt(id, 10)
	}
	payload, err := envelope.Bytes()
	if err != nil {
		return Event{}, err
	}
	id, err := strconv.ParseInt(envelope.EventID, 10, 64)
	if err != nil {
		id, err = idgen.NextID()
		if err != nil {
			return Event{}, err
		}
	}
	now := time.Now()
	return Event{
		ID:            id,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     envelope.Type,
		EventKey:      envelope.Key,
		Payload:       string(payload),
		Status:        StatusPending,
		NextRetryAt:   now,
	}, nil
}

// Envelope 将数据库中保存的 JSON payload 还原为统一事件 Envelope。
func (e Event) Envelope() (events.Envelope, error) {
	return events.DecodeEnvelope([]byte(e.Payload))
}

// Store 定义 Worker 需要的持久化操作。
// 具体实现可以是 GORM/MySQL，也可以在测试中替换为内存实现。
type Store interface {
	FetchDue(ctx context.Context, limit int, lockFor time.Duration) ([]Event, error)
	MarkPublished(ctx context.Context, id int64) error
	MarkRetry(ctx context.Context, id int64, publishErr error) error
	MarkDead(ctx context.Context, id int64, publishErr error) error
	Requeue(ctx context.Context, id int64) error
}

// GormStore 是基于 MySQL/GORM 的 Outbox 存储实现。
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 创建基于 GORM 的 Outbox 存储。
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// AutoMigrate 对 Outbox 表做非破坏性迁移，不删除已有事件。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Event{})
}

// Save 在没有外部业务事务时直接写入 Outbox 事件。
func (s *GormStore) Save(ctx context.Context, event Event) error {
	return s.db.WithContext(ctx).Create(&event).Error
}

// SaveTx 使用调用方传入的业务事务写入 Outbox 事件。
// 这是保证“业务数据和事件记录同时提交或同时回滚”的关键入口。
func (s *GormStore) SaveTx(ctx context.Context, tx *gorm.DB, event Event) error {
	return tx.WithContext(ctx).Create(&event).Error
}

// FetchDue 锁定并返回已到重试时间的 pending/retrying 事件。
// locked_until 用于降低多个 Worker 并发时重复发布同一条事件的概率。
func (s *GormStore) FetchDue(ctx context.Context, limit int, lockFor time.Duration) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	now := time.Now()
	lockedUntil := now.Add(lockFor)
	var selected []Event
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []int64
		if err := tx.Model(&Event{}).
			Where("status IN ? AND next_retry_at <= ? AND (locked_until IS NULL OR locked_until < ?)", []string{StatusPending, StatusRetrying}, now, now).
			Order("next_retry_at ASC, id ASC").
			Limit(limit).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Model(&Event{}).
			Where("id IN ?", ids).
			Updates(map[string]interface{}{
				"locked_until": lockedUntil,
				"updated_at":   now,
			}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Order("id ASC").Find(&selected).Error
	})
	return selected, err
}

// MarkPublished 将事件标记为已发布，并清理锁定状态和上次错误。
func (s *GormStore) MarkPublished(ctx context.Context, id int64) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&Event{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       StatusPublished,
			"published_at": now,
			"locked_until": nil,
			"last_error":   "",
			"updated_at":   now,
		}).Error
}

// MarkRetry 记录发布失败原因，并根据重试次数安排下一次退避重试。
func (s *GormStore) MarkRetry(ctx context.Context, id int64, publishErr error) error {
	now := time.Now()
	var event Event
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&event).Error; err != nil {
			return err
		}
		nextRetry := now.Add(backoff(event.RetryCount + 1))
		return tx.Model(&Event{}).Where("id = ?", id).Updates(map[string]interface{}{
			"status":        StatusRetrying,
			"retry_count":   event.RetryCount + 1,
			"next_retry_at": nextRetry,
			"locked_until":  nil,
			"last_error":    publishErr.Error(),
			"updated_at":    now,
		}).Error
	})
}

// MarkDead 将事件移入死信状态。dead 事件不会再被普通 Worker 扫描，避免长期故障事件无限占用发布循环。
func (s *GormStore) MarkDead(ctx context.Context, id int64, publishErr error) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&Event{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       StatusDead,
		"locked_until": nil,
		"last_error":   publishErr.Error(),
		"updated_at":   now,
	}).Error
}

// Requeue 将 dead/published/retrying 事件显式放回 pending 队列，用于人工修复配置或 Kafka 后重放。
func (s *GormStore) Requeue(ctx context.Context, id int64) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&Event{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        StatusPending,
		"retry_count":   0,
		"next_retry_at": now,
		"locked_until":  nil,
		"last_error":    "",
		"published_at":  nil,
		"updated_at":    now,
	}).Error
}

// backoff 根据重试次数计算指数退避时间，上限控制在 60 秒，避免故障时过度打爆 Kafka 或网络。
func backoff(retryCount int) time.Duration {
	if retryCount < 1 {
		retryCount = 1
	}
	seconds := math.Min(float64(int64(1)<<uint(min(retryCount-1, 6))), 60)
	return time.Duration(seconds) * time.Second
}

// Worker 轮询 Outbox 表并把到期事件发布到配置的 Publisher。
// 它可以在各业务服务内启动，也可以日后拆成独立发布进程。
type Worker struct {
	store      Store
	publisher  Publisher
	limit      int
	lockFor    time.Duration
	interval   time.Duration
	maxRetries int
}

// NewWorker 创建带保守默认参数的 Outbox Worker。
func NewWorker(store Store, publisher Publisher) *Worker {
	return &Worker{
		store:      store,
		publisher:  publisher,
		limit:      50,
		lockFor:    30 * time.Second,
		interval:   time.Second,
		maxRetries: 10,
	}
}

// SetMaxRetries 设置单条事件进入 dead 状态前允许的最大发布尝试次数。
func (w *Worker) SetMaxRetries(maxRetries int) {
	if maxRetries > 0 {
		w.maxRetries = maxRetries
	}
}

// Run 持续处理到期事件，直到 context 被取消。
func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.store == nil || w.publisher == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("outbox处理失败: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOnce 拉取一批到期事件并逐条尝试发布。
// 单条事件失败只会进入 retry，不会阻断同批次其他事件继续处理。
func (w *Worker) ProcessOnce(ctx context.Context) error {
	if w == nil || w.store == nil || w.publisher == nil {
		return nil
	}
	records, err := w.store.FetchDue(ctx, w.limit, w.lockFor)
	if err != nil {
		return err
	}
	for _, record := range records {
		envelope, err := record.Envelope()
		if err != nil {
			decodeErr := fmt.Errorf("decode envelope: %w", err)
			if record.RetryCount+1 >= w.maxRetries {
				_ = w.store.MarkDead(ctx, record.ID, decodeErr)
				continue
			}
			_ = w.store.MarkRetry(ctx, record.ID, decodeErr)
			continue
		}
		if err := w.publisher.Publish(ctx, envelope); err != nil {
			if record.RetryCount+1 >= w.maxRetries {
				_ = w.store.MarkDead(ctx, record.ID, err)
				continue
			}
			_ = w.store.MarkRetry(ctx, record.ID, err)
			continue
		}
		if err := w.store.MarkPublished(ctx, record.ID); err != nil {
			return err
		}
	}
	return nil
}
