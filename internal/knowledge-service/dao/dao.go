// Package dao 实现 knowledge-service 治理数据的持久化访问。
package dao

import (
	"ClaranAIM/internal/knowledge-service/model"
	"context"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 初始化 knowledge-service 自己拥有的审核表。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.GraphReviewCandidate{}); err != nil {
		return nil, err
	}
	return db, nil
}

// Repository 定义图谱候选审核需要的持久化能力。
type Repository interface {
	SaveCandidate(ctx context.Context, candidate *model.GraphReviewCandidate) error
	ListCandidates(ctx context.Context, filter CandidateFilter) ([]model.GraphReviewCandidate, int64, error)
	GetCandidate(ctx context.Context, id int64) (*model.GraphReviewCandidate, error)
	UpdateCandidateStatus(ctx context.Context, id, reviewerID int64, status, note string, reviewedAt time.Time) error
}

type CandidateFilter struct {
	OwnerID  int64
	Status   string
	ItemType string
	Limit    int
	Offset   int
}

type repositoryImpl struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repositoryImpl{db: db}
}

func (r *repositoryImpl) SaveCandidate(ctx context.Context, candidate *model.GraphReviewCandidate) error {
	return r.db.WithContext(ctx).Save(candidate).Error
}

func (r *repositoryImpl) ListCandidates(ctx context.Context, filter CandidateFilter) ([]model.GraphReviewCandidate, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.GraphReviewCandidate{})
	if filter.OwnerID > 0 {
		query = query.Where("owner_id = ?", filter.OwnerID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.ItemType != "" {
		query = query.Where("item_type = ?", filter.ItemType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var rows []model.GraphReviewCandidate
	err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(filter.Offset).Find(&rows).Error
	return rows, total, err
}

func (r *repositoryImpl) GetCandidate(ctx context.Context, id int64) (*model.GraphReviewCandidate, error) {
	var candidate model.GraphReviewCandidate
	err := r.db.WithContext(ctx).First(&candidate, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &candidate, err
}

func (r *repositoryImpl) UpdateCandidateStatus(ctx context.Context, id, reviewerID int64, status, note string, reviewedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.GraphReviewCandidate{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      status,
			"reviewer_id": reviewerID,
			"review_note": note,
			"reviewed_at": reviewedAt,
		}).Error
}
