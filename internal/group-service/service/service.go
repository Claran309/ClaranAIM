// Package service 实现 group-service 的业务逻辑层
// 包含群组 CRUD、成员管理、权限校验等核心业务规则
package service

import (
	"ClaranAIM/internal/group-service/dao"
	"ClaranAIM/internal/group-service/model"
	"ClaranAIM/pkg/cache"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// GroupService 定义群领域业务契约。
// group-service 拥有群资料、成员关系、角色、禁言状态，以及供 msg-core-service 维护消息扇出的 group.* Outbox 事件。
type GroupService interface {
	CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*model.Group, error)
	CreateGroupWithID(ctx context.Context, groupID int64, name string, ownerID int64, memberIDs []int64) (*model.Group, error)
	DeleteGroup(ctx context.Context, groupID, operatorID int64) error
	UpdateGroup(ctx context.Context, groupID, operatorID int64, name, announcement string) error
	GetGroup(ctx context.Context, groupID int64) (*model.Group, error)
	GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error)
	InviteMember(ctx context.Context, groupID, operatorID int64, userIDs []int64) error
	KickMember(ctx context.Context, groupID, operatorID, userID int64) error
	MuteMember(ctx context.Context, groupID, operatorID, userID int64, durationMinutes int64) error
	UnmuteMember(ctx context.Context, groupID, operatorID, userID int64) error
	SetRole(ctx context.Context, groupID, operatorID, userID int64, role string) error
	GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error)
	CheckMember(ctx context.Context, groupID, userID int64) (bool, string, error)
	TransferOwner(ctx context.Context, groupID, operatorID, newOwnerID int64) error
	PinGroup(ctx context.Context, groupID, operatorID int64, isPinned bool) error
	AdminListGroups(ctx context.Context, keyword string, ownerID, limit, offset int64) ([]model.Group, int64, error)
	AdminUpdateGroupStatus(ctx context.Context, adminID, groupID int64, status, reason string) (*model.Group, error)
}

// groupServiceImpl 串联群组仓储和可选 Redis。
// 群资料、成员快照和用户群列表都采用 cache-aside + 写后删除，群事件通过事务 Outbox 交给 Kafka。
type groupServiceImpl struct {
	repo  dao.GroupRepository
	redis *redis.RedisClient
}

// NewGroupService 使用仓储和可选 Redis 缓存创建群业务服务。
func NewGroupService(repo dao.GroupRepository, r *redis.RedisClient) GroupService {
	return &groupServiceImpl{repo: repo, redis: r}
}

// CreateGroup 创建群组
// 流程：创建群组记录 → 群主自动成为成员(role=owner) → 添加其他成员(role=member)
func (s *groupServiceImpl) CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*model.Group, error) {
	return s.createGroup(ctx, 0, name, ownerID, memberIDs)
}

// CreateGroupWithID 创建使用外部分配群 ID 的群组。
//
// DTM Saga 分支不能依赖前一个 HTTP 分支的返回值继续传参，因此 api-gateway 会在
// 提交 Saga 前预生成 group_id，并把同一个 ID 传给 group-service 和 msg-core-service。
func (s *groupServiceImpl) CreateGroupWithID(ctx context.Context, groupID int64, name string, ownerID int64, memberIDs []int64) (*model.Group, error) {
	if groupID <= 0 {
		return nil, errors.New("group_id不能为空")
	}
	return s.createGroup(ctx, groupID, name, ownerID, memberIDs)
}

