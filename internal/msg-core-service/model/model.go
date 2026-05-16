package model

import "time"

// Conversation 会话表模型
// 记录用户之间的会话信息，包括私聊和群聊两种类型
// 私聊(private)：两个用户之间的一对一会话
// 群聊(group)：多个用户之间的多人会话
type Conversation struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Type      string    `json:"type" gorm:"size:20;not null"`
	GroupID   int64     `json:"group_id" gorm:"default:0"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Conversation) TableName() string {
	return "conversations"
}

// ConversationParticipant 会话参与者表模型
// 记录每个会话中的参与者，实现会话与用户的多对多关系
// 每个用户加入会话时创建一条记录
type ConversationParticipant struct {
	ID                int64     `json:"id" gorm:"primaryKey;autoIncrement"`    // 记录ID，自增主键
	ConversationID    int64     `json:"conversation_id" gorm:"index;not null"` // 所属会话ID，索引（加速按会话查询）
	UserID            int64     `json:"user_id" gorm:"index;not null"`         // 参与者用户ID，索引（加速按用户查询）
	LastReadMessageID int64     `json:"last_read_message_id" gorm:"default:0"` // 用户在该会话的已读游标
	LastReadAt        time.Time `json:"last_read_at"`                          // 最近一次标记已读时间
	Draft             string    `json:"draft" gorm:"type:text"`                // 多端共享草稿
	IsPinned          bool      `json:"is_pinned" gorm:"default:false"`        // 多端共享置顶
	NotifyEnabled     bool      `json:"notify_enabled" gorm:"default:true"`    // 多端共享通知设置
	JoinedAt          time.Time `json:"joined_at" gorm:"autoCreateTime"`       // 加入会话时间
}

func (ConversationParticipant) TableName() string {
	return "conversation_participants"
}

// Message 消息表模型
// 记录会话中的每一条消息
// 消息类型支持文本(text)、图片(image)等，当前阶段主要使用文本
type Message struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`     // 消息ID，自增主键
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`  // 所属会话ID，索引（加速按会话查询消息）
	SenderID       int64     `json:"sender_id" gorm:"index;not null"`        // 发送者用户ID，索引
	Content        string    `json:"content" gorm:"type:text"`               // 消息内容，text类型支持长文本
	MsgType        string    `json:"msg_type" gorm:"size:20;default:text"`   // 消息类型：text/image/file/voice/broadcast
	ReplyToID      int64     `json:"reply_to_id" gorm:"index;default:0"`     // 引用/回复的源消息ID
	Status         string    `json:"status" gorm:"size:20;default:sent"`     // sent/recalled/deleted
	IsEdited       bool      `json:"is_edited" gorm:"default:false"`         // 是否被编辑过
	EditedAt       time.Time `json:"edited_at"`                              // 最近编辑时间
	MentionUserIDs []int64   `json:"mention_user_ids" gorm:"-"`              // @用户列表，运行时字段
	MentionAll     bool      `json:"mention_all" gorm:"default:false"`       // 是否 @所有人
	MentionsJSON   string    `json:"-" gorm:"column:mention_user_ids;type:text"` // @用户列表的JSON持久化
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`       // 发送时间
}

func (Message) TableName() string {
	return "messages"
}

type MessageEditRecord struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	MessageID      int64     `json:"message_id" gorm:"index;not null"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`
	EditorID       int64     `json:"editor_id" gorm:"index;not null"`
	OldContent     string    `json:"old_content" gorm:"type:text"`
	NewContent     string    `json:"new_content" gorm:"type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (MessageEditRecord) TableName() string {
	return "message_edit_records"
}
