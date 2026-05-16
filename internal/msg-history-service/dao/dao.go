package dao

import (
	"ClaranAIM/internal/msg-history-service/model"
	"context"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 初始化数据库连接并自动迁移表结构
// AutoMigrate 只做非破坏性迁移，避免服务重启清空已有数据。
// dsn: MySQL数据源连接字符串
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// 需要自动迁移的模型列表
	models := []interface{}{
		&model.MessageHistory{},
		&model.OfflineMessage{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}

	return db, nil
}

// HistoryRepository 消息历史数据访问接口
// 定义了消息历史服务所需的所有数据库操作
type HistoryRepository interface {
	// SaveMessage 保存消息到历史记录
	SaveMessage(ctx context.Context, msg *model.MessageHistory) error
	// GetHistoryByConversation 按会话查询历史消息（支持游标分页）
	GetHistoryByConversation(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error)
	// SearchHistory 在指定会话中搜索历史消息
	SearchHistory(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.MessageHistory, error)
	// AddOfflineMessage 添加离线消息记录（用户未读消息）
	AddOfflineMessage(ctx context.Context, userID, messageID int64) error
	// GetOfflineMessages 获取用户的未读离线消息列表
	GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error)
	// MarkOfflineRead 批量标记离线消息为已读
	MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error
	// GetUnreadCount 获取用户的未读消息总数
	GetUnreadCount(ctx context.Context, userID int64) (int64, error)
}

type historyRepositoryImpl struct {
	db *gorm.DB
}

func NewHistoryRepo(db *gorm.DB) HistoryRepository {
	return &historyRepositoryImpl{db: db}
}

// SaveMessage 保存消息到历史记录表
// 消息归档存储，用于长期历史查询
func (r *historyRepositoryImpl) SaveMessage(ctx context.Context, msg *model.MessageHistory) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// GetHistoryByConversation 按会话查询历史消息
// 使用游标分页：beforeID > 0 时只查询ID小于beforeID的消息
// 按 ID DESC 排序，取最新的 limit 条
func (r *historyRepositoryImpl) GetHistoryByConversation(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error) {
	var msgs []model.MessageHistory
	query := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	err := query.Order("id DESC").Limit(int(limit)).Find(&msgs).Error
	return msgs, err
}

// SearchHistory 在指定会话列表中搜索历史消息
// 使用 LIKE 模糊匹配，按时间倒序排列
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

// AddOfflineMessage 添加离线消息记录
// 当接收者不在线时，将消息记录到离线消息表
// 用户上线后可查询此表获取未读消息
func (r *historyRepositoryImpl) AddOfflineMessage(ctx context.Context, userID, messageID int64) error {
	offline := &model.OfflineMessage{
		UserID:    userID,
		MessageID: messageID,
		IsRead:    false,
	}
	return r.db.WithContext(ctx).Create(offline).Error
}

// GetOfflineMessages 获取用户的未读离线消息
// 只返回 is_read = false 的记录
func (r *historyRepositoryImpl) GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error) {
	var msgs []model.OfflineMessage
	err := r.db.WithContext(ctx).Where("user_id = ? AND is_read = false", userID).Find(&msgs).Error
	return msgs, err
}

// MarkOfflineRead 批量标记离线消息为已读
// 同时记录已读时间，用于消息已读回执
func (r *historyRepositoryImpl) MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&model.OfflineMessage{}).
		Where("user_id = ? AND message_id IN ?", userID, messageIDs).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error
}

// GetUnreadCount 获取用户的未读消息总数
// 用于前端显示未读消息角标
func (r *historyRepositoryImpl) GetUnreadCount(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.OfflineMessage{}).
		Where("user_id = ? AND is_read = false", userID).
		Count(&count).Error
	return count, err
}
