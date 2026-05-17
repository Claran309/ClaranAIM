package service

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/pkg/cache/redis"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/outbox"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/transport"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// MessageService defines the message-domain operations owned by msg-core-service.
//
// This service owns conversations, message facts, per-user message state,
// message read cursors and the transactional outbox records that eventually
// become Kafka/WebSocket events. API gateway and other services should call this
// interface instead of touching message tables directly.
type MessageService interface {
	CreateConversation(ctx context.Context, convType string, participantIDs []int64, groupID int64) (*model.Conversation, error)
	GetConversation(ctx context.Context, conversationID int64) (*model.Conversation, error)
	GetUserConversations(ctx context.Context, userID int64) ([]UserConversationInfo, error)
	SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error)
	SendMessageExt(ctx context.Context, opts SendMessageOptions) (*model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID, messageID int64) error
	DeleteLocalMessage(ctx context.Context, conversationID, userID, messageID int64) error
	EditMessage(ctx context.Context, messageID, editorID int64, content string) (*model.Message, error)
	RecallMessage(ctx context.Context, messageID, operatorID int64) error
	GetHistory(ctx context.Context, conversationID, userID int64, limit, beforeID int64) ([]model.Message, error)
	SearchMessages(ctx context.Context, userID int64, keyword string, limit int64) ([]model.Message, error)
	SearchMessagesInConversations(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.Message, error)
	SearchMessagesAdvanced(ctx context.Context, opts SearchMessagesOptions) ([]model.Message, error)
	GetConversationParticipants(ctx context.Context, conversationID int64) ([]int64, error)
	ApplyGroupEvent(ctx context.Context, envelope events.Envelope) error
}

const (
	// MessageStatusSent is the normal persisted state for a delivered message fact.
	MessageStatusSent = "sent"
	// MessageStatusRecalled means the message fact remains for ordering/audit,
	// but clients should render it as recalled rather than showing original text.
	MessageStatusRecalled = "recalled"
)

const defaultRecallWindow = 2 * time.Minute

// SendMessageOptions carries the full send-message contract, including fields
// that are optional in the simple SendMessage wrapper.
type SendMessageOptions struct {
	ConversationID int64
	SenderID       int64
	Content        string
	MsgType        string
	ReplyToID      int64
	MentionUserIDs []int64
	MentionAll     bool
}

// SearchMessagesOptions describes an advanced history search scoped by user,
// conversations, keyword and optional time range.
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
	repo         dao.MessageRepository
	redis        *redis.RedisClient
	groupClient  groupservice.Client
	recallWindow time.Duration
}

type messageEvent struct {
	eventType     string
	data          messageEventData
	targetUserIDs []int64
}

type messageEventData struct {
	Type           string
	ConversationID int64
	SenderID       int64
	Content        string
	MsgType        string
	MsgID          int64
	CreatedAt      string
	ReplyToID      int64
	Status         string
	IsEdited       bool
	EditedAt       string
	MentionUserIDs []int64
	MentionAll     bool
	UserID         int64
}

var errConversationAccessDenied = errors.New("无权访问该会话")

// NewMessageService connects the message domain with Redis, WebSocket push and
// group-service. If group-service discovery fails, private chat still works and
// group access falls back to local conversation_participants.
func NewMessageService(repo dao.MessageRepository, redisClient *redis.RedisClient, etcdEndpoints []string) MessageService {
	return NewMessageServiceWithPublisher(repo, redisClient, etcdEndpoints)
}

// NewMessageServiceWithPublisher creates the message service with optional
// group-service discovery. The historical name is kept for compatibility; event
// publishing now happens through the transactional outbox stored by the repo.
func NewMessageServiceWithPublisher(repo dao.MessageRepository, redisClient *redis.RedisClient, etcdEndpoints []string) MessageService {
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
	return &messageServiceImpl{repo: repo, redis: redisClient, groupClient: groupClient, recallWindow: defaultRecallWindow}
}

// CreateConversation creates or reuses a private/group conversation.
//
// Private conversations are de-duplicated by participant pair. Group
// conversations are de-duplicated by group_id and participants are synchronized
// from the latest group member snapshot so message fanout remains correct.
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

// GetConversation returns one conversation fact or an explicit not-found error.
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

