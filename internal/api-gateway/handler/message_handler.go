// Package handler 实现 API 网关的消息相关 HTTP 请求处理
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/pkg/response"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

// MessageHandler 处理所有消息相关的 HTTP 请求
type MessageHandler struct{}

// NewMessageHandler 创建消息 HTTP handler。
//
// 当前 handler 本身无状态，所有跨服务能力都通过 client 包中的 Kitex
// RPC 客户端完成；保留构造函数可以让路由注册处保持统一的依赖注入风格。
func NewMessageHandler() *MessageHandler {
	return &MessageHandler{}
}

func bindJSONUseNumber(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(c.Request.Body()))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

func numberToInt64(value json.Number) (int64, error) {
	return strconv.ParseInt(value.String(), 10, 64)
}

// CreateConversation 创建会话
// POST /api/v1/message/conversation
// 请求体: {type, participant_ids}
// type 为 "private"（私聊）或 "group"（群聊）
// 私聊会话自动去重：如果两人已有私聊会话，直接返回已有会话ID
func (h *MessageHandler) CreateConversation(ctx context.Context, c *app.RequestContext) {
	type createConvReq struct {
		Type           string        `json:"type"`
		ParticipantIDs []json.Number `json:"participant_ids"`
		GroupID        json.Number   `json:"group_id"`
	}
	var req createConvReq
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	participantIDs := make([]int64, 0, len(req.ParticipantIDs))
	for _, rawUID := range req.ParticipantIDs {
		uid, err := numberToInt64(rawUID)
		if err != nil || uid <= 0 {
			response.BadRequest(c, "无效的参与者用户ID")
			return
		}
		participantIDs = append(participantIDs, uid)
		userResp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(uid))
		if !userInfoLookupOK(userResp, err) {
			response.BadRequest(c, fmt.Sprintf("参与者用户 %d 不存在", uid))
			return
		}
	}
	var groupID int64
	if req.GroupID.String() != "" {
		parsedGroupID, err := numberToInt64(req.GroupID)
		if err != nil {
			response.BadRequest(c, "无效的群组ID")
			return
		}
		groupID = parsedGroupID
	}

	resp, err := client.MessageClient.CreateConversation(ctx, client.NewCreateConversationReq(req.Type, participantIDs, groupID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// SendMessage 发送消息
// POST /api/v1/message/send
// 请求体: {conversation_id, content, msg_type}
// 完整流程：消息落库 → 更新会话时间戳 → 缓存更新 → WebSocket 实时推送
func (h *MessageHandler) SendMessage(ctx context.Context, c *app.RequestContext) {
	type sendMsgReq struct {
		ConversationID json.Number   `json:"conversation_id"`  // 会话ID
		Content        string        `json:"content"`          // 消息内容
		MsgType        string        `json:"msg_type"`         // 消息类型：text(文本)，默认 text
		ReplyToID      json.Number   `json:"reply_to_id"`      // 引用消息ID
		MentionUserIDs []json.Number `json:"mention_user_ids"` // @用户列表
		MentionAll     bool          `json:"mention_all"`      // 是否@所有人
	}
	var req sendMsgReq
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	conversationID, err := numberToInt64(req.ConversationID)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "无效的会话ID")
		return
	}
	var replyToID int64
	if req.ReplyToID.String() != "" {
		replyToID, err = numberToInt64(req.ReplyToID)
		if err != nil {
			response.BadRequest(c, "无效的引用消息ID")
			return
		}
	}
	mentionUserIDs := make([]int64, 0, len(req.MentionUserIDs))
	for _, mentionID := range req.MentionUserIDs {
		id, err := numberToInt64(mentionID)
		if err != nil {
			response.BadRequest(c, "无效的@用户ID")
			return
		}
		mentionUserIDs = append(mentionUserIDs, id)
	}

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	participantsResp, err := client.MessageClient.GetConversationParticipants(ctx, &message.GetConversationParticipantsReq{ConversationId: conversationID})
	if err == nil && participantsResp != nil && participantsResp.UserIds != nil {
		for _, pid := range participantsResp.UserIds {
			if pid == id {
				continue
			}
			userResp, userErr := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(pid))
			if !userInfoLookupOK(userResp, userErr) {
				response.Error(c, fmt.Sprintf("对方用户 %d 已不存在，无法发送消息", pid))
				return
			}
		}
	}

	resp, err := client.MessageClient.SendMessage(ctx, client.NewSendMessageExtReq(conversationID, id, req.Content, req.MsgType, replyToID, mentionUserIDs, req.MentionAll))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// MarkConversationRead advances the current user's read cursor for a conversation.
