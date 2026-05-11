package service

import (
	"ClaranAIM/internal/user-service/dao"
	"ClaranAIM/internal/user-service/model"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/password"
	"context"
	"errors"
	"fmt"
	"time"
)

type UserService interface {
	Register(ctx context.Context, username, pwd, nickname string) (*model.User, error)
	Login(ctx context.Context, username, pwd, jwtSecret string, expiration int64) (string, *model.User, error)
	GetUserInfo(ctx context.Context, userID int64) (*model.User, error)
	UpdateUserInfo(ctx context.Context, userID int64, nickname, email, phone string) error
	UpdateAvatar(ctx context.Context, userID int64, avatar string) error
	UpdateStatus(ctx context.Context, userID int64, status string) error
	AddFriend(ctx context.Context, userID, friendID, groupID int64, remark string) error
	DeleteFriend(ctx context.Context, userID, friendID int64) error
	GetFriendList(ctx context.Context, userID int64) ([]FriendInfo, error)
	UpdateFriendRemark(ctx context.Context, userID, friendID, groupID int64, remark string) error
	CreateFriendGroup(ctx context.Context, userID int64, name string) (*model.FriendGroup, error)
	GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error)
	MoveFriendGroup(ctx context.Context, userID, friendID, groupID int64) error
	BatchGetUserInfo(ctx context.Context, ids []int64) ([]model.User, error)
}