// GetUserConversations builds the sidebar projection for one user.
//
// It joins conversation participants, latest message, unread count and optional
// group-service metadata. Empty results are cached as a null marker to reduce
// repeated penetration queries for new users.
func (s *messageServiceImpl) GetUserConversations(ctx context.Context, userID int64) ([]UserConversationInfo, error) {
	if s.redis != nil {
		cacheKey := fmt.Sprintf("user:conversations:%d", userID)
		var cached []UserConversationInfo
		hit, err := s.redis.GetJSON(ctx, cacheKey, &cached)
		if err == nil && hit != "" {
			if s.redis.IsNullHit(hit) {
				return nil, nil
			}
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
		info.UnreadCount = countUnreadMessages(ctx, s.repo, conv.ID, userID, p.LastReadMessageID)

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

	if s.redis != nil {
		cacheKey := fmt.Sprintf("user:conversations:%d", userID)
		if len(result) == 0 {
			s.redis.SetNull(ctx, cacheKey, time.Minute, 30*time.Second)
		} else {
			s.redis.SetJSONWithJitter(ctx, cacheKey, result, 5*time.Minute, 30*time.Second)
		}
	}
	return result, nil
}

// SendMessage sends a simple message without reply or mention metadata.
func (s *messageServiceImpl) SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error) {
	return s.SendMessageExt(ctx, SendMessageOptions{
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
		MsgType:        msgType,
	})
}

// SendMessageExt persists a message and its delivery event atomically.
//
// The transaction writes the message fact, updates conversation sorting time,
// creates per-user message states, and appends a message.created outbox record.
// Kafka/WebSocket publication happens later from the outbox worker, so a process
// crash after DB commit cannot lose the event.
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
	var participants []model.ConversationParticipant
	var msgEvent messageEvent
	if err := s.repo.WithTransaction(ctx, func(tx dao.MessageRepository) error {
		if err := tx.CreateMessage(ctx, msg); err != nil {
			return err
		}
		hydrateMessageRuntimeFields(msg)
		var err error
		participants, err = tx.GetParticipants(ctx, opts.ConversationID)
		if err != nil {
			return err
		}

		// messages 是服务端事实；conversation.updated_at 是会话列表排序依据；
		// message_user_states 是每个用户看到这条消息时的个人状态。三者共同完成
		// “消息已发送，但每个用户可以有不同本地视图”的 IM 语义。
		conv.UpdatedAt = msg.CreatedAt
		if err := tx.UpdateConversation(ctx, conv); err != nil {
			return err
		}

		if err := s.createMessageUserStatesWithRepo(ctx, tx, msg, participants); err != nil {
			return err
		}
		targetUserIDs := userIDsFromParticipants(participants)
		msgEvent = messageEvent{
			eventType: events.EventTypeMessageCreated,
			data: messageEventData{
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
			},
			targetUserIDs: targetUserIDs,
		}
		return s.saveMessageEvent(ctx, tx, msgEvent.eventType, msgEvent.data, msgEvent.targetUserIDs)
	}); err != nil {
		return nil, err
	}
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
		s.redis.SetJSONWithJitter(ctx, cacheKey, recentMsg, 10*time.Minute, time.Minute)
		for _, p := range participants {
			s.invalidateConversationCache(ctx, p.UserID)
		}
	}

	return msg, nil
}

// MarkConversationRead advances one user's read state through messageID.
//
// The method updates both the conversation-level read cursor and the per-message
// read_at rows, then appends a read-receipt outbox event for other participants.
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
	} else {
		msg, err := s.repo.GetMessageByID(ctx, messageID)
		if err != nil {
			return err
		}
		if msg == nil {
			return errors.New("消息不存在")
		}
		if msg.ConversationID != conversationID {
			return errors.New("已读消息不属于当前会话")
		}
	}
	readAt := time.Now()
	if err := s.repo.WithTransaction(ctx, func(tx dao.MessageRepository) error {
		if err := tx.UpdateParticipantReadCursor(ctx, conversationID, userID, messageID, readAt); err != nil {
			return err
		}
		if err := tx.MarkMessagesReadThrough(ctx, conversationID, userID, messageID, readAt); err != nil {
			return err
		}
		return s.saveReadReceipt(ctx, tx, conversationID, userID, messageID)
	}); err != nil {
		return err
	}
	s.invalidateConversationCache(ctx, userID)
	return nil
}

// DeleteLocalMessage 实现 IM 中常见的“只删除我本地聊天记录”语义。
//
// 这里不会删除 messages 表中的消息，也不会影响其他参与者的历史。它只在
// message_user_states 写入 local_deleted_at，之后 GetHistory 会按当前用户过滤该消息。
func (s *messageServiceImpl) DeleteLocalMessage(ctx context.Context, conversationID, userID, messageID int64) error {
	if messageID <= 0 {
		return errors.New("message_id不能为空")
	}
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
	msg, err := s.repo.GetMessageByID(ctx, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return errors.New("消息不存在")
	}
	if msg.ConversationID != conversationID {
		return errors.New("消息不属于当前会话")
	}
	if err := s.repo.MarkMessageLocalDeleted(ctx, conversationID, userID, messageID, time.Now()); err != nil {
		return err
	}
	s.invalidateConversationCache(ctx, userID)
	return nil
}

