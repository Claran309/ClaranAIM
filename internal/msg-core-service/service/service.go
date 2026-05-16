package service

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/internal/msg-core-service/push"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/pkg/cache/redis"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/transport"
	etcd "github.com/kitex-contrib/registry-etcd"
)

type MessageService interface {
	CreateConversation(ctx context.Context, convType string, participantIDs []int64, groupID int64) (*model.Conversation, error)
	GetConversation(ctx context.Context, conversationID int64) (*model.Conversation, error)
	GetUserConversations(ctx context.Context, userID int64) ([]UserConversationInfo, error)
	SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error)
	SendMessageExt(ctx context.Context, opts SendMessageOptions) (*model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID, messageID int64) error
	EditMessage(ctx context.Context, messageID, editorID int64, content string) (*model.Message, error)
	RecallMessage(ctx context.Context, messageID, operatorID int64) error
	GetHistory(ctx context.Context, conversationID, userID int64, limit, beforeID int64) ([]model.Message, error)
	SearchMessages(ctx context.Context, userID int64, keyword string, limit int64) ([]model.Message, error)
	SearchMessagesInConversations(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.Message, error)
	SearchMessagesAdvanced(ctx context.Context, opts SearchMessagesOptions) ([]model.Message, error)
	GetConversationParticipants(ctx context.Context, conversationID int64) ([]int64, error)
}

const (
	MessageStatusSent     = "sent"
	MessageStatusRecalled = "recalled"
)

const defaultRecallWindow = 2 * time.Minute

type SendMessageOptions struct {
	ConversationID int64
	SenderID       int64
	Content        string
	MsgType        string
	ReplyToID      int64
	MentionUserIDs []int64
	MentionAll     bool
}

type SearchMessagesOptions struct {
	UserID          int64
	ConversationIDs []int64
	Keyword         string
	Limit           int64
	StartAt         time.Time
	EndAt           time.Time
}

// UserConversationInfo is a sidebar-ready projection assembled from conversations,
// participants and the latest message. Keeping this shape in the service avoids
// forcing the browser to fan out one request per conversation row.
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
	IsDeletedGroup  bool    `json:"is_deleted_group"`
}

type messageServiceImpl struct {
	repo        dao.MessageRepository
	pushClient  *push.PushClient
	redis       *redis.RedisClient
	groupClient groupservice.Client
	recallWindow time.Duration
}

var errConversationAccessDenied = errors.New("无权访问该会话")

// NewMessageService connects the message domain with Redis, WebSocket push and
// group-service. If group-service discovery fails, private chat still works and
// group access falls back to local conversation_participants.
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
	return &messageServiceImpl{repo: repo, pushClient: pushClient, redis: redisClient, groupClient: groupClient, recallWindow: defaultRecallWindow}
}

