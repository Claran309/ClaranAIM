package model

import "time"

// Conversation 会话表模型
// 记录用户之间的会话信息，包括私聊和群聊两种类型
// 私聊(private)：两个用户之间的一对一会话
// 群聊(group)：多个用户之间的多人会话
type Conversation struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"` // 会话ID，自增主键
	Type      string    `json:"type" gorm:"size:20;not null"`       // 会话类型：private(私聊) / group(群聊)
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`   // 创建时间
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`   // 更新时间（收到新消息时自动更新，用于排序）
}

func (Conversation) TableName() string {
	return "conversations"
}

// ConversationParticipant 会话参与者表模型
// 记录每个会话中的参与者，实现会话与用户的多对多关系
// 每个用户加入会话时创建一条记录
type ConversationParticipant struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"` // 记录ID，自增主键
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"` // 所属会话ID，索引（加速按会话查询）
	UserID         int64     `json:"user_id" gorm:"index;not null"`         // 参与者用户ID，索引（加速按用户查询）
	JoinedAt       time.Time `json:"joined_at" gorm:"autoCreateTime"`       // 加入会话时间
}

func (ConversationParticipant) TableName() string {
	return "conversation_participants"
}

// Message 消息表模型
// 记录会话中的每一条消息
// 消息类型支持文本(text)、图片(image)等，当前阶段主要使用文本
type Message struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"` // 消息ID，自增主键
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"` // 所属会话ID，索引（加速按会话查询消息）
	SenderID       int64     `json:"sender_id" gorm:"index;not null"`       // 发送者用户ID，索引
	Content        string    `json:"content" gorm:"type:text"`              // 消息内容，text类型支持长文本
	MsgType        string    `json:"msg_type" gorm:"size:20;default:text"`  // 消息类型：text(文本) / image(图片) 等，默认文本
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`      // 发送时间
}

func (Message) TableName() string {
	return "messages"
}
