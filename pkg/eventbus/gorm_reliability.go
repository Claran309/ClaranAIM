package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// TryClaim 用主键唯一约束实现跨实例原子抢占。
// 首次插入成功即拿到处理权；已存在时只允许 failed 立即重试，
// 或 processing 超过租约窗口后由其他实例接管。
func (s *GormReliabilityStore) TryClaim(ctx context.Context, key string, envelope events.Envelope) (bool, error) {
	record := gormRecordFromEnvelope(key, envelope)
	record.State = DeliveryStateProcessing
	record.Attempts = 0
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 1 {
		return true, nil
	}
	staleBefore := time.Now().Add(-staleProcessingWindow)
	claim := s.db.WithContext(ctx).Model(&GormDeliveryRecord{}).
		Where("`key` = ? AND (state = ? OR (state = ? AND updated_at < ?))",
			key, DeliveryStateFailed, DeliveryStateProcessing, staleBefore).
		Updates(map[string]interface{}{
			"state":      DeliveryStateProcessing,
			"updated_at": time.Now(),
		})
	if claim.Error != nil {
		return false, claim.Error
	}
	return claim.RowsAffected == 1, nil
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
