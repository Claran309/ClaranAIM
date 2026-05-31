// Package dao 负责 memory-service 的持久化访问。
package dao

import (
	"ClaranAIM/internal/memory-service/model"
	"context"
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 打开 MySQL，并对记忆表执行非破坏性迁移。
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

// MemoryFilter 描述记忆事实查询条件。
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

// MemoryRepository 定义 service 层使用的记忆持久化操作。
type MemoryRepository interface {
	Create(ctx context.Context, fact *model.MemoryFact) error
	Update(ctx context.Context, fact *model.MemoryFact) error
	GetByID(ctx context.Context, id int64) (*model.MemoryFact, error)
	List(ctx context.Context, filter MemoryFilter) ([]model.MemoryFact, int64, error)
	Delete(ctx context.Context, id int64) error
	Touch(ctx context.Context, ids []int64, at time.Time) error
}

// memoryRepositoryImpl 是基于 GORM 的记忆仓储实现。
type memoryRepositoryImpl struct {
	db *gorm.DB
}

// NewMemoryRepo 创建基于 GORM 的记忆仓储。
func NewMemoryRepo(db *gorm.DB) MemoryRepository {
	return &memoryRepositoryImpl{db: db}
}

// Create 写入一条记忆事实；ID 由模型 hook 生成，DAO 不覆盖调用方传入的可见性和范围字段。
func (r *memoryRepositoryImpl) Create(ctx context.Context, fact *model.MemoryFact) error {
	return r.db.WithContext(ctx).Create(fact).Error
}

// Update 保存完整记忆对象，适合 service 层先做权限校验和字段归一化后整体落库。
func (r *memoryRepositoryImpl) Update(ctx context.Context, fact *model.MemoryFact) error {
	return r.db.WithContext(ctx).Save(fact).Error
}

// GetByID 按主键读取记忆；不存在时返回 nil,nil，让 service 层决定是 404 还是忽略。
func (r *memoryRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.MemoryFact, error) {
	var fact model.MemoryFact
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&fact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &fact, err
}

// List 根据范围、类型和可用状态过滤记忆，并返回分页总数。
// 默认分页上限控制在 100 条以内，避免 Agent 召回时一次性把长期记忆全部注入 prompt。
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

// Delete 使用 GORM 软删除记忆，保留审计和未来恢复空间。
func (r *memoryRepositoryImpl) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.MemoryFact{}, id).Error
}

// Touch 批量更新最后使用时间，用于记录哪些记忆被 Agent 召回过。
func (r *memoryRepositoryImpl) Touch(ctx context.Context, ids []int64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MemoryFact{}).Where("id IN ?", ids).Update("last_used_at", at).Error
}

// applyFilter 把跨服务 DTO 转成 GORM 条件。
// IncludeDisabled=false 是默认安全行为，避免已被用户关闭的记忆继续参与 Agent 个性化。
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
