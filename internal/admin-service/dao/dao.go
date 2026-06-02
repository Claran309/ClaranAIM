// Package dao 实现 admin-service 管理域数据的持久化访问。
package dao

import (
	"ClaranAIM/internal/admin-service/model"
	"context"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 初始化管理后台自己的数据表。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.SystemNotice{}, &model.AdminAuditLog{}); err != nil {
		return nil, err
	}
	return db, nil
}

// Repository 定义 admin-service 自有管理数据的访问能力。
type Repository interface {
	SaveNotice(ctx context.Context, notice *model.SystemNotice) error
	ListNotices(ctx context.Context, includeDisabled bool, limit, offset int) ([]model.SystemNotice, int64, error)
	GetNotice(ctx context.Context, id int64) (*model.SystemNotice, error)
	CreateAudit(ctx context.Context, log *model.AdminAuditLog) error
	ListAuditLogs(ctx context.Context, filter AuditFilter) ([]model.AdminAuditLog, int64, error)
}

type AuditFilter struct {
	Action     string
	TargetType string
	Limit      int
	Offset     int
}

type repositoryImpl struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repositoryImpl{db: db}
}

func (r *repositoryImpl) SaveNotice(ctx context.Context, notice *model.SystemNotice) error {
	return r.db.WithContext(ctx).Save(notice).Error
}

func (r *repositoryImpl) ListNotices(ctx context.Context, includeDisabled bool, limit, offset int) ([]model.SystemNotice, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.SystemNotice{})
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit, offset = normalizePage(limit, offset)
	var rows []model.SystemNotice
	err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *repositoryImpl) GetNotice(ctx context.Context, id int64) (*model.SystemNotice, error) {
	var notice model.SystemNotice
	err := r.db.WithContext(ctx).First(&notice, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &notice, err
}

func (r *repositoryImpl) CreateAudit(ctx context.Context, log *model.AdminAuditLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *repositoryImpl) ListAuditLogs(ctx context.Context, filter AuditFilter) ([]model.AdminAuditLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.AdminAuditLog{})
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.TargetType != "" {
		query = query.Where("target_type = ?", filter.TargetType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit, offset := normalizePage(filter.Limit, filter.Offset)
	var rows []model.AdminAuditLog
	err := query.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
