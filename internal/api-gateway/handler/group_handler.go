// Package handler contains api-gateway HTTP handlers. GroupHandler adapts the
// browser's group-management REST calls to group-service RPCs while preserving
// the authenticated operator identity from JWT middleware.
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/pkg/response"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

// GroupHandler handles group metadata, membership, role, mute and pin endpoints.
// Permission checks live in group-service; this layer only validates transport
// shape and enriches requests with the current operator ID.
type GroupHandler struct{}

// NewGroupHandler constructs the stateless group HTTP handler used by the router.
func NewGroupHandler() *GroupHandler {
	return &GroupHandler{}
}

func parseJSONNumberList(values []json.Number, name string) ([]int64, error) {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseInt(value.String(), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("无效的%s", name)
		}
		result = append(result, id)
	}
	return result, nil
}

func parseJSONNumber(value json.Number, name string) (int64, error) {
	id, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的%s", name)
	}
	return id, nil
}

func rawJSONField(c *app.RequestContext, field string) (json.RawMessage, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(c.Request.Body(), &raw); err != nil {
		return nil, false
	}
	value, ok := raw[field]
	return value, ok
}

func bindGroupJSONUseNumber(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// CreateGroup creates a group with the current user as owner.
//
// The gateway pre-checks invited user IDs so the UI can report invalid members
// before group-service writes group membership and group-created events.
func (h *GroupHandler) CreateGroup(ctx context.Context, c *app.RequestContext) {
	type createGroupReq struct {
		Name      string        `json:"name"`
		MemberIDs []json.Number `json:"member_ids"`
	}
	var req createGroupReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	memberIDs, err := parseJSONNumberList(req.MemberIDs, "成员用户ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	for _, uid := range memberIDs {
		userResp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(uid))
		if !userInfoLookupOK(userResp, err) {
			response.BadRequest(c, fmt.Sprintf("用户 %d 不存在", uid))
			return
		}
	}

	resp, err := client.GroupClient.CreateGroup(ctx, client.NewCreateGroupReq(req.Name, id, memberIDs))
	if err != nil {
		response.Error(c, err.Error())
		return
	}

	response.Success(c, resp)
}

// GetGroup returns group metadata such as name, owner, avatar and announcement.
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