// createGroup 封装普通创建和 DTM 预分配 ID 创建两条路径。
// 它要求三人及以上才建群，群资料、成员关系和 group.created Outbox 事件必须在同一事务提交。
func (s *groupServiceImpl) createGroup(ctx context.Context, groupID int64, name string, ownerID int64, memberIDs []int64) (*model.Group, error) {
	if name == "" {
		return nil, errors.New("群名不能为空")
	}
	if groupID == 0 {
		var err error
		groupID, err = s.newUniqueGroupID(ctx)
		if err != nil {
			return nil, err
		}
	}

	validMembers := make([]int64, 0)
	for _, uid := range memberIDs {
		if uid != ownerID && uid > 0 {
			validMembers = append(validMembers, uid)
		}
	}
	if len(validMembers) < 2 {
		return nil, errors.New("群聊至少需要3人（包括创建者），2人请使用私聊")
	}

	group := &model.Group{
		ID:      groupID,
		Name:    name,
		OwnerID: ownerID,
		Status:  "active",
	}

	if err := s.repo.WithTransaction(ctx, func(tx dao.GroupRepository) error {
		// 群资料、群主/成员记录和 group.created 事件在同一事务提交。
		// Kafka 不可用时 Outbox 行仍会保留，后续由 Worker 重试发布。
		if err := tx.CreateGroup(ctx, group); err != nil {
			return err
		}

		ownerMember := &model.GroupMember{
			GroupID: group.ID,
			UserID:  ownerID,
			Role:    "owner",
		}
		if err := tx.AddMember(ctx, ownerMember); err != nil {
			return err
		}

		for _, uid := range memberIDs {
			if uid == ownerID {
				continue
			}
			member := &model.GroupMember{
				GroupID: group.ID,
				UserID:  uid,
				Role:    "member",
			}
			if err := tx.AddMember(ctx, member); err != nil {
				return err
			}
		}
		allMembers := append([]int64{ownerID}, validMembers...)
		return s.saveGroupEvent(ctx, tx, events.EventTypeGroupCreated, group.ID, events.GroupCreatedPayload{
			GroupID:   group.ID,
			OwnerID:   ownerID,
			MemberIDs: dedupeInt64(allMembers),
			Name:      group.Name,
		})
	}); err != nil {
		return nil, err
	}

	s.invalidateGroupCache(ctx, group.ID)
	s.invalidateUserGroupsCache(ctx, ownerID)
	for _, uid := range memberIDs {
		s.invalidateUserGroupsCache(ctx, uid)
	}

	return group, nil
}

// DeleteGroup 解散群组
// 权限：只有群主才能解散群组
func (s *groupServiceImpl) DeleteGroup(ctx context.Context, groupID, operatorID int64) error {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("群组不存在")
	}
	if group.OwnerID != operatorID {
		return errors.New("只有群主才能解散群组")
	}

	members, _ := s.repo.GetGroupMembers(ctx, groupID)

	if err := s.repo.WithTransaction(ctx, func(tx dao.GroupRepository) error {
		// 删除群资料和写入 group.deleted Outbox 事件必须在同一事务内完成。
		// 这样 Kafka 投递后，msg-core-service 能可靠地把侧边栏会话标成“已解散群”占位。
		if err := tx.DeleteGroup(ctx, groupID); err != nil {
			return err
		}
		return s.saveGroupEvent(ctx, tx, events.EventTypeGroupDeleted, groupID, events.GroupDeletedPayload{
			GroupID:    groupID,
			OperatorID: operatorID,
			MemberIDs:  memberIDsFromMembers(members),
		})
	}); err != nil {
		return err
	}

	s.invalidateGroupCache(ctx, groupID)
	s.invalidateGroupMembersCache(ctx, groupID)
	for _, m := range members {
		s.invalidateUserGroupsCache(ctx, m.UserID)
	}

	return nil
}

// UpdateGroup 更新群组信息（名称/公告）
// 权限：群主和管理员可以修改
func (s *groupServiceImpl) UpdateGroup(ctx context.Context, groupID, operatorID int64, name, announcement string) error {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("群组不存在")
	}

	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}

	if name != "" {
		group.Name = name
	}
	// 空公告是有效值，表示操作者明确清空公告。
	// api-gateway 会在调用本方法前保留“字段未传”和“传了空字符串”的区别。
	group.Announcement = announcement

	if err := s.repo.UpdateGroup(ctx, group); err != nil {
		return err
	}

	s.invalidateGroupCache(ctx, groupID)
	return nil
}

// GetGroup 获取群组信息
func (s *groupServiceImpl) GetGroup(ctx context.Context, groupID int64) (*model.Group, error) {
	if s.redis != nil {
		// CacheAsideJSON 对未命中写空值标记，并给 TTL 加随机抖动。
		// 这样不存在的群号不会反复穿透到数据库，也能降低同批缓存同时过期的风险。
		policy := cache.GroupInfoPolicy(groupID)
		var cached model.Group
		found, err := s.redis.CacheAsideJSON(ctx, policy.Key, &cached, policy.TTL, func(ctx context.Context) (interface{}, bool, error) {
			group, err := s.repo.GetGroupByID(ctx, groupID)
			if err != nil {
				return nil, false, err
			}
			if group == nil {
				return nil, false, nil
			}
			return group, true, nil
		})
		if err == nil && found {
			return &cached, nil
		}
		if err == nil && !found {
			return nil, errors.New("群组不存在")
		}
	}

	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("群组不存在")
	}

	return group, nil
}

