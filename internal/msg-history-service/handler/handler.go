package handler

import (
	"ClaranAIM/internal/msg-history-service/service"
	"ClaranAIM/kitex_gen/message"
	"context"
)

// HistoryServiceImpl 消息历史服务的RPC Handler
// 实现由 Thrift IDL 生成的 message.HistoryService 接口
// 负责接收RPC请求，调用Service层处理业务逻辑，将结果转换为Thrift响应格式
type HistoryServiceImpl struct {
	svc service.HistoryService
}

// NewHistoryServiceImpl 创建消息历史 RPC handler。
//
// handler 负责把 Kitex 生成的请求结构转换为 service 调用，并把内部模型转换为
// IDL 响应结构，保持 RPC 层不直接访问数据库。
func NewHistoryServiceImpl(svc service.HistoryService) message.HistoryService {
	return &HistoryServiceImpl{svc: svc}
}

// SaveMessage 保存消息到历史记录 RPC 方法
// 将消息归档存储到历史记录表，用于长期保存和后续查询
// 通常由 msg-core-service 在发送消息后调用
func (h *HistoryServiceImpl) SaveMessage(ctx context.Context, req *message.SaveMessageReq) (resp *message.SaveMessageResp, err error) {
	msg, err := h.svc.SaveMessage(ctx, req.ConversationId, req.SenderId, req.Content, req.MsgType)
	if err != nil {
		return &message.SaveMessageResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.SaveMessageResp{Success: true, MessageId: msg.ID, Msg: "保存成功"}, nil
}

// GetHistory 获取会话历史消息 RPC 方法
// 支持游标分页：通过 beforeID 加载更早的消息
// 返回结果按时间正序排列（从旧到新）
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

// SearchHistory 搜索历史消息 RPC 方法
// 在指定的会话列表中搜索包含关键词的消息
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

// GetOfflineMessages 获取用户离线消息 RPC 方法
// 用户上线后调用，获取离线期间收到的所有未读消息
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

// MarkOfflineRead 标记离线消息为已读 RPC 方法
// 用户查看消息后调用，批量标记指定消息为已读
func (h *HistoryServiceImpl) MarkOfflineRead(ctx context.Context, req *message.MarkOfflineReadReq) (resp *message.MarkOfflineReadResp, err error) {
	err = h.svc.MarkOfflineRead(ctx, req.UserId, req.MessageIds)
	if err != nil {
		return &message.MarkOfflineReadResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.MarkOfflineReadResp{Success: true, Msg: "标记已读成功"}, nil
}

// GetUnreadCount 获取未读消息数 RPC 方法
// 用于前端显示未读消息角标
func (h *HistoryServiceImpl) GetUnreadCount(ctx context.Context, req *message.GetUnreadCountReq) (resp *message.GetUnreadCountResp, err error) {
	count, err := h.svc.GetUnreadCount(ctx, req.UserId)
	if err != nil {
		return &message.GetUnreadCountResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.GetUnreadCountResp{Success: true, Count: count}, nil
}
