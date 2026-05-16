package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/pkg/response"
	"context"
	"fmt"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

type GroupHandler struct{}

func NewGroupHandler() *GroupHandler {
	return &GroupHandler{}
}

func (h *GroupHandler) CreateGroup(ctx context.Context, c *app.RequestContext) {
	type createGroupReq struct {
		Name      string  `json:"name"`
		MemberIDs []int64 `json:"member_ids"`
	}
	var req createGroupReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)

	for _, uid := range req.MemberIDs {
		userResp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(uid))
		if err != nil || !userResp.Success {
			response.BadRequest(c, fmt.Sprintf("用户 %d 不存在", uid))
			return
		}
	}

	resp, err := client.GroupClient.CreateGroup(ctx, client.NewCreateGroupReq(req.Name, id, req.MemberIDs))
	if err != nil {
		response.Error(c, err.Error())
		return
	}

	if resp.Success && resp.GroupId > 0 {
		allMembers := append([]int64{id}, req.MemberIDs...)
		_, convErr := client.MessageClient.CreateConversation(ctx, client.NewCreateConversationReq("group", allMembers, resp.GroupId))
		if convErr != nil {
			fmt.Printf("创建群聊会话失败: %v\n", convErr)
		}
	}

	response.Success(c, resp)
}

func (h *GroupHandler) GetGroup(ctx context.Context, c *app.RequestContext) {
	groupIDStr := c.Param("id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的群组ID")
		return
	}
	resp, err := client.GroupClient.GetGroup(ctx, client.NewGetGroupReq(groupID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) GetUserGroups(ctx context.Context, c *app.RequestContext) {
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.GetUserGroups(ctx, client.NewGetUserGroupsReq(id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) InviteMember(ctx context.Context, c *app.RequestContext) {
	type inviteReq struct {
		GroupID int64   `json:"group_id"`
		UserIDs []int64 `json:"user_ids"`
	}
	var req inviteReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)

	for _, uid := range req.UserIDs {
		userResp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(uid))
		if err != nil || !userResp.Success {
			response.BadRequest(c, fmt.Sprintf("用户 %d 不存在", uid))
			return
		}
	}

	resp, err := client.GroupClient.InviteMember(ctx, client.NewInviteMemberReq(req.GroupID, id, req.UserIDs))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if resp.Success {
		if membersResp, membersErr := client.GroupClient.GetGroupMembers(ctx, client.NewGetGroupMembersReq(req.GroupID)); membersErr == nil && membersResp.Success {
			memberIDs := make([]int64, 0, len(membersResp.Members))
			for _, m := range membersResp.Members {
				memberIDs = append(memberIDs, m.UserId)
			}
			if len(memberIDs) >= 2 {
				_, _ = client.MessageClient.CreateConversation(ctx, client.NewCreateConversationReq("group", memberIDs, req.GroupID))
			}
		}
	}
	response.Success(c, resp)
}

func (h *GroupHandler) KickMember(ctx context.Context, c *app.RequestContext) {
	type kickReq struct {
		GroupID int64 `json:"group_id"`
		UserID  int64 `json:"user_id"`
	}
	var req kickReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.KickMember(ctx, client.NewKickMemberReq(req.GroupID, id, req.UserID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) GetGroupMembers(ctx context.Context, c *app.RequestContext) {
	groupIDStr := c.Param("id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的群组ID")
		return
	}
	resp, err := client.GroupClient.GetGroupMembers(ctx, client.NewGetGroupMembersReq(groupID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if resp == nil || !resp.Success {
		response.Success(c, resp)
		return
	}

	members := make([]map[string]interface{}, 0, len(resp.Members))
	for _, m := range resp.Members {
		item := map[string]interface{}{
			"id":          m.Id,
			"group_id":    m.GroupId,
			"user_id":     m.UserId,
			"role":        m.Role,
			"muted_until": m.MutedUntil,
			"joined_at":   m.JoinedAt,
		}
		if userResp, userErr := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(m.UserId)); userErr == nil && userResp != nil && userResp.Success && userResp.User != nil {
			item["username"] = userResp.User.Username
			item["nickname"] = userResp.User.Nickname
			item["avatar"] = userResp.User.Avatar
		}
		members = append(members, item)
	}

	response.Success(c, map[string]interface{}{
		"success": true,
		"members": members,
		"msg":     resp.Msg,
	})
}

func (h *GroupHandler) TransferOwner(ctx context.Context, c *app.RequestContext) {
	type transferReq struct {
		GroupID    int64 `json:"group_id"`
		NewOwnerID int64 `json:"new_owner_id"`
	}
	var req transferReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.TransferOwner(ctx, client.NewTransferOwnerReq(req.GroupID, id, req.NewOwnerID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) UpdateGroupInfo(ctx context.Context, c *app.RequestContext) {
	type updateReq struct {
		GroupID      int64  `json:"group_id"`
		Name         string `json:"name"`
		Announcement string `json:"announcement"`
	}
	var req updateReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.UpdateGroup(ctx, client.NewUpdateGroupReq(req.GroupID, id, req.Name, req.Announcement))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) PinGroup(ctx context.Context, c *app.RequestContext) {
	type pinReq struct {
		GroupID  int64 `json:"group_id"`
		IsPinned bool  `json:"is_pinned"`
	}
	var req pinReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.PinGroup(ctx, client.NewPinGroupReq(req.GroupID, id, req.IsPinned))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) MuteMember(ctx context.Context, c *app.RequestContext) {
	type muteReq struct {
		GroupID         int64 `json:"group_id"`
		UserID          int64 `json:"user_id"`
		DurationMinutes int64 `json:"duration_minutes"`
	}
	var req muteReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.MuteMember(ctx, client.NewMuteMemberReq(req.GroupID, id, req.UserID, req.DurationMinutes))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) UnmuteMember(ctx context.Context, c *app.RequestContext) {
	type unmuteReq struct {
		GroupID int64 `json:"group_id"`
		UserID  int64 `json:"user_id"`
	}
	var req unmuteReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.UnmuteMember(ctx, client.NewUnmuteMemberReq(req.GroupID, id, req.UserID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) SetRole(ctx context.Context, c *app.RequestContext) {
	type setRoleReq struct {
		GroupID int64  `json:"group_id"`
		UserID  int64  `json:"user_id"`
		Role    string `json:"role"`
	}
	var req setRoleReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.SetRole(ctx, client.NewSetRoleReq(req.GroupID, id, req.UserID, req.Role))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *GroupHandler) DeleteGroup(ctx context.Context, c *app.RequestContext) {
	type deleteReq struct {
		GroupID int64 `json:"group_id"`
	}
	var req deleteReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.GroupClient.DeleteGroup(ctx, client.NewDeleteGroupReq(req.GroupID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
