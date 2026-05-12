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

// UserService 用户业务逻辑接口
// 定义用户服务所需的所有业务方法
type UserService interface {
	Register(ctx context.Context, username, pwd, nickname string) (*model.User, error)                         // 用户注册：用户名去重 + bcrypt加密 + 写库 + 缓存
	Login(ctx context.Context, username, pwd, jwtSecret string, expiration int64) (string, *model.User, error) // 用户登录：bcrypt校验 + 生成JWT + 更新在线状态 + 缓存
	GetUserInfo(ctx context.Context, userID int64) (*model.User, error)                                        // 获取用户信息：优先Redis缓存，未命中查MySQL
	UpdateUserInfo(ctx context.Context, userID int64, nickname, email, phone string) error                     // 更新用户信息：只更新非空字段，刷新缓存
	UpdateAvatar(ctx context.Context, userID int64, avatar string) error                                       // 更新用户头像
	UpdateStatus(ctx context.Context, userID int64, status string) error                                       // 更新在线状态：同时更新MySQL和Redis
	AddFriend(ctx context.Context, userID, friendID, groupID int64, remark string) error                       // 添加好友：双向添加，清除双方缓存
	DeleteFriend(ctx context.Context, userID, friendID int64) error                                            // 删除好友：双向删除，清除双方缓存
	GetFriendList(ctx context.Context, userID int64) ([]FriendInfo, error)                                     // 获取好友列表：含好友信息和分组，支持缓存
	UpdateFriendRemark(ctx context.Context, userID, friendID, groupID int64, remark string) error              // 更新好友备注和分组
	CreateFriendGroup(ctx context.Context, userID int64, name string) (*model.FriendGroup, error)              // 创建好友分组
	GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error)                            // 获取好友分组列表
	MoveFriendGroup(ctx context.Context, userID, friendID, groupID int64) error                                // 移动好友到指定分组
	BatchGetUserInfo(ctx context.Context, ids []int64) ([]model.User, error)                                   // 批量获取用户信息：优先缓存，未命中查DB
}

// FriendInfo 好友信息结构体
// 在好友列表中展示的完整好友信息，包含好友用户详情和分组信息
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

// userServiceImpl 用户业务逻辑实现
// 依赖 UserRepository 和可选的 Redis 缓存
type userServiceImpl struct {
	repo  dao.UserRepository
	redis *redis.RedisClient
}

// NewUserService 创建 UserService 实例，注入 DAO 层和可选的 Redis 缓存
func NewUserService(repo dao.UserRepository, r *redis.RedisClient) UserService {
	return &userServiceImpl{repo: repo, redis: r}
}

// Register 用户注册
// 流程：校验参数 → 用户名去重 → bcrypt加密密码 → 写库 → 缓存用户信息
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

// Login 用户登录
// 流程：校验参数 → 查询用户 → bcrypt校验密码 → 生成JWT → 更新在线状态 → 缓存
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

// GetUserInfo 获取用户信息
// 优先从Redis缓存读取，未命中则查MySQL并回写缓存
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

// UpdateUserInfo 更新用户信息（昵称/邮箱/手机）
// 只更新非空字段，更新后刷新Redis缓存
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

// UpdateAvatar 更新用户头像URL
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

// UpdateStatus 更新用户在线状态
// 同时更新MySQL和Redis中的在线状态
// 并清除所有好友的好友列表缓存，确保好友看到的在线状态实时更新
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

		friends, _ := s.repo.GetFriendList(ctx, userID)
		for _, f := range friends {
			s.redis.Del(ctx, fmt.Sprintf("user:friends:%d", f.FriendID))
		}
	}

	return nil
}

// AddFriend 添加好友
// 双向添加：A添加B时，B的好友列表也会出现A
// 添加后清除双方的好友列表缓存
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

// DeleteFriend 删除好友
// 双向删除：A删除B时，B的好友列表也会移除A
func (s *userServiceImpl) DeleteFriend(ctx context.Context, userID, friendID int64) error {
	if err := s.repo.DeleteFriend(ctx, userID, friendID); err != nil {
		return err
	}
	_ = s.repo.DeleteFriend(ctx, friendID, userID)

	s.invalidateFriendCache(ctx, userID)
	s.invalidateFriendCache(ctx, friendID)

	return nil
}

// GetFriendList 获取好友列表
// 返回好友信息、备注、分组、在线状态等完整信息，支持Redis缓存
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

// UpdateFriendRemark 更新好友备注和分组
// 更新后清除好友列表缓存
func (s *userServiceImpl) UpdateFriendRemark(ctx context.Context, userID, friendID, groupID int64, remark string) error {
	if err := s.repo.UpdateFriendRemark(ctx, userID, friendID, groupID, remark); err != nil {
		return err
	}
	s.invalidateFriendCache(ctx, userID)
	return nil
}

// CreateFriendGroup 创建好友分组
// 创建后清除好友分组缓存
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

// GetFriendGroups 获取好友分组列表
// 支持Redis缓存，缓存10分钟
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

// MoveFriendGroup 移动好友到指定分组
// 更新后清除好友列表缓存
func (s *userServiceImpl) MoveFriendGroup(ctx context.Context, userID, friendID, groupID int64) error {
	if err := s.repo.UpdateFriendRemark(ctx, userID, friendID, groupID, ""); err != nil {
		return err
	}
	s.invalidateFriendCache(ctx, userID)
	return nil
}

// BatchGetUserInfo 批量获取用户信息
// 优先从Redis缓存读取，未命中的ID再查MySQL
// 用于好友列表等需要批量获取用户信息的场景
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

// cacheUserInfo 缓存用户信息到Redis
// 缓存15分钟，key格式：user:info:{userID}
func (s *userServiceImpl) cacheUserInfo(ctx context.Context, user *model.User) {
	if s.redis == nil {
		return
	}
	cacheKey := fmt.Sprintf("user:info:%d", user.ID)
	s.redis.SetJSON(ctx, cacheKey, user, 15*time.Minute)
}

// invalidateFriendCache 清除好友列表缓存
// 当好友关系发生变化时调用（添加/删除好友）
func (s *userServiceImpl) invalidateFriendCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("user:friends:%d", userID))
}
