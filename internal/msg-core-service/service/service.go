package service

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/internal/msg-core-service/push"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/pkg/cache/redis"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/transport"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// MessageService 消息核心服务接口
// 提供会话管理和消息收发的核心业务逻辑
type MessageService interface {
	CreateConversation(ctx context.Context, convType string, participantIDs []int64, groupID int64) (*model.Conversation, error)
	GetConversation(ctx context.Context, conversationID int64) (*model.Conversation, error)
	GetUserConversations(ctx context.Context, userID int64) ([]UserConversationInfo, error)
	SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error)
	GetHistory(ctx context.Context, conversationID, userID int64, limit, beforeID int64) ([]model.Message, error)
	SearchMessages(ctx context.Context, userID int64, keyword string, limit int64) ([]model.Message, error)
	SearchMessagesInConversations(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.Message, error)
	GetConversationParticipants(ctx context.Context, conversationID int64) ([]int64, error)
}

// UserConversationInfo 用户会话列表项
// 包含会话基本信息和最后一条消息摘要，用于前端会话列表展示
type UserConversationInfo struct {
	ConversationID  int64   `json:"conversation_id"`
	Type            string  `json:"type"`
	LastMessage     string  `json:"last_message"`
	LastMessageTime string  `json:"last_message_time"`
	UnreadCount     int64   `json:"unread_count"`
	TargetName      string  `json:"target_name"`
	TargetAvatar    string  `json:"target_avatar"`
	ParticipantIDs  []int64 `json:"participant_ids"`
	LastSenderID    int64   `json:"last_sender_id"`
	GroupID         int64   `json:"group_id"`
}

type messageServiceImpl struct {
	repo        dao.MessageRepository
	pushClient  *push.PushClient
	redis       *redis.RedisClient
	groupClient groupservice.Client
}

func NewMessageService(repo dao.MessageRepository, pushClient *push.PushClient, redisClient *redis.RedisClient, etcdEndpoints []string) MessageService {
	var groupClient groupservice.Client
	etcdResolver, err := etcd.NewEtcdResolver(etcdEndpoints)
	if err != nil {
		log.Printf("创建etcd resolver失败，禁言检查将不可用: %v", err)
	} else {
		groupClient, err = groupservice.NewClient("group-service",
			client.WithResolver(etcdResolver),
			client.WithTransportProtocol(transport.TTHeader),
		)
		if err != nil {
			log.Printf("创建group-service客户端失败，禁言检查将不可用: %v", err)
		}
	}
	return &messageServiceImpl{repo: repo, pushClient: pushClient, redis: redisClient, groupClient: groupClient}
}

// CreateConversation 创建会话
// 流程：校验参数 → 私聊查重 → 创建会话记录 → 添加参与者 → 清除缓存
// 私聊类型会自动检查两个用户之间是否已有会话，避免重复创建
func (s *messageServiceImpl) CreateConversation(ctx context.Context, convType string, participantIDs []int64, groupID int64) (*model.Conversation, error) {
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
		Type:    convType,
		GroupID: groupID,
	}
	if err := s.repo.CreateConversation(ctx, conv); err != nil {
		return nil, err
	}

	// 第2步：添加所有参与者
	for _, uid := range participantIDs {
		p := &model.ConversationParticipant{
			ConversationID: conv.ID,
			UserID:         uid,
		}
		if err := s.repo.AddParticipant(ctx, p); err != nil {
			return nil, err
		}
	}

	// 第3步：清除所有参与者的会话列表缓存，使其重新加载
	for _, uid := range participantIDs {
		s.invalidateConversationCache(ctx, uid)
	}

	return conv, nil
}

// GetConversation 获取会话详情
// 根据会话ID查询会话信息，不存在则返回错误
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

