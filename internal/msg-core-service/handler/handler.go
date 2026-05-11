package handler

import (
	"ClaranAIM/internal/msg-core-service/service"
	"ClaranAIM/kitex_gen/message"
	"context"
)

type MessageServiceImpl struct {
	svc service.MessageService
}

func NewMessageServiceImpl(svc service.MessageService) message.MessageService {
	return &MessageServiceImpl{svc: svc}
}

func (h *MessageServiceImpl) CreateConversation(ctx context.Context, req *message.CreateConversationReq) (resp *message.CreateConversationResp, err error) {
	conv, err := h.svc.CreateConversation(ctx, req.Type, req.ParticipantIds)
	if err != nil {
		return &message.CreateConversationResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.CreateConversationResp{Success: true, ConversationId: conv.ID, Msg: "创建会话成功"}, nil
}

func (h *MessageServiceImpl) GetConversation(ctx context.Context, req *message.GetConversationReq) (resp *message.GetConversationResp, err error) {
	conv, err := h.svc.GetConversation(ctx, req.ConversationId)
	if err != nil {
		return &message.GetConversationResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.GetConversationResp{
		Success: true,
		Conversation: &message.Conversation{
			Id:        conv.ID,
			Type:      conv.Type,
			CreatedAt: conv.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: conv.UpdatedAt.Format("2006-01-02 15:04:05"),
		},
	}, nil
}

func (h *MessageServiceImpl) GetUserConversations(ctx context.Context, req *message.GetUserConversationsReq) (resp *message.GetUserConversationsResp, err error) {
	convs, err := h.svc.GetUserConversations(ctx, req.UserId)
	if err != nil {
		return &message.GetUserConversationsResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*message.UserConversationInfo
	for _, c := range convs {
		list = append(list, &message.UserConversationInfo{
			ConversationId:  c.ConversationID,
			Type:            c.Type,
			LastMessage:     c.LastMessage,
			LastMessageTime: c.LastMessageTime,
		})
	}
	return &message.GetUserConversationsResp{Success: true, Conversations: list}, nil
}

func (h *MessageServiceImpl) SendMessage(ctx context.Context, req *message.SendMessageReq) (resp *message.SendMessageResp, err error) {
	msg, err := h.svc.SendMessage(ctx, req.ConversationId, req.SenderId, req.Content, req.MsgType)
	if err != nil {
		return &message.SendMessageResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.SendMessageResp{
		Success:  true,
		MsgId:    msg.ID,
		Msg:      "发送成功",
		SendTime: msg.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

func (h *MessageServiceImpl) GetHistory(ctx context.Context, req *message.GetHistoryReq) (resp *message.GetHistoryResp, err error) {
	msgs, err := h.svc.GetHistory(ctx, req.ConversationId, req.UserId, req.Limit, req.BeforeId)
	if err != nil {
		return &message.GetHistoryResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*message.Message
	for _, m := range msgs {
		list = append(list, &message.Message{
			Id:             m.ID,
			ConversationId: m.ConversationID,
			SenderId:       m.SenderID,
			Content:        m.Content,
			MsgType:        m.MsgType,
			CreatedAt:      m.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &message.GetHistoryResp{Success: true, Messages: list}, nil
}

func (h *MessageServiceImpl) SearchMessages(ctx context.Context, req *message.SearchMessagesReq) (resp *message.SearchMessagesResp, err error) {
	msgs, err := h.svc.SearchMessages(ctx, req.UserId, req.Keyword, req.Limit)
	if err != nil {
		return &message.SearchMessagesResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*message.Message
	for _, m := range msgs {
		list = append(list, &message.Message{
			Id:             m.ID,
			ConversationId: m.ConversationID,
			SenderId:       m.SenderID,
			Content:        m.Content,
			MsgType:        m.MsgType,
			CreatedAt:      m.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return &message.SearchMessagesResp{Success: true, Messages: list}, nil
}

func (h *MessageServiceImpl) GetConversationParticipants(ctx context.Context, req *message.GetConversationParticipantsReq) (resp *message.GetConversationParticipantsResp, err error) {
	userIDs, err := h.svc.GetConversationParticipants(ctx, req.ConversationId)
	if err != nil {
		return &message.GetConversationParticipantsResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.GetConversationParticipantsResp{Success: true, UserIds: userIDs}, nil
}
