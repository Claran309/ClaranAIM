package model

import "time"

type User struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Username  string    `json:"username" gorm:"uniqueIndex;size:50;not null"`
	Password  string    `json:"-" gorm:"size:255;not null"`
	Nickname  string    `json:"nickname" gorm:"size:50"`
	Avatar    string    `json:"avatar" gorm:"size:255"`
	Email     string    `json:"email" gorm:"size:100"`
	Phone     string    `json:"phone" gorm:"size:20"`
	Status    string    `json:"status" gorm:"size:20;default:offline"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (User) TableName() string {
	return "users"
}

type Friend struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    int64     `json:"user_id" gorm:"index;not null"`
	FriendID  int64     `json:"friend_id" gorm:"index;not null"`
	GroupID   int64     `json:"group_id" gorm:"index"`
	Remark    string    `json:"remark" gorm:"size:50"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (Friend) TableName() string {
	return "friends"
}

type FriendGroup struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID    int64     `json:"user_id" gorm:"index;not null"`
	Name      string    `json:"name" gorm:"size:50;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (FriendGroup) TableName() string {
	return "friend_groups"
}
