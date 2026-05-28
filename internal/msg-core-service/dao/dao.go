package dao

import (
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		&model.MessageUserState{},
		&model.MessageEditRecord{},
		&model.MessageTranslation{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}
	if err := outbox.AutoMigrate(db); err != nil {
		return nil, err
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
	// DeleteConversation 删除一个会话及其参与者记录。
	DeleteConversation(ctx context.Context, conversationID int64) error
	// AddParticipant 添加会话参与者
	AddParticipant(ctx context.Context, p *model.ConversationParticipant) error
	// RemoveParticipant 移除会话参与者
	RemoveParticipant(ctx context.Context, conversationID, userID int64) error
	// GetParticipants 获取会话的所有参与者
	GetParticipants(ctx context.Context, conversationID int64) ([]model.ConversationParticipant, error)
	// GetUserConversations 获取用户参与的所有会话（返回参与者记录）
	GetUserConversations(ctx context.Context, userID int64) ([]model.ConversationParticipant, error)
	// CreateMessage 创建新消息
	CreateMessage(ctx context.Context, msg *model.Message) error
	// GetMessageByID 根据 ID 查询消息事实，不存在时返回 nil。
	GetMessageByID(ctx context.Context, messageID int64) (*model.Message, error)
	// GetMessageByClientMsgID 根据幂等发送 key 查询已存在消息。
	GetMessageByClientMsgID(ctx context.Context, clientMsgID string) (*model.Message, error)
	// UpdateMessage 持久化消息编辑元数据或撤回状态等变更。
	UpdateMessage(ctx context.Context, msg *model.Message) error
	// CreateEditRecord 追加一条不可变的消息编辑审计记录。
	CreateEditRecord(ctx context.Context, record *model.MessageEditRecord) error
	// GetTranslation 查询某用户、目标语言和源文本 hash 对应的翻译缓存。
	GetTranslation(ctx context.Context, messageID, userID int64, targetLanguage, sourceHash string) (*model.MessageTranslation, error)
	// SaveTranslation 保存一条翻译结果。
	SaveTranslation(ctx context.Context, translation *model.MessageTranslation) error
	// UpsertMessageUserState 创建或合并某个用户对单条消息的本地状态。
	UpsertMessageUserState(ctx context.Context, state *model.MessageUserState) error
	// MarkMessagesReadThrough 将 messageID 之前的消息标记为该用户已读。
	MarkMessagesReadThrough(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error
	// MarkMessageLocalDeleted 仅在某个用户本地视图中隐藏一条消息。
	MarkMessageLocalDeleted(ctx context.Context, conversationID, userID, messageID int64, deletedAt time.Time) error
	// UpdateParticipantReadCursor 保存会话级已读游标。
	UpdateParticipantReadCursor(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error
	// UpdateParticipantSettings 保存用户级会话 UI 设置。
	UpdateParticipantSettings(ctx context.Context, conversationID, userID int64, draft *string, isPinned *bool, notifyEnabled *bool) error
	// GetMessages 获取会话的消息列表，支持分页（beforeID游标分页）
	GetMessages(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.Message, error)
	// GetMessagesForUser 返回按用户本地删除状态过滤后的历史消息。
	GetMessagesForUser(ctx context.Context, conversationID, userID, limit, beforeID int64) ([]model.Message, error)
	// CountUnreadMessages 统计某用户未读且未撤回的消息数。
	CountUnreadMessages(ctx context.Context, conversationID, userID, lastReadMessageID int64) (int64, error)
	// GetMessageReadStats 计算可见消息的已读回执统计。
	GetMessageReadStats(ctx context.Context, conversationID int64, messageIDs []int64, viewerID int64) (map[int64]MessageReadStat, error)
	// SearchMessages 在指定会话中搜索包含关键词的消息
	SearchMessages(ctx context.Context, conversationIDs []int64, keyword string, limit int64, startAt, endAt *time.Time) ([]model.Message, error)
	// FindPrivateConversation 查找两个用户之间的私聊会话（避免重复创建）
	FindPrivateConversation(ctx context.Context, userID1, userID2 int64) (*model.Conversation, error)
	// FindGroupConversation 根据groupID查找群聊会话（避免重复创建）
	FindGroupConversation(ctx context.Context, groupID int64) (*model.Conversation, error)
	// WithTransaction 在同一个数据库事务中执行一组仓储操作。
	WithTransaction(ctx context.Context, fn func(tx MessageRepository) error) error
	// SaveOutboxEvent 在业务数据同一事务中保存一条领域事件。
	SaveOutboxEvent(ctx context.Context, event outbox.Event) error
}

// messageRepositoryImpl 是基于 GORM 的消息仓储实现。
type messageRepositoryImpl struct {
	db *gorm.DB
}

// MessageReadStat 是单条消息的聚合已读回执状态。
type MessageReadStat struct {
	MessageID      int64
	ReadCount      int64
	RecipientCount int64
	IsReadByMe     bool
}

// NewMessageRepo 创建基于 GORM 的消息仓储。
func NewMessageRepo(db *gorm.DB) MessageRepository {
	return &messageRepositoryImpl{db: db}
}

// WithTransaction 使用 GORM 事务包装一组仓储操作。
func (r *messageRepositoryImpl) WithTransaction(ctx context.Context, fn func(tx MessageRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&messageRepositoryImpl{db: tx})
	})
}

// SaveOutboxEvent 插入待发布 Outbox 记录，稍后由 Worker 发布到 Kafka。
func (r *messageRepositoryImpl) SaveOutboxEvent(ctx context.Context, event outbox.Event) error {
	return r.db.WithContext(ctx).Create(&event).Error
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

// DeleteConversation 删除会话及其参与者关系。
//
// 当前只用于 DTM Saga 补偿创建群聊会话失败后的清理；普通用户删除消息历史不应调用它。
func (r *messageRepositoryImpl) DeleteConversation(ctx context.Context, conversationID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", conversationID).Delete(&model.ConversationParticipant{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", conversationID).Delete(&model.Conversation{}).Error
	})
}

