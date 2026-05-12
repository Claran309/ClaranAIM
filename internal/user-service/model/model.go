// Package model 定义 user-service 的数据库模型
// 每个模型对应一张数据库表，通过 GORM 的 AutoMigrate 自动建表
package model

import "time"

// User 用户表模型
// 存储用户的基本信息和认证数据
// Password 字段使用 json:"-" 防止在 API 响应中泄露密码
type User struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`       // 用户ID，自增主键
	Username  string    `json:"username" gorm:"uniqueIndex;size:50;not null"` // 用户名，唯一索引，用于登录
	Password  string    `json:"-" gorm:"size:255;not null"`               // bcrypt 加密后的密码，不序列化到 JSON
	Nickname  string    `json:"nickname" gorm:"size:50"`                  // 昵称，显示名称
	Avatar    string    `json:"avatar" gorm:"size:255"`                   // 头像 URL
	Email     string    `json:"email" gorm:"size:100"`                    // 邮箱
	Phone     string    `json:"phone" gorm:"size:20"`                     // 手机号
	Status    string    `json:"status" gorm:"size:20;default:offline"`    // 在线状态：online/offline
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`         // 创建时间
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`         // 更新时间
}

// TableName 指定表名为 "users"
func (User) TableName() string {
	return "users"
}

// Friend 好友关系表模型
// 好友关系是双向的：A 添加 B 时会插入两条记录（A→B 和 B→A）
// 这样查询任一方的好友列表都只需一次查询
type Friend struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`  // 记录ID，自增主键
	UserID    int64     `json:"user_id" gorm:"index;not null"`       // 用户ID，索引
	FriendID  int64     `json:"friend_id" gorm:"index;not null"`     // 好友ID，索引
	GroupID   int64     `json:"group_id" gorm:"index"`               // 好友分组ID，0 表示默认分组
	Remark    string    `json:"remark" gorm:"size:50"`               // 好友备注名
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`    // 添加时间
}

// TableName 指定表名为 "friends"
func (Friend) TableName() string {
	return "friends"
}

// FriendGroup 好友分组表模型
// 用于对好友进行分类管理，如"同事""家人""同学"
type FriendGroup struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"` // 分组ID，自增主键
	UserID    int64     `json:"user_id" gorm:"index;not null"`      // 所属用户ID，索引
	Name      string    `json:"name" gorm:"size:50;not null"`       // 分组名称
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`   // 创建时间
}

// TableName 指定表名为 "friend_groups"
func (FriendGroup) TableName() string {
	return "friend_groups"
}
