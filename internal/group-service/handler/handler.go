package handler

import (
	"ClaranAIM/internal/group-service/service"
	"ClaranAIM/kitex_gen/group"
	"context"
)

type GroupServiceImpl struct {
	svc service.GroupService
}

func NewGroupServiceImpl(svc service.GroupService) group.GroupService {
	return &GroupServiceImpl{svc: svc}
}

func (h *GroupServiceImpl) CreateGroup(ctx context.Context, req *group.CreateGroupReq) (resp *group.CreateGroupResp, err error) {
	g, err := h.svc.CreateGroup(ctx, req.Name, req.OwnerId, req.MemberIds)
	if err != nil {
		return &group.CreateGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.CreateGroupResp{Success: true, GroupId: g.ID, Msg: "创建群组成功"}, nil
}

func (h *GroupServiceImpl) DeleteGroup(ctx context.Context, req *group.DeleteGroupReq) (resp *group.DeleteGroupResp, err error) {
	err = h.svc.DeleteGroup(ctx, req.GroupId, req.OperatorId)
	if err != nil {
		return &group.DeleteGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.DeleteGroupResp{Success: true, Msg: "解散群组成功"}, nil
}

func (h *GroupServiceImpl) UpdateGroup(ctx context.Context, req *group.UpdateGroupReq) (resp *group.UpdateGroupResp, err error) {
	err = h.svc.UpdateGroup(ctx, req.GroupId, req.OperatorId, req.Name, req.Announcement)
	if err != nil {
		return &group.UpdateGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.UpdateGroupResp{Success: true, Msg: "更新群组成功"}, nil
}

func (h *GroupServiceImpl) GetGroup(ctx context.Context, req *group.GetGroupReq) (resp *group.GetGroupResp, err error) {
	g, err := h.svc.GetGroup(ctx, req.GroupId)
	if err != nil {
		return &group.GetGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.GetGroupResp{
		Success: true,
		Group: &group.Group{
			Id:           g.ID,
			Name:         g.Name,
			Avatar:       g.Avatar,
			OwnerId:      g.OwnerID,
			Announcement: g.Announcement,
			CreatedAt:    g.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    g.UpdatedAt.Format("2006-01-02 15:04:05"),
		},
	}, nil
}

func (h *GroupServiceImpl) GetUserGroups(ctx context.Context, req *group.GetUserGroupsReq) (resp *group.GetUserGroupsResp, err error) {
	groups, err := h.svc.GetUserGroups(ctx, req.UserId)
	if err != nil {
		return &group.GetUserGroupsResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*group.Group
	for _, g := range groups {
		list = append(list, &group.Group{
			Id:           g.ID,
			Name:         g.Name,
			Avatar:       g.Avatar,
			OwnerId:      g.OwnerID,
			Announcement: g.Announcement,
			CreatedAt:    g.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    g.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &group.GetUserGroupsResp{Success: true, Groups: list}, nil
}

func (h *GroupServiceImpl) InviteMember(ctx context.Context, req *group.InviteMemberReq) (resp *group.InviteMemberResp, err error) {
	err = h.svc.InviteMember(ctx, req.GroupId, req.OperatorId, req.UserIds)
	if err != nil {
		return &group.InviteMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.InviteMemberResp{Success: true, Msg: "邀请成员成功"}, nil
}

func (h *GroupServiceImpl) KickMember(ctx context.Context, req *group.KickMemberReq) (resp *group.KickMemberResp, err error) {
	err = h.svc.KickMember(ctx, req.GroupId, req.OperatorId, req.UserId)
	if err != nil {
		return &group.KickMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.KickMemberResp{Success: true, Msg: "踢出成员成功"}, nil
}

func (h *GroupServiceImpl) MuteMember(ctx context.Context, req *group.MuteMemberReq) (resp *group.MuteMemberResp, err error) {
	err = h.svc.MuteMember(ctx, req.GroupId, req.OperatorId, req.UserId, req.DurationMinutes)
	if err != nil {
		return &group.MuteMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.MuteMemberResp{Success: true, Msg: "禁言成功"}, nil
}

func (h *GroupServiceImpl) SetRole(ctx context.Context, req *group.SetRoleReq) (resp *group.SetRoleResp, err error) {
	err = h.svc.SetRole(ctx, req.GroupId, req.OperatorId, req.UserId, req.Role)
	if err != nil {
		return &group.SetRoleResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.SetRoleResp{Success: true, Msg: "设置角色成功"}, nil
}

func (h *GroupServiceImpl) GetGroupMembers(ctx context.Context, req *group.GetGroupMembersReq) (resp *group.GetGroupMembersResp, err error) {
	members, err := h.svc.GetGroupMembers(ctx, req.GroupId)
	if err != nil {
		return &group.GetGroupMembersResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*group.GroupMember
	for _, m := range members {
		mutedUntil := ""
		if m.MutedUntil != nil {
			mutedUntil = m.MutedUntil.Format("2006-01-02 15:04:05")
		}
		list = append(list, &group.GroupMember{
			Id:         m.ID,
			GroupId:    m.GroupID,
			UserId:     m.UserID,
			Role:       m.Role,
			MutedUntil: mutedUntil,
			JoinedAt:   m.JoinedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &group.GetGroupMembersResp{Success: true, Members: list}, nil
}

func (h *GroupServiceImpl) CheckMember(ctx context.Context, req *group.CheckMemberReq) (resp *group.CheckMemberResp, err error) {
	isMember, role, err := h.svc.CheckMember(ctx, req.GroupId, req.UserId)
	if err != nil {
		return &group.CheckMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.CheckMemberResp{Success: true, IsMember: isMember, Role: role}, nil
}

func (h *GroupServiceImpl) TransferOwner(ctx context.Context, req *group.TransferOwnerReq) (resp *group.TransferOwnerResp, err error) {
	err = h.svc.TransferOwner(ctx, req.GroupId, req.OperatorId, req.NewOwnerId_)
	if err != nil {
		return &group.TransferOwnerResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.TransferOwnerResp{Success: true, Msg: "转让群主成功"}, nil
}
