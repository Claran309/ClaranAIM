package service

import (
	"ClaranAIM/kitex_gen/memory"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/kitex_gen/rag/ragservice"
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
	client ragservice.Client
}

func NewRAGClientSink(client ragservice.Client) *RAGClientSink {
	return &RAGClientSink{client: client}
}

func (s *RAGClientSink) Archive(ctx context.Context, input RAGArchiveInput) error {
	if s == nil || s.client == nil {
		return nil
	}
	resp, err := s.client.IngestDocument(ctx, &rag.IngestDocumentReq{
		OwnerId:        input.OwnerID,
		Title:          input.Title,
		Content:        input.Content,
		Source:         input.Source,
		SourceType:     "conversation",
		Visibility:     "private",
		GroupId:        input.GroupID,
		ConversationId: input.ConversationID,
	})
	if err != nil {
		return err
	}
	if !resp.GetSuccess() {
		return errors.New(resp.GetMsg())
	}
	return nil
}

// MemoryClientSink 把长期有用事实写入 pending memory candidate。
type MemoryClientSink struct {
	client memoryservice.Client
}

func NewMemoryClientSink(client memoryservice.Client) *MemoryClientSink {
	return &MemoryClientSink{client: client}
}

func (s *MemoryClientSink) CreateCandidate(ctx context.Context, input MemoryCandidateArchiveInput) error {
	if s == nil || s.client == nil {
		return nil
	}
	resp, err := s.client.CreateCandidate(ctx, &memory.CreateCandidateReq{
		BotId:          input.AgentID,
		UserId:         input.UserID,
		OwnerUserId:    input.OwnerUserID,
		ConversationId: input.ConversationID,
		Scope:          "conversation",
		Type:           "chat_summary",
		Title:          input.Title,
		Content:        input.Content,
		Source:         "conversation-intelligence",
		Evidence:       input.Evidence,
		Confidence:     input.Confidence,
		Importance:     input.Importance,
	})
	if err != nil {
		return err
	}
	if !resp.GetSuccess() {
		return errors.New(resp.GetMsg())
	}
	return nil
}