// GetUserGroups 获取用户所在的所有群组
func (s *groupServiceImpl) GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error) {
	if s.redis != nil {
		policy := cache.UserGroupsPolicy(userID)
		var cached []model.Group
		hit, err := s.redis.GetJSON(ctx, policy.Key, &cached)
		if err == nil && hit != "" {
			if s.redis.IsNullHit(hit) {
				return nil, nil
			}
			return cached, nil
		}
	}

	groups, err := s.repo.GetUserGroups(ctx, userID)
	if err != nil {
		return nil, err
	}

	if s.redis != nil {
		policy := cache.UserGroupsPolicy(userID)
		if len(groups) == 0 {
			s.redis.SetNull(ctx, policy.Key, policy.NullTTL, policy.NullJitter)
		} else {
			s.redis.SetJSONWithJitter(ctx, policy.Key, groups, policy.TTL, policy.Jitter)
		}
	}

	return groups, nil
}

// AdminListGroups 为 admin-service 提供全局群运营列表。
// 该方法只读，不绕过普通群管理接口执行写操作。
func (s *groupServiceImpl) AdminListGroups(ctx context.Context, keyword string, ownerID, limit, offset int64) ([]model.Group, int64, error) {
	return s.repo.AdminListGroups(ctx, keyword, ownerID, limit, offset)
}

// AdminUpdateGroupStatus 由管理端封禁或解封群聊。
// 它只修改群治理状态，不删除成员和历史消息；msg-core-service 在发送链路读取该状态并拒绝已封禁群的新消息。
func (s *groupServiceImpl) AdminUpdateGroupStatus(ctx context.Context, adminID, groupID int64, status, reason string) (*model.Group, error) {
	_ = adminID
	_ = reason
	status = normalizeGroupStatus(status)
	if status == "" {
		return nil, errors.New("status只能是active或banned")
	}
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("群组不存在")
	}
	if err := s.repo.UpdateGroupStatus(ctx, groupID, status); err != nil {
		return nil, err
	}
	group.Status = status
	s.invalidateGroupCache(ctx, groupID)
	return group, nil
}

// InviteMember 邀请成员加入群组
// 权限：群主和管理员可以邀请
// 已在群中的用户不重复添加
func (s *groupServiceImpl) InviteMember(ctx context.Context, groupID, operatorID int64, userIDs []int64) error {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("群组不存在")
	}
	userIDs = dedupeInt64(userIDs)
	if len(userIDs) == 0 {
		return errors.New("被邀请用户不能为空")
	}
	selfJoin := len(userIDs) == 1 && userIDs[0] == operatorID
	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if !selfJoin && (member == nil || (member.Role != "owner" && member.Role != "admin")) {
		return errors.New("权限不足")
	}

	var members []model.GroupMember
	if err := s.repo.WithTransaction(ctx, func(tx dao.GroupRepository) error {
		// 事件 payload 携带完整成员快照，而不是只携带本次邀请的人。
		// msg-core-service 可以用完整视图重建会话参与者，并在重复投递时保持幂等。
		for _, uid := range userIDs {
			existing, _ := tx.GetMember(ctx, groupID, uid)
			if existing != nil {
				continue
			}
			newMember := &model.GroupMember{
				GroupID: groupID,
				UserID:  uid,
				Role:    "member",
			}
			if err := tx.AddMember(ctx, newMember); err != nil {
				return err
			}
		}
		var err error
		members, err = tx.GetGroupMembers(ctx, groupID)
		if err != nil {
			return err
		}
		return s.saveGroupEvent(ctx, tx, events.EventTypeGroupMemberInvited, groupID, events.GroupMemberInvitedPayload{
			GroupID:    groupID,
			OperatorID: operatorID,
			UserIDs:    userIDs,
			MemberIDs:  memberIDsFromMembers(members),
		})
	}); err != nil {
		return err
	}

	s.invalidateGroupMembersCache(ctx, groupID)
	for _, uid := range userIDs {
		s.invalidateUserGroupsCache(ctx, uid)
	}

	return nil
}

