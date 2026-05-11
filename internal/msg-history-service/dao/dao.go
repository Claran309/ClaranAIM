package dao

import (
	"ClaranAIM/internal/msg-history-service/model"
	"context"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	models := []interface{}{
		&model.MessageHistory{},
		&model.OfflineMessage{},
	}

	for _, m := range models {
		if db.Migrator().HasTable(m) {
			if err := db.Migrator().DropTable(m); err != nil {
				return nil, err
			}
		}
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}

	return db, nil
}

type HistoryRepository interface {
	SaveMessage(ctx context.Context, msg *model.MessageHistory) error
	GetHistoryByConversation(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error)
	SearchHistory(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.MessageHistory, error)
	AddOfflineMessage(ctx context.Context, userID, messageID int64) error
	GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error)
	MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error
	GetUnreadCount(ctx context.Context, userID int64) (int64, error)
}

type historyRepositoryImpl struct {
	db *gorm.DB
}

func NewHistoryRepo(db *gorm.DB) HistoryRepository {
	return &historyRepositoryImpl{db: db}
}

func (r *historyRepositoryImpl) SaveMessage(ctx context.Context, msg *model.MessageHistory) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *historyRepositoryImpl) GetHistoryByConversation(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error) {
	var msgs []model.MessageHistory
	query := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	err := query.Order("id DESC").Limit(int(limit)).Find(&msgs).Error
	return msgs, err
}

func (r *historyRepositoryImpl) SearchHistory(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.MessageHistory, error) {
	var msgs []model.MessageHistory
	err := r.db.WithContext(ctx).
		Where("conversation_id IN ?", conversationIDs).
		Where("content LIKE ?", "%"+keyword+"%").
		Order("id DESC").
		Limit(int(limit)).
		Find(&msgs).Error
	return msgs, err
}

func (r *historyRepositoryImpl) AddOfflineMessage(ctx context.Context, userID, messageID int64) error {
	offline := &model.OfflineMessage{
		UserID:    userID,
		MessageID: messageID,
		IsRead:    false,
	}
	return r.db.WithContext(ctx).Create(offline).Error
}

func (r *historyRepositoryImpl) GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error) {
	var msgs []model.OfflineMessage
	err := r.db.WithContext(ctx).Where("user_id = ? AND is_read = false", userID).Find(&msgs).Error
	return msgs, err
}

func (r *historyRepositoryImpl) MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.OfflineMessage{}).
		Where("user_id = ? AND message_id IN ?", userID, messageIDs).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error
}

func (r *historyRepositoryImpl) GetUnreadCount(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.OfflineMessage{}).
		Where("user_id = ? AND is_read = false", userID).
		Count(&count).Error
	return count, err
}