type FriendInfo struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"user_id"`
	FriendID     int64  `json:"friend_id"`
	GroupID      int64  `json:"group_id"`
	Remark       string `json:"remark"`
	FriendName   string `json:"friend_name"`
	FriendAvatar string `json:"friend_avatar"`
	FriendStatus string `json:"friend_status"`
	GroupName    string `json:"group_name"`
}

type userServiceImpl struct {
	repo  dao.UserRepository
	redis *redis.RedisClient
}

func NewUserService(repo dao.UserRepository, r *redis.RedisClient) UserService {
	return &userServiceImpl{repo: repo, redis: r}
}

func (s *userServiceImpl) Register(ctx context.Context, username, pwd, nickname string) (*model.User, error) {
	if username == "" || pwd == "" {
		return nil, errors.New("用户名和密码不能为空")
	}

	existing, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("用户名已存在")
	}

	hashedPwd, err := password.HashPassword(pwd)
	if err != nil {
		return nil, err
	}

	if nickname == "" {
		nickname = username
	}

	user := &model.User{
		Username: username,
		Password: hashedPwd,
		Nickname: nickname,
		Status:   "offline",
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	s.cacheUserInfo(ctx, user)

	return user, nil
}

func (s *userServiceImpl) Login(ctx context.Context, username, pwd, jwtSecret string, expiration int64) (string, *model.User, error) {
	if username == "" || pwd == "" {
		return "", nil, errors.New("用户名和密码不能为空")
	}

	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return "", nil, err
	}
	if user == nil {
		return "", nil, errors.New("用户不存在")
	}

	if !password.CheckPassword(pwd, user.Password) {
		return "", nil, errors.New("密码错误")
	}

	token, err := jwt.GenerateToken(jwtSecret, user.ID, user.Username, expiration)
	if err != nil {
		return "", nil, err
	}

	user.Status = "online"
	_ = s.repo.UpdateUser(ctx, user)

	s.cacheUserInfo(ctx, user)
	if s.redis != nil {
		s.redis.Set(ctx, "online:user:"+fmt.Sprintf("%d", user.ID), "1", 30*time.Minute)
	}

	return token, user, nil
}

func (s *userServiceImpl) GetUserInfo(ctx context.Context, userID int64) (*model.User, error) {
	if s.redis != nil {
		user := &model.User{}
		cacheKey := fmt.Sprintf("user:info:%d", userID)
		hit, err := s.redis.GetJSON(ctx, cacheKey, user)
		if err == nil && hit != "" {
			return user, nil
		}
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("用户不存在")
	}

	s.cacheUserInfo(ctx, user)

	return user, nil
}

func (s *userServiceImpl) UpdateUserInfo(ctx context.Context, userID int64, nickname, email, phone string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("用户不存在")
	}

	if nickname != "" {
		user.Nickname = nickname
	}
	if email != "" {
		user.Email = email
	}
	if phone != "" {
		user.Phone = phone
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}

	s.cacheUserInfo(ctx, user)
	return nil
}

func (s *userServiceImpl) UpdateAvatar(ctx context.Context, userID int64, avatar string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("用户不存在")
	}
	user.Avatar = avatar
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	s.cacheUserInfo(ctx, user)
	return nil
}

func (s *userServiceImpl) UpdateStatus(ctx context.Context, userID int64, status string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("用户不存在")
	}
	user.Status = status
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	s.cacheUserInfo(ctx, user)

	if s.redis != nil {
		if status == "online" {
			s.redis.Set(ctx, "online:user:"+fmt.Sprintf("%d", userID), "1", 30*time.Minute)
		} else {
			s.redis.Del(ctx, "online:user:"+fmt.Sprintf("%d", userID))
		}
	}

	return nil
}

func (s *userServiceImpl) AddFriend(ctx context.Context, userID, friendID, groupID int64, remark string) error {
	if userID == friendID {
		return errors.New("不能添加自己为好友")
	}

	existing, err := s.repo.GetFriendByUserAndFriendID(ctx, userID, friendID)
	if err != nil {
		return err
	}
	if existing != nil {
		return errors.New("已经是好友关系")
	}

	friend := &model.Friend{
		UserID:   userID,
		FriendID: friendID,
		GroupID:  groupID,
		Remark:   remark,
	}
	if err := s.repo.AddFriend(ctx, friend); err != nil {
		return err
	}

	reverseFriend := &model.Friend{
		UserID:   friendID,
		FriendID: userID,
		GroupID:  0,
	}
	_ = s.repo.AddFriend(ctx, reverseFriend)

	s.invalidateFriendCache(ctx, userID)
	s.invalidateFriendCache(ctx, friendID)

	return nil
}

func (s *userServiceImpl) DeleteFriend(ctx context.Context, userID, friendID int64) error {
	if err := s.repo.DeleteFriend(ctx, userID, friendID); err != nil {
		return err
	}
	_ = s.repo.DeleteFriend(ctx, friendID, userID)

	s.invalidateFriendCache(ctx, userID)
	s.invalidateFriendCache(ctx, friendID)

	return nil
}

func (s *userServiceImpl) GetFriendList(ctx context.Context, userID int64) ([]FriendInfo, error) {
	if s.redis != nil {
		cacheKey := fmt.Sprintf("user:friends:%d", userID)
		var cached []FriendInfo
		hit, err := s.redis.GetJSON(ctx, cacheKey, &cached)
		if err == nil && hit != "" {
			return cached, nil
		}
	}

	friends, err := s.repo.GetFriendList(ctx, userID)
	if err != nil {
		return nil, err
	}

	var result []FriendInfo
	for _, f := range friends {
		friendUser, _ := s.repo.GetUserByID(ctx, f.FriendID)
		var groupName string
		if f.GroupID > 0 {
			group, _ := s.repo.GetFriendGroupByID(ctx, f.GroupID)
			if group != nil {
				groupName = group.Name
			}
		}

		info := FriendInfo{
			ID:       f.ID,
			UserID:   f.UserID,
			FriendID: f.FriendID,
			GroupID:  f.GroupID,
			Remark:   f.Remark,
		}
		if friendUser != nil {
			info.FriendName = friendUser.Nickname
			info.FriendAvatar = friendUser.Avatar
			info.FriendStatus = friendUser.Status
		}
		info.GroupName = groupName

		result = append(result, info)
	}

	if s.redis != nil && len(result) > 0 {
		cacheKey := fmt.Sprintf("user:friends:%d", userID)
		s.redis.SetJSON(ctx, cacheKey, result, 5*time.Minute)
	}

	return result, nil
}

func (s *userServiceImpl) UpdateFriendRemark(ctx context.Context, userID, friendID, groupID int64, remark string) error {
	if err := s.repo.UpdateFriendRemark(ctx, userID, friendID, groupID, remark); err != nil {
		return err
	}
	s.invalidateFriendCache(ctx, userID)
	return nil
}

func (s *userServiceImpl) CreateFriendGroup(ctx context.Context, userID int64, name string) (*model.FriendGroup, error) {
	group := &model.FriendGroup{
		UserID: userID,
		Name:   name,
	}
	if err := s.repo.CreateFriendGroup(ctx, group); err != nil {
		return nil, err
	}

	if s.redis != nil {
		cacheKey := fmt.Sprintf("user:friend_groups:%d", userID)
		s.redis.Del(ctx, cacheKey)
	}

	return group, nil
}

func (s *userServiceImpl) GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error) {
	if s.redis != nil {
		cacheKey := fmt.Sprintf("user:friend_groups:%d", userID)
		var cached []model.FriendGroup
		hit, err := s.redis.GetJSON(ctx, cacheKey, &cached)
		if err == nil && hit != "" {
			return cached, nil
		}
	}

	groups, err := s.repo.GetFriendGroups(ctx, userID)
	if err != nil {
		return nil, err
	}

	if s.redis != nil && len(groups) > 0 {
		cacheKey := fmt.Sprintf("user:friend_groups:%d", userID)
		s.redis.SetJSON(ctx, cacheKey, groups, 10*time.Minute)
	}

	return groups, nil
}

func (s *userServiceImpl) MoveFriendGroup(ctx context.Context, userID, friendID, groupID int64) error {
	if err := s.repo.UpdateFriendRemark(ctx, userID, friendID, groupID, ""); err != nil {
		return err
	}
	s.invalidateFriendCache(ctx, userID)
	return nil
}

func (s *userServiceImpl) BatchGetUserInfo(ctx context.Context, ids []int64) ([]model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	if s.redis != nil {
		var cachedUsers []model.User
		var missedIDs []int64

		for _, id := range ids {
			user := &model.User{}
			cacheKey := fmt.Sprintf("user:info:%d", id)
			hit, err := s.redis.GetJSON(ctx, cacheKey, user)
			if err == nil && hit != "" {
				cachedUsers = append(cachedUsers, *user)
			} else {
				missedIDs = append(missedIDs, id)
			}
		}

		if len(missedIDs) == 0 {
			return cachedUsers, nil
		}

		dbUsers, err := s.repo.BatchGetUsersByIDs(ctx, missedIDs)
		if err != nil {
			return nil, err
		}

		for _, u := range dbUsers {
			s.cacheUserInfo(ctx, &u)
			cachedUsers = append(cachedUsers, u)
		}

		return cachedUsers, nil
	}

	return s.repo.BatchGetUsersByIDs(ctx, ids)
}

func (s *userServiceImpl) cacheUserInfo(ctx context.Context, user *model.User) {
	if s.redis == nil {
		return
	}
	cacheKey := fmt.Sprintf("user:info:%d", user.ID)
	s.redis.SetJSON(ctx, cacheKey, user, 15*time.Minute)
}

func (s *userServiceImpl) invalidateFriendCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("user:friends:%d", userID))
}
