// Package dao owns memory-service persistence.
package dao

import (
	"ClaranAIM/internal/memory-service/model"
	"context"
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB opens MySQL and migrates memory tables without dropping data.
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.MemoryFact{}); err != nil {
		return nil, err
	}
	return db, nil
}

// MemoryFilter describes query constraints for memory facts.
type MemoryFilter struct {
	BotID           int64
	UserID          int64
	OwnerUserID     int64
	GroupID         int64
	ConversationID  int64
	SessionID       string
	Scopes          []string
	Types           []string
	IncludeDisabled bool
	Limit           int
	Offset          int
}

// MemoryRepository defines persistence operations used by the service layer.
type MemoryRepository interface {
	Create(ctx context.Context, fact *model.MemoryFact) error
	Update(ctx context.Context, fact *model.MemoryFact) error
	GetByID(ctx context.Context, id int64) (*model.MemoryFact, error)
	List(ctx context.Context, filter MemoryFilter) ([]model.MemoryFact, int64, error)
	Delete(ctx context.Context, id int64) error
	Touch(ctx context.Context, ids []int64, at time.Time) error
}

type memoryRepositoryImpl struct {
	db *gorm.DB
}

// NewMemoryRepo creates a GORM-backed repository.
func NewMemoryRepo(db *gorm.DB) MemoryRepository {
	return &memoryRepositoryImpl{db: db}
}

func (r *memoryRepositoryImpl) Create(ctx context.Context, fact *model.MemoryFact) error {
	return r.db.WithContext(ctx).Create(fact).Error
}

func (r *memoryRepositoryImpl) Update(ctx context.Context, fact *model.MemoryFact) error {
	return r.db.WithContext(ctx).Save(fact).Error
}

func (r *memoryRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.MemoryFact, error) {
	var fact model.MemoryFact
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&fact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &fact, err
}

func (r *memoryRepositoryImpl) List(ctx context.Context, filter MemoryFilter) ([]model.MemoryFact, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.MemoryFact{})
	query = applyFilter(query, filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var facts []model.MemoryFact
	err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(offset).Find(&facts).Error
	return facts, total, err
}

func (r *memoryRepositoryImpl) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.MemoryFact{}, id).Error
}

func (r *memoryRepositoryImpl) Touch(ctx context.Context, ids []int64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MemoryFact{}).Where("id IN ?", ids).Update("last_used_at", at).Error
}

func applyFilter(query *gorm.DB, filter MemoryFilter) *gorm.DB {
	if filter.BotID > 0 {
		query = query.Where("bot_id = ?", filter.BotID)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.OwnerUserID > 0 {
		query = query.Where("owner_user_id = ?", filter.OwnerUserID)
	}
	if filter.GroupID > 0 {
		query = query.Where("group_id = ?", filter.GroupID)
	}
	if filter.ConversationID > 0 {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.SessionID != "" {
		query = query.Where("session_id = ?", filter.SessionID)
	}
	if len(filter.Scopes) > 0 {
		query = query.Where("scope IN ?", filter.Scopes)
	}
	if len(filter.Types) > 0 {
		query = query.Where("type IN ?", filter.Types)
	}
	if !filter.IncludeDisabled {
		query = query.Where("enabled = ?", true)
	}
	return query
}
