package model

import "time"

type MessageHistory struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`
	SenderID       int64     `json:"sender_id" gorm:"index;not null"`
	Content        string    `json:"content" gorm:"type:text"`
	MsgType        string    `json:"msg_type" gorm:"size:20;default:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (MessageHistory) TableName() string {
	return "message_history"
}

type OfflineMessage struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID     int64     `json:"user_id" gorm:"index;not null"`
	MessageID  int64     `json:"message_id" gorm:"index;not null"`
	IsRead     bool      `json:"is_read" gorm:"default:false"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
	ReadAt     *time.Time
}

func (OfflineMessage) TableName() string {
	return "offline_messages"
}