// KickMember 踢出群组成员
// 权限：群主和管理员可以踢人，但不能踢出群主
func (s *groupServiceImpl) KickMember(ctx context.Context, groupID, operatorID, userID int64) error {
	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}

	target, err := s.repo.GetMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if target == nil {
		return errors.New("该用户不在群组中")
	}
	if target.Role == "owner" {
		return errors.New("不能踢出群主")
	}

	var members []model.GroupMember
	if err := s.repo.WithTransaction(ctx, func(tx dao.GroupRepository) error {
		// 事件同时携带被踢用户和剩余成员快照。
		// 下游服务可以准确移除旧参与者，并刷新受影响用户的侧边栏缓存。
		if err := tx.RemoveMember(ctx, groupID, userID); err != nil {
			return err
		}
		var err error
		members, err = tx.GetGroupMembers(ctx, groupID)
		if err != nil {
			return err
		}
		return s.saveGroupEvent(ctx, tx, events.EventTypeGroupMemberKicked, groupID, events.GroupMemberKickedPayload{
			GroupID:    groupID,
			OperatorID: operatorID,
			UserID:     userID,
			MemberIDs:  memberIDsFromMembers(members),
		})
	}); err != nil {
		return err
	}

	s.invalidateGroupMembersCache(ctx, groupID)
	s.invalidateUserGroupsCache(ctx, userID)

	return nil
}

// MuteMember 禁言群组成员
// 权限：群主和管理员可以禁言
// durationMinutes 为禁言时长（分钟），0 表示解除禁言
func (s *groupServiceImpl) MuteMember(ctx context.Context, groupID, operatorID, userID int64, durationMinutes int64) error {
	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}

	mutedUntil := time.Now().Add(time.Duration(durationMinutes) * time.Minute)
	if err := s.repo.UpdateMuteStatus(ctx, groupID, userID, &mutedUntil); err != nil {
		return err
	}

	s.invalidateGroupMembersCache(ctx, groupID)
	return nil
}

// SetRole 设置群组成员角色
// 权限：只有群主才能设置角色
// 角色只能是 admin 或 member（不能设置 owner，转让群主用 TransferOwner）
func (s *groupServiceImpl) SetRole(ctx context.Context, groupID, operatorID, userID int64, role string) error {
	if role != "admin" && role != "member" {
		return errors.New("无效的角色")
	}

	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || member.Role != "owner" {
		return errors.New("只有群主才能设置角色")
	}

	if err := s.repo.UpdateMemberRole(ctx, groupID, userID, role); err != nil {
		return err
	}

	s.invalidateGroupMembersCache(ctx, groupID)
	return nil
}

// GetGroupMembers 获取群组成员列表
func (s *groupServiceImpl) GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("群组不存在")
	}

	if s.redis != nil {
		policy := cache.GroupMembersPolicy(groupID)
		var cached []model.GroupMember
		hit, err := s.redis.GetJSON(ctx, policy.Key, &cached)
		if err == nil && hit != "" {
			if s.redis.IsNullHit(hit) {
				return nil, nil
			}
			return cached, nil
		}
	}

	members, err := s.repo.GetGroupMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}

	if s.redis != nil {
		policy := cache.GroupMembersPolicy(groupID)
		if len(members) == 0 {
			s.redis.SetNull(ctx, policy.Key, policy.NullTTL, policy.NullJitter)
		} else {
			s.redis.SetJSONWithJitter(ctx, policy.Key, members, policy.TTL, policy.Jitter)
		}
	}

	return members, nil
}

// CheckMember 检查用户是否为群组成员，并返回其角色
// 返回值：(是否成员, 角色, 错误)
func (s *groupServiceImpl) CheckMember(ctx context.Context, groupID, userID int64) (bool, string, error) {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return false, "", err
	}
	if group == nil {
		return false, "", errors.New("群组不存在")
	}

	member, err := s.repo.GetMember(ctx, groupID, userID)
	if err != nil {
		return false, "", err
	}
	if member == nil {
		return false, "", nil
	}
	return true, member.Role, nil
}

