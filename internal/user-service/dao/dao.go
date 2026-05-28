// Package dao 实现 user-service 的数据访问层
// 定义 UserRepository 接口和基于 GORM + MySQL 的实现
// 所有数据库操作都使用 WithContext 支持请求级超时控制
package dao

import (
	"ClaranAIM/internal/user-service/model"
	"context"
	"errors"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 初始化数据库连接并自动迁移表结构
// AutoMigrate 只做非破坏性迁移，避免服务重启清空已有数据。
// 生产环境复杂变更应使用增量迁移工具（如 golang-migrate）。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// 需要自动迁移的模型列表。FriendGroup 迁移前先清理历史重复分组，
	// 否则新增 (user_id,name) 唯一索引时会因为旧数据重复导致服务无法启动。
	models := []interface{}{
		&model.User{},
		&model.Friend{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}
	if err := mergeDuplicateFriendGroups(db); err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.FriendGroup{}); err != nil {
		return nil, err
	}

	return db, nil
}

// duplicateFriendGroup 定义当前包使用的数据结构或接口，用于在业务层、持久化层和传输层之间传递明确语义。
type duplicateFriendGroup struct {
	UserID int64
	Name   string
	KeepID int64
}

// mergeDuplicateFriendGroups 合并旧版本可能已经写入的重复好友分组。
//
// 历史表在没有唯一索引时允许同一用户创建多个同名分组。现在模型要求
// (user_id,name) 唯一，AutoMigrate 创建索引前必须先把重复数据收敛：
// 1. 每组重复数据保留 ID 最小的一条；
// 2. friends.group_id 指向重复分组的记录迁移到保留分组；
// 3. 删除其余重复 friend_groups。
//
// 这个过程只在 friend_groups 表已存在时执行，不会创建新表或清空数据。
func mergeDuplicateFriendGroups(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.FriendGroup{}) {
		return nil
	}

	var groups []duplicateFriendGroup
	if err := db.
		Table("friend_groups").
		Select("user_id, name, MIN(id) AS keep_id").
		Group("user_id, name").
		Having("COUNT(*) > 1").
		Scan(&groups).Error; err != nil {
		return err
	}

	for _, g := range groups {
		var duplicateIDs []int64
		if err := db.
			Model(&model.FriendGroup{}).
			Where("user_id = ? AND name = ? AND id <> ?", g.UserID, g.Name, g.KeepID).
			Pluck("id", &duplicateIDs).Error; err != nil {
			return err
		}
		if len(duplicateIDs) == 0 {
			continue
		}
		if err := db.
			Model(&model.Friend{}).
			Where("group_id IN ?", duplicateIDs).
			Update("group_id", g.KeepID).Error; err != nil {
			return err
		}
		if err := db.
			Where("id IN ?", duplicateIDs).
			Delete(&model.FriendGroup{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// UserRepository 用户数据访问接口
// 面向接口编程，便于单元测试时替换为 mock 实现
type UserRepository interface {
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	UpdateUser(ctx context.Context, user *model.User) error
	BatchGetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error)
	AddFriend(ctx context.Context, friend *model.Friend) error
	DeleteFriend(ctx context.Context, userID, friendID int64) error
	GetFriendList(ctx context.Context, userID int64) ([]model.Friend, error)
	GetFriendByUserAndFriendID(ctx context.Context, userID, friendID int64) (*model.Friend, error)
	UpdateFriendRemark(ctx context.Context, userID, friendID, groupID int64, remark string) error
	CreateFriendGroup(ctx context.Context, group *model.FriendGroup) error
	GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error)
	GetFriendGroupByID(ctx context.Context, id int64) (*model.FriendGroup, error)
}

// userRepositoryImpl 基于 GORM 的 UserRepository 实现
type userRepositoryImpl struct {
	db *gorm.DB
}

// NewUserRepo 创建 UserRepository 实例，注入数据库连接
func NewUserRepo(db *gorm.DB) UserRepository {
	return &userRepositoryImpl{db: db}
}

// CreateUser 创建新用户记录
func (r *userRepositoryImpl) CreateUser(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// GetUserByUsername 根据用户名查询用户（用于登录和注册去重）
// 未找到时返回 nil, nil（而非 error），调用方需检查返回值是否为 nil
func (r *userRepositoryImpl) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

// GetUserByID 根据用户ID查询用户
func (r *userRepositoryImpl) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

// UpdateUser 更新用户信息（全字段更新）
func (r *userRepositoryImpl) UpdateUser(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// BatchGetUsersByIDs 批量查询用户信息（用于好友列表中获取好友详情）
func (r *userRepositoryImpl) BatchGetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	var users []model.User
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error
	return users, err
}

// AddFriend 添加好友关系记录
func (r *userRepositoryImpl) AddFriend(ctx context.Context, friend *model.Friend) error {
	return r.db.WithContext(ctx).Create(friend).Error
}

// DeleteFriend 删除好友关系记录（单向删除，Service 层负责双向删除）
func (r *userRepositoryImpl) DeleteFriend(ctx context.Context, userID, friendID int64) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		Delete(&model.Friend{}).Error
}

// GetFriendList 获取用户的好友列表
func (r *userRepositoryImpl) GetFriendList(ctx context.Context, userID int64) ([]model.Friend, error) {
	var friends []model.Friend
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&friends).Error
	return friends, err
}

// GetFriendByUserAndFriendID 查询两个用户之间的好友关系（用于去重检查）
func (r *userRepositoryImpl) GetFriendByUserAndFriendID(ctx context.Context, userID, friendID int64) (*model.Friend, error) {
	var friend model.Friend
	err := r.db.WithContext(ctx).Where("user_id = ? AND friend_id = ?", userID, friendID).First(&friend).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &friend, err
}

// UpdateFriendRemark 更新好友的备注和分组
// 只更新非空字段，空字段保持原值不变
func (r *userRepositoryImpl) UpdateFriendRemark(ctx context.Context, userID, friendID, groupID int64, remark string) error {
	updates := map[string]interface{}{}
	if remark != "" {
		updates["remark"] = remark
	}
	if groupID > 0 {
		updates["group_id"] = groupID
	}
	return r.db.WithContext(ctx).
		Model(&model.Friend{}).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		Updates(updates).Error
}

// CreateFriendGroup 创建好友分组
func (r *userRepositoryImpl) CreateFriendGroup(ctx context.Context, group *model.FriendGroup) error {
	return r.db.WithContext(ctx).Create(group).Error
}

// GetFriendGroups 获取用户的所有好友分组
func (r *userRepositoryImpl) GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error) {
	var groups []model.FriendGroup
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&groups).Error
	return groups, err
}

// GetFriendGroupByID 根据ID查询好友分组
func (r *userRepositoryImpl) GetFriendGroupByID(ctx context.Context, id int64) (*model.FriendGroup, error) {
	var group model.FriendGroup
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &group, err
}
