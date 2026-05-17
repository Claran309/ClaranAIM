// Package service 实现 group-service 的业务逻辑层
// 包含群组 CRUD、成员管理、权限校验等核心业务规则
package service

import (
	"ClaranAIM/internal/group-service/dao"
	"ClaranAIM/internal/group-service/model"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// GroupService defines the group-domain business contract.
//
// group-service owns group metadata, membership, roles, mute state and the
// group.* outbox events consumed by msg-core-service to maintain message fanout.
type GroupService interface {
	CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*model.Group, error)
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
}

type groupServiceImpl struct {
	repo  dao.GroupRepository
	redis *redis.RedisClient
}

// NewGroupService creates the group service with a repository and optional Redis
// cache client.
func NewGroupService(repo dao.GroupRepository, r *redis.RedisClient) GroupService {
	return &groupServiceImpl{repo: repo, redis: r}
}

// CreateGroup 创建群组
// 流程：创建群组记录 → 群主自动成为成员(role=owner) → 添加其他成员(role=member)
func (s *groupServiceImpl) CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*model.Group, error) {
	if name == "" {
		return nil, errors.New("群名不能为空")
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
		Name:    name,
		OwnerID: ownerID,
	}

	if err := s.repo.WithTransaction(ctx, func(tx dao.GroupRepository) error {
		// The group row, owner/member rows and group.created event are committed
		// together. If Kafka is unavailable, the outbox row remains for retry.
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
		// Deleting the group and writing group.deleted into the outbox in one
		// transaction lets msg-core-service reliably mark conversation sidebars
		// as deleted-group placeholders after Kafka delivery.
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
	// Empty announcement is a valid value: it means the operator intentionally
	// cleared the announcement. The API gateway preserves omitted-field semantics
	// before this service method is called.
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
		// CacheAsideJSON writes null markers for misses and uses TTL jitter, so
		// non-existent group IDs do not repeatedly penetrate the database.
		cacheKey := fmt.Sprintf("group:info:%d", groupID)
		var cached model.Group
		found, err := s.redis.CacheAsideJSON(ctx, cacheKey, &cached, 15*time.Minute, func(ctx context.Context) (interface{}, bool, error) {
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
		cacheKey := fmt.Sprintf("user:groups:%d", userID)
		var cached []model.Group
		hit, err := s.redis.GetJSON(ctx, cacheKey, &cached)
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
		cacheKey := fmt.Sprintf("user:groups:%d", userID)
		if len(groups) == 0 {
			s.redis.SetNull(ctx, cacheKey, time.Minute, 30*time.Second)
		} else {
			s.redis.SetJSONWithJitter(ctx, cacheKey, groups, 5*time.Minute, 30*time.Second)
		}
	}

	return groups, nil
}

// InviteMember 邀请成员加入群组
// 权限：群主和管理员可以邀请
// 已在群中的用户不重复添加
func (s *groupServiceImpl) InviteMember(ctx context.Context, groupID, operatorID int64, userIDs []int64) error {
	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}

	var members []model.GroupMember
	if err := s.repo.WithTransaction(ctx, func(tx dao.GroupRepository) error {
		// The event payload contains the full member snapshot, not only invited
		// users. msg-core-service can rebuild conversation participants from a
		// complete view and remain idempotent across retries.
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
			UserIDs:    dedupeInt64(userIDs),
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
		// Publish the remaining member snapshot together with the kicked user so
		// downstream services can remove exactly the old participant and refresh
		// sidebar caches for affected users.
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
		cacheKey := fmt.Sprintf("group:members:%d", groupID)
		var cached []model.GroupMember
		hit, err := s.redis.GetJSON(ctx, cacheKey, &cached)
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
		cacheKey := fmt.Sprintf("group:members:%d", groupID)
		if len(members) == 0 {
			s.redis.SetNull(ctx, cacheKey, time.Minute, 30*time.Second)
		} else {
			s.redis.SetJSONWithJitter(ctx, cacheKey, members, 10*time.Minute, time.Minute)
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

func (s *groupServiceImpl) invalidateGroupCache(ctx context.Context, groupID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("group:info:%d", groupID))
}

func (s *groupServiceImpl) invalidateGroupMembersCache(ctx context.Context, groupID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("group:members:%d", groupID))
}

func (s *groupServiceImpl) invalidateUserGroupsCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("user:groups:%d", userID))
}

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

func memberIDsFromMembers(members []model.GroupMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return dedupeInt64(ids)
}

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
