package service

import (
	"ClaranAIM/internal/user-service/dao"
	"ClaranAIM/internal/user-service/model"
	"ClaranAIM/pkg/cache"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/password"
	"context"
	"errors"
	"fmt"
	"strings"
)

// UserService 用户业务逻辑接口
// 定义用户服务所需的所有业务方法
type UserService interface {
	Register(ctx context.Context, username, pwd, nickname string) (*model.User, error)
	RegisterSystemUser(ctx context.Context, username, pwd, nickname string) (*model.User, error)
	Login(ctx context.Context, username, pwd, jwtSecret string, accessExpiration, refreshExpiration int64) (TokenPair, *model.User, error)
	GetUserInfo(ctx context.Context, userID int64) (*model.User, error)
	UpdateUserInfo(ctx context.Context, userID int64, profile UserProfileUpdate) error
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
	AdminListUsers(ctx context.Context, keyword, role, status string, includeSystem bool, limit, offset int64) ([]model.User, int64, error)
}

// TokenPair 是登录成功后返回的一组访问凭证。
//
// AccessToken 用于短期接口鉴权，RefreshToken 用于在访问 token 过期后换取新的
// access token，二者都会携带用户角色以支持管理接口鉴权。
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// UserProfileUpdate 承载个人资料页可编辑字段。
// FullUpdate=true 时按表单提交值覆盖资料字段，允许用户主动清空签名、简介等内容；
// FullUpdate=false 保持旧接口语义，只更新非空字段，兼容已有 RPC 调用。
type UserProfileUpdate struct {
	Nickname   string
	Email      string
	Phone      string
	Avatar     string
	Cover      string
	Signature  string
	Bio        string
	Location   string
	Website    string
	Gender     string
	Birthday   string
	FullUpdate bool
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
	return s.register(ctx, username, pwd, nickname, false)
}

// RegisterSystemUser 为 Agent 等内部参与者创建系统账号。
// 系统用户可以出现在 IM 成员和消息发送者字段中，但不能通过密码登录，因此不会被当作真人账号使用。
func (s *userServiceImpl) RegisterSystemUser(ctx context.Context, username, pwd, nickname string) (*model.User, error) {
	return s.register(ctx, username, pwd, nickname, true)
}

// register 复用真人注册和 Agent 系统用户注册的公共流程。
// 真人和系统用户都需要唯一用户名、bcrypt 密码和 10 位 UID；区别只在 IsSystem 标记，
// 登录时会据此拒绝系统用户用密码进入应用。
func (s *userServiceImpl) register(ctx context.Context, username, pwd, nickname string, isSystem bool) (*model.User, error) {
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

	var user *model.User
	for i := 0; i < 5; i++ {
		uid, err := idgen.NewUID10()
		if err != nil {
			return nil, err
		}
		existingUID, err := s.repo.GetUserByID(ctx, uid)
		if err != nil {
			return nil, err
		}
		if existingUID != nil {
			continue
		}
		user = &model.User{
			ID:       uid,
			Username: username,
			Password: hashedPwd,
			Nickname: nickname,
			Role:     jwt.RoleUser,
			IsSystem: isSystem,
			Status:   "offline",
		}
		if err := s.repo.CreateUser(ctx, user); err != nil {
			return nil, err
		}
		break
	}
	if user == nil {
		return nil, errors.New("生成用户UID失败，请重试")
	}

	s.cacheUserInfo(ctx, user)

	return user, nil
}

// Login 用户登录
// 流程：校验参数 → 查询用户 → bcrypt校验密码 → 生成JWT → 更新在线状态 → 缓存
func (s *userServiceImpl) Login(ctx context.Context, username, pwd, jwtSecret string, accessExpiration, refreshExpiration int64) (TokenPair, *model.User, error) {
	if username == "" || pwd == "" {
		return TokenPair{}, nil, errors.New("用户名和密码不能为空")
	}

	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return TokenPair{}, nil, err
	}
	if user == nil {
		return TokenPair{}, nil, errors.New("用户不存在")
	}
	if user.IsSystem {
		return TokenPair{}, nil, errors.New("系统用户不允许密码登录")
	}

	if !password.CheckPassword(pwd, user.Password) {
		return TokenPair{}, nil, errors.New("密码错误")
	}

	role := user.Role
	if role == "" {
		role = jwt.RoleUser
		user.Role = role
	}
	accessToken, err := jwt.GenerateAccessToken(jwtSecret, user.ID, user.Username, role, accessExpiration)
	if err != nil {
		return TokenPair{}, nil, err
	}
	refreshToken, err := jwt.GenerateRefreshToken(jwtSecret, user.ID, user.Username, role, refreshExpiration)
	if err != nil {
		return TokenPair{}, nil, err
	}

	user.Status = "online"
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return TokenPair{}, nil, fmt.Errorf("更新用户在线状态失败: %w", err)
	}

	s.cacheUserInfo(ctx, user)
	if s.redis != nil {
		onlinePolicy := cache.OnlineUserPolicy(user.ID)
		s.redis.SetWithJitter(ctx, onlinePolicy.Key, "1", onlinePolicy.TTL, onlinePolicy.Jitter)

		friends, _ := s.repo.GetFriendList(ctx, user.ID)
		for _, f := range friends {
			s.redis.Del(ctx, cache.UserFriendsPolicy(f.FriendID).Key)
		}
	}

	return TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, user, nil
}

