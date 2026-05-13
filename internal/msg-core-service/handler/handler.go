package handler

import (
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/internal/msg-core-service/service"
	"ClaranAIM/kitex_gen/message"
	"context"
)

// MessageServiceImpl 消息核心服务的RPC Handler
// 实现由 Thrift IDL 生成的 message.MessageService 接口
// 负责接收RPC请求，调用Service层处理业务逻辑，将结果转换为Thrift响应格式
type MessageServiceImpl struct {
	svc service.MessageService
}

func NewMessageServiceImpl(svc service.MessageService) message.MessageService {
	return &MessageServiceImpl{svc: svc}
}

// CreateConversation 创建会话 RPC 方法
// 接收会话类型和参与者ID列表，调用Service创建会话
// 返回创建的会话ID，私聊时如果会话已存在则返回已有会话ID
func (h *MessageServiceImpl) CreateConversation(ctx context.Context, req *message.CreateConversationReq) (resp *message.CreateConversationResp, err error) {
	conv, err := h.svc.CreateConversation(ctx, req.Type, req.ParticipantIds, req.GroupId)
	if err != nil {
		return &message.CreateConversationResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.CreateConversationResp{Success: true, ConversationId: conv.ID, Msg: "创建会话成功"}, nil
}

// GetConversation 获取会话详情 RPC 方法
// 根据会话ID查询会话的类型、创建时间、更新时间等信息
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
			GroupId:   conv.GroupID,
			CreatedAt: conv.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: conv.UpdatedAt.Format("2006-01-02 15:04:05"),
		},
	}, nil
}

// GetUserConversations 获取用户会话列表 RPC 方法
// 返回用户参与的所有会话，包含最后一条消息摘要
// 用于前端左侧会话列表展示
func (h *MessageServiceImpl) GetUserConversations(ctx context.Context, req *message.GetUserConversationsReq) (resp *message.GetUserConversationsResp, err error) {
	convs, err := h.svc.GetUserConversations(ctx, req.UserId)
	if err != nil {
		return &message.GetUserConversationsResp{Success: false, Msg: err.Error()}, nil
	}

	var list []*message.UserConversationInfo
	for _, c := range convs {
		item := &message.UserConversationInfo{
			ConversationId:  c.ConversationID,
			Type:            c.Type,
			LastMessage:     c.LastMessage,
			LastMessageTime: c.LastMessageTime,
			UnreadCount:     c.UnreadCount,
			TargetName:      c.TargetName,
			TargetAvatar:    c.TargetAvatar,
			LastSenderId:    c.LastSenderID,
			GroupId:         c.GroupID,
		}
		if len(c.ParticipantIDs) > 0 {
			item.ParticipantIds = c.ParticipantIDs
		}
		list = append(list, item)
	}
	return &message.GetUserConversationsResp{Success: true, Conversations: list}, nil
}

// SendMessage 发送消息 RPC 方法
// 核心方法：存储消息 → 更新会话时间 → 缓存 → WebSocket推送
// api-gateway 调用此方法发送消息，消息会自动推送给所有会话参与者
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

// GetHistory 获取会话历史消息 RPC 方法
// 支持游标分页：通过 beforeID 加载更早的消息
// 返回结果按时间正序排列（从旧到新）
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

// SearchMessages 搜索消息 RPC 方法
// 如果请求中指定了 ConversationIds，则只在指定会话中搜索
// 否则在用户参与的所有会话中搜索
func (h *MessageServiceImpl) SearchMessages(ctx context.Context, req *message.SearchMessagesReq) (resp *message.SearchMessagesResp, err error) {
	var msgs []model.Message
	if len(req.ConversationIds) > 0 {
		msgs, err = h.svc.SearchMessagesInConversations(ctx, req.ConversationIds, req.Keyword, req.Limit)
	} else {
		msgs, err = h.svc.SearchMessages(ctx, req.UserId, req.Keyword, req.Limit)
	}
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

// GetConversationParticipants 获取会话参与者 RPC 方法
// 返回指定会话中所有参与者的用户ID列表
// 用于前端展示会话成员或后端确定消息推送目标
func (h *MessageServiceImpl) GetConversationParticipants(ctx context.Context, req *message.GetConversationParticipantsReq) (resp *message.GetConversationParticipantsResp, err error) {
	userIDs, err := h.svc.GetConversationParticipants(ctx, req.ConversationId)
	if err != nil {
		return &message.GetConversationParticipantsResp{Success: false, Msg: err.Error()}, nil
	}
	return &message.GetConversationParticipantsResp{Success: true, UserIds: userIDs}, nil
}
