package handler

import (
	"ClaranAIM/internal/msg-history-service/service"
	"ClaranAIM/kitex_gen/message"
	"context"
)

type HistoryServiceImpl struct {
	svc service.HistoryService
}

func NewHistoryServiceImpl(svc service.HistoryService) message.HistoryService {
	return &HistoryServiceImpl{svc: svc}
}

func (h *HistoryServiceImpl) SaveMessage(ctx context.Context, req *message.SaveMessageReq) (resp *message.SaveMessageResp, err error) {
	msg, err := h.svc.SaveMessage(ctx, req.ConversationId, req.SenderId, req.Content, req.MsgType)
	if err != nil {
		return &message.SaveMessageResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.SaveMessageResp{Success: true, MessageId: msg.ID, Msg: "保存成功"}, nil
}

func (h *HistoryServiceImpl) GetHistory(ctx context.Context, req *message.GetHistoryReq) (resp *message.GetHistoryResp, err error) {
	msgs, err := h.svc.GetHistory(ctx, req.ConversationId, req.Limit, req.BeforeId)
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

func (h *HistoryServiceImpl) SearchHistory(ctx context.Context, req *message.SearchMessagesReq) (resp *message.SearchMessagesResp, err error) {
	msgs, err := h.svc.SearchHistory(ctx, req.ConversationIds, req.Keyword, req.Limit)
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

func (h *HistoryServiceImpl) GetOfflineMessages(ctx context.Context, req *message.GetOfflineMessagesReq) (resp *message.GetOfflineMessagesResp, err error) {
	offlineMsgs, err := h.svc.GetOfflineMessages(ctx, req.UserId)
	if err != nil {
		return &message.GetOfflineMessagesResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*message.OfflineMessage
	for _, m := range offlineMsgs {
		readAt := ""
		if m.ReadAt != nil {
			readAt = m.ReadAt.Format("2006-01-02 15:04:05")
		}
		list = append(list, &message.OfflineMessage{
			Id:        m.ID,
			UserId:    m.UserID,
			MessageId: m.MessageID,
			IsRead:    m.IsRead,
			CreatedAt: m.CreatedAt.Format("2006-01-02 15:04:05"),
			ReadAt:    readAt,
		})
	}
	return &message.GetOfflineMessagesResp{Success: true, Messages: list}, nil
}

func (h *HistoryServiceImpl) MarkOfflineRead(ctx context.Context, req *message.MarkOfflineReadReq) (resp *message.MarkOfflineReadResp, err error) {
	err = h.svc.MarkOfflineRead(ctx, req.UserId, req.MessageIds)
	if err != nil {
		return &message.MarkOfflineReadResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.MarkOfflineReadResp{Success: true, Msg: "标记已读成功"}, nil
}

func (h *HistoryServiceImpl) GetUnreadCount(ctx context.Context, req *message.GetUnreadCountReq) (resp *message.GetUnreadCountResp, err error) {
	count, err := h.svc.GetUnreadCount(ctx, req.UserId)
	if err != nil {
		return &message.GetUnreadCountResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.GetUnreadCountResp{Success: true, Count: count}, nil
}