// GetUserInfo 获取用户信息
// 优先从Redis缓存读取，未命中则查MySQL并回写缓存
func (s *userServiceImpl) GetUserInfo(ctx context.Context, userID int64) (*model.User, error) {
	if s.redis != nil {
		user := &model.User{}
		policy := cache.UserInfoPolicy(userID)
		found, err := s.redis.CacheAsideJSON(ctx, policy.Key, user, policy.TTL, func(ctx context.Context) (interface{}, bool, error) {
			dbUser, err := s.repo.GetUserByID(ctx, userID)
			if err != nil {
				return nil, false, err
			}
			if dbUser == nil {
				return nil, false, nil
			}
			return dbUser, true, nil
		})
		if err == nil && found {
			return user, nil
		}
		if err == nil && !found {
			return nil, errors.New("用户不存在")
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

// UpdateUserInfo 更新用户资料。
// 个人资料页使用 FullUpdate 覆盖式保存，允许清空字段；旧调用未设置 FullUpdate 时仍只更新非空字段。
// 数据库写入成功后按“写后删除”策略删除用户信息缓存，下一次读取再回源并重建缓存。
func (s *userServiceImpl) UpdateUserInfo(ctx context.Context, userID int64, profile UserProfileUpdate) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("用户不存在")
	}

	if profile.FullUpdate {
		user.Nickname = profile.Nickname
		user.Email = profile.Email
		user.Phone = profile.Phone
		user.Avatar = profile.Avatar
		user.Cover = profile.Cover
		user.Signature = profile.Signature
		user.Bio = profile.Bio
		user.Location = profile.Location
		user.Website = profile.Website
		user.Gender = profile.Gender
		user.Birthday = profile.Birthday
	} else {
		if profile.Nickname != "" {
			user.Nickname = profile.Nickname
		}
		if profile.Email != "" {
			user.Email = profile.Email
		}
		if profile.Phone != "" {
			user.Phone = profile.Phone
		}
		if profile.Avatar != "" {
			user.Avatar = profile.Avatar
		}
		if profile.Cover != "" {
			user.Cover = profile.Cover
		}
		if profile.Signature != "" {
			user.Signature = profile.Signature
		}
		if profile.Bio != "" {
			user.Bio = profile.Bio
		}
		if profile.Location != "" {
			user.Location = profile.Location
		}
		if profile.Website != "" {
			user.Website = profile.Website
		}
		if profile.Gender != "" {
			user.Gender = profile.Gender
		}
		if profile.Birthday != "" {
			user.Birthday = profile.Birthday
		}
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}

	s.invalidateUserInfoCache(ctx, userID)
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
	s.invalidateUserInfoCache(ctx, userID)
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
	s.invalidateUserInfoCache(ctx, userID)

	if s.redis != nil {
		if status == "online" {
			onlinePolicy := cache.OnlineUserPolicy(userID)
			s.redis.SetWithJitter(ctx, onlinePolicy.Key, "1", onlinePolicy.TTL, onlinePolicy.Jitter)
		} else {
			s.redis.Del(ctx, cache.OnlineUserPolicy(userID).Key)
		}

		friends, _ := s.repo.GetFriendList(ctx, userID)
		for _, f := range friends {
			s.redis.Del(ctx, cache.UserFriendsPolicy(f.FriendID).Key)
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

	friendUser, err := s.repo.GetUserByID(ctx, friendID)
	if err != nil {
		return fmt.Errorf("查询好友用户失败: %w", err)
	}
	if friendUser == nil {
		return errors.New("目标用户不存在")
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
	if err := s.repo.AddFriend(ctx, reverseFriend); err != nil {
		rollbackErr := s.repo.DeleteFriend(ctx, userID, friendID)
		if rollbackErr != nil {
			return fmt.Errorf("添加反向好友关系失败: %w；回滚正向关系也失败: %v", err, rollbackErr)
		}
		return fmt.Errorf("添加反向好友关系失败: %w", err)
	}

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
	if err := s.repo.DeleteFriend(ctx, friendID, userID); err != nil {
		return fmt.Errorf("删除反向好友关系失败: %w", err)
	}

	s.invalidateFriendCache(ctx, userID)
	s.invalidateFriendCache(ctx, friendID)

	return nil
}

// GetFriendList 获取好友列表
// 返回好友信息、备注、分组、在线状态等完整信息，支持Redis缓存
func (s *userServiceImpl) GetFriendList(ctx context.Context, userID int64) ([]FriendInfo, error) {
	if s.redis != nil {
		policy := cache.UserFriendsPolicy(userID)
		var cached []FriendInfo
		hit, err := s.redis.GetJSON(ctx, policy.Key, &cached)
		if err == nil && hit != "" {
			if s.redis.IsNullHit(hit) {
				return nil, nil
			}
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
			if info.FriendName == "" {
				info.FriendName = friendUser.Username
			}
			info.FriendAvatar = friendUser.Avatar
			info.FriendStatus = friendUser.Status
		}
		info.GroupName = groupName

		result = append(result, info)
	}

	if s.redis != nil {
		policy := cache.UserFriendsPolicy(userID)
		if len(result) == 0 {
			s.redis.SetNull(ctx, policy.Key, policy.NullTTL, policy.NullJitter)
		} else {
			s.redis.SetJSONWithJitter(ctx, policy.Key, result, policy.TTL, policy.Jitter)
		}
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
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("分组名称不能为空")
	}
	groups, err := s.repo.GetFriendGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if strings.EqualFold(group.Name, name) {
			return nil, errors.New("分组已存在")
		}
	}
	group := &model.FriendGroup{
		UserID: userID,
		Name:   name,
	}
	if err := s.repo.CreateFriendGroup(ctx, group); err != nil {
		return nil, err
	}

	if s.redis != nil {
		s.redis.Del(ctx, cache.FriendGroupsPolicy(userID).Key)
	}

	return group, nil
}

// GetFriendGroups 获取好友分组列表
// 支持Redis缓存，缓存10分钟
func (s *userServiceImpl) GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error) {
	if s.redis != nil {
		policy := cache.FriendGroupsPolicy(userID)
		var cached []model.FriendGroup
		hit, err := s.redis.GetJSON(ctx, policy.Key, &cached)
		if err == nil && hit != "" {
			if s.redis.IsNullHit(hit) {
				return nil, nil
			}
			return cached, nil
		}
	}

	groups, err := s.repo.GetFriendGroups(ctx, userID)
	if err != nil {
		return nil, err
	}

	if s.redis != nil {
		policy := cache.FriendGroupsPolicy(userID)
		if len(groups) == 0 {
			s.redis.SetNull(ctx, policy.Key, policy.NullTTL, policy.NullJitter)
		} else {
			s.redis.SetJSONWithJitter(ctx, policy.Key, groups, policy.TTL, policy.Jitter)
		}
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
			policy := cache.UserInfoPolicy(id)
			hit, err := s.redis.GetJSON(ctx, policy.Key, user)
			if err == nil && hit != "" {
				if s.redis.IsNullHit(hit) {
					continue
				}
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

// AdminListUsers 为 admin-service 提供用户运营列表。
// 它不做角色鉴权，调用方必须来自只挂在 /api/v1/admin 下的管理链路。
func (s *userServiceImpl) AdminListUsers(ctx context.Context, keyword, role, status string, includeSystem bool, limit, offset int64) ([]model.User, int64, error) {
	return s.repo.AdminListUsers(ctx, strings.TrimSpace(keyword), strings.TrimSpace(role), strings.TrimSpace(status), includeSystem, limit, offset)
}

// cacheUserInfo 缓存用户信息到Redis
// 缓存15分钟，key格式：user:info:{userID}
func (s *userServiceImpl) cacheUserInfo(ctx context.Context, user *model.User) {
	if s.redis == nil {
		return
	}
	policy := cache.UserInfoPolicy(user.ID)
	s.redis.SetJSONWithJitter(ctx, policy.Key, user, policy.TTL, policy.Jitter)
}

// invalidateUserInfoCache 执行用户资料的写后删除策略。
// 用户昵称、头像、个人资料等写库成功后先删缓存，下一次读取再回源重建，避免写缓存失败造成脏读。
func (s *userServiceImpl) invalidateUserInfoCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, cache.UserInfoPolicy(userID).Key)
}

// invalidateFriendCache 清除好友列表缓存
// 当好友关系发生变化时调用（添加/删除好友）
func (s *userServiceImpl) invalidateFriendCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, cache.UserFriendsPolicy(userID).Key)
}
