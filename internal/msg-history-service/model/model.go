package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// MessageHistory 消息历史记录表模型
// 与 msg-core-service 中的 Message 表结构相同，但存储在独立的数据库表中
// 用于消息的长期归档存储，支持历史消息查询和搜索
// 与 Message 表的分离体现了微服务的数据隔离原则
type MessageHistory struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement:false"` // 消息历史记录ID，雪花ID
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`    // 所属会话ID，索引
	SenderID       int64     `json:"sender_id" gorm:"index;not null"`          // 发送者用户ID，索引
	Content        string    `json:"content" gorm:"type:text"`                 // 消息内容
	MsgType        string    `json:"msg_type" gorm:"size:20;default:text"`     // 消息类型：text/image等
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`         // 消息发送时间
}

// BeforeCreate 在历史消息写入前补充分布式雪花 ID。
func (m *MessageHistory) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&m.ID)
}

// TableName 指定历史消息表名。
func (MessageHistory) TableName() string {
	return "message_history"
}

// OfflineMessage 离线消息表模型
// 记录用户未读的消息，用于实现离线消息推送和未读计数
// 当用户上线时，可以查询此表获取离线期间收到的消息
type OfflineMessage struct {
	ID        int64      `json:"id" gorm:"primaryKey;autoIncrement:false"` // 离线消息记录ID，雪花ID
	UserID    int64      `json:"user_id" gorm:"index;not null"`            // 接收者用户ID，索引（加速按用户查询）
	MessageID int64      `json:"message_id" gorm:"index;not null"`         // 关联的消息ID，索引
	IsRead    bool       `json:"is_read" gorm:"default:false"`             // 是否已读，默认false
	CreatedAt time.Time  `json:"created_at" gorm:"autoCreateTime"`         // 创建时间（消息产生时间）
	ReadAt    *time.Time // 已读时间，nil表示未读
}

// BeforeCreate 在离线消息记录写入前补充分布式雪花 ID。
func (m *OfflineMessage) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&m.ID)
}

// TableName 指定离线消息表名。
func (OfflineMessage) TableName() string {
	return "offline_messages"
}

// fillSnowflakeID 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
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
