package dao

import (
	"ClaranAIM/internal/msg-core-service/model"
	"context"
	"errors"
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
		&model.Conversation{},
		&model.ConversationParticipant{},
		&model.Message{},
		&model.MessageEditRecord{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}

	return db, nil
}

// MessageRepository 消息数据访问接口
// 定义了消息核心服务所需的所有数据库操作
type MessageRepository interface {
	// CreateConversation 创建新会话
	CreateConversation(ctx context.Context, conv *model.Conversation) error
	// GetConversationByID 根据ID查询会话信息
	GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error)
	// UpdateConversation 更新会话信息（主要用于更新UpdatedAt时间戳）
	UpdateConversation(ctx context.Context, conv *model.Conversation) error
	// AddParticipant 添加会话参与者
	AddParticipant(ctx context.Context, p *model.ConversationParticipant) error
	// GetParticipants 获取会话的所有参与者
	GetParticipants(ctx context.Context, conversationID int64) ([]model.ConversationParticipant, error)
	// GetUserConversations 获取用户参与的所有会话（返回参与者记录）
	GetUserConversations(ctx context.Context, userID int64) ([]model.ConversationParticipant, error)
	// CreateMessage 创建新消息
	CreateMessage(ctx context.Context, msg *model.Message) error
	GetMessageByID(ctx context.Context, messageID int64) (*model.Message, error)
	UpdateMessage(ctx context.Context, msg *model.Message) error
	CreateEditRecord(ctx context.Context, record *model.MessageEditRecord) error
	UpdateParticipantReadCursor(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error
	UpdateParticipantSettings(ctx context.Context, conversationID, userID int64, draft *string, isPinned *bool, notifyEnabled *bool) error
	// GetMessages 获取会话的消息列表，支持分页（beforeID游标分页）
	GetMessages(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.Message, error)
	// SearchMessages 在指定会话中搜索包含关键词的消息
	SearchMessages(ctx context.Context, conversationIDs []int64, keyword string, limit int64, startAt, endAt *time.Time) ([]model.Message, error)
	// FindPrivateConversation 查找两个用户之间的私聊会话（避免重复创建）
	FindPrivateConversation(ctx context.Context, userID1, userID2 int64) (*model.Conversation, error)
	// FindGroupConversation 根据groupID查找群聊会话（避免重复创建）
	FindGroupConversation(ctx context.Context, groupID int64) (*model.Conversation, error)
}

type messageRepositoryImpl struct {
	db *gorm.DB
}

func NewMessageRepo(db *gorm.DB) MessageRepository {
	return &messageRepositoryImpl{db: db}
}

// CreateConversation 创建新会话记录
func (r *messageRepositoryImpl) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	return r.db.WithContext(ctx).Create(conv).Error
}

// GetConversationByID 根据ID查询会话
// 如果会话不存在返回 nil, nil（不视为错误）
func (r *messageRepositoryImpl) GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&conv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &conv, err
}

// UpdateConversation 更新会话信息
// 主要用于在收到新消息时更新 UpdatedAt 字段，使会话列表按最新消息排序
func (r *messageRepositoryImpl) UpdateConversation(ctx context.Context, conv *model.Conversation) error {
	return r.db.WithContext(ctx).Save(conv).Error
}

// AddParticipant 添加会话参与者
// 用户加入会话时调用，建立用户与会话的关联
func (r *messageRepositoryImpl) AddParticipant(ctx context.Context, p *model.ConversationParticipant) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// GetParticipants 获取指定会话的所有参与者
// 用于消息推送时确定目标用户列表
func (r *messageRepositoryImpl) GetParticipants(ctx context.Context, conversationID int64) ([]model.ConversationParticipant, error) {
	var participants []model.ConversationParticipant
	err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Find(&participants).Error
	return participants, err
}

// GetUserConversations 获取用户参与的所有会话
// 返回参与者记录列表，通过 ConversationID 可进一步查询会话详情
func (r *messageRepositoryImpl) GetUserConversations(ctx context.Context, userID int64) ([]model.ConversationParticipant, error) {
	var participants []model.ConversationParticipant
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&participants).Error
	return participants, err
}

// CreateMessage 创建新消息记录
func (r *messageRepositoryImpl) CreateMessage(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *messageRepositoryImpl) GetMessageByID(ctx context.Context, messageID int64) (*model.Message, error) {
	var msg model.Message
	err := r.db.WithContext(ctx).Where("id = ?", messageID).First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &msg, err
}

func (r *messageRepositoryImpl) UpdateMessage(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Save(msg).Error
}

func (r *messageRepositoryImpl) CreateEditRecord(ctx context.Context, record *model.MessageEditRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *messageRepositoryImpl) UpdateParticipantReadCursor(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Updates(map[string]interface{}{
			"last_read_message_id": messageID,
			"last_read_at":         readAt,
		}).Error
}

func (r *messageRepositoryImpl) UpdateParticipantSettings(ctx context.Context, conversationID, userID int64, draft *string, isPinned *bool, notifyEnabled *bool) error {
	updates := map[string]interface{}{}
	if draft != nil {
		updates["draft"] = *draft
	}
	if isPinned != nil {
		updates["is_pinned"] = *isPinned
	}
	if notifyEnabled != nil {
		updates["notify_enabled"] = *notifyEnabled
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Updates(updates).Error
}

// GetMessages 获取会话的消息列表
// 使用游标分页：beforeID > 0 时只查询ID小于beforeID的消息（加载更早的消息）
// 按 ID DESC 排序，取最新的 limit 条
func (r *messageRepositoryImpl) GetMessages(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.Message, error) {
	var messages []model.Message
	query := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	err := query.Order("id DESC").Limit(int(limit)).Find(&messages).Error
	return messages, err
}

// SearchMessages 在指定会话列表中搜索包含关键词的消息
// 使用 LIKE 模糊匹配，按时间倒序排列
func (r *messageRepositoryImpl) SearchMessages(ctx context.Context, conversationIDs []int64, keyword string, limit int64, startAt, endAt *time.Time) ([]model.Message, error) {
	var messages []model.Message
	query := r.db.WithContext(ctx).
		Where("conversation_id IN ?", conversationIDs).
		Where("content LIKE ?", "%"+keyword+"%")
	if startAt != nil {
		query = query.Where("created_at >= ?", *startAt)
	}
	if endAt != nil {
		query = query.Where("created_at <= ?", *endAt)
	}
	err := query.Order("id DESC").Limit(int(limit)).Find(&messages).Error
	return messages, err
}

// FindPrivateConversation 查找两个用户之间已有的私聊会话
// 通过子查询查找同时包含两个用户的 private 类型会话
// 如果不存在返回 nil, nil，用于避免重复创建私聊会话
func (r *messageRepositoryImpl) FindPrivateConversation(ctx context.Context, userID1, userID2 int64) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).
		Where("type = ?", "private").
		Where("id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ?) AND id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ?)", userID1, userID2).
		First(&conv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &conv, err
}

// FindGroupConversation 根据groupID查找群聊会话
// 如果不存在返回 nil, nil，用于避免重复创建群聊会话
func (r *messageRepositoryImpl) FindGroupConversation(ctx context.Context, groupID int64) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).
		Where("type = ? AND group_id = ?", "group", groupID).
		First(&conv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &conv, err
}