// AddParticipant 添加会话参与者
// 用户加入会话时调用，建立用户与会话的关联
func (r *messageRepositoryImpl) AddParticipant(ctx context.Context, p *model.ConversationParticipant) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// RemoveParticipant 从会话扇出列表中移除一个用户。
func (r *messageRepositoryImpl) RemoveParticipant(ctx context.Context, conversationID, userID int64) error {
	return r.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Delete(&model.ConversationParticipant{}).Error
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

// GetMessageByID 根据 ID 加载一条消息事实。
func (r *messageRepositoryImpl) GetMessageByID(ctx context.Context, messageID int64) (*model.Message, error) {
	var msg model.Message
	err := r.db.WithContext(ctx).Where("id = ?", messageID).First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &msg, err
}

// GetMessageByClientMsgID 查询之前由同一幂等 key 发送出的消息。
func (r *messageRepositoryImpl) GetMessageByClientMsgID(ctx context.Context, clientMsgID string) (*model.Message, error) {
	if clientMsgID == "" {
		return nil, nil
	}
	var msg model.Message
	err := r.db.WithContext(ctx).Where("client_msg_id = ?", clientMsgID).First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &msg, err
}

// UpdateMessage 保存变更后的消息事实。
func (r *messageRepositoryImpl) UpdateMessage(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Save(msg).Error
}

// CreateEditRecord 为消息编辑追加一条审计记录。
func (r *messageRepositoryImpl) CreateEditRecord(ctx context.Context, record *model.MessageEditRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// GetTranslation 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *messageRepositoryImpl) GetTranslation(ctx context.Context, messageID, userID int64, targetLanguage, sourceHash string) (*model.MessageTranslation, error) {
	var translation model.MessageTranslation
	err := r.db.WithContext(ctx).
		Where("message_id = ? AND user_id = ? AND target_language = ? AND source_text_hash = ?", messageID, userID, targetLanguage, sourceHash).
		First(&translation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &translation, err
}

// SaveTranslation 是当前包对外暴露的方法，负责承接对应的业务流程、参数校验或适配逻辑。
func (r *messageRepositoryImpl) SaveTranslation(ctx context.Context, translation *model.MessageTranslation) error {
	return r.db.WithContext(ctx).Create(translation).Error
}

// UpsertMessageUserState 创建或合并用户级消息状态。
//
// 发送消息时会为每个参与者写一条状态；后续已读、重新投递或本地删除都可能再次
// 写同一 user_id + message_id。这里用唯一键做幂等合并，避免重试或多端同步造成重复行。
func (r *messageRepositoryImpl) UpsertMessageUserState(ctx context.Context, state *model.MessageUserState) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "message_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"conversation_id":  state.ConversationID,
				"delivered_at":     gorm.Expr("COALESCE(VALUES(delivered_at), delivered_at)"),
				"read_at":          gorm.Expr("COALESCE(VALUES(read_at), read_at)"),
				"local_deleted_at": gorm.Expr("COALESCE(VALUES(local_deleted_at), local_deleted_at)"),
				"updated_at":       time.Now(),
			}),
		}).
		Create(state).Error
}

