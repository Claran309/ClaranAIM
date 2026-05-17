// Package dao 实现 group-service 的数据访问层
// 定义 GroupRepository 接口和基于 GORM + MySQL 的实现
package dao

import (
	"ClaranAIM/internal/group-service/model"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 初始化数据库连接并自动迁移群组相关表
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	models := []interface{}{
		&model.Group{},
		&model.GroupMember{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}
	if err := outbox.AutoMigrate(db); err != nil {
		return nil, err
	}

	return db, nil
}

// GroupRepository 群组数据访问接口
type GroupRepository interface {
	CreateGroup(ctx context.Context, group *model.Group) error
	GetGroupByID(ctx context.Context, id int64) (*model.Group, error)
	UpdateGroup(ctx context.Context, group *model.Group) error
	DeleteGroup(ctx context.Context, id int64) error
	GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error)
	AddMember(ctx context.Context, member *model.GroupMember) error
	RemoveMember(ctx context.Context, groupID, userID int64) error
	GetMember(ctx context.Context, groupID, userID int64) (*model.GroupMember, error)
	GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error)
	UpdateMemberRole(ctx context.Context, groupID, userID int64, role string) error
	UpdateMuteStatus(ctx context.Context, groupID, userID int64, mutedUntil *time.Time) error
	UpdateOwner(ctx context.Context, groupID, newOwnerID int64) error
	PinGroup(ctx context.Context, groupID int64, isPinned bool) error
	WithTransaction(ctx context.Context, fn func(tx GroupRepository) error) error
	SaveOutboxEvent(ctx context.Context, event outbox.Event) error
}

// groupRepositoryImpl 基于 GORM 的 GroupRepository 实现
type groupRepositoryImpl struct {
	db *gorm.DB
}

// NewGroupRepo 创建 GroupRepository 实例
func NewGroupRepo(db *gorm.DB) GroupRepository {
	return &groupRepositoryImpl{db: db}
}

// WithTransaction executes group repository operations inside one DB transaction.
func (r *groupRepositoryImpl) WithTransaction(ctx context.Context, fn func(tx GroupRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&groupRepositoryImpl{db: tx})
	})
}

// SaveOutboxEvent stores a group-domain event for transactional outbox delivery.
func (r *groupRepositoryImpl) SaveOutboxEvent(ctx context.Context, event outbox.Event) error {
	return r.db.WithContext(ctx).Create(&event).Error
}

// CreateGroup 创建群组记录
func (r *groupRepositoryImpl) CreateGroup(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Create(group).Error
}

// GetGroupByID 根据群组ID查询群组信息
func (r *groupRepositoryImpl) GetGroupByID(ctx context.Context, id int64) (*model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &group, err
}

// UpdateGroup 更新群组信息（全字段）
func (r *groupRepositoryImpl) UpdateGroup(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Save(group).Error
}

// DeleteGroup 删除群组记录（仅删除 groups 表，不级联删除成员记录）
func (r *groupRepositoryImpl) DeleteGroup(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Group{}).Error
}

// GetUserGroups 获取用户所在的所有群组
// 通过 JOIN group_members 表查询，只返回用户是成员的群组
func (r *groupRepositoryImpl) GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).
		Joins("JOIN group_members ON group_members.group_id = groups.id").
		Where("group_members.user_id = ?", userID).
		Find(&groups).Error
	return groups, err
}

// AddMember 添加群组成员记录
func (r *groupRepositoryImpl) AddMember(ctx context.Context, member *model.GroupMember) error {
	return r.db.WithContext(ctx).Create(member).Error
}

// RemoveMember 移除群组成员记录
func (r *groupRepositoryImpl) RemoveMember(ctx context.Context, groupID, userID int64) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&model.GroupMember{}).Error
}

// GetMember 查询用户在群组中的成员信息（含角色）
// 用于权限校验：判断操作者是否为 owner/admin
func (r *groupRepositoryImpl) GetMember(ctx context.Context, groupID, userID int64) (*model.GroupMember, error) {
	var member model.GroupMember
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &member, err
}

// GetGroupMembers 获取群组的所有成员列表
func (r *groupRepositoryImpl) GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	var members []model.GroupMember
	err := r.db.WithContext(ctx).Where("group_id = ?", groupID).Find(&members).Error
	return members, err
}

// UpdateMemberRole 更新群组成员的角色（owner/admin/member）
func (r *groupRepositoryImpl) UpdateMemberRole(ctx context.Context, groupID, userID int64, role string) error {
	return r.db.WithContext(ctx).
		Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("role", role).Error
}

// UpdateMuteStatus 更新群组成员的禁言状态
// mutedUntil 为 nil 表示解除禁言
func (r *groupRepositoryImpl) UpdateMuteStatus(ctx context.Context, groupID, userID int64, mutedUntil *time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("muted_until", mutedUntil).Error
}

// UpdateOwner 转让群主：更新 groups 表的 owner_id 字段
func (r *groupRepositoryImpl) UpdateOwner(ctx context.Context, groupID, newOwnerID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.Group{}).
		Where("id = ?", groupID).
		Update("owner_id", newOwnerID).Error
}

// PinGroup 置顶/取消置顶群组
func (r *groupRepositoryImpl) PinGroup(ctx context.Context, groupID int64, isPinned bool) error {
	return r.db.WithContext(ctx).
		Model(&model.Group{}).
		Where("id = ?", groupID).
		Update("is_pinned", isPinned).Error
}
