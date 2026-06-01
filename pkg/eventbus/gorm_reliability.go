package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"time"

	"gorm.io/gorm"
)

// GormDeliveryRecord 是 processed_events 表模型。
// 表名使用通用名称，所有服务都可以独立迁移并按 consumer 字段隔离处理记录。
type GormDeliveryRecord struct {
	Key          string    `gorm:"primaryKey;size:160"`
	Consumer     string    `gorm:"size:80;index;not null"`
	EventID      string    `gorm:"size:80;index;not null"`
	EventType    string    `gorm:"size:80;index;not null"`
	EventKey     string    `gorm:"size:120;index"`
	State        string    `gorm:"size:20;index;not null"`
	Attempts     int       `gorm:"not null;default:0"`
	ErrorMessage string    `gorm:"type:text"`
	Payload      string    `gorm:"type:json"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

func (GormDeliveryRecord) TableName() string {
	return "processed_events"
}

// AutoMigrateReliabilityStore 创建 consumer 幂等和死信表。
func AutoMigrateReliabilityStore(db *gorm.DB) error {
	return db.AutoMigrate(&GormDeliveryRecord{})
}

// GormReliabilityStore 是基于 MySQL 的 consumer 幂等/DLQ 存储。
type GormReliabilityStore struct {
	db *gorm.DB
}

func NewGormReliabilityStore(db *gorm.DB) *GormReliabilityStore {
	return &GormReliabilityStore{db: db}
}

func (s *GormReliabilityStore) IsProcessed(ctx context.Context, key string) (bool, error) {
	var record GormDeliveryRecord
	err := s.db.WithContext(ctx).Where("`key` = ?", key).First(&record).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return record.State == DeliveryStateProcessed, nil
}

func (s *GormReliabilityStore) RecordFailure(ctx context.Context, key string, envelope events.Envelope, handleErr error) (int, error) {
	var attempts int
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record GormDeliveryRecord
		err := tx.Where("`key` = ?", key).First(&record).Error
		if err == gorm.ErrRecordNotFound {
			record = gormRecordFromEnvelope(key, envelope)
			record.State = DeliveryStateFailed
			record.Attempts = 1
			record.ErrorMessage = handleErr.Error()
			attempts = record.Attempts
			return tx.Create(&record).Error
		}
		if err != nil {
			return err
		}
		record.Attempts++
		record.State = DeliveryStateFailed
		record.ErrorMessage = handleErr.Error()
		record.Payload = envelopePayload(envelope)
		attempts = record.Attempts
		return tx.Save(&record).Error
	})
	return attempts, err
}

func (s *GormReliabilityStore) MarkProcessed(ctx context.Context, key string, envelope events.Envelope) error {
	record := gormRecordFromEnvelope(key, envelope)
	record.State = DeliveryStateProcessed
	return s.db.WithContext(ctx).
		Where("`key` = ?", key).
		Assign(map[string]interface{}{
			"consumer":      record.Consumer,
			"event_id":      record.EventID,
			"event_type":    record.EventType,
			"event_key":     record.EventKey,
			"state":         DeliveryStateProcessed,
			"error_message": "",
			"payload":       record.Payload,
		}).
		FirstOrCreate(&record).Error
}

func (s *GormReliabilityStore) MarkDeadLetter(ctx context.Context, key string, envelope events.Envelope, handleErr error) error {
	record := gormRecordFromEnvelope(key, envelope)
	record.State = DeliveryStateDead
	record.ErrorMessage = handleErr.Error()
	return s.db.WithContext(ctx).
		Where("`key` = ?", key).
		Assign(map[string]interface{}{
			"consumer":      record.Consumer,
			"event_id":      record.EventID,
			"event_type":    record.EventType,
			"event_key":     record.EventKey,
			"state":         DeliveryStateDead,
			"error_message": record.ErrorMessage,
			"payload":       record.Payload,
		}).
		FirstOrCreate(&record).Error
}

func gormRecordFromEnvelope(key string, envelope events.Envelope) GormDeliveryRecord {
	return GormDeliveryRecord{
		Key:       key,
		Consumer:  consumerFromKey(key),
		EventID:   envelope.EventID,
		EventType: envelope.Type,
		EventKey:  envelope.Key,
		Payload:   envelopePayload(envelope),
	}
}

func envelopePayload(envelope events.Envelope) string {
	payload, _ := envelope.Bytes()
	return string(payload)
}

func consumerFromKey(key string) string {
	for i, ch := range key {
		if ch == ':' {
			return key[:i]
		}
	}
	return key
}
