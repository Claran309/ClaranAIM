package model

import "time"

type Group struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Name         string    `json:"name" gorm:"size:100;not null"`
	Avatar       string    `json:"avatar" gorm:"size:255"`
	OwnerID      int64     `json:"owner_id" gorm:"index;not null"`
	Announcement string    `json:"announcement" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Group) TableName() string {
	return "groups"
}

type GroupMember struct {
	ID        int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	GroupID   int64      `json:"group_id" gorm:"index;not null"`
	UserID    int64      `json:"user_id" gorm:"index;not null"`
	Role      string     `json:"role" gorm:"size:20;default:member"`
	MutedUntil *time.Time `json:"muted_until"`
	JoinedAt  time.Time  `json:"joined_at" gorm:"autoCreateTime"`
}

func (GroupMember) TableName() string {
	return "group_members"
}
