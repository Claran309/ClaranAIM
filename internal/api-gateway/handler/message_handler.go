// Package handler 实现 API 网关的消息相关 HTTP 请求处理
package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/pkg/response"
	"context"
	"fmt"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

// MessageHandler 处理所有消息相关的 HTTP 请求
type MessageHandler struct{}

func NewMessageHandler() *MessageHandler {
	return &MessageHandler{}
}

// CreateConversation 创建会话
// POST /api/v1/message/conversation
// 请求体: {type, participant_ids}
// type 为 "private"（私聊）或 "group"（群聊）
// 私聊会话自动去重：如果两人已有私聊会话，直接返回已有会话ID
func (h *MessageHandler) CreateConversation(ctx context.Context, c *app.RequestContext) {
	type createConvReq struct {
		Type           string  `json:"type"`
		ParticipantIDs []int64 `json:"participant_ids"`
		GroupID        int64   `json:"group_id"`
	}
	var req createConvReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	for _, uid := range req.ParticipantIDs {
		userResp, err := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(uid))
		if err != nil || !userResp.Success {
			response.BadRequest(c, fmt.Sprintf("参与者用户 %d 不存在", uid))
			return
		}
	}

	resp, err := client.MessageClient.CreateConversation(ctx, client.NewCreateConversationReq(req.Type, req.ParticipantIDs, req.GroupID))
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
		ConversationID int64   `json:"conversation_id"`  // 会话ID
		Content        string  `json:"content"`          // 消息内容
		MsgType        string  `json:"msg_type"`         // 消息类型：text(文本)，默认 text
		ReplyToID      int64   `json:"reply_to_id"`      // 引用消息ID
		MentionUserIDs []int64 `json:"mention_user_ids"` // @用户列表
		MentionAll     bool    `json:"mention_all"`      // 是否@所有人
	}
	var req sendMsgReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	userID, _ := c.Get("userID")
	id := userID.(int64)

	participantsResp, err := client.MessageClient.GetConversationParticipants(ctx, &message.GetConversationParticipantsReq{ConversationId: req.ConversationID})
	if err == nil && participantsResp != nil && participantsResp.UserIds != nil {
		for _, pid := range participantsResp.UserIds {
			if pid == id {
				continue
			}
			userResp, userErr := client.UserClient.GetUserInfo(ctx, client.NewGetUserInfoReq(pid))
			if userErr != nil || !userResp.Success {
				response.Error(c, fmt.Sprintf("对方用户 %d 已不存在，无法发送消息", pid))
				return
			}
		}
	}

	resp, err := client.MessageClient.SendMessage(ctx, client.NewSendMessageExtReq(req.ConversationID, id, req.Content, req.MsgType, req.ReplyToID, req.MentionUserIDs, req.MentionAll))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *MessageHandler) MarkConversationRead(ctx context.Context, c *app.RequestContext) {
	type markReadReq struct {
		ConversationID int64 `json:"conversation_id"`
		MessageID      int64 `json:"message_id"`
	}
	var req markReadReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.MessageClient.MarkConversationRead(ctx, &message.MarkConversationReadReq{
		ConversationId: req.ConversationID,
		UserId:         id,
		MessageId:      req.MessageID,
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *MessageHandler) EditMessage(ctx context.Context, c *app.RequestContext) {
	type editReq struct {
		MessageID int64  `json:"message_id"`
		Content   string `json:"content"`
	}
	var req editReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.MessageClient.EditMessage(ctx, &message.EditMessageReq{
		MessageId: req.MessageID,
		EditorId:  id,
		Content:   req.Content,
	})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *MessageHandler) RecallMessage(ctx context.Context, c *app.RequestContext) {
	type recallReq struct {
		MessageID int64 `json:"message_id"`
	}
	var req recallReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	userID, _ := c.Get("userID")
	id := userID.(int64)
	resp, err := client.MessageClient.RecallMessage(ctx, &message.RecallMessageReq{
		MessageId:  req.MessageID,
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

	userID, _ := c.Get("userID")
	id := userID.(int64)

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

	userID, _ := c.Get("userID")
	id := userID.(int64)

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
	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.MessageClient.GetUserConversations(ctx, &message.GetUserConversationsReq{UserId: id})
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}
