package dao

import (
	"ClaranAIM/internal/user-service/model"
	"context"
	"errors"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	models := []interface{}{
		&model.User{},
		&model.Friend{},
		&model.FriendGroup{},
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

type userRepositoryImpl struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) UserRepository {
	return &userRepositoryImpl{db: db}
}

func (r *userRepositoryImpl) CreateUser(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *userRepositoryImpl) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *userRepositoryImpl) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &user, err
}

func (r *userRepositoryImpl) UpdateUser(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *userRepositoryImpl) BatchGetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	var users []model.User
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error
	return users, err
}

func (r *userRepositoryImpl) AddFriend(ctx context.Context, friend *model.Friend) error {
	return r.db.WithContext(ctx).Create(friend).Error
}

func (r *userRepositoryImpl) DeleteFriend(ctx context.Context, userID, friendID int64) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		Delete(&model.Friend{}).Error
}

func (r *userRepositoryImpl) GetFriendList(ctx context.Context, userID int64) ([]model.Friend, error) {
	var friends []model.Friend
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&friends).Error
	return friends, err
}

func (r *userRepositoryImpl) GetFriendByUserAndFriendID(ctx context.Context, userID, friendID int64) (*model.Friend, error) {
	var friend model.Friend
	err := r.db.WithContext(ctx).Where("user_id = ? AND friend_id = ?", userID, friendID).First(&friend).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &friend, err
}

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

func (r *userRepositoryImpl) CreateFriendGroup(ctx context.Context, group *model.FriendGroup) error {
	return r.db.WithContext(ctx).Create(group).Error
}

func (r *userRepositoryImpl) GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error) {
	var groups []model.FriendGroup
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&groups).Error
	return groups, err
}

func (r *userRepositoryImpl) GetFriendGroupByID(ctx context.Context, id int64) (*model.FriendGroup, error) {
	var group model.FriendGroup
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &group, err
}
