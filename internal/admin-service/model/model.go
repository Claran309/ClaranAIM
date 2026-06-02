// Package model 保存 admin-service 自己拥有的管理域数据。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// SystemNotice 是管理员发布的系统公告。
// 公告可以面向全体、普通用户、管理员或 Agent 管理者；当前 MVP 先在管理后台展示，
// 后续可接入 websocket-gateway 做系统消息推送。
type SystemNotice struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Title     string    `json:"title" gorm:"size:255;not null"`
	Content   string    `json:"content" gorm:"type:text"`
	Level     string    `json:"level" gorm:"size:32;index;default:info"`
	Audience  string    `json:"audience" gorm:"size:64;index;default:all"`
	Enabled   bool      `json:"enabled" gorm:"index;default:true"`
	CreatedBy int64     `json:"created_by" gorm:"index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SystemNotice) TableName() string {
	return "admin_system_notices"
}

func (n *SystemNotice) BeforeCreate(tx *gorm.DB) error {
	if n.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		n.ID = id
	}
	return nil
}

// AdminAuditLog 记录管理员在管理后台执行的动作。
// 它不是业务审计的替代品，而是管理操作审计，例如发布公告、审批候选、删除文件等。
type AdminAuditLog struct {
	ID         int64     `json:"id" gorm:"primaryKey"`
	AdminID    int64     `json:"admin_id" gorm:"index;not null"`
	Action     string    `json:"action" gorm:"size:128;index;not null"`
	TargetType string    `json:"target_type" gorm:"size:64;index"`
	TargetID   string    `json:"target_id" gorm:"size:128;index"`
	Detail     string    `json:"detail" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at"`
}

func (AdminAuditLog) TableName() string {
	return "admin_audit_logs"
}

func (l *AdminAuditLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		l.ID = id
	}
	return nil
}