// MarkMessagesReadThrough 把用户在一个会话中读到 messageID 之前的消息统一标记已读。
//
// conversation_participants.last_read_message_id 是会话级游标，适合快速计算未读；
// message_user_states.read_at 是消息级状态，适合未来做已读回执、单条消息状态和多端同步。
func (r *messageRepositoryImpl) MarkMessagesReadThrough(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error {
	var messages []model.Message
	if err := r.db.WithContext(ctx).
		Select("id", "conversation_id").
		Where("conversation_id = ? AND id <= ?", conversationID, messageID).
		Find(&messages).Error; err != nil {
		return err
	}
	for _, msg := range messages {
		deliveredAt := readAt
		state := &model.MessageUserState{
			ConversationID: msg.ConversationID,
			MessageID:      msg.ID,
			UserID:         userID,
			DeliveredAt:    &deliveredAt,
			ReadAt:         &readAt,
		}
		if err := r.UpsertMessageUserState(ctx, state); err != nil {
			return err
		}
	}
	return nil
}

// MarkMessageLocalDeleted 只隐藏当前用户自己的消息视图，不删除 messages 中的消息事实。
func (r *messageRepositoryImpl) MarkMessageLocalDeleted(ctx context.Context, conversationID, userID, messageID int64, deletedAt time.Time) error {
	state := &model.MessageUserState{
		ConversationID: conversationID,
		MessageID:      messageID,
		UserID:         userID,
		LocalDeletedAt: &deletedAt,
	}
	return r.UpsertMessageUserState(ctx, state)
}

// UpdateParticipantReadCursor 保存最新已读消息，用于未读数计算。
func (r *messageRepositoryImpl) UpdateParticipantReadCursor(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Updates(map[string]interface{}{
			"last_read_message_id": messageID,
			"last_read_at":         readAt,
		}).Error
}

// UpdateParticipantSettings 更新用户级会话设置。
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

// GetMessagesForUser 查询某个用户视角下的历史消息。
//
// 与 GetMessages 的区别是：这里会过滤 message_user_states.local_deleted_at 不为空的行，
// 实现“我删除了本地聊天记录，但不影响其他人和服务端消息事实”的 IM 语义。
func (r *messageRepositoryImpl) GetMessagesForUser(ctx context.Context, conversationID, userID, limit, beforeID int64) ([]model.Message, error) {
	var messages []model.Message
	query := r.db.WithContext(ctx).
		Table("messages").
		Select("messages.*").
		Joins("LEFT JOIN message_user_states mus ON mus.message_id = messages.id AND mus.user_id = ?", userID).
		Where("messages.conversation_id = ?", conversationID).
		Where("(mus.local_deleted_at IS NULL)")
	if beforeID > 0 {
		query = query.Where("messages.id < ?", beforeID)
	}
	err := query.Order("messages.id DESC").Limit(int(limit)).Find(&messages).Error
	return messages, err
}

// CountUnreadMessages 用 SQL 在服务端统计未读数，避免每个会话拉取大量消息到内存遍历。
func (r *messageRepositoryImpl) CountUnreadMessages(ctx context.Context, conversationID, userID, lastReadMessageID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("messages").
		Joins("LEFT JOIN message_user_states mus ON mus.message_id = messages.id AND mus.user_id = ?", userID).
		Where("messages.conversation_id = ?", conversationID).
		Where("messages.id > ?", lastReadMessageID).
		Where("messages.sender_id <> ?", userID).
		Where("messages.status <> ?", "recalled").
		Where("mus.local_deleted_at IS NULL").
		Count(&count).Error
	return count, err
}

// GetMessageReadStats 聚合消息已读人数和当前查看者已读状态。
func (r *messageRepositoryImpl) GetMessageReadStats(ctx context.Context, conversationID int64, messageIDs []int64, viewerID int64) (map[int64]MessageReadStat, error) {
	stats := make(map[int64]MessageReadStat, len(messageIDs))
	if len(messageIDs) == 0 {
		return stats, nil
	}

	type row struct {
		MessageID      int64
		ReadCount      int64
		RecipientCount int64
		IsReadByMe     int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("messages").
		Select(`messages.id AS message_id,
			COALESCE(SUM(CASE WHEN mus.user_id <> messages.sender_id AND mus.read_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS read_count,
			COALESCE(SUM(CASE WHEN cp.user_id <> messages.sender_id THEN 1 ELSE 0 END), 0) AS recipient_count,
			COALESCE(MAX(CASE WHEN mus.user_id = ? AND mus.read_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS is_read_by_me`, viewerID).
		Joins("LEFT JOIN conversation_participants cp ON cp.conversation_id = messages.conversation_id").
		Joins("LEFT JOIN message_user_states mus ON mus.message_id = messages.id AND mus.user_id = cp.user_id").
		Where("messages.conversation_id = ? AND messages.id IN ?", conversationID, messageIDs).
		Group("messages.id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		stats[row.MessageID] = MessageReadStat{
			MessageID:      row.MessageID,
			ReadCount:      row.ReadCount,
			RecipientCount: row.RecipientCount,
			IsReadByMe:     row.IsReadByMe > 0,
		}
	}
	return stats, nil
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
