// Package handler 实现 group-service 的 RPC 入口层
// 接收 Kitex 框架分发的 Thrift RPC 请求，调用 Service 层处理业务逻辑
// 本层只负责参数转换和结果封装，不包含业务逻辑
package handler

import (
	"ClaranAIM/internal/group-service/model"
	"ClaranAIM/internal/group-service/service"
	"ClaranAIM/kitex_gen/group"
	"context"
)

// GroupServiceImpl 群组服务的RPC Handler
// 实现由 Thrift IDL 生成的 group.GroupService 接口
// 作为 RPC 服务端，接收 api-gateway 的远程调用
type GroupServiceImpl struct {
	svc service.GroupService
}

// NewGroupServiceImpl 创建 RPC handler，注入 Service 层实例
func NewGroupServiceImpl(svc service.GroupService) group.GroupService {
	return &GroupServiceImpl{svc: svc}
}

// CreateGroup 创建群组 RPC 方法
// 流程：校验参数 → Service 创建群组（群主自动成为owner成员 + 添加其他成员）→ 返回群组ID
func (h *GroupServiceImpl) CreateGroup(ctx context.Context, req *group.CreateGroupReq) (resp *group.CreateGroupResp, err error) {
	g, err := h.svc.CreateGroup(ctx, req.Name, req.OwnerId, req.MemberIds)
	if err != nil {
		return &group.CreateGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.CreateGroupResp{Success: true, GroupId: g.ID, Msg: "创建群组成功"}, nil
}

// DeleteGroup 解散群组 RPC 方法
// 权限：只有群主才能解散群组
func (h *GroupServiceImpl) DeleteGroup(ctx context.Context, req *group.DeleteGroupReq) (resp *group.DeleteGroupResp, err error) {
	err = h.svc.DeleteGroup(ctx, req.GroupId, req.OperatorId)
	if err != nil {
		return &group.DeleteGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.DeleteGroupResp{Success: true, Msg: "解散群组成功"}, nil
}

// UpdateGroup 更新群组信息 RPC 方法
// 可修改群名称和群公告，权限：群主和管理员
func (h *GroupServiceImpl) UpdateGroup(ctx context.Context, req *group.UpdateGroupReq) (resp *group.UpdateGroupResp, err error) {
	err = h.svc.UpdateGroup(ctx, req.GroupId, req.OperatorId, req.Name, req.Announcement)
	if err != nil {
		return &group.UpdateGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.UpdateGroupResp{Success: true, Msg: "更新群组成功"}, nil
}

// GetGroup 获取群组信息 RPC 方法
// 返回群组名称、群主ID、群公告等基本信息
func (h *GroupServiceImpl) GetGroup(ctx context.Context, req *group.GetGroupReq) (resp *group.GetGroupResp, err error) {
	g, err := h.svc.GetGroup(ctx, req.GroupId)
	if err != nil {
		return &group.GetGroupResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.GetGroupResp{
		Success: true,
		Group:   toRPCGroup(*g),
	}, nil
}

// GetUserGroups 获取用户所在的所有群组 RPC 方法
// 通过 JOIN group_members 表查询用户参与的所有群组
func (h *GroupServiceImpl) GetUserGroups(ctx context.Context, req *group.GetUserGroupsReq) (resp *group.GetUserGroupsResp, err error) {
	groups, err := h.svc.GetUserGroups(ctx, req.UserId)
	if err != nil {
		return &group.GetUserGroupsResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*group.Group
	for _, g := range groups {
		list = append(list, toRPCGroup(g))
	}
	return &group.GetUserGroupsResp{Success: true, Groups: list}, nil
}

// InviteMember 邀请成员加入群组 RPC 方法
// 权限：群主和管理员可以邀请，已在群中的用户不重复添加
func (h *GroupServiceImpl) InviteMember(ctx context.Context, req *group.InviteMemberReq) (resp *group.InviteMemberResp, err error) {
	err = h.svc.InviteMember(ctx, req.GroupId, req.OperatorId, req.UserIds)
	if err != nil {
		return &group.InviteMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.InviteMemberResp{Success: true, Msg: "邀请成员成功"}, nil
}

// KickMember 踢出群组成员 RPC 方法
// 权限：群主和管理员可以踢人，但不能踢出群主
func (h *GroupServiceImpl) KickMember(ctx context.Context, req *group.KickMemberReq) (resp *group.KickMemberResp, err error) {
	err = h.svc.KickMember(ctx, req.GroupId, req.OperatorId, req.UserId)
	if err != nil {
		return &group.KickMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.KickMemberResp{Success: true, Msg: "踢出成员成功"}, nil
}

// MuteMember 禁言群组成员 RPC 方法
// 权限：群主和管理员可以禁言，durationMinutes 为禁言时长（分钟）
func (h *GroupServiceImpl) MuteMember(ctx context.Context, req *group.MuteMemberReq) (resp *group.MuteMemberResp, err error) {
	err = h.svc.MuteMember(ctx, req.GroupId, req.OperatorId, req.UserId, req.DurationMinutes)
	if err != nil {
		return &group.MuteMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.MuteMemberResp{Success: true, Msg: "禁言成功"}, nil
}

// SetRole 设置群组成员角色 RPC 方法
// 权限：只有群主才能设置角色，角色只能是 admin 或 member
func (h *GroupServiceImpl) SetRole(ctx context.Context, req *group.SetRoleReq) (resp *group.SetRoleResp, err error) {
	err = h.svc.SetRole(ctx, req.GroupId, req.OperatorId, req.UserId, req.Role)
	if err != nil {
		return &group.SetRoleResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.SetRoleResp{Success: true, Msg: "设置角色成功"}, nil
}

// GetGroupMembers 获取群组成员列表 RPC 方法
// 返回所有成员及其角色（owner/admin/member）和禁言状态
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

// CheckMember 检查用户是否为群组成员 RPC 方法
// 返回是否为成员和角色信息，用于权限校验
func (h *GroupServiceImpl) CheckMember(ctx context.Context, req *group.CheckMemberReq) (resp *group.CheckMemberResp, err error) {
	isMember, role, err := h.svc.CheckMember(ctx, req.GroupId, req.UserId)
	if err != nil {
		return &group.CheckMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.CheckMemberResp{Success: true, IsMember: isMember, Role: role}, nil
}

// TransferOwner 转让群主 RPC 方法
// 权限：只有群主才能转让，新群主必须是群成员
// 流程：原群主变为admin → 新群主变为owner → 更新groups表的owner_id
func (h *GroupServiceImpl) TransferOwner(ctx context.Context, req *group.TransferOwnerReq) (resp *group.TransferOwnerResp, err error) {
	err = h.svc.TransferOwner(ctx, req.GroupId, req.OperatorId, req.NewOwnerId_)
	if err != nil {
		return &group.TransferOwnerResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.TransferOwnerResp{Success: true, Msg: "转让群主成功"}, nil
}

// UnmuteMember 解除禁言 RPC 方法
// 权限：群主和管理员可以解除禁言
func (h *GroupServiceImpl) UnmuteMember(ctx context.Context, req *group.UnmuteMemberReq) (resp *group.UnmuteMemberResp, err error) {
	err = h.svc.UnmuteMember(ctx, req.GroupId, req.OperatorId, req.UserId)
	if err != nil {
		return &group.UnmuteMemberResp{Success: false, Msg: err.Error()}, nil
	}
	return &group.UnmuteMemberResp{Success: true, Msg: "解除禁言成功"}, nil
}

// PinGroup 置顶/取消置顶群组 RPC 方法
// 权限：群主和管理员可以置顶
func (h *GroupServiceImpl) PinGroup(ctx context.Context, req *group.PinGroupReq) (resp *group.PinGroupResp, err error) {
	err = h.svc.PinGroup(ctx, req.GroupId, req.OperatorId, req.IsPinned)
	if err != nil {
		return &group.PinGroupResp{Success: false, Msg: err.Error()}, nil
	}
	msg := "取消置顶成功"
	if req.IsPinned {
		msg = "置顶成功"
	}
	return &group.PinGroupResp{Success: true, Msg: msg}, nil
}

// AdminListGroups 返回管理端全局群列表。
// 该 RPC 不做角色判断，必须由 api-gateway 的 admin 分组保护。
func (h *GroupServiceImpl) AdminListGroups(ctx context.Context, req *group.AdminListGroupsReq) (resp *group.AdminListGroupsResp, err error) {
	groups, total, err := h.svc.AdminListGroups(ctx, req.GetKeyword(), req.GetOwnerId(), req.GetLimit(), req.GetOffset())
	if err != nil {
		return &group.AdminListGroupsResp{Success: false, Msg: err.Error()}, nil
	}
	list := make([]*group.Group, 0, len(groups))
	for _, g := range groups {
		list = append(list, toRPCGroup(g))
	}
	return &group.AdminListGroupsResp{Success: true, Groups: list, Total: total, Msg: "获取成功"}, nil
}

func toRPCGroup(g model.Group) *group.Group {
	return &group.Group{
		Id:           g.ID,
		Name:         g.Name,
		Avatar:       g.Avatar,
		OwnerId:      g.OwnerID,
		Announcement: g.Announcement,
		IsPinned:     g.IsPinned,
		CreatedAt:    g.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    g.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
