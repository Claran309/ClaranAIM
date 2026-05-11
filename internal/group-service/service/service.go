package service

import (
	"ClaranAIM/internal/group-service/dao"
	"ClaranAIM/internal/group-service/model"
	"context"
	"errors"
	"time"
)

type GroupService interface {
	CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*model.Group, error)
	DeleteGroup(ctx context.Context, groupID, operatorID int64) error
	UpdateGroup(ctx context.Context, groupID, operatorID int64, name, announcement string) error
	GetGroup(ctx context.Context, groupID int64) (*model.Group, error)
	GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error)
	InviteMember(ctx context.Context, groupID, operatorID int64, userIDs []int64) error
	KickMember(ctx context.Context, groupID, operatorID, userID int64) error
	MuteMember(ctx context.Context, groupID, operatorID, userID int64, durationMinutes int64) error
	SetRole(ctx context.Context, groupID, operatorID, userID int64, role string) error
	GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error)
	CheckMember(ctx context.Context, groupID, userID int64) (bool, string, error)
	TransferOwner(ctx context.Context, groupID, operatorID, newOwnerID int64) error
}

type groupServiceImpl struct {
	repo dao.GroupRepository
}

func NewGroupService(repo dao.GroupRepository) GroupService {
	return &groupServiceImpl{repo: repo}
}

func (s *groupServiceImpl) CreateGroup(ctx context.Context, name string, ownerID int64, memberIDs []int64) (*model.Group, error) {
	if name == "" {
		return nil, errors.New("群名不能为空")
	}

	group := &model.Group{
		Name:    name,
		OwnerID: ownerID,
	}

	if err := s.repo.CreateGroup(ctx, group); err != nil {
		return nil, err
	}

	// 群主自动成为成员
	ownerMember := &model.GroupMember{
		GroupID: group.ID,
		UserID:  ownerID,
		Role:    "owner",
	}
	if err := s.repo.AddMember(ctx, ownerMember); err != nil {
		return nil, err
	}

	// 添加其他成员
	for _, uid := range memberIDs {
		if uid == ownerID {
			continue
		}
		member := &model.GroupMember{
			GroupID: group.ID,
			UserID:  uid,
			Role:    "member",
		}
		_ = s.repo.AddMember(ctx, member)
	}

	return group, nil
}

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
	return s.repo.DeleteGroup(ctx, groupID)
}

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
	if announcement != "" {
		group.Announcement = announcement
	}

	return s.repo.UpdateGroup(ctx, group)
}

func (s *groupServiceImpl) GetGroup(ctx context.Context, groupID int64) (*model.Group, error) {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("群组不存在")
	}
	return group, nil
}

func (s *groupServiceImpl) GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error) {
	return s.repo.GetUserGroups(ctx, userID)
}

func (s *groupServiceImpl) InviteMember(ctx context.Context, groupID, operatorID int64, userIDs []int64) error {
	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}

	for _, uid := range userIDs {
		existing, _ := s.repo.GetMember(ctx, groupID, uid)
		if existing != nil {
			continue
		}
		newMember := &model.GroupMember{
			GroupID: groupID,
			UserID:  uid,
			Role:    "member",
		}
		if err := s.repo.AddMember(ctx, newMember); err != nil {
			return err
		}
	}
	return nil
}

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

	return s.repo.RemoveMember(ctx, groupID, userID)
}

func (s *groupServiceImpl) MuteMember(ctx context.Context, groupID, operatorID, userID int64, durationMinutes int64) error {
	member, err := s.repo.GetMember(ctx, groupID, operatorID)
	if err != nil {
		return err
	}
	if member == nil || (member.Role != "owner" && member.Role != "admin") {
		return errors.New("权限不足")
	}

	mutedUntil := time.Now().Add(time.Duration(durationMinutes) * time.Minute)
	return s.repo.UpdateMuteStatus(ctx, groupID, userID, &mutedUntil)
}

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

	return s.repo.UpdateMemberRole(ctx, groupID, userID, role)
}

func (s *groupServiceImpl) GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	return s.repo.GetGroupMembers(ctx, groupID)
}

func (s *groupServiceImpl) CheckMember(ctx context.Context, groupID, userID int64) (bool, string, error) {
	member, err := s.repo.GetMember(ctx, groupID, userID)
	if err != nil {
		return false, "", err
	}
	if member == nil {
		return false, "", nil
	}
	return true, member.Role, nil
}

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

	// 原群主变为管理员
	if err := s.repo.UpdateMemberRole(ctx, groupID, operatorID, "admin"); err != nil {
		return err
	}
	// 新群主
	if err := s.repo.UpdateMemberRole(ctx, groupID, newOwnerID, "owner"); err != nil {
		return err
	}
	// 更新群组
	return s.repo.UpdateOwner(ctx, groupID, newOwnerID)
}