//
// message_id may be omitted or zero, in which case msg-core-service marks the
// latest visible message as read. The service layer then publishes a read event
// so other participants can update their read receipts.
func (h *MessageHandler) MarkConversationRead(ctx context.Context, c *app.RequestContext) {
	type markReadReq struct {
		ConversationID json.Number `json:"conversation_id"`
		MessageID      json.Number `json:"message_id"`
	}
	var req markReadReq
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	conversationID, err := numberToInt64(req.ConversationID)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "无效的会话ID")
		return
	}
	var messageID int64
	if req.MessageID.String() != "" {
		messageID, err = numberToInt64(req.MessageID)
		if err != nil {
			response.BadRequest(c, "无效的消息ID")
			return
		}
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.MessageClient.MarkConversationRead(ctx, &message.MarkConversationReadReq{
		ConversationId: conversationID,
		UserId:         id,
		MessageId:      messageID,
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteLocalMessage 删除当前登录用户自己的本地消息视图。
//
// 这个 HTTP 接口对应常规 IM 的“删除聊天记录/删除本地消息”，不是“撤回”。
// api-gateway 只负责从 JWT 中取当前用户、绑定请求参数，并把用户身份传给
// msg-core-service；服务端消息事实是否仍然存在、其他用户是否可见，都由
// msg-core-service 的 message_user_states 规则决定。
func (h *MessageHandler) DeleteLocalMessage(ctx context.Context, c *app.RequestContext) {
	type deleteLocalReq struct {
		ConversationID json.Number `json:"conversation_id"`
		MessageID      json.Number `json:"message_id"`
	}
	var req deleteLocalReq
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	conversationID, err := numberToInt64(req.ConversationID)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "无效的会话ID")
		return
	}
	messageID, err := numberToInt64(req.MessageID)
	if err != nil || messageID <= 0 {
		response.BadRequest(c, "无效的消息ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.MessageClient.DeleteLocalMessage(ctx, client.NewDeleteLocalMessageReq(conversationID, id, messageID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// EditMessage updates the content of one message authored by the current user.
// Ownership, edit window and message-state rules are enforced by msg-core-service.
func (h *MessageHandler) EditMessage(ctx context.Context, c *app.RequestContext) {
	type editReq struct {
		MessageID json.Number `json:"message_id"`
		Content   string      `json:"content"`
	}
	var req editReq
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	messageID, err := numberToInt64(req.MessageID)
	if err != nil || messageID <= 0 {
		response.BadRequest(c, "无效的消息ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.MessageClient.EditMessage(ctx, &message.EditMessageReq{
		MessageId: messageID,
		EditorId:  id,
		Content:   req.Content,
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// RecallMessage recalls one message for all participants when msg-core-service
// accepts the current user as an allowed operator.
func (h *MessageHandler) RecallMessage(ctx context.Context, c *app.RequestContext) {
	type recallReq struct {
		MessageID json.Number `json:"message_id"`
	}
	var req recallReq
	if err := bindJSONUseNumber(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	messageID, err := numberToInt64(req.MessageID)
	if err != nil || messageID <= 0 {
		response.BadRequest(c, "无效的消息ID")
		return
	}
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	resp, err := client.MessageClient.RecallMessage(ctx, &message.RecallMessageReq{
		MessageId:  messageID,
		OperatorId: id,
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetHistory 获取消息历史
// GET /api/v1/message/history/:id?limit=50&before_id=0
// 路径参数 id 为会话ID
// 查询参数 limit 为每页条数（默认50），before_id 为游标（加载此ID之前的消息）
// 使用游标分页而非偏移分页，避免深分页性能问题
func (h *MessageHandler) GetHistory(ctx context.Context, c *app.RequestContext) {
	conversationIDStr := c.Param("id")
	conversationID, err := strconv.ParseInt(conversationIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的会话ID")
		return
	}

	limitStr := c.DefaultQuery("limit", "50") // 每页条数，默认50
	limit, _ := strconv.ParseInt(limitStr, 10, 64)
	beforeIDStr := c.DefaultQuery("before_id", "0") // 游标，0表示从头开始
	beforeID, _ := strconv.ParseInt(beforeIDStr, 10, 64)

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.MessageClient.GetHistory(ctx, client.NewGetHistoryReq(conversationID, id, limit, beforeID))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// SearchMessages 搜索消息
// GET /api/v1/message/search?keyword=xxx&limit=20&conversation_id=0
// conversation_id 为可选参数：如果提供则只在指定会话中搜索，否则搜索用户所有会话
func (h *MessageHandler) SearchMessages(ctx context.Context, c *app.RequestContext) {
	keyword := c.Query("keyword")
	if keyword == "" {
		response.BadRequest(c, "搜索关键词不能为空")
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.ParseInt(limitStr, 10, 64)

	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	conversationIDStr := c.DefaultQuery("conversation_id", "0")
	conversationID, _ := strconv.ParseInt(conversationIDStr, 10, 64)
	startAt := c.Query("start_at")
	endAt := c.Query("end_at")

	var resp interface{}
	var err error

	if conversationID > 0 {
		resp, err = client.MessageClient.SearchMessages(ctx, client.NewSearchMessagesAdvancedReq(id, []int64{conversationID}, keyword, limit, startAt, endAt))
	} else {
		resp, err = client.MessageClient.SearchMessages(ctx, client.NewSearchMessagesAdvancedReq(id, nil, keyword, limit, startAt, endAt))
	}

	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

// GetUserConversations 获取当前用户的会话列表
// GET /api/v1/message/conversations
// 返回用户参与的所有会话，附带最后一条消息内容
func (h *MessageHandler) GetUserConversations(ctx context.Context, c *app.RequestContext) {
	id, ok := requireCurrentUserID(c)
	if !ok {
		return
	}

	resp, err := client.MessageClient.GetUserConversations(ctx, &message.GetUserConversationsReq{UserId: id})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
