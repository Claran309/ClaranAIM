package service

import (
	"ClaranAIM/internal/msg-history-service/dao"
	"ClaranAIM/internal/msg-history-service/model"
	"context"
	"errors"
)

type HistoryService interface {
	SaveMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.MessageHistory, error)
	GetHistory(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error)
	SearchHistory(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.MessageHistory, error)
	AddOfflineMessage(ctx context.Context, userID, messageID int64) error
	GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error)
	MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error
	GetUnreadCount(ctx context.Context, userID int64) (int64, error)
}

type historyServiceImpl struct {
	repo dao.HistoryRepository
}

func NewHistoryService(repo dao.HistoryRepository) HistoryService {
	return &historyServiceImpl{repo: repo}
}

func (s *historyServiceImpl) SaveMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.MessageHistory, error) {
	if content == "" {
		return nil, errors.New("消息内容不能为空")
	}
	if msgType == "" {
		msgType = "text"
	}

	msg := &model.MessageHistory{
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
		MsgType:        msgType,
	}

	if err := s.repo.SaveMessage(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (s *historyServiceImpl) GetHistory(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error) {
	if limit <= 0 {
		limit = 50
	}
	msgs, err := s.repo.GetHistoryByConversation(ctx, conversationID, limit, beforeID)
	if err != nil {
		return nil, err
	}

	// 反转为时间正序
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *historyServiceImpl) SearchHistory(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.MessageHistory, error) {
	if keyword == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.SearchHistory(ctx, conversationIDs, keyword, limit)
}

func (s *historyServiceImpl) AddOfflineMessage(ctx context.Context, userID, messageID int64) error {
	return s.repo.AddOfflineMessage(ctx, userID, messageID)
}

func (s *historyServiceImpl) GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error) {
	return s.repo.GetOfflineMessages(ctx, userID)
}

func (s *historyServiceImpl) MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error {
	return s.repo.MarkOfflineRead(ctx, userID, messageIDs)
}

func (s *historyServiceImpl) GetUnreadCount(ctx context.Context, userID int64) (int64, error) {
	return s.repo.GetUnreadCount(ctx, userID)
}