// EditMessage updates one sent message and records an edit audit row.
//
// Only the original sender may edit. The message fact is updated in the same
// transaction as the edit record and outbox event so history, audit and realtime
// notifications stay consistent.
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
	editedAt := time.Now()
	msg.Content = content
	msg.IsEdited = true
	msg.EditedAt = &editedAt
	if err := s.repo.WithTransaction(ctx, func(tx dao.MessageRepository) error {
		if err := tx.UpdateMessage(ctx, msg); err != nil {
			return err
		}
		if err := tx.CreateEditRecord(ctx, &model.MessageEditRecord{
			MessageID:      msg.ID,
			ConversationID: msg.ConversationID,
			EditorID:       editorID,
			OldContent:     oldContent,
			NewContent:     content,
		}); err != nil {
			return err
		}
		return s.saveMessageState(ctx, tx, msg, events.EventTypeMessageEdited, "message_edited")
	}); err != nil {
		return nil, err
	}
	s.invalidateConversationParticipantsCache(ctx, msg.ConversationID)
	hydrateMessageRuntimeFields(msg)
	return msg, nil
}

// RecallMessage hides the original content for all participants within the
// recall window. The row remains in messages so ordering and audit references do
// not collapse.
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
	if err := s.repo.WithTransaction(ctx, func(tx dao.MessageRepository) error {
		if err := tx.UpdateMessage(ctx, msg); err != nil {
			return err
		}
		return s.saveMessageState(ctx, tx, msg, events.EventTypeMessageRecalled, "message_recalled")
	}); err != nil {
		return err
	}
	s.invalidateConversationParticipantsCache(ctx, msg.ConversationID)
	return nil
}

// GetHistory returns one page of messages visible to userID.
//
// It enforces membership, filters per-user local deletions and hydrates runtime
// fields such as mention IDs and read receipt counts before returning messages
// in chronological order for the frontend.
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

	messages, err := s.repo.GetMessagesForUser(ctx, conversationID, userID, limit, beforeID)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		hydrateMessageRuntimeFields(&messages[i])
	}
	if err := s.hydrateReadReceiptFields(ctx, conversationID, userID, messages); err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// SearchMessages searches across all conversations the user participates in.
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

// SearchMessagesInConversations searches an explicit conversation set.
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

// SearchMessagesAdvanced is the shared search implementation with optional
// user authorization and time range filtering.
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

// GetConversationParticipants returns the current participant user IDs for a
// conversation. API gateway uses this to validate peer users before sending.
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

func (s *messageServiceImpl) hydrateReadReceiptFields(ctx context.Context, conversationID, viewerID int64, messages []model.Message) error {
	if len(messages) == 0 {
		return nil
	}
	messageIDs := make([]int64, 0, len(messages))
	for _, msg := range messages {
		messageIDs = append(messageIDs, msg.ID)
	}
	stats, err := s.repo.GetMessageReadStats(ctx, conversationID, messageIDs, viewerID)
	if err != nil {
		return err
	}
	for i := range messages {
		stat := stats[messages[i].ID]
		messages[i].ReadCount = stat.ReadCount
		messages[i].RecipientCount = stat.RecipientCount
		messages[i].IsReadByMe = stat.IsReadByMe
	}
	return nil
}

func (s *messageServiceImpl) invalidateConversationCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("user:conversations:%d", userID))
}

func (s *messageServiceImpl) createMessageUserStates(ctx context.Context, msg *model.Message, participants []model.ConversationParticipant) error {
	return s.createMessageUserStatesWithRepo(ctx, s.repo, msg, participants)
}

