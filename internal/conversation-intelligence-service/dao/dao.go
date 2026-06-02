// Package dao 实现 conversation-intelligence-service 的持久化访问。
package dao

import (
	"ClaranAIM/internal/conversation-intelligence-service/model"
	"context"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 初始化数据库并迁移会话智能归档表。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.DigestJob{}, &model.ConversationArtifact{}, &model.ConversationActivityCursor{}); err != nil {
		return nil, err
	}
	return db, nil
}

// Repository 定义 service 层需要的持久化能力。
type Repository interface {
	CreateJob(ctx context.Context, job *model.DigestJob) error
	UpdateJob(ctx context.Context, job *model.DigestJob) error
	GetJob(ctx context.Context, id int64) (*model.DigestJob, error)
	ListJobs(ctx context.Context, filter JobFilter) ([]model.DigestJob, int64, error)
	ListRetryableJobs(ctx context.Context, options RetryableJobOptions) ([]model.DigestJob, error)
	CreateArtifacts(ctx context.Context, artifacts []model.ConversationArtifact) error
	ListArtifacts(ctx context.Context, filter ArtifactFilter) ([]model.ConversationArtifact, int64, error)
	UpsertActivity(ctx context.Context, activity ActivityUpsert) error
	ListDueActivities(ctx context.Context, options DueActivityOptions) ([]model.ConversationActivityCursor, error)
	MarkActivityDigested(ctx context.Context, cursorID, jobID int64, digestedAt time.Time) error
}

// JobFilter 描述归档任务状态页的查询条件。
type JobFilter struct {
	ConversationID int64
	ViewerID       int64
	Status         string
	Limit          int
	Offset         int
}

// RetryableJobOptions 描述调度器筛选失败重试任务的条件。
type RetryableJobOptions struct {
	Limit int
	Now   time.Time
}

// ArtifactFilter 描述产物列表查询条件。
type ArtifactFilter struct {
	ConversationID int64
	ViewerID       int64
	ArtifactType   string
	Limit          int
	Offset         int
}

// ActivityUpsert 描述一次消息事件对会话活跃游标的推进。
type ActivityUpsert struct {
	ConversationID    int64
	ViewerID          int64
	AgentID           int64
	MessageID         int64
	MessageCountDelta int
	OccurredAt        time.Time
}

// DueActivityOptions 描述调度器筛选待归档活跃会话的条件。
type DueActivityOptions struct {
	MessageThreshold int
	WindowMinutes    int
	Limit            int
	Now              time.Time
}

type repositoryImpl struct {
	db *gorm.DB
}

// NewRepository 创建 GORM 仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repositoryImpl{db: db}
}

func (r *repositoryImpl) CreateJob(ctx context.Context, job *model.DigestJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *repositoryImpl) UpdateJob(ctx context.Context, job *model.DigestJob) error {
	return r.db.WithContext(ctx).Save(job).Error
}

func (r *repositoryImpl) GetJob(ctx context.Context, id int64) (*model.DigestJob, error) {
	var job model.DigestJob
	err := r.db.WithContext(ctx).First(&job, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &job, err
}

func (r *repositoryImpl) ListJobs(ctx context.Context, filter JobFilter) ([]model.DigestJob, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.DigestJob{})
	if filter.ViewerID > 0 {
		query = query.Where("viewer_id = ?", filter.ViewerID)
	}
	if filter.ConversationID > 0 {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var jobs []model.DigestJob
	err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(filter.Offset).Find(&jobs).Error
	return jobs, total, err
}

func (r *repositoryImpl) ListRetryableJobs(ctx context.Context, options RetryableJobOptions) ([]model.DigestJob, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	limit := options.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var jobs []model.DigestJob
	err := r.db.WithContext(ctx).
		Where("status = ? AND retry_count < max_retries AND (next_run_at IS NULL OR next_run_at <= ?)", model.JobStatusFailed, now).
		Order("next_run_at ASC, updated_at ASC").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

func (r *repositoryImpl) CreateArtifacts(ctx context.Context, artifacts []model.ConversationArtifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&artifacts).Error
}

func (r *repositoryImpl) ListArtifacts(ctx context.Context, filter ArtifactFilter) ([]model.ConversationArtifact, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ConversationArtifact{})
	if filter.ConversationID > 0 {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.ViewerID > 0 {
		query = query.Where("viewer_id = ?", filter.ViewerID)
	}
	if filter.ArtifactType != "" {
		query = query.Where("artifact_type = ?", filter.ArtifactType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var artifacts []model.ConversationArtifact
	err := query.Order("id DESC").Limit(limit).Offset(filter.Offset).Find(&artifacts).Error
	return artifacts, total, err
}

func (r *repositoryImpl) UpsertActivity(ctx context.Context, activity ActivityUpsert) error {
	now := activity.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	delta := activity.MessageCountDelta
	if delta <= 0 {
		delta = 1
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cursor model.ConversationActivityCursor
		err := tx.Where("conversation_id = ? AND viewer_id = ?", activity.ConversationID, activity.ViewerID).First(&cursor).Error
		if err == gorm.ErrRecordNotFound {
			cursor = model.ConversationActivityCursor{
				ConversationID:  activity.ConversationID,
				ViewerID:        activity.ViewerID,
				AgentID:         activity.AgentID,
				FirstMessageID:  activity.MessageID,
				LastMessageID:   activity.MessageID,
				PendingMessages: delta,
				WindowStartedAt: now,
				LastActivityAt:  now,
			}
			return tx.Create(&cursor).Error
		}
		if err != nil {
			return err
		}
		if cursor.FirstMessageID == 0 || activity.MessageID < cursor.FirstMessageID {
			cursor.FirstMessageID = activity.MessageID
		}
		if activity.MessageID > cursor.LastMessageID {
			cursor.LastMessageID = activity.MessageID
		}
		if activity.AgentID > 0 {
			cursor.AgentID = activity.AgentID
		}
		if cursor.WindowStartedAt.IsZero() {
			cursor.WindowStartedAt = now
		}
		cursor.LastActivityAt = now
		cursor.PendingMessages += delta
		return tx.Save(&cursor).Error
	})
}

func (r *repositoryImpl) ListDueActivities(ctx context.Context, options DueActivityOptions) ([]model.ConversationActivityCursor, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	threshold := options.MessageThreshold
	if threshold <= 0 {
		threshold = 100
	}
	windowMinutes := options.WindowMinutes
	if windowMinutes <= 0 {
		windowMinutes = 60
	}
	limit := options.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	deadline := now.Add(-time.Duration(windowMinutes) * time.Minute)
	var cursors []model.ConversationActivityCursor
	err := r.db.WithContext(ctx).
		Where("pending_messages >= ? OR window_started_at <= ?", threshold, deadline).
		Order("last_activity_at ASC").
		Limit(limit).
		Find(&cursors).Error
	return cursors, err
}

func (r *repositoryImpl) MarkActivityDigested(ctx context.Context, cursorID, jobID int64, digestedAt time.Time) error {
	if digestedAt.IsZero() {
		digestedAt = time.Now()
	}
	return r.db.WithContext(ctx).Model(&model.ConversationActivityCursor{}).Where("id = ?", cursorID).Updates(map[string]interface{}{
		"pending_messages":   0,
		"first_message_id":   0,
		"last_message_id":    0,
		"window_started_at":  digestedAt,
		"last_digest_job_id": jobID,
		"last_digest_at":     digestedAt,
	}).Error
}
