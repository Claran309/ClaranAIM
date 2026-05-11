package service

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/internal/msg-core-service/push"
	"ClaranAIM/pkg/cache/redis"
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

type MessageService interface {
	CreateConversation(ctx context.Context, convType string, participantIDs []int64) (*model.Conversation, error)
	GetConversation(ctx context.Context, conversationID int64) (*model.Conversation, error)
	GetUserConversations(ctx context.Context, userID int64) ([]UserConversationInfo, error)
	SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error)
	GetHistory(ctx context.Context, conversationID, userID int64, limit, beforeID int64) ([]model.Message, error)
	SearchMessages(ctx context.Context, userID int64, keyword string, limit int64) ([]model.Message, error)
	GetConversationParticipants(ctx context.Context, conversationID int64) ([]int64, error)
}

type UserConversationInfo struct {
	ConversationID  int64  `json:"conversation_id"`
	Type            string `json:"type"`
	LastMessage     string `json:"last_message"`
	LastMessageTime string `json:"last_message_time"`
	UnreadCount     int64  `json:"unread_count"`
	TargetName      string `json:"target_name"`
	TargetAvatar    string `json:"target_avatar"`
}

type messageServiceImpl struct {
	repo       dao.MessageRepository
	pushClient *push.PushClient
	redis      *redis.RedisClient
}

func NewMessageService(repo dao.MessageRepository, pushClient *push.PushClient, r *redis.RedisClient) MessageService {
	return &messageServiceImpl{repo: repo, pushClient: pushClient, redis: r}
}

func (s *messageServiceImpl) CreateConversation(ctx context.Context, convType string, participantIDs []int64) (*model.Conversation, error) {
	if convType != "private" && convType != "group" {
		return nil, errors.New("无效的会话类型")
	}
	if len(participantIDs) < 2 {
		return nil, errors.New("参与者至少为2人")
	}

	if convType == "private" && len(participantIDs) == 2 {
		existing, err := s.repo.FindPrivateConversation(ctx, participantIDs[0], participantIDs[1])
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	conv := &model.Conversation{
		Type: convType,
	}
	if err := s.repo.CreateConversation(ctx, conv); err != nil {
		return nil, err
	}

	for _, uid := range participantIDs {
		p := &model.ConversationParticipant{
			ConversationID: conv.ID,
			UserID:         uid,
		}
		if err := s.repo.AddParticipant(ctx, p); err != nil {
			return nil, err
		}
	}

	for _, uid := range participantIDs {
		s.invalidateConversationCache(ctx, uid)
	}

	return conv, nil
}

func (s *messageServiceImpl) GetConversation(ctx context.Context, conversationID int64) (*model.Conversation, error) {
	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("会话不存在")
	}
	return conv, nil
}

func (s *messageServiceImpl) GetUserConversations(ctx context.Context, userID int64) ([]UserConversationInfo, error) {
	if s.redis != nil {
		cacheKey := fmt.Sprintf("user:conversations:%d", userID)
		var cached []UserConversationInfo
		hit, err := s.redis.GetJSON(ctx, cacheKey, &cached)
		if err == nil && hit != "" {
			return cached, nil
		}
	}

	participants, err := s.repo.GetUserConversations(ctx, userID)
	if err != nil {
		return nil, err
	}

	var result []UserConversationInfo
	for _, p := range participants {
		conv, _ := s.repo.GetConversationByID(ctx, p.ConversationID)
		if conv == nil {
			continue
		}

		info := UserConversationInfo{
			ConversationID: conv.ID,
			Type:           conv.Type,
		}

		msgs, _ := s.repo.GetMessages(ctx, conv.ID, 1, 0)
		if len(msgs) > 0 {
			info.LastMessage = msgs[0].Content
			info.LastMessageTime = msgs[0].CreatedAt.Format("2006-01-02 15:04:05")
		}

		result = append(result, info)
	}

	if s.redis != nil && len(result) > 0 {
		cacheKey := fmt.Sprintf("user:conversations:%d", userID)
		s.redis.SetJSON(ctx, cacheKey, result, 5*time.Minute)
	}

	return result, nil
}

func (s *messageServiceImpl) SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error) {
	if content == "" {
		return nil, errors.New("消息内容不能为空")
	}

	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("会话不存在")
	}

	if msgType == "" {
		msgType = "text"
	}

	msg := &model.Message{
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
		MsgType:        msgType,
	}

	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return nil, err
	}

	conv.UpdatedAt = msg.CreatedAt
	_ = s.repo.UpdateConversation(ctx, conv)

	// 缓存最近消息
	if s.redis != nil {
		cacheKey := fmt.Sprintf("conversation:recent:%d", conversationID)
		recentMsg := map[string]interface{}{
			"id":              msg.ID,
			"conversation_id": msg.ConversationID,
			"sender_id":       msg.SenderID,
			"content":         msg.Content,
			"msg_type":        msg.MsgType,
			"created_at":      msg.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		s.redis.SetJSON(ctx, cacheKey, recentMsg, 10*time.Minute)

		participants, _ := s.repo.GetParticipants(ctx, conversationID)
		for _, p := range participants {
			s.invalidateConversationCache(ctx, p.UserID)
		}
	}

	// WebSocket推送
	if s.pushClient != nil {
		participants, err := s.repo.GetParticipants(ctx, conversationID)
		if err == nil {
			var targetUserIDs []int64
			for _, p := range participants {
				targetUserIDs = append(targetUserIDs, p.UserID)
			}

			pushData := push.MessageData{
				Type:           "new_message",
				ConversationID: conversationID,
				SenderID:       senderID,
				Content:        content,
				MsgType:        msgType,
				MsgID:          msg.ID,
				CreatedAt:      msg.CreatedAt.Format("2006-01-02 15:04:05"),
			}

			if err := s.pushClient.PushMessage(targetUserIDs, pushData); err != nil {
				log.Printf("WebSocket推送失败: %v", err)
			}
		}
	}

	return msg, nil
}

func (s *messageServiceImpl) GetHistory(ctx context.Context, conversationID, userID int64, limit, beforeID int64) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	messages, err := s.repo.GetMessages(ctx, conversationID, limit, beforeID)
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (s *messageServiceImpl) SearchMessages(ctx context.Context, userID int64, keyword string, limit int64) ([]model.Message, error) {
	if keyword == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	if limit <= 0 {
		limit = 20
	}

	participants, err := s.repo.GetUserConversations(ctx, userID)
	if err != nil {
		return nil, err
	}

	var convIDs []int64
	for _, p := range participants {
		convIDs = append(convIDs, p.ConversationID)
	}

	if len(convIDs) == 0 {
		return nil, nil
	}

	return s.repo.SearchMessages(ctx, convIDs, keyword, limit)
}

func (s *messageServiceImpl) GetConversationParticipants(ctx context.Context, conversationID int64) ([]int64, error) {
	participants, err := s.repo.GetParticipants(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	var userIDs []int64
	for _, p := range participants {
		userIDs = append(userIDs, p.UserID)
	}
	return userIDs, nil
}

func (s *messageServiceImpl) invalidateConversationCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("user:conversations:%d", userID))
}