func (s *messageServiceImpl) CreateConversation(ctx context.Context, convType string, participantIDs []int64, groupID int64) (*model.Conversation, error) {
	if convType != "private" && convType != "group" {
		return nil, errors.New("无效的会话类型")
	}
	participantIDs = dedupeUserIDs(participantIDs)
	if len(participantIDs) < 2 {
		return nil, errors.New("参与者至少为2人")
	}
	if convType == "group" && groupID <= 0 {
		return nil, errors.New("group_id不能为空")
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

	if convType == "group" {
		existing, err := s.repo.FindGroupConversation(ctx, groupID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			// group_members is owned by group-service, while message delivery fans
			// out through conversation_participants. Reusing a group conversation
			// must sync newly invited members into the message domain.
			if err := s.syncConversationParticipants(ctx, existing.ID, participantIDs); err != nil {
				return nil, err
			}
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
	if err := s.syncConversationParticipants(ctx, conv.ID, participantIDs); err != nil {
		return nil, err
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

	result := make([]UserConversationInfo, 0, len(participants))
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
			ParticipantIDs: make([]int64, 0, len(allParticipants)),
		}
		for _, ap := range allParticipants {
			info.ParticipantIDs = append(info.ParticipantIDs, ap.UserID)
		}

		msgs, _ := s.repo.GetMessages(ctx, conv.ID, 1, 0)
		if len(msgs) > 0 {
			hydrateMessageRuntimeFields(&msgs[0])
			info.LastMessage = msgs[0].Content
			if msgs[0].Status == MessageStatusRecalled {
				info.LastMessage = "[消息已撤回]"
			}
			info.LastMessageTime = msgs[0].CreatedAt.Format("2006-01-02 15:04:05")
			info.LastSenderID = msgs[0].SenderID
		}
		info.UnreadCount = countUnreadMessages(ctx, s.repo, conv.ID, p.LastReadMessageID)

		if conv.Type == "private" {
			for _, ap := range allParticipants {
				if ap.UserID != userID {
					info.TargetName = fmt.Sprintf("用户%d", ap.UserID)
					break
				}
			}
		} else if conv.Type == "group" && conv.GroupID > 0 {
			if s.groupClient != nil {
				groupResp, groupErr := s.groupClient.GetGroup(ctx, &group.GetGroupReq{GroupId: conv.GroupID})
				if groupErr == nil && groupResp != nil {
					if !groupResp.Success || groupResp.Group == nil {
						info.IsDeletedGroup = true
						info.TargetName = fmt.Sprintf("群聊#%d（已解散）", conv.GroupID)
					} else {
						info.TargetName = groupResp.Group.Name
					}
				}
			}
			if info.TargetName == "" {
				info.TargetName = fmt.Sprintf("群聊#%d", conv.GroupID)
			}
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
	return s.SendMessageExt(ctx, SendMessageOptions{
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
		MsgType:        msgType,
	})
}

func (s *messageServiceImpl) SendMessageExt(ctx context.Context, opts SendMessageOptions) (*model.Message, error) {
	content := opts.Content
	if content == "" {
		return nil, errors.New("消息内容不能为空")
	}

	conv, err := s.repo.GetConversationByID(ctx, opts.ConversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("会话不存在")
	}
	// Every message write passes through the same participant gate used by
	// history reads, so guessed conversation IDs cannot be used to send messages.
	if err := s.ensureConversationParticipant(ctx, conv, opts.SenderID); err != nil {
		return nil, err
	}
	if err := s.checkGroupMute(ctx, conv, opts.SenderID); err != nil {
		return nil, err
	}

	msgType := opts.MsgType
	if msgType == "" {
		msgType = "text"
	}
	mentions := dedupeUserIDs(opts.MentionUserIDs)
	mentionsJSON, err := marshalMentionUserIDs(mentions)
	if err != nil {
		return nil, err
	}
	if opts.ReplyToID > 0 {
		replyMsg, err := s.repo.GetMessageByID(ctx, opts.ReplyToID)
		if err != nil {
			return nil, err
		}
		if replyMsg == nil || replyMsg.ConversationID != opts.ConversationID {
			return nil, errors.New("引用消息不存在")
		}
	}

	msg := &model.Message{
		ConversationID: opts.ConversationID,
		SenderID:       opts.SenderID,
		Content:        content,
		MsgType:        msgType,
		ReplyToID:      opts.ReplyToID,
		Status:         MessageStatusSent,
		MentionUserIDs: mentions,
		MentionsJSON:   mentionsJSON,
		MentionAll:     opts.MentionAll,
	}
	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return nil, err
	}
	hydrateMessageRuntimeFields(msg)

	conv.UpdatedAt = msg.CreatedAt
	_ = s.repo.UpdateConversation(ctx, conv)

	participants, _ := s.repo.GetParticipants(ctx, opts.ConversationID)
	if s.redis != nil {
		cacheKey := fmt.Sprintf("conversation:recent:%d", opts.ConversationID)
		recentMsg := map[string]interface{}{
			"id":              msg.ID,
			"conversation_id": msg.ConversationID,
			"sender_id":       msg.SenderID,
			"content":         msg.Content,
			"msg_type":        msg.MsgType,
			"created_at":      msg.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		s.redis.SetJSON(ctx, cacheKey, recentMsg, 10*time.Minute)
		for _, p := range participants {
			s.invalidateConversationCache(ctx, p.UserID)
		}
	}

	if s.pushClient != nil {
		// WebSocket gateway does not know conversation membership. msg-core turns
		// the conversation into concrete user IDs before asking the gateway to push.
		targetUserIDs := make([]int64, 0, len(participants))
		for _, p := range participants {
			targetUserIDs = append(targetUserIDs, p.UserID)
		}
		pushData := push.MessageData{
			Type:           "new_message",
			ConversationID: opts.ConversationID,
			SenderID:       opts.SenderID,
			Content:        content,
			MsgType:        msgType,
			MsgID:          msg.ID,
			CreatedAt:      msg.CreatedAt.Format("2006-01-02 15:04:05"),
			ReplyToID:      msg.ReplyToID,
			Status:         msg.Status,
			MentionUserIDs: msg.MentionUserIDs,
			MentionAll:     msg.MentionAll,
		}
		if err := s.pushClient.PushMessage(targetUserIDs, pushData); err != nil {
			log.Printf("WebSocket推送失败: %v", err)
		}
	}

	return msg, nil
}

func (s *messageServiceImpl) MarkConversationRead(ctx context.Context, conversationID, userID, messageID int64) error {
	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv == nil {
		return errors.New("会话不存在")
	}
	if err := s.ensureConversationParticipant(ctx, conv, userID); err != nil {
		return err
	}
	if messageID <= 0 {
		msgs, err := s.repo.GetMessages(ctx, conversationID, 1, 0)
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			return nil
		}
		messageID = msgs[0].ID
	}
	if err := s.repo.UpdateParticipantReadCursor(ctx, conversationID, userID, messageID, time.Now()); err != nil {
		return err
	}
	s.invalidateConversationCache(ctx, userID)
	return nil
}

func (s *messageServiceImpl) EditMessage(ctx context.Context, messageID, editorID int64, content string) (*model.Message, error) {
	if content == "" {
		return nil, errors.New("消息内容不能为空")
	}
	msg, err := s.repo.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, errors.New("消息不存在")
	}
	if msg.SenderID != editorID {
		return nil, errConversationAccessDenied
	}
	if msg.Status == MessageStatusRecalled {
		return nil, errors.New("已撤回消息不能编辑")
	}
	conv, err := s.repo.GetConversationByID(ctx, msg.ConversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("会话不存在")
	}
	if err := s.ensureConversationParticipant(ctx, conv, editorID); err != nil {
		return nil, err
	}

	oldContent := msg.Content
	msg.Content = content
	msg.IsEdited = true
	msg.EditedAt = time.Now()
	if err := s.repo.UpdateMessage(ctx, msg); err != nil {
		return nil, err
	}
	_ = s.repo.CreateEditRecord(ctx, &model.MessageEditRecord{
		MessageID:      msg.ID,
		ConversationID: msg.ConversationID,
		EditorID:       editorID,
		OldContent:     oldContent,
		NewContent:     content,
	})
	s.invalidateConversationParticipantsCache(ctx, msg.ConversationID)
	s.pushMessageState(ctx, msg, "message_edited")
	hydrateMessageRuntimeFields(msg)
	return msg, nil
}

func (s *messageServiceImpl) RecallMessage(ctx context.Context, messageID, operatorID int64) error {
	msg, err := s.repo.GetMessageByID(ctx, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return errors.New("消息不存在")
	}
	if msg.SenderID != operatorID {
		return errConversationAccessDenied
	}
	window := s.recallWindow
	if window == 0 {
		window = defaultRecallWindow
	}
	if time.Since(msg.CreatedAt) > window {
		return errors.New("消息已超过可撤回时间")
	}
	conv, err := s.repo.GetConversationByID(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	if conv == nil {
		return errors.New("会话不存在")
	}
	if err := s.ensureConversationParticipant(ctx, conv, operatorID); err != nil {
		return err
	}
	msg.Content = ""
	msg.Status = MessageStatusRecalled
	if err := s.repo.UpdateMessage(ctx, msg); err != nil {
		return err
	}
	s.invalidateConversationParticipantsCache(ctx, msg.ConversationID)
	s.pushMessageState(ctx, msg, "message_recalled")
	return nil
}

func (s *messageServiceImpl) GetHistory(ctx context.Context, conversationID, userID int64, limit, beforeID int64) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("会话不存在")
	}
	// History contains private data. The caller must be a participant before
	// the service returns any page of messages.
	if err := s.ensureConversationParticipant(ctx, conv, userID); err != nil {
		return nil, err
	}

	messages, err := s.repo.GetMessages(ctx, conversationID, limit, beforeID)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		hydrateMessageRuntimeFields(&messages[i])
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (s *messageServiceImpl) SearchMessages(ctx context.Context, userID int64, keyword string, limit int64) ([]model.Message, error) {
	if keyword == "" {
		return nil, errors.New("搜索关键字不能为空")
	}
	if limit <= 0 {
		limit = 20
	}

	participants, err := s.repo.GetUserConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	convIDs := make([]int64, 0, len(participants))
	for _, p := range participants {
		convIDs = append(convIDs, p.ConversationID)
	}
	if len(convIDs) == 0 {
		return nil, nil
	}
	return s.repo.SearchMessages(ctx, convIDs, keyword, limit, nil, nil)
}

func (s *messageServiceImpl) SearchMessagesInConversations(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.Message, error) {
	if keyword == "" {
		return nil, errors.New("搜索关键字不能为空")
	}
	if len(conversationIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	return s.SearchMessagesAdvanced(ctx, SearchMessagesOptions{
		ConversationIDs: conversationIDs,
		Keyword:         keyword,
		Limit:           limit,
	})
}

func (s *messageServiceImpl) SearchMessagesAdvanced(ctx context.Context, opts SearchMessagesOptions) ([]model.Message, error) {
	if opts.Keyword == "" {
		return nil, errors.New("搜索关键字不能为空")
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	convIDs := dedupeUserIDs(opts.ConversationIDs)
	if len(convIDs) == 0 {
		participants, err := s.repo.GetUserConversations(ctx, opts.UserID)
		if err != nil {
			return nil, err
		}
		for _, p := range participants {
			convIDs = append(convIDs, p.ConversationID)
		}
	} else if opts.UserID > 0 {
		allowed := map[int64]struct{}{}
		participants, err := s.repo.GetUserConversations(ctx, opts.UserID)
		if err != nil {
			return nil, err
		}
		for _, p := range participants {
			allowed[p.ConversationID] = struct{}{}
		}
		for _, convID := range convIDs {
			if _, ok := allowed[convID]; !ok {
				return nil, errConversationAccessDenied
			}
		}
	}
	if len(convIDs) == 0 {
		return nil, nil
	}
	var startAt, endAt *time.Time
	if !opts.StartAt.IsZero() {
		startAt = &opts.StartAt
	}
	if !opts.EndAt.IsZero() {
		endAt = &opts.EndAt
	}
	msgs, err := s.repo.SearchMessages(ctx, convIDs, opts.Keyword, opts.Limit, startAt, endAt)
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		hydrateMessageRuntimeFields(&msgs[i])
	}
	return msgs, nil
}

func (s *messageServiceImpl) GetConversationParticipants(ctx context.Context, conversationID int64) ([]int64, error) {
	participants, err := s.repo.GetParticipants(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	userIDs := make([]int64, 0, len(participants))
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

func (s *messageServiceImpl) invalidateConversationParticipantsCache(ctx context.Context, conversationID int64) {
	participants, err := s.repo.GetParticipants(ctx, conversationID)
	if err != nil {
		return
	}
	for _, p := range participants {
		s.invalidateConversationCache(ctx, p.UserID)
	}
}

func (s *messageServiceImpl) pushMessageState(ctx context.Context, msg *model.Message, eventType string) {
	if s.pushClient == nil || msg == nil {
		return
	}
	participants, err := s.repo.GetParticipants(ctx, msg.ConversationID)
	if err != nil {
		return
	}
	targetUserIDs := make([]int64, 0, len(participants))
	for _, p := range participants {
		targetUserIDs = append(targetUserIDs, p.UserID)
	}
	if err := s.pushClient.PushMessage(targetUserIDs, push.MessageData{
		Type:           eventType,
		ConversationID: msg.ConversationID,
		SenderID:       msg.SenderID,
		Content:        msg.Content,
		MsgType:        msg.MsgType,
		MsgID:          msg.ID,
		CreatedAt:      msg.CreatedAt.Format("2006-01-02 15:04:05"),
		ReplyToID:      msg.ReplyToID,
		Status:         msg.Status,
		IsEdited:       msg.IsEdited,
		EditedAt:       formatTime(msg.EditedAt),
		MentionUserIDs: msg.MentionUserIDs,
		MentionAll:     msg.MentionAll,
	}); err != nil {
		log.Printf("WebSocket消息状态推送失败: %v", err)
	}
}

func (s *messageServiceImpl) fetchGroupMembers(ctx context.Context, groupID int64) (*group.GetGroupMembersResp, error) {
	if s.groupClient == nil {
		return nil, nil
	}
	resp, err := s.groupClient.GetGroupMembers(ctx, &group.GetGroupMembersReq{GroupId: groupID})
	if err != nil {
		return nil, err
	}
	if resp != nil && !resp.Success {
		return resp, errConversationAccessDenied
	}
	return resp, nil
}

func (s *messageServiceImpl) ensureConversationParticipant(ctx context.Context, conv *model.Conversation, userID int64) error {
	if conv.Type == "group" && conv.GroupID > 0 && s.groupClient != nil {
		membersResp, err := s.fetchGroupMembers(ctx, conv.GroupID)
		if err == nil && membersResp != nil && membersResp.Success {
			// group_members is the source of truth for group membership. Syncing
			// here lets old conversations pick up new members without a migration.
			memberIDs := make([]int64, 0, len(membersResp.Members))
			isMember := false
			for _, m := range membersResp.Members {
				memberIDs = append(memberIDs, m.UserId)
				if m.UserId == userID {
					isMember = true
				}
			}
			if err := s.syncConversationParticipants(ctx, conv.ID, memberIDs); err != nil {
				return err
			}
			if !isMember {
				return errConversationAccessDenied
			}
			return nil
		}
		if errors.Is(err, errConversationAccessDenied) {
			return err
		}
	}

	participants, err := s.repo.GetParticipants(ctx, conv.ID)
	if err != nil {
		return err
	}
	for _, p := range participants {
		if p.UserID == userID {
			return nil
		}
	}
	return errConversationAccessDenied
}

func (s *messageServiceImpl) checkGroupMute(ctx context.Context, conv *model.Conversation, senderID int64) error {
	if conv.Type != "group" || conv.GroupID <= 0 || s.groupClient == nil {
		return nil
	}
	membersResp, err := s.fetchGroupMembers(ctx, conv.GroupID)
	if errors.Is(err, errConversationAccessDenied) {
		return err
	}
	if err != nil || membersResp == nil || !membersResp.Success {
		return nil
	}
	for _, m := range membersResp.Members {
		if m.UserId != senderID {
			continue
		}
		if m.MutedUntil == "" {
			return nil
		}
		mutedUntil, parseErr := time.Parse("2006-01-02 15:04:05", m.MutedUntil)
		if parseErr == nil && time.Now().Before(mutedUntil) {
			return fmt.Errorf("你已被禁言，解除时间: %s", m.MutedUntil)
		}
		return nil
	}
	return errConversationAccessDenied
}

func (s *messageServiceImpl) syncConversationParticipants(ctx context.Context, conversationID int64, userIDs []int64) error {
	userIDs = dedupeUserIDs(userIDs)
	if len(userIDs) == 0 {
		return nil
	}

	participants, err := s.repo.GetParticipants(ctx, conversationID)
	if err != nil {
		return err
	}
	existing := make(map[int64]struct{}, len(participants))
	for _, p := range participants {
		existing[p.UserID] = struct{}{}
	}
	for _, userID := range userIDs {
		if _, ok := existing[userID]; ok {
			continue
		}
		if err := s.repo.AddParticipant(ctx, &model.ConversationParticipant{
			ConversationID: conversationID,
			UserID:         userID,
		}); err != nil {
			return err
		}
		// A newly synced participant needs a fresh sidebar conversation list.
		s.invalidateConversationCache(ctx, userID)
	}
	return nil
}

func dedupeUserIDs(userIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(userIDs))
	result := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		result = append(result, userID)
	}
	return result
}

func marshalMentionUserIDs(userIDs []int64) (string, error) {
	if len(userIDs) == 0 {
		return "", nil
	}
	data, err := json.Marshal(userIDs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func hydrateMessageRuntimeFields(msg *model.Message) {
	if msg == nil || msg.MentionsJSON == "" {
		return
	}
	var ids []int64
	if err := json.Unmarshal([]byte(msg.MentionsJSON), &ids); err == nil {
		msg.MentionUserIDs = dedupeUserIDs(ids)
	}
}

func countUnreadMessages(ctx context.Context, repo dao.MessageRepository, conversationID, lastReadMessageID int64) int64 {
	msgs, err := repo.GetMessages(ctx, conversationID, 1000, 0)
	if err != nil {
		return 0
	}
	var count int64
	for _, msg := range msgs {
		if msg.ID > lastReadMessageID && msg.Status != MessageStatusRecalled {
			count++
		}
	}
	return count
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
