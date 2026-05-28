package service

import (
	"ClaranAIM/internal/msg-history-service/dao"
	"ClaranAIM/internal/msg-history-service/model"
	"context"
	"errors"
)

// HistoryService 消息历史服务接口
// 提供消息归档存储、历史查询、离线消息管理等功能
type HistoryService interface {
	// SaveMessage 保存消息到历史记录
	SaveMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.MessageHistory, error)
	// GetHistory 获取会话历史消息（支持游标分页，时间正序返回）
	GetHistory(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error)
	// SearchHistory 在指定会话中搜索历史消息
	SearchHistory(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.MessageHistory, error)
	// AddOfflineMessage 添加离线消息记录
	AddOfflineMessage(ctx context.Context, userID, messageID int64) error
	// GetOfflineMessages 获取用户的未读离线消息
	GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error)
	// MarkOfflineRead 批量标记离线消息为已读
	MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error
	// GetUnreadCount 获取用户未读消息数
	GetUnreadCount(ctx context.Context, userID int64) (int64, error)
}

// historyServiceImpl 定义当前包使用的数据结构或接口，用于在业务层、持久化层和传输层之间传递明确语义。
type historyServiceImpl struct {
	repo dao.HistoryRepository
}

// NewHistoryService 创建消息历史业务服务。
//
// service 层承载参数校验、分页顺序调整和离线消息语义，DAO 层只负责具体 SQL/GORM 操作。
func NewHistoryService(repo dao.HistoryRepository) HistoryService {
	return &historyServiceImpl{repo: repo}
}

// SaveMessage 保存消息到历史记录
// 流程：校验参数 → 创建历史记录 → 持久化到数据库
// 与 msg-core-service 的消息存储分离，实现数据的长期归档
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

// GetHistory 获取会话历史消息
// 使用游标分页：beforeID > 0 时加载更早的消息
// 返回结果按时间正序排列（从旧到新），方便前端展示
func (s *historyServiceImpl) GetHistory(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.MessageHistory, error) {
	if limit <= 0 {
		limit = 50
	}
	msgs, err := s.repo.GetHistoryByConversation(ctx, conversationID, limit, beforeID)
	if err != nil {
		return nil, err
	}

	// 反转为时间正序（从旧到新）
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

// SearchHistory 在指定会话中搜索历史消息
// 使用 LIKE 模糊匹配关键词
func (s *historyServiceImpl) SearchHistory(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.MessageHistory, error) {
	if keyword == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.SearchHistory(ctx, conversationIDs, keyword, limit)
}

// AddOfflineMessage 添加离线消息记录
// 当用户不在线时，将消息记录到离线消息表
// 用户上线后通过 GetOfflineMessages 获取未读消息
func (s *historyServiceImpl) AddOfflineMessage(ctx context.Context, userID, messageID int64) error {
	return s.repo.AddOfflineMessage(ctx, userID, messageID)
}

// GetOfflineMessages 获取用户的未读离线消息列表
// 返回所有 is_read = false 的离线消息记录
func (s *historyServiceImpl) GetOfflineMessages(ctx context.Context, userID int64) ([]model.OfflineMessage, error) {
	return s.repo.GetOfflineMessages(ctx, userID)
}

// MarkOfflineRead 批量标记离线消息为已读
// 用户查看消息后调用，同时记录已读时间
func (s *historyServiceImpl) MarkOfflineRead(ctx context.Context, userID int64, messageIDs []int64) error {
	return s.repo.MarkOfflineRead(ctx, userID, messageIDs)
}

// GetUnreadCount 获取用户的未读消息总数
// 用于前端显示未读消息角标（如聊天图标上的红色数字）
func (s *historyServiceImpl) GetUnreadCount(ctx context.Context, userID int64) (int64, error) {
	return s.repo.GetUnreadCount(ctx, userID)
}