// TransferOwner 转让群主
// 权限：只有群主才能转让
// 流程：原群主变为管理员 → 新群主变为 owner → 更新 groups 表的 owner_id
func (s *groupServiceImpl) TransferOwner(ctx context.Context, groupID, operatorID, newOwnerID int64) error {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("群组不存在")
	}
	if group.OwnerID != operatorID {
		return errors.New("只有群主才能转让群组")
	}

	newOwner, err := s.repo.GetMember(ctx, groupID, newOwnerID)
	if err != nil {
		return err
	}
	if newOwner == nil {
		return errors.New("新群主不在群组中")
	}

	if err := s.repo.UpdateMemberRole(ctx, groupID, operatorID, "admin"); err != nil {
		return err
	}
	if err := s.repo.UpdateMemberRole(ctx, groupID, newOwnerID, "owner"); err != nil {
		return err
	}
	if err := s.repo.UpdateOwner(ctx, groupID, newOwnerID); err != nil {
		return err
	}

	s.invalidateGroupCache(ctx, groupID)
	s.invalidateGroupMembersCache(ctx, groupID)
	s.invalidateUserGroupsCache(ctx, operatorID)
	s.invalidateUserGroupsCache(ctx, newOwnerID)

	return nil
}

// UnmuteMember 解除禁言
// 权限：群主和管理员可以解除禁言
func (s *groupServiceImpl) UnmuteMember(ctx context.Context, groupID, operatorID, userID int64) error {
	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}
	if err := s.repo.UpdateMuteStatus(ctx, groupID, userID, nil); err != nil {
		return err
	}

	s.invalidateGroupMembersCache(ctx, groupID)
	return nil
}

// PinGroup 置顶/取消置顶群组
// 权限：群主和管理员可以置顶
func (s *groupServiceImpl) PinGroup(ctx context.Context, groupID, operatorID int64, isPinned bool) error {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return errors.New("群组不存在")
	}

	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}

	if err := s.repo.PinGroup(ctx, groupID, isPinned); err != nil {
		return err
	}

	s.invalidateGroupCache(ctx, groupID)
	return nil
}

// invalidateGroupCache 删除群资料缓存，下一次读取会从数据库回源。
func (s *groupServiceImpl) invalidateGroupCache(ctx context.Context, groupID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, cache.GroupInfoPolicy(groupID).Key)
}

// invalidateGroupMembersCache 删除群成员缓存；邀请、踢人、禁言、角色变化后都必须调用。
func (s *groupServiceImpl) invalidateGroupMembersCache(ctx context.Context, groupID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, cache.GroupMembersPolicy(groupID).Key)
}

// invalidateUserGroupsCache 删除用户所在群列表缓存，确保侧边栏能看到最新入群/退群结果。
func (s *groupServiceImpl) invalidateUserGroupsCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, cache.UserGroupsPolicy(userID).Key)
}

// saveGroupEvent 将群领域事件写入事务 Outbox。
// 调用方应在群资料/成员变更的同一个数据库事务里调用，保证业务事实提交后事件最终一定会发布。
func (s *groupServiceImpl) saveGroupEvent(ctx context.Context, repo dao.GroupRepository, eventType string, groupID int64, payload interface{}) error {
	envelope, err := events.NewEnvelope(eventType, strconv.FormatInt(groupID, 10), payload)
	if err != nil {
		return err
	}
	record, err := outbox.NewEvent("group", groupID, envelope)
	if err != nil {
		return err
	}
	return repo.SaveOutboxEvent(ctx, record)
}

// newUniqueGroupID 生成面向用户展示的 10 位群号，并用数据库查询兜底随机碰撞。
// 碰撞概率很低，但这里仍重试 5 次，避免用户看到创建失败。
func (s *groupServiceImpl) newUniqueGroupID(ctx context.Context) (int64, error) {
	for i := 0; i < 5; i++ {
		id, err := idgen.NewUID10()
		if err != nil {
			return 0, err
		}
		existing, err := s.repo.GetGroupByID(ctx, id)
		if err != nil {
			return 0, err
		}
		if existing == nil {
			return id, nil
		}
	}
	return 0, errors.New("生成群号失败，请重试")
}

// memberIDsFromMembers 从成员快照中提取去重后的用户 ID，供 Outbox 事件和缓存失效使用。
func memberIDsFromMembers(members []model.GroupMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return dedupeInt64(ids)
}

// dedupeInt64 去掉空 ID 和重复 ID，保持输入顺序，避免重复写成员或重复推送事件。
func dedupeInt64(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// normalizeGroupStatus 收敛管理端传入的群治理状态。
// 空值和历史脏值不在这里静默接受，避免“封禁/解封成功但实际状态不可解释”。
func normalizeGroupStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "active", "online", "normal":
		return "active"
	case "banned", "disabled":
		return "banned"
	default:
		return ""
	}
}