// GetUserGroups returns all groups visible to the current user.
func (h *GroupHandler) GetUserGroups(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.GetUserGroups(ctx, client.NewGetUserGroupsReq(id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// InviteMember adds users to a group. group-service performs the actual role
// checks and emits membership events for msg-core-service synchronization.
func (h *GroupHandler) InviteMember(ctx context.Context, c *app.RequestContext) {
	type inviteReq struct {
		GroupID json.Number   `json:"group_id"`
		UserIDs []json.Number `json:"user_ids"`
	}
	var req inviteReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	userIDs, err := parseJSONNumberList(req.UserIDs, "被邀请用户ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	for _, uid := range userIDs {
		userResp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(uid))
		if !userInfoLookupOK(userResp, err) {
			response.BadRequest(c, fmt.Sprintf("用户 %d 不存在", uid))
			return
		}
	}

	resp, err := client.GroupClient.InviteMember(ctx, client.NewInviteMemberReq(groupID, id, userIDs))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// KickMember removes one user from a group after group-service validates that
// the current operator has permission.
func (h *GroupHandler) KickMember(ctx context.Context, c *app.RequestContext) {
	type kickReq struct {
		GroupID json.Number `json:"group_id"`
		UserID  json.Number `json:"user_id"`
	}
	var req kickReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	targetUserID, err := parseJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.KickMember(ctx, client.NewKickMemberReq(groupID, id, targetUserID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetGroupMembers returns group membership enriched with user profile fields.
//
// group-service owns membership, but the browser also needs display names and
// avatars. The gateway joins those profile fields through user-service to avoid
// coupling group-service to the user table.
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

// TransferOwner transfers group ownership to another member.
func (h *GroupHandler) TransferOwner(ctx context.Context, c *app.RequestContext) {
	type transferReq struct {
		GroupID    json.Number `json:"group_id"`
		NewOwnerID json.Number `json:"new_owner_id"`
	}
	var req transferReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	newOwnerID, err := parseJSONNumber(req.NewOwnerID, "新群主用户ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.TransferOwner(ctx, client.NewTransferOwnerReq(groupID, id, newOwnerID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UpdateGroupInfo updates group name and announcement.
//
// announcement has two distinct meanings: omitted means "leave unchanged" and
// an explicit empty string means "clear announcement". The gateway preserves
// that distinction before converting the request into the current Thrift shape.
func (h *GroupHandler) UpdateGroupInfo(ctx context.Context, c *app.RequestContext) {
	type updateReq struct {
		GroupID      json.Number `json:"group_id"`
		Name         string      `json:"name"`
		Announcement string      `json:"announcement"`
	}
	var req updateReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	if _, exists := rawJSONField(c, "announcement"); !exists {
		groupResp, err := client.GroupClient.GetGroup(ctx, client.NewGetGroupReq(groupID))
		if err != nil {
			response.Error(c, err.Error())
			return
		}
		if groupResp == nil || !groupResp.Success || groupResp.Group == nil {
			response.Error(c, "群组不存在")
			return
		}
		req.Announcement = groupResp.Group.Announcement
	}
	resp, err := client.GroupClient.UpdateGroup(ctx, client.NewUpdateGroupReq(groupID, id, req.Name, req.Announcement))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// PinGroup updates the current user's pin state for a group in the sidebar.
func (h *GroupHandler) PinGroup(ctx context.Context, c *app.RequestContext) {
	type pinReq struct {
		GroupID  json.Number `json:"group_id"`
		IsPinned bool        `json:"is_pinned"`
	}
	var req pinReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.PinGroup(ctx, client.NewPinGroupReq(groupID, id, req.IsPinned))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// MuteMember applies a timed mute to one group member.
func (h *GroupHandler) MuteMember(ctx context.Context, c *app.RequestContext) {
	type muteReq struct {
		GroupID         json.Number `json:"group_id"`
		UserID          json.Number `json:"user_id"`
		DurationMinutes int64       `json:"duration_minutes"`
	}
	var req muteReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	targetUserID, err := parseJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.MuteMember(ctx, client.NewMuteMemberReq(groupID, id, targetUserID, req.DurationMinutes))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// UnmuteMember clears a member's group mute state.
func (h *GroupHandler) UnmuteMember(ctx context.Context, c *app.RequestContext) {
	type unmuteReq struct {
		GroupID json.Number `json:"group_id"`
		UserID  json.Number `json:"user_id"`
	}
	var req unmuteReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	targetUserID, err := parseJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.UnmuteMember(ctx, client.NewUnmuteMemberReq(groupID, id, targetUserID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// SetRole changes a group member's role, such as member/admin.
func (h *GroupHandler) SetRole(ctx context.Context, c *app.RequestContext) {
	type setRoleReq struct {
		GroupID json.Number `json:"group_id"`
		UserID  json.Number `json:"user_id"`
		Role    string      `json:"role"`
	}
	var req setRoleReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	targetUserID, err := parseJSONNumber(req.UserID, "用户ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.SetRole(ctx, client.NewSetRoleReq(groupID, id, targetUserID, req.Role))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteGroup dissolves a group. msg-core-service keeps historical group
// conversations visible as deleted-group placeholders until users hide them.
func (h *GroupHandler) DeleteGroup(ctx context.Context, c *app.RequestContext) {
	type deleteReq struct {
		GroupID json.Number `json:"group_id"`
	}
	var req deleteReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群组ID")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.GroupClient.DeleteGroup(ctx, client.NewDeleteGroupReq(groupID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
