package handler

import (
	"ClaranAIM/internal/api-gateway/client"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/pkg/response"
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
)

type MessageHandler struct{}

func NewMessageHandler() *MessageHandler {
	return &MessageHandler{}
}

func (h *MessageHandler) CreateConversation(ctx context.Context, c *app.RequestContext) {
	type createConvReq struct {
		Type           string  `json:"type"`
		ParticipantIDs []int64 `json:"participant_ids"`
	}
	var req createConvReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	resp, err := client.MessageClient.CreateConversation(ctx, client.NewCreateConversationReq(req.Type, req.ParticipantIDs))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *MessageHandler) SendMessage(ctx context.Context, c *app.RequestContext) {
	type sendMsgReq struct {
		ConversationID int64  `json:"conversation_id"`
		Content        string `json:"content"`
		MsgType        string `json:"msg_type"`
	}
	var req sendMsgReq
	if err := c.BindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}

	userID, _ := c.Get("userID")
	id := userID.(int64)

	resp, err := client.MessageClient.SendMessage(ctx, client.NewSendMessageReq(req.ConversationID, id, req.Content, req.MsgType))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *MessageHandler) GetHistory(ctx context.Context, c *app.RequestContext) {
	conversationIDStr := c.Param("id")
	conversationID, err := strconv.ParseInt(conversationIDStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的会话ID")
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.ParseInt(limitStr, 10, 64)
	beforeIDStr := c.DefaultQuery("before_id", "0")
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

	resp, err := client.MessageClient.SearchMessages(ctx, client.NewSearchMessagesReq(id, keyword, limit))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, resp)
}

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
