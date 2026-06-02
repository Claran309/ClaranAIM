package service

import (
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/pkg/memoryclient"
	"ClaranAIM/pkg/ragclient"
	"context"
	"errors"
	"time"
)

// MessageRPCWindowFetcher 通过 msg-core-service 拉取当前 viewer 可见的消息窗口。
type MessageRPCWindowFetcher struct {
	client messageservice.Client
}

func NewMessageRPCWindowFetcher(client messageservice.Client) *MessageRPCWindowFetcher {
	return &MessageRPCWindowFetcher{client: client}
}

func (f *MessageRPCWindowFetcher) FetchWindow(ctx context.Context, input FetchWindowInput) (MessageWindow, error) {
	if f == nil || f.client == nil {
		return MessageWindow{}, errors.New("msg-core-service客户端未配置")
	}
	participantsResp, err := f.client.GetConversationParticipants(ctx, &message.GetConversationParticipantsReq{ConversationId: input.ConversationID})
	if err != nil {
		return MessageWindow{}, err
	}
	if !participantsResp.GetSuccess() {
		return MessageWindow{}, errors.New(participantsResp.GetMsg())
	}
	historyResp, err := f.client.GetHistory(ctx, &message.GetHistoryReq{ConversationId: input.ConversationID, UserId: input.ViewerID, Limit: int64(input.Limit), BeforeId: input.BeforeID})
	if err != nil {
		return MessageWindow{}, err
	}
	if !historyResp.GetSuccess() {
		return MessageWindow{}, errors.New(historyResp.GetMsg())
	}
	messages := make([]ConversationMessage, 0, len(historyResp.GetMessages()))
	for _, msg := range historyResp.GetMessages() {
		if msg == nil {
			continue
		}
		createdAt, _ := time.ParseInLocation("2006-01-02 15:04:05", msg.GetCreatedAt(), time.Local)
		messages = append(messages, ConversationMessage{
			ID:             msg.GetId(),
			ConversationID: msg.GetConversationId(),
			SenderID:       msg.GetSenderId(),
			Content:        msg.GetContent(),
			MsgType:        msg.GetMsgType(),
			CreatedAt:      createdAt,
			ReplyToID:      msg.GetReplyToId(),
		})
	}
	messages = filterWindowMessages(messages, input)
	return MessageWindow{Messages: messages, Participants: participantsResp.GetUserIds()}, nil
}

// RAGClientSink 把会话摘要/主题块写入 rag-service。
type RAGClientSink struct {
	service ragclient.Service
}

func NewRAGClientSink(service ragclient.Service) *RAGClientSink {
	return &RAGClientSink{service: service}
}

func (s *RAGClientSink) Archive(ctx context.Context, input RAGArchiveInput) error {
	if s == nil || s.service == nil {
		return nil
	}
	_, err := s.service.IngestDocument(ctx, input.OwnerID, ragclient.IngestInput{
		Title:          input.Title,
		Content:        input.Content,
		Source:         input.Source,
		SourceType:     "conversation",
		Visibility:     ragclient.VisibilityPrivate,
		GroupID:        input.GroupID,
		ConversationID: input.ConversationID,
	})
	return err
}

// MemoryClientSink 把长期有用事实写入 pending memory candidate。
type MemoryClientSink struct {
	service memoryclient.Service
}

func NewMemoryClientSink(service memoryclient.Service) *MemoryClientSink {
	return &MemoryClientSink{service: service}
}

func (s *MemoryClientSink) CreateCandidate(ctx context.Context, input MemoryCandidateArchiveInput) error {
	if s == nil || s.service == nil {
		return nil
	}
	_, err := s.service.CreateCandidate(ctx, memoryclient.CandidateInput{
		BotID:          input.AgentID,
		UserID:         input.UserID,
		OwnerUserID:    input.OwnerUserID,
		ConversationID: input.ConversationID,
		Scope:          memoryclient.ScopeConversation,
		Type:           memoryclient.TypeChatSummary,
		Title:          input.Title,
		Content:        input.Content,
		Source:         "conversation-intelligence",
		Evidence:       input.Evidence,
		Confidence:     input.Confidence,
		Importance:     input.Importance,
	})
	return err
}