// GetUserConversations 获取用户的会话列表
// 流程：查缓存 → 查数据库 → 组装会话信息 → 写缓存
// 每个会话项包含会话ID、类型、最后一条消息等摘要信息
func (s *messageServiceImpl) GetUserConversations(ctx context.Context, userID int64) ([]UserConversationInfo, error) {
	// 第1步：尝试从Redis缓存获取
	if s.redis != nil {
		cacheKey := fmt.Sprintf("user:conversations:%d", userID)
		var cached []UserConversationInfo
		hit, err := s.redis.GetJSON(ctx, cacheKey, &cached)
		if err == nil && hit != "" {
			return cached, nil
		}
	}

	// 第2步：缓存未命中，从数据库查询
	participants, err := s.repo.GetUserConversations(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 第3步：组装会话列表信息
	var result []UserConversationInfo
	for _, p := range participants {
		conv, _ := s.repo.GetConversationByID(ctx, p.ConversationID)
		if conv == nil {
			continue
		}

		allParticipants, _ := s.repo.GetParticipants(ctx, conv.ID)

		info := UserConversationInfo{
			ConversationID: conv.ID,
			Type:           conv.Type,
			GroupID:        conv.GroupID,
			ParticipantIDs: make([]int64, 0),
		}

		for _, ap := range allParticipants {
			info.ParticipantIDs = append(info.ParticipantIDs, ap.UserID)
		}

		msgs, _ := s.repo.GetMessages(ctx, conv.ID, 1, 0)
		if len(msgs) > 0 {
			info.LastMessage = msgs[0].Content
			info.LastMessageTime = msgs[0].CreatedAt.Format("2006-01-02 15:04:05")
			info.LastSenderID = msgs[0].SenderID
		}

		if conv.Type == "private" {
			for _, ap := range allParticipants {
				if ap.UserID != userID {
					info.TargetName = fmt.Sprintf("用户%d", ap.UserID)
					break
				}
			}
		} else if conv.Type == "group" && conv.GroupID > 0 {
			info.TargetName = fmt.Sprintf("群聊#%d", conv.GroupID)
		}

		result = append(result, info)
	}

	// 第4步：写入Redis缓存，5分钟过期
	if s.redis != nil && len(result) > 0 {
		cacheKey := fmt.Sprintf("user:conversations:%d", userID)
		s.redis.SetJSON(ctx, cacheKey, result, 5*time.Minute)
	}

	return result, nil
}

// SendMessage 发送消息（核心方法）
// 完整流程：校验 → 存储消息 → 更新会话时间 → 缓存最近消息 → 清除会话列表缓存 → WebSocket推送
func (s *messageServiceImpl) SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error) {
	// 参数校验
	if content == "" {
		return nil, errors.New("消息内容不能为空")
	}

	// 验证会话是否存在
	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("会话不存在")
	}

	if conv.Type == "group" && conv.GroupID > 0 && s.groupClient != nil {
		membersResp, err := s.groupClient.GetGroupMembers(ctx, &group.GetGroupMembersReq{GroupId: conv.GroupID})
		if err == nil && membersResp.Success {
			for _, m := range membersResp.Members {
				if m.UserId == senderID && m.MutedUntil != "" {
					mutedUntil, parseErr := time.Parse("2006-01-02 15:04:05", m.MutedUntil)
					if parseErr == nil && time.Now().Before(mutedUntil) {
						return nil, fmt.Errorf("你已被禁言，解除时间: %s", m.MutedUntil)
					}
				}
			}
		}
	}

	if msgType == "" {
		msgType = "text"
	}

	// 第1步：创建消息记录
	msg := &model.Message{
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
		MsgType:        msgType,
	}

	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return nil, err
	}

	// 第2步：更新会话的UpdatedAt时间戳（使会话列表按最新消息排序）
	conv.UpdatedAt = msg.CreatedAt
	_ = s.repo.UpdateConversation(ctx, conv)

	// 第3步：缓存最近消息到Redis（10分钟过期）
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

		// 清除所有参与者的会话列表缓存（因为最后一条消息已更新）
		participants, _ := s.repo.GetParticipants(ctx, conversationID)
		for _, p := range participants {
			s.invalidateConversationCache(ctx, p.UserID)
		}
	}

	// 第4步：通过WebSocket推送消息给所有参与者（实现实时通讯）
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

// GetHistory 获取会话历史消息
// 使用游标分页：beforeID > 0 时加载更早的消息
// 返回结果按时间正序排列（从旧到新）
func (s *messageServiceImpl) GetHistory(ctx context.Context, conversationID, userID int64, limit, beforeID int64) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	// 从数据库查询（按ID倒序）
	messages, err := s.repo.GetMessages(ctx, conversationID, limit, beforeID)
	if err != nil {
		return nil, err
	}

	// 反转为时间正序（从旧到新），方便前端展示
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// SearchMessages 搜索消息
// 先获取用户参与的所有会话ID，然后在这些会话中搜索包含关键词的消息
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

// SearchMessagesInConversations 在指定会话中搜索消息
// 直接在给定的会话ID列表中搜索，不需要获取用户的所有会话
func (s *messageServiceImpl) SearchMessagesInConversations(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.Message, error) {
	if keyword == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	if len(conversationIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	return s.repo.SearchMessages(ctx, conversationIDs, keyword, limit)
}

// GetConversationParticipants 获取会话参与者ID列表
// 用于确定消息推送的目标用户
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

// invalidateConversationCache 清除用户的会话列表缓存
// 当会话信息发生变化时（新消息、新会话等）调用
func (s *messageServiceImpl) invalidateConversationCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("user:conversations:%d", userID))
}
