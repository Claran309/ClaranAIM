// Package handler 包含 api-gateway 的 HTTP 处理器。
// 本文件把浏览器的群管理 REST 请求适配为 group-service RPC，并始终沿用 JWT 中的操作者身份。
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/pkg/config"
	clardtm "ClaranAIM/pkg/dtm"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/response"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// GroupHandler 处理群资料、成员、角色、禁言和置顶接口。
// 权限判断属于 group-service；网关只校验传输层形状并补充当前操作者 ID。
type GroupHandler struct{}

// 下面这组变量保存当前包需要复用的运行时状态或配置入口，调用方应通过公开函数间接使用。
var dtmCfg *config.DTMConfig

// InitDTMConfig 保存创建群 Saga 需要使用的 DTM 分支地址。
func InitDTMConfig(cfg config.DTMConfig) {
	dtmCfg = &cfg
}

// NewGroupHandler 创建无状态群组 HTTP handler，供 router 注册路由。
func NewGroupHandler() *GroupHandler {
	return &GroupHandler{}
}

// parseJSONNumberList 将 JSON 数字数组解析为正整数 ID 列表，避免大整数精度丢失。
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

// parseJSONNumber 将 JSON 数字字段解析为正 int64。
func parseJSONNumber(value json.Number, name string) (int64, error) {
	id, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的%s", name)
	}
	return id, nil
}

// rawJSONField 判断请求体中某个字段是否显式出现，用于区分“不修改”和“清空字段”。
func rawJSONField(c *app.RequestContext, field string) (json.RawMessage, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(c.Request.Body(), &raw); err != nil {
		return nil, false
	}
	value, ok := raw[field]
	return value, ok
}

// bindGroupJSONUseNumber 使用 json.Number 解码群组请求，保护 10 位群号和雪花 ID 的精度。
func bindGroupJSONUseNumber(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// CreateGroup 创建群聊，并把当前用户设为群主。
// 网关会先检查被邀请用户是否存在，让前端在 group-service 写成员关系和事件前得到清晰错误。
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

	if dtmCfg != nil && dtmCfg.Enabled {
		h.createGroupWithDTM(ctx, c, req.Name, id, memberIDs)
		return
	}

	resp, err := client.GroupClient.CreateGroup(ctx, client.NewCreateGroupReq(req.Name, id, memberIDs))
	if err != nil {
		response.Error(c, err.Error())
		return
	}

	response.Success(c, resp)
}

// createGroupWithDTM 使用 DTM Saga 同时创建群资料和群会话。
// 这是低频管理动作，适合用 Saga 兜底跨服务一致性；高频消息链路不走 DTM。
func (h *GroupHandler) createGroupWithDTM(ctx context.Context, c *app.RequestContext, name string, ownerID int64, memberIDs []int64) {
	groupID, err := idgen.NewUID10()
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	conversationID, err := idgen.NextID()
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	participants := dedupeGroupParticipants(ownerID, memberIDs)
	manager := clardtm.NewManager(dtmCfg.Server)
	saga, err := manager.NewSagaLocal()
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	groupPayload := map[string]interface{}{
		"group_id":   groupID,
		"name":       name,
		"owner_id":   ownerID,
		"member_ids": memberIDs,
	}
	conversationPayload := map[string]interface{}{
		"conversation_id": conversationID,
		"group_id":        groupID,
		"participant_ids": participants,
	}
	groupURL := strings.TrimRight(dtmCfg.GroupServiceURL, "/")
	msgURL := strings.TrimRight(dtmCfg.MsgCoreServiceURL, "/")
	saga.AddStep(groupURL+"/dtm/group/create", groupURL+"/dtm/group/create_compensate", groupPayload)
	saga.AddStep(msgURL+"/dtm/message/group-conversation/create", msgURL+"/dtm/message/group-conversation/create_compensate", conversationPayload)
	if err := saga.Submit(ctx); err != nil {
		response.Error(c, "DTM创建群组事务提交失败: "+err.Error())
		return
	}
	response.Success(c, map[string]interface{}{
		"success":         true,
		"group_id":        groupID,
		"conversation_id": conversationID,
		"gid":             saga.GID(),
		"msg":             "创建群组事务已提交",
	})
}

// dedupeGroupParticipants 合并群主和邀请成员并去重，确保群会话参与者列表稳定。
func dedupeGroupParticipants(ownerID int64, memberIDs []int64) []int64 {
	seen := map[int64]struct{}{}
	result := make([]int64, 0, len(memberIDs)+1)
	for _, id := range append([]int64{ownerID}, memberIDs...) {
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

// GetGroup 查询群名称、群主、头像、公告等群资料。
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

// GetUserGroups 查询当前用户可见的所有群聊。
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

// InviteMember 邀请用户入群。
// group-service 负责实际角色校验，并发布成员变更事件供 msg-core-service 同步会话成员。
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

// JoinGroupByID 允许当前用户输入公开 10 位群号加入群聊。
// 该接口只提交当前用户自己；替别人邀请仍走 /group/invite 并由 group-service 做角色校验。
func (h *GroupHandler) JoinGroupByID(ctx context.Context, c *app.RequestContext) {
	type joinReq struct {
		GroupID json.Number `json:"group_id"`
	}
	var req joinReq
	if err := bindGroupJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	groupID, err := parseJSONNumber(req.GroupID, "群号")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if groupID < 1000000000 || groupID > 9999999999 {
		response.BadRequest(c, "群号必须是10位数字")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	groupResp, err := client.GroupClient.GetGroup(ctx, client.NewGetGroupReq(groupID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if groupResp == nil || !groupResp.Success || groupResp.Group == nil {
		response.BadRequest(c, "群聊不存在")
		return
	}
	checkResp, err := client.GroupClient.CheckMember(ctx, client.NewCheckMemberReq(groupID, id))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if checkResp != nil && checkResp.Success && checkResp.IsMember {
		response.Success(c, map[string]interface{}{
			"success":  true,
			"group_id": groupID,
			"msg":      "你已在该群聊中",
			"joined":   false,
		})
		return
	}
	resp, err := client.GroupClient.InviteMember(ctx, client.NewInviteMemberReq(groupID, id, []int64{id}))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	if resp == nil || !resp.Success {
		response.Success(c, resp)
		return
	}
	response.Success(c, map[string]interface{}{
		"success":    true,
		"group_id":   groupID,
		"group_name": groupResp.Group.Name,
		"msg":        "加入群聊成功",
		"joined":     true,
	})
}

// KickMember 移除一个群成员，具体权限由 group-service 校验。
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

// GetGroupMembers 查询群成员并补充用户展示资料。
// group-service 拥有成员关系，浏览器还需要昵称和头像；网关通过 user-service 拼接展示字段，
// 避免 group-service 直接耦合用户表。
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

// TransferOwner 将群主身份转让给另一名群成员。
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

// UpdateGroupInfo 更新群名称和公告。
// announcement 未出现表示保持不变；显式传空字符串表示清空公告，网关在转成 Thrift 请求前保留这个区别。
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

// PinGroup 更新当前用户侧边栏里的群置顶状态。
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

// MuteMember 对某个群成员设置定时禁言。
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

// UnmuteMember 清除某个成员的群禁言状态。
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

// SetRole 修改群成员角色，例如普通成员或管理员。
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

// DeleteGroup 解散群聊。
// msg-core-service 会保留历史群会话，并以“已解散群”的占位形式展示，直到用户主动隐藏。
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
