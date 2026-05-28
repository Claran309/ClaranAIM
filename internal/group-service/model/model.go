// Package model 定义 group-service 的数据库模型
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// Group 群组表模型
// 存储群组的基本信息，群主通过 OwnerID 关联到 users 表
type Group struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement:false"` // 群组ID，雪花ID
	Name         string    `json:"name" gorm:"size:100;not null"`            // 群组名称
	Avatar       string    `json:"avatar" gorm:"size:255"`                   // 群头像 URL
	OwnerID      int64     `json:"owner_id" gorm:"index;not null"`           // 群主用户ID，索引
	Announcement string    `json:"announcement" gorm:"type:text"`            // 群公告
	IsPinned     bool      `json:"is_pinned" gorm:"default:false"`           // 是否置顶（群内全局置顶标识）
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`         // 创建时间
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`         // 更新时间
}

// BeforeCreate 在群组写入前补充 10 位公开群号。
//
// 群号和用户 UID 一样面向用户复制、搜索和加入群聊，因此不用普通雪花 ID。
// 调用方仍需依靠 groups.id 唯一索引兜底极低概率的随机碰撞。
func (g *Group) BeforeCreate(tx *gorm.DB) error {
	if g.ID != 0 {
		return nil
	}
	id, err := idgen.NewUID10()
	if err != nil {
		return err
	}
	g.ID = id
	return nil
}

// TableName 指定表名为 "groups"
func (Group) TableName() string {
	return "groups"
}

// GroupMember 群组成员表模型
// 记录群组中每个成员的角色和状态
// 角色层级：owner(群主) > admin(管理员) > member(普通成员)
type GroupMember struct {
	ID         int64      `json:"id" gorm:"primaryKey;autoIncrement:false"` // 记录ID，雪花ID
	GroupID    int64      `json:"group_id" gorm:"index;not null"`           // 群组ID，索引
	UserID     int64      `json:"user_id" gorm:"index;not null"`            // 用户ID，索引
	Role       string     `json:"role" gorm:"size:20;default:member"`       // 角色：owner/admin/member
	MutedUntil *time.Time `json:"muted_until"`                              // 禁言截止时间，nil 表示未禁言
	JoinedAt   time.Time  `json:"joined_at" gorm:"autoCreateTime"`          // 加入时间
}

// BeforeCreate 在群成员关系写入前补充分布式雪花 ID。
func (m *GroupMember) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&m.ID)
}

// TableName 指定表名为 "group_members"
func (GroupMember) TableName() string {
	return "group_members"
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