func (s *messageServiceImpl) createMessageUserStatesWithRepo(ctx context.Context, repo dao.MessageRepository, msg *model.Message, participants []model.ConversationParticipant) error {
	if msg == nil {
		return nil
	}
	deliveredAt := msg.CreatedAt
	readAt := msg.CreatedAt
	for _, p := range participants {
		state := &model.MessageUserState{
			ConversationID: msg.ConversationID,
			MessageID:      msg.ID,
			UserID:         p.UserID,
			DeliveredAt:    &deliveredAt,
		}
		// 发送者自己的客户端已经完成发送动作，因此服务端视角下可直接视为已投递且已读。
		// 接收者保持未读，直到显式调用 MarkConversationRead 更新游标。
		if p.UserID == msg.SenderID {
			state.ReadAt = &readAt
		}
		if err := repo.UpsertMessageUserState(ctx, state); err != nil {
			return err
		}
	}
	return nil
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

func (s *messageServiceImpl) saveMessageState(ctx context.Context, repo dao.MessageRepository, msg *model.Message, eventType, pushType string) error {
	if msg == nil {
		return nil
	}
	participants, err := repo.GetParticipants(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	return s.saveMessageEvent(ctx, repo, eventType, messageEventData{
		Type:           pushType,
		ConversationID: msg.ConversationID,
		SenderID:       msg.SenderID,
		Content:        msg.Content,
		MsgType:        msg.MsgType,
		MsgID:          msg.ID,
		CreatedAt:      msg.CreatedAt.Format("2006-01-02 15:04:05"),
		ReplyToID:      msg.ReplyToID,
		Status:         msg.Status,
		IsEdited:       msg.IsEdited,
		EditedAt:       formatOptionalTime(msg.EditedAt),
		MentionUserIDs: msg.MentionUserIDs,
		MentionAll:     msg.MentionAll,
	}, userIDsFromParticipants(participants))
}

func (s *messageServiceImpl) saveReadReceipt(ctx context.Context, repo dao.MessageRepository, conversationID, readerID, messageID int64) error {
	participants, err := repo.GetParticipants(ctx, conversationID)
	if err != nil {
		return err
	}
	targetUserIDs := make([]int64, 0, len(participants))
	for _, p := range participants {
		if p.UserID != readerID {
			targetUserIDs = append(targetUserIDs, p.UserID)
		}
	}
	if len(targetUserIDs) == 0 {
		return nil
	}
	return s.saveMessageEvent(ctx, repo, events.EventTypeMessageRead, messageEventData{
		Type:           "message_read",
		ConversationID: conversationID,
		MsgID:          messageID,
		UserID:         readerID,
	}, targetUserIDs)
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

func countUnreadMessages(ctx context.Context, repo dao.MessageRepository, conversationID, userID, lastReadMessageID int64) int64 {
	count, err := repo.CountUnreadMessages(ctx, conversationID, userID, lastReadMessageID)
	if err != nil {
		return 0
	}
	return count
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// ApplyGroupEvent applies group-service membership events to the message domain.
//
// group-service owns group_members, while msg-core-service owns
// conversation_participants used for message fanout. This consumer bridges those
// domains asynchronously from Kafka/Outbox events.
func (s *messageServiceImpl) ApplyGroupEvent(ctx context.Context, envelope events.Envelope) error {
	switch envelope.Type {
	case events.EventTypeGroupCreated:
		payload, err := events.DecodePayload[events.GroupCreatedPayload](envelope)
		if err != nil {
			return err
		}
		_, err = s.CreateConversation(ctx, "group", payload.MemberIDs, payload.GroupID)
		return err
	case events.EventTypeGroupMemberInvited:
		payload, err := events.DecodePayload[events.GroupMemberInvitedPayload](envelope)
		if err != nil {
			return err
		}
		_, err = s.CreateConversation(ctx, "group", payload.MemberIDs, payload.GroupID)
		return err
	case events.EventTypeGroupMemberKicked:
		payload, err := events.DecodePayload[events.GroupMemberKickedPayload](envelope)
		if err != nil {
			return err
		}
		conv, err := s.repo.FindGroupConversation(ctx, payload.GroupID)
		if err != nil || conv == nil {
			return err
		}
		if err := s.repo.RemoveParticipant(ctx, conv.ID, payload.UserID); err != nil {
			return err
		}
		s.invalidateConversationCache(ctx, payload.UserID)
		return nil
	case events.EventTypeGroupDeleted:
		payload, err := events.DecodePayload[events.GroupDeletedPayload](envelope)
		if err != nil {
			return err
		}
		for _, userID := range payload.MemberIDs {
			s.invalidateConversationCache(ctx, userID)
		}
		return nil
	default:
		return nil
	}
}

func (s *messageServiceImpl) saveMessageEvent(ctx context.Context, repo dao.MessageRepository, eventType string, data messageEventData, targetUserIDs []int64) error {
	payload := events.MessagePayload{
		Type:           data.Type,
		ConversationID: data.ConversationID,
		SenderID:       data.SenderID,
		Content:        data.Content,
		MsgType:        data.MsgType,
		MsgID:          data.MsgID,
		CreatedAt:      data.CreatedAt,
		ReplyToID:      data.ReplyToID,
		Status:         data.Status,
		IsEdited:       data.IsEdited,
		EditedAt:       data.EditedAt,
		MentionUserIDs: data.MentionUserIDs,
		MentionAll:     data.MentionAll,
		UserID:         data.UserID,
		TargetUserIDs:  dedupeUserIDs(targetUserIDs),
	}
	envelope, err := events.NewEnvelope(eventType, strconv.FormatInt(data.ConversationID, 10), payload)
	if err != nil {
		return err
	}
	record, err := outbox.NewEvent("message", data.MsgID, envelope)
	if err != nil {
		return err
	}
	return repo.SaveOutboxEvent(ctx, record)
}

func userIDsFromParticipants(participants []model.ConversationParticipant) []int64 {
	userIDs := make([]int64, 0, len(participants))
	for _, p := range participants {
		userIDs = append(userIDs, p.UserID)
	}
	return dedupeUserIDs(userIDs)
}
