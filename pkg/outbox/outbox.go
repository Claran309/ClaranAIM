// Package outbox implements the transactional outbox pattern.
//
// Business services write Event rows in the same MySQL transaction as their
// domain changes. A background Worker later publishes due rows to Kafka and marks
// them published, which closes the crash gap between "DB committed" and "Kafka
// message sent".
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

const (
	// StatusPending means the event has not been published yet.
	StatusPending = "pending"
	// StatusPublished means the publisher accepted the event.
	StatusPublished = "published"
	// StatusRetrying means a previous publish attempt failed and should be retried.
	StatusRetrying = "retrying"
)

// Publisher is the event publication dependency used by Worker.
type Publisher interface {
	Publish(ctx context.Context, envelope events.Envelope) error
}

// Event is one row in event_outbox.
//
// Payload stores the full events.Envelope JSON. Status/RetryCount/NextRetryAt
// let workers publish asynchronously and retry with backoff without losing the
// relationship to the business aggregate.
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

// TableName keeps the outbox table name stable.
func (Event) TableName() string {
	return "event_outbox"
}

// BeforeCreate assigns a snowflake ID when the caller did not use envelope.EventID.
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

// NewEvent converts an event envelope into a pending outbox row.
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

// Envelope decodes the stored payload back into an event envelope.
func (e Event) Envelope() (events.Envelope, error) {
	return events.DecodeEnvelope([]byte(e.Payload))
}

// Store defines the persistence operations the worker needs.
type Store interface {
	FetchDue(ctx context.Context, limit int, lockFor time.Duration) ([]Event, error)
	MarkPublished(ctx context.Context, id int64) error
	MarkRetry(ctx context.Context, id int64, publishErr error) error
}

// GormStore is a MySQL/GORM-backed outbox store.
type GormStore struct {
	db *gorm.DB
}

// NewGormStore creates a GORM-backed outbox store.
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// AutoMigrate performs non-destructive migration for the outbox table.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Event{})
}

// Save inserts an outbox event outside an existing transaction.
func (s *GormStore) Save(ctx context.Context, event Event) error {
	return s.db.WithContext(ctx).Create(&event).Error
}

// SaveTx inserts an outbox event using the caller's business transaction.
func (s *GormStore) SaveTx(ctx context.Context, tx *gorm.DB, event Event) error {
	return tx.WithContext(ctx).Create(&event).Error
}

// FetchDue locks and returns pending/retrying events whose retry time has arrived.
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

// MarkPublished marks an event as successfully published.
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

// MarkRetry records a publish failure and schedules the next attempt.
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

func backoff(retryCount int) time.Duration {
	if retryCount < 1 {
		retryCount = 1
	}
	seconds := math.Min(float64(int64(1)<<uint(min(retryCount-1, 6))), 60)
	return time.Duration(seconds) * time.Second
}

// Worker polls the outbox and publishes events to the configured Publisher.
type Worker struct {
	store     Store
	publisher Publisher
	limit     int
	lockFor   time.Duration
	interval  time.Duration
}

// NewWorker creates an outbox worker with conservative default polling settings.
func NewWorker(store Store, publisher Publisher) *Worker {
	return &Worker{
		store:     store,
		publisher: publisher,
		limit:     50,
		lockFor:   30 * time.Second,
		interval:  time.Second,
	}
}

// Run processes due events until the context is canceled.
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

// ProcessOnce fetches one batch and attempts to publish every event in it.
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
			_ = w.store.MarkRetry(ctx, record.ID, fmt.Errorf("decode envelope: %w", err))
			continue
		}
		if err := w.publisher.Publish(ctx, envelope); err != nil {
			_ = w.store.MarkRetry(ctx, record.ID, err)
			continue
		}
		if err := w.store.MarkPublished(ctx, record.ID); err != nil {
			return err
		}
	}
	return nil
}
