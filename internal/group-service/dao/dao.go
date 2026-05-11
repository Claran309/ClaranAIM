package dao

import (
	"ClaranAIM/internal/group-service/model"
	"context"
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

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
		if db.Migrator().HasTable(m) {
			if err := db.Migrator().DropTable(m); err != nil {
				return nil, err
			}
		}
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}

	return db, nil
}

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
}

type groupRepositoryImpl struct {
	db *gorm.DB
}

func NewGroupRepo(db *gorm.DB) GroupRepository {
	return &groupRepositoryImpl{db: db}
}

func (r *groupRepositoryImpl) CreateGroup(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Create(group).Error
}

func (r *groupRepositoryImpl) GetGroupByID(ctx context.Context, id int64) (*model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &group, err
}

func (r *groupRepositoryImpl) UpdateGroup(ctx context.Context, group *model.Group) error {
	return r.db.WithContext(ctx).Save(group).Error
}

func (r *groupRepositoryImpl) DeleteGroup(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Group{}).Error
}

func (r *groupRepositoryImpl) GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).
		Joins("JOIN group_members ON group_members.group_id = groups.id").
		Where("group_members.user_id = ?", userID).
		Find(&groups).Error
	return groups, err
}

func (r *groupRepositoryImpl) AddMember(ctx context.Context, member *model.GroupMember) error {
	return r.db.WithContext(ctx).Create(member).Error
}

func (r *groupRepositoryImpl) RemoveMember(ctx context.Context, groupID, userID int64) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&model.GroupMember{}).Error
}

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

func (r *groupRepositoryImpl) GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	var members []model.GroupMember
	err := r.db.WithContext(ctx).Where("group_id = ?", groupID).Find(&members).Error
	return members, err
}

func (r *groupRepositoryImpl) UpdateMemberRole(ctx context.Context, groupID, userID int64, role string) error {
	return r.db.WithContext(ctx).
		Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("role", role).Error
}

func (r *groupRepositoryImpl) UpdateMuteStatus(ctx context.Context, groupID, userID int64, mutedUntil *time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Update("muted_until", mutedUntil).Error
}

func (r *groupRepositoryImpl) UpdateOwner(ctx context.Context, groupID, newOwnerID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.Group{}).
		Where("id = ?", groupID).
		Update("owner_id", newOwnerID).Error
}
