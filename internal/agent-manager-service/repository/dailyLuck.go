package repository

import (
	"time"

	"gorm.io/gorm"
)

// User 是 daily luck/签到功能使用的轻量用户积分模型。
//
// 这里以 IP 作为主键，属于独立演示功能的数据结构，不等同于 user-service 的正式
// 用户账号模型。
type User struct {
	IP           string     `gorm:"primaryKey"`
	Username     string     `gorm:"uniqueIndex;size:64"`
	Points       int        `gorm:"default:0"` // 积分
	LastSignInAt *time.Time // 上次签到时间
}

// UserRepository 封装 daily luck 用户积分表的读写操作。
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建 daily luck 用户仓储。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetUser 获取或创建用户
func (r *UserRepository) GetUser(IP string) (*User, error) {
	var user User
	err := r.db.FirstOrCreate(&user, User{IP: IP}).Error
	return &user, err
}

// UpdateSignIn 更新签到信息
func (r *UserRepository) UpdateSignIn(user *User, addPoints int) error {
	return r.db.Model(user).Updates(map[string]interface{}{
		"points":          user.Points + addPoints,
		"last_sign_in_at": time.Now(),
	}).Error
}
