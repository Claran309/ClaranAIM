package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// Conversation 会话表模型
// 记录用户之间的会话信息，包括私聊和群聊两种类型
// 私聊(private)：两个用户之间的一对一会话
// 群聊(group)：多个用户之间的多人会话
type Conversation struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Type      string    `json:"type" gorm:"size:20;not null"`
	GroupID   int64     `json:"group_id" gorm:"default:0"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate assigns a snowflake ID before inserting a conversation.
func (c *Conversation) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&c.ID)
}

// TableName keeps the conversation table name stable across GORM naming changes.
func (Conversation) TableName() string {
	return "conversations"
}

// ConversationParticipant 会话参与者表模型
// 记录每个会话中的参与者，实现会话与用户的多对多关系
// 每个用户加入会话时创建一条记录
type ConversationParticipant struct {
	ID                int64      `json:"id" gorm:"primaryKey;autoIncrement:false"` // 记录ID，雪花ID
	ConversationID    int64      `json:"conversation_id" gorm:"index;not null"`    // 所属会话ID，索引（加速按会话查询）
	UserID            int64      `json:"user_id" gorm:"index;not null"`            // 参与者用户ID，索引（加速按用户查询）
	LastReadMessageID int64      `json:"last_read_message_id" gorm:"default:0"`    // 用户在该会话的已读游标
	LastReadAt        *time.Time `json:"last_read_at"`                             // 最近一次标记已读时间，nil 表示从未读过
	Draft             string     `json:"draft" gorm:"type:text"`                   // 多端共享草稿
	IsPinned          bool       `json:"is_pinned" gorm:"default:false"`           // 多端共享置顶
	NotifyEnabled     bool       `json:"notify_enabled" gorm:"default:true"`       // 多端共享通知设置
	JoinedAt          time.Time  `json:"joined_at" gorm:"autoCreateTime"`          // 加入会话时间
}

// BeforeCreate assigns a snowflake ID before inserting a participant row.
func (p *ConversationParticipant) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&p.ID)
}

// TableName keeps the participant table name explicit.
func (ConversationParticipant) TableName() string {
	return "conversation_participants"
}

// Message 消息表模型
// 记录会话中的每一条消息
// 消息类型支持文本(text)、图片(image)等，当前阶段主要使用文本
type Message struct {
	ID             int64      `json:"id" gorm:"primaryKey;autoIncrement:false"`            // 消息ID，雪花ID
	ConversationID int64      `json:"conversation_id" gorm:"index;not null"`               // 所属会话ID，索引（加速按会话查询消息）
	SenderID       int64      `json:"sender_id" gorm:"index;not null"`                     // 发送者用户ID，索引
	Content        string     `json:"content" gorm:"type:text"`                            // 消息内容，text类型支持长文本
	ClientMsgID    *string    `json:"client_msg_id,omitempty" gorm:"size:128;uniqueIndex"` // 客户端/内部调用方提供的幂等键，避免重试生成重复消息
	MsgType        string     `json:"msg_type" gorm:"size:20;default:text"`                // 消息类型：text/image/file/voice/broadcast
	ReplyToID      int64      `json:"reply_to_id" gorm:"index;default:0"`                  // 引用/回复的源消息ID
	Status         string     `json:"status" gorm:"size:20;default:sent"`                  // sent/recalled/deleted
	IsEdited       bool       `json:"is_edited" gorm:"default:false"`                      // 是否被编辑过
	EditedAt       *time.Time `json:"edited_at"`                                           // 最近编辑时间，nil 表示从未编辑过
	MentionUserIDs []int64    `json:"mention_user_ids" gorm:"-"`                           // @用户列表，运行时字段
	MentionAll     bool       `json:"mention_all" gorm:"default:false"`                    // 是否 @所有人
	MentionsJSON   string     `json:"-" gorm:"column:mention_user_ids;type:text"`          // @用户列表的JSON持久化
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`                    // 发送时间
	ReadCount      int64      `json:"read_count" gorm:"-"`                                 // 发送者视角下已读接收人数，运行时计算
	RecipientCount int64      `json:"recipient_count" gorm:"-"`                            // 除发送者外的接收人数，运行时计算
	IsReadByMe     bool       `json:"is_read_by_me" gorm:"-"`                              // 当前查询用户是否已读该消息，运行时计算
}

// BeforeCreate assigns a snowflake ID before inserting a message.
func (m *Message) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&m.ID)
}

// TableName keeps the message table name explicit.
func (Message) TableName() string {
	return "messages"
}

// MessageUserState 保存“某个用户看到某条消息时的个人状态”。
//
// messages 表是服务端消息事实：一条消息是否存在、是谁发送、正文是什么、是否撤回，
// 都由 messages 统一表达。接收人删除本地聊天记录、阅读消息、收到消息这些行为，
// 不应该直接修改 messages，否则会影响其他参与者的历史视图。
//
// 因此这里单独建 per-user 状态表：
//   - DeliveredAt：服务端认为该用户已经可获得此消息的时间；当前阶段发送成功即写入，
//     后续多端在线投递可以改成真实设备 ACK 时间。
//   - ReadAt：该用户读到这条消息的时间，用于已读回执和未读统计。
//   - LocalDeletedAt：该用户本地删除/隐藏此消息的时间。历史查询会过滤它，
//     但其他用户仍然能看到 messages 中的原始消息。
type MessageUserState struct {
	ID             int64      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	ConversationID int64      `json:"conversation_id" gorm:"index:idx_msg_user_state_conv_user_msg,priority:1;not null"`
	MessageID      int64      `json:"message_id" gorm:"uniqueIndex:uk_msg_user_state_user_msg;index:idx_msg_user_state_conv_user_msg,priority:3;not null"`
	UserID         int64      `json:"user_id" gorm:"uniqueIndex:uk_msg_user_state_user_msg;index:idx_msg_user_state_conv_user_msg,priority:2;not null"`
	DeliveredAt    *time.Time `json:"delivered_at"`
	ReadAt         *time.Time `json:"read_at" gorm:"index"`
	LocalDeletedAt *time.Time `json:"local_deleted_at" gorm:"index"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate assigns a snowflake ID before inserting per-user state.
func (s *MessageUserState) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&s.ID)
}

// TableName keeps the per-user message state table name explicit.
func (MessageUserState) TableName() string {
	return "message_user_states"
}

// MessageEditRecord is an immutable audit row for message edits.
type MessageEditRecord struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`
	MessageID      int64     `json:"message_id" gorm:"index;not null"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`
	EditorID       int64     `json:"editor_id" gorm:"index;not null"`
	OldContent     string    `json:"old_content" gorm:"type:text"`
	NewContent     string    `json:"new_content" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// BeforeCreate assigns a snowflake ID before inserting an edit record.
func (r *MessageEditRecord) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&r.ID)
}

// TableName keeps the edit audit table name explicit.
func (MessageEditRecord) TableName() string {
	return "message_edit_records"
}

func fillSnowflakeID(id *int64) error {
	if *id != 0 {
		return nil
	}
	nextID, err := idgen.NextID()
	if err != nil {
		return err
	}
	*id = nextID
	return nil
}
