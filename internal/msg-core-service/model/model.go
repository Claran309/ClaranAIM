package model

import "time"

type Conversation struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Type      string    `json:"type" gorm:"size:20;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Conversation) TableName() string {
	return "conversations"
}

type ConversationParticipant struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`
	UserID         int64     `json:"user_id" gorm:"index;not null"`
	JoinedAt       time.Time `json:"joined_at" gorm:"autoCreateTime"`
}

func (ConversationParticipant) TableName() string {
	return "conversation_participants"
}

type Message struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ConversationID int64     `json:"conversation_id" gorm:"index;not null"`
	SenderID       int64     `json:"sender_id" gorm:"index;not null"`
	Content        string    `json:"content" gorm:"type:text"`
	MsgType        string    `json:"msg_type" gorm:"size:20;default:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (Message) TableName() string {
	return "messages"
}
