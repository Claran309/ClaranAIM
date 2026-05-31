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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/transport"
	etcd "github.com/kitex-contrib/registry-etcd"
)

// MessageService 定义 msg-core-service 拥有的消息领域操作。
// 该服务拥有会话、消息事实、用户级消息状态、已读游标和事务 Outbox 记录；
// 这些 Outbox 记录最终会发布成 Kafka/WebSocket 事件。api-gateway 和其他服务应通过接口调用，
// 不应直接访问消息表。
type MessageService interface {
	CreateConversation(ctx context.Context, convType string, participantIDs []int64, groupID int64) (*model.Conversation, error)
	CreateGroupConversationWithID(ctx context.Context, conversationID, groupID int64, participantIDs []int64) (*model.Conversation, error)
	CompensateGroupConversation(ctx context.Context, groupID, conversationID int64) error
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
	TranslateMessage(ctx context.Context, input TranslateMessageInput) (TranslateMessageResult, error)
	GetConversationParticipants(ctx context.Context, conversationID int64) ([]int64, error)
	ApplyGroupEvent(ctx context.Context, envelope events.Envelope) error
}

// 消息状态会写入 messages.status 并被前端直接展示，新增状态需要同步客户端渲染逻辑。
const (
	// MessageStatusSent 表示消息事实已经正常持久化并可投递。
	MessageStatusSent = "sent"
	// MessageStatusRecalled 表示消息事实仍保留用于排序和审计，但客户端应展示为已撤回。
	MessageStatusRecalled = "recalled"
)

// 默认撤回窗口限制普通用户只能撤回短时间内发送的消息，后续管理员策略可在这里扩展。
const defaultRecallWindow = 2 * time.Minute

// SendMessageOptions 表示完整发消息契约。
// 简化版 SendMessage 会填充其中的基础字段，引用、@、幂等键等高级能力通过该结构传入。
type SendMessageOptions struct {
	ConversationID int64
	SenderID       int64
	Content        string
	MsgType        string
	ReplyToID      int64
	MentionUserIDs []int64
	MentionAll     bool
	ClientMsgID    string
}

// SearchMessagesOptions 描述高级历史搜索条件：用户、会话范围、关键词和可选时间区间。
type SearchMessagesOptions struct {
	UserID          int64
	ConversationIDs []int64
	Keyword         string
	Limit           int64
	StartAt         time.Time
	EndAt           time.Time
}

// UserConversationInfo 是前端侧边栏可直接使用的会话投影。
// 它由会话、参与者、最新消息和未读数聚合而来，避免浏览器对每个会话再发多次请求。
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

// messageServiceImpl 是 MessageService 的默认实现，组合消息仓储、Redis、群服务客户端和翻译依赖。
type messageServiceImpl struct {
	repo                dao.MessageRepository
	redis               *redis.RedisClient
	groupClient         groupservice.Client
	recallWindow        time.Duration
	translationSettings TranslationSettings
	translationLLM      TranslationLLM
}

// NewMessageServiceForTest 创建不依赖外部 RPC 客户端的消息服务，供单元测试使用。
func NewMessageServiceForTest(repo dao.MessageRepository) *messageServiceImpl {
	return &messageServiceImpl{repo: repo, recallWindow: defaultRecallWindow}
}

// messageEvent 表示一次待写入 Outbox 的消息领域事件。
type messageEvent struct {
	eventType     string
	data          messageEventData
	targetUserIDs []int64
}

// messageEventData 是消息事件的业务载荷，会被转换为 pkg/events 的统一 payload。
type messageEventData struct {
	Type             string
	ConversationID   int64
	ConversationType string
	SenderID         int64
	Content          string
	MsgType          string
	MsgID            int64
	CreatedAt        string
	ReplyToID        int64
	Status           string
	IsEdited         bool
	EditedAt         string
	MentionUserIDs   []int64
	MentionAll       bool
	UserID           int64
}

// mediaMessagePayload 表示聊天消息中嵌入的媒体引用信息。
type mediaMessagePayload struct {
	ID          string `json:"id"`
	FileID      int64  `json:"file_id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// errConversationAccessDenied 是会话可见性校验的统一错误，便于 handler 转成稳定的权限提示。
var errConversationAccessDenied = errors.New("无权访问该会话")

// NewMessageService 连接消息领域、Redis 和 group-service。
// 如果 group-service 发现失败，私聊仍可工作，群聊访问会退回本地 conversation_participants 快照。
func NewMessageService(repo dao.MessageRepository, redisClient *redis.RedisClient, etcdEndpoints []string) MessageService {
	return NewMessageServiceWithPublisher(repo, redisClient, etcdEndpoints)
}

// NewMessageServiceWithPublisher 创建带可选 group-service 发现的消息服务。
// 函数名保留历史兼容；当前事件发布已经统一通过仓储写事务 Outbox 完成。
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

// CreateConversation 创建或复用私聊/群聊会话。
// 私聊按参与者二元组去重；群聊按 group_id 去重，并从最新群成员快照同步参与者，
// 保证消息扇出目标正确。
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
			// 必须把新邀请成员同步进消息领域。
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

// CreateGroupConversationWithID 使用调用方预分配的 ID 创建群会话。
// 这是创建群 DTM Saga 中 msg-core-service 的正向分支；网关会在提交 Saga 前预分配
// group_id 和 conversation_id，使 DTM 可以独立调用 HTTP 分支而不依赖前一分支返回值。
func (s *messageServiceImpl) CreateGroupConversationWithID(ctx context.Context, conversationID, groupID int64, participantIDs []int64) (*model.Conversation, error) {
	if conversationID <= 0 {
		return nil, errors.New("conversation_id不能为空")
	}
	if groupID <= 0 {
		return nil, errors.New("group_id不能为空")
	}
	participantIDs = dedupeUserIDs(participantIDs)
	if len(participantIDs) < 2 {
		return nil, errors.New("参与者至少为2人")
	}

	existing, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Type != "group" || existing.GroupID != groupID {
			return nil, errors.New("conversation_id已被其他会话占用")
		}
		if err := s.syncConversationParticipants(ctx, existing.ID, participantIDs); err != nil {
			return nil, err
		}
		return existing, nil
	}

	groupExisting, err := s.repo.FindGroupConversation(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if groupExisting != nil {
		if groupExisting.ID != conversationID {
			return nil, errors.New("group_id已绑定其他会话")
		}
		if err := s.syncConversationParticipants(ctx, groupExisting.ID, participantIDs); err != nil {
			return nil, err
		}
		return groupExisting, nil
	}

	conv := &model.Conversation{
		ID:      conversationID,
		Type:    "group",
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

// CompensateGroupConversation 删除由 DTM Saga 创建的群会话。
// 该方法具备幂等性：DTM 可能重复补偿，已删除后再次补偿仍应视为成功。
func (s *messageServiceImpl) CompensateGroupConversation(ctx context.Context, groupID, conversationID int64) error {
	if conversationID <= 0 || groupID <= 0 {
		return nil
	}
	conv, err := s.repo.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv == nil {
		return nil
	}
	if conv.Type != "group" || conv.GroupID != groupID {
		return errors.New("补偿目标会话与群组不匹配")
	}
	return s.repo.DeleteConversation(ctx, conversationID)
}

// GetConversation 返回一个会话事实；不存在时返回明确错误。
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

// GetUserConversations 构建某个用户的侧边栏会话投影。
// 它会聚合会话参与者、最新消息、未读数和可选群资料；空结果会缓存为空值标记，
// 降低新用户反复查询造成的缓存穿透。
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

// SendMessage 发送不带引用和 @ 元数据的普通消息。
func (s *messageServiceImpl) SendMessage(ctx context.Context, conversationID, senderID int64, content, msgType string) (*model.Message, error) {
	return s.SendMessageExt(ctx, SendMessageOptions{
		ConversationID: conversationID,
		SenderID:       senderID,
		Content:        content,
		MsgType:        msgType,
	})
}

// SendMessageExt 以原子方式持久化消息和投递事件。
// 事务内会写入消息事实、更新会话排序时间、创建用户级消息状态，并追加 message.created Outbox 事件。
// Kafka/WebSocket 发布稍后由 Outbox Worker 完成，因此进程在 DB 提交后崩溃也不会丢事件。
func (s *messageServiceImpl) SendMessageExt(ctx context.Context, opts SendMessageOptions) (*model.Message, error) {
	content := opts.Content
	if content == "" {
		return nil, errors.New("消息内容不能为空")
	}
	clientMsgID := strings.TrimSpace(opts.ClientMsgID)
	if clientMsgID != "" {
		existing, err := s.repo.GetMessageByClientMsgID(ctx, clientMsgID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			if existing.ConversationID != opts.ConversationID || existing.SenderID != opts.SenderID {
				return nil, errors.New("client_msg_id已被其他消息使用")
			}
			hydrateMessageRuntimeFields(existing)
			return existing, nil
		}
	}

	conv, err := s.repo.GetConversationByID(ctx, opts.ConversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("会话不存在")
	}
	// 每次写消息都经过与历史读取相同的参与者校验，用户不能靠猜测 conversation_id 向无权会话发消息。
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
	if clientMsgID != "" {
		msg.ClientMsgID = &clientMsgID
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
				Type:             "new_message",
				ConversationID:   opts.ConversationID,
				ConversationType: conv.Type,
				SenderID:         opts.SenderID,
				Content:          content,
				MsgType:          msgType,
				MsgID:            msg.ID,
				CreatedAt:        msg.CreatedAt.Format("2006-01-02 15:04:05"),
				ReplyToID:        msg.ReplyToID,
				Status:           msg.Status,
				MentionUserIDs:   msg.MentionUserIDs,
				MentionAll:       msg.MentionAll,
			},
			targetUserIDs: targetUserIDs,
		}
		if err := s.saveMessageEvent(ctx, tx, msgEvent.eventType, msgEvent.data, msgEvent.targetUserIDs); err != nil {
			return err
		}
		return s.saveAgentNativeIMEvent(ctx, tx, conv, msg, targetUserIDs)
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

// MarkConversationRead 将某个用户的已读状态推进到 messageID。
// 该方法同时更新会话级已读游标和消息级已读状态。
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
		if err := s.saveReadReceipt(ctx, tx, conversationID, userID, messageID); err != nil {
			return err
		}
		return s.saveAgentNativeReadEvent(ctx, tx, conv, userID, messageID, readAt)
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

// EditMessage 更新一条已发送消息，并记录编辑审计行。
// 只有原发送者可以编辑；消息事实、编辑记录和 Outbox 事件在同一事务中更新，保证历史、审计和实时通知一致。
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
		if err := s.saveMessageState(ctx, tx, msg, events.EventTypeMessageEdited, "message_edited"); err != nil {
			return err
		}
		return s.saveAgentNativeMessageStateEvent(ctx, tx, conv, msg, events.EventTypeMessageEdited, editorID)
	}); err != nil {
		return nil, err
	}
	s.invalidateConversationParticipantsCache(ctx, msg.ConversationID)
	hydrateMessageRuntimeFields(msg)
	return msg, nil
}

// RecallMessage 在可撤回窗口内对所有参与者隐藏原始内容。
// 消息行仍保留在 messages 表中，确保排序和审计引用不塌陷。
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
		if err := s.saveMessageState(ctx, tx, msg, events.EventTypeMessageRecalled, "message_recalled"); err != nil {
			return err
		}
		return s.saveAgentNativeMessageStateEvent(ctx, tx, conv, msg, events.EventTypeMessageRecalled, operatorID)
	}); err != nil {
		return err
	}
	s.invalidateConversationParticipantsCache(ctx, msg.ConversationID)
	return nil
}

// GetHistory 返回 userID 可见的一页历史消息。
// 它会校验成员身份、过滤用户本地删除状态，并补充 @ 和已读回执等运行时字段，
// 最后按时间正序返回给前端。
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
	// 历史消息包含私密数据，调用方必须先是会话参与者；未通过校验时服务不会返回任何消息页。
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

// SearchMessages 在用户参与的全部会话中搜索消息。
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

// SearchMessagesInConversations 在显式指定的会话集合中搜索消息。
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

// SearchMessagesAdvanced 是共享的高级搜索实现，支持可选用户鉴权和时间范围过滤。
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

// GetConversationParticipants 返回当前会话参与者用户 ID。
// api-gateway 会在发送消息前用它校验对端用户。
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

// hydrateReadReceiptFields 为历史消息补齐“我是否已读”和“已读/应读人数”。
// 这些字段不直接存在 messages 表中，需要从用户级消息状态表按批量 message_id 统计。
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

// invalidateConversationCache 删除某个用户的会话列表缓存。
// 任何影响侧边栏排序、最后一条消息或未读数的写操作后都应调用它。
func (s *messageServiceImpl) invalidateConversationCache(ctx context.Context, userID int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, fmt.Sprintf("user:conversations:%d", userID))
}

// createMessageUserStates 为消息参与者初始化投递/已读状态。
// 默认走当前 service 的仓储，事务场景使用 createMessageUserStatesWithRepo 注入事务 repo。
func (s *messageServiceImpl) createMessageUserStates(ctx context.Context, msg *model.Message, participants []model.ConversationParticipant) error {
	return s.createMessageUserStatesWithRepo(ctx, s.repo, msg, participants)
}

// createMessageUserStatesWithRepo 在指定仓储上写入用户级消息状态。
// 发送者立即标记已读，接收者只标记已投递，未读数由 ReadAt 是否为空决定。
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

// invalidateConversationParticipantsCache 刷新会话内所有参与者的会话列表缓存。
// 群成员同步、撤回、编辑和新消息都会改变不同用户看到的会话摘要。
func (s *messageServiceImpl) invalidateConversationParticipantsCache(ctx context.Context, conversationID int64) {
	participants, err := s.repo.GetParticipants(ctx, conversationID)
	if err != nil {
		return
	}
	for _, p := range participants {
		s.invalidateConversationCache(ctx, p.UserID)
	}
}

// saveMessageState 把消息编辑/撤回等状态变化写入 Outbox。
// WebSocket 网关消费该事件后更新在线客户端，同时保留 Kafka 重试能力。
func (s *messageServiceImpl) saveMessageState(ctx context.Context, repo dao.MessageRepository, msg *model.Message, eventType, pushType string) error {
	if msg == nil {
		return nil
	}
	participants, err := repo.GetParticipants(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	conv, err := repo.GetConversationByID(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	conversationType := ""
	if conv != nil {
		conversationType = conv.Type
	}
	return s.saveMessageEvent(ctx, repo, eventType, messageEventData{
		Type:             pushType,
		ConversationID:   msg.ConversationID,
		ConversationType: conversationType,
		SenderID:         msg.SenderID,
		Content:          msg.Content,
		MsgType:          msg.MsgType,
		MsgID:            msg.ID,
		CreatedAt:        msg.CreatedAt.Format("2006-01-02 15:04:05"),
		ReplyToID:        msg.ReplyToID,
		Status:           msg.Status,
		IsEdited:         msg.IsEdited,
		EditedAt:         formatOptionalTime(msg.EditedAt),
		MentionUserIDs:   msg.MentionUserIDs,
		MentionAll:       msg.MentionAll,
	}, userIDsFromParticipants(participants))
}

// saveReadReceipt 把某个用户的已读动作广播给同会话其他参与者。
// 读者本人不需要收到自己的 read 事件，因此 targetUserIDs 会排除 readerID。
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

// fetchGroupMembers 从 group-service 读取群成员快照。
// msg-core-service 把 group_members 当作群聊权限源，而不是只相信 conversation_participants 的旧数据。
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

// ensureConversationParticipant 校验用户是否有权访问会话。
// 群聊优先同步 group-service 成员表，保证新入群成员能读取会话、被踢成员立即失去权限。
func (s *messageServiceImpl) ensureConversationParticipant(ctx context.Context, conv *model.Conversation, userID int64) error {
	if conv.Type == "group" && conv.GroupID > 0 && s.groupClient != nil {
		membersResp, err := s.fetchGroupMembers(ctx, conv.GroupID)
		if err == nil && membersResp != nil && membersResp.Success {
			// group_members is the source of truth for group membership. Syncing
			// 在这里同步可让旧会话无需迁移也能感知新成员。
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

// checkGroupMute 在发送群消息前检查禁言状态。
// 如果 group-service 暂不可用，本地开发阶段选择放行，避免消息核心链路因依赖抖动完全不可用。
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

// syncConversationParticipants 把 group-service 成员快照补写到 conversation_participants。
// 它只追加缺失成员，不删除旧成员；踢人权限仍由 ensureConversationParticipant 的实时群成员校验兜底。
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
		// 新同步的参与者需要刷新侧边栏会话列表。
		s.invalidateConversationCache(ctx, userID)
	}
	return nil
}

// dedupeUserIDs 过滤非正 ID 并保持首次出现顺序，用于参与者和推送目标去重。
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

// joinInt64 把 ID 列表序列化为逗号分隔字符串，用于事件幂等键和审计元数据。
func joinInt64(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

// marshalMentionUserIDs 将 @ 用户 ID 列表保存为 JSON 字符串。
// 空列表存为空字符串，避免数据库里到处出现无意义的 []。
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

// hydrateMessageRuntimeFields 从持久化 JSON 字段恢复运行时字段。
// 目前主要恢复 MentionUserIDs，方便 handler、事件和前端统一使用切片。
func hydrateMessageRuntimeFields(msg *model.Message) {
	if msg == nil || msg.MentionsJSON == "" {
		return
	}
	var ids []int64
	if err := json.Unmarshal([]byte(msg.MentionsJSON), &ids); err == nil {
		msg.MentionUserIDs = dedupeUserIDs(ids)
	}
}

// countUnreadMessages 查询失败时返回 0，避免会话列表因为未读统计失败整体不可用。
func countUnreadMessages(ctx context.Context, repo dao.MessageRepository, conversationID, userID, lastReadMessageID int64) int64 {
	count, err := repo.CountUnreadMessages(ctx, conversationID, userID, lastReadMessageID)
	if err != nil {
		return 0
	}
	return count
}

// formatOptionalTime 将可空时间转为前端/RPC 使用的本地时间字符串。
func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// formatTime 将非指针时间转成统一展示格式，零值返回空字符串。
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// ApplyGroupEvent 将 group-service 成员事件应用到消息领域。
//
// group-service owns group_members, while msg-core-service owns
// conversation_participants used for message fanout. This consumer bridges those
// 这通过 Kafka/Outbox 事件异步桥接群领域和消息领域。
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
		conv, err := s.CreateConversation(ctx, "group", payload.MemberIDs, payload.GroupID)
		if err != nil {
			return err
		}
		return s.saveAgentNativeGroupMemberEvent(ctx, conv, payload.OperatorID, events.EventTypeGroupMemberJoined, payload.UserIDs, payload.MemberIDs)
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
		if err := s.saveAgentNativeGroupMemberEvent(ctx, conv, payload.OperatorID, events.EventTypeGroupMemberLeft, []int64{payload.UserID}, payload.MemberIDs); err != nil {
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

// saveMessageEvent 把传统消息推送事件写入 Outbox。
// 业务数据库提交和 Kafka 发布之间通过 Outbox 解耦，避免“消息已落库但事件丢失”。
func (s *messageServiceImpl) saveMessageEvent(ctx context.Context, repo dao.MessageRepository, eventType string, data messageEventData, targetUserIDs []int64) error {
	payload := events.MessagePayload{
		Type:             data.Type,
		ConversationID:   data.ConversationID,
		ConversationType: data.ConversationType,
		SenderID:         data.SenderID,
		Content:          data.Content,
		MsgType:          data.MsgType,
		MsgID:            data.MsgID,
		CreatedAt:        data.CreatedAt,
		ReplyToID:        data.ReplyToID,
		Status:           data.Status,
		IsEdited:         data.IsEdited,
		EditedAt:         data.EditedAt,
		MentionUserIDs:   data.MentionUserIDs,
		MentionAll:       data.MentionAll,
		UserID:           data.UserID,
		TargetUserIDs:    dedupeUserIDs(targetUserIDs),
		ParticipantIDs:   dedupeUserIDs(targetUserIDs),
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

// saveAgentNativeGroupMemberEvent 把群成员变更转成统一 IM 事件。
// Agent dispatcher 会用这些事件决定是否静默记录群成员变化或触发群助手。
func (s *messageServiceImpl) saveAgentNativeGroupMemberEvent(ctx context.Context, conv *model.Conversation, operatorID int64, eventType string, userIDs []int64, participantIDs []int64) error {
	if conv == nil || conv.ID <= 0 || len(userIDs) == 0 {
		return nil
	}
	userIDs = dedupeUserIDs(userIDs)
	participantIDs = dedupeUserIDs(participantIDs)
	payload := events.IMEventPayload{
		EventType:        eventType,
		ConversationID:   conv.ID,
		ConversationType: conv.Type,
		SenderID:         operatorID,
		ParticipantIDs:   participantIDs,
		Permission: events.PermissionContext{
			Scope:          conv.Type,
			VisibleUserIDs: participantIDs,
			GroupRole:      "admin",
			CanReadFiles:   true,
			CanWrite:       true,
		},
		OccurredAt:     time.Now().Format(time.RFC3339Nano),
		IdempotencyKey: fmt.Sprintf("%s:%d:%s", eventType, conv.ID, joinInt64(userIDs)),
		Metadata: map[string]string{
			"group_id":    strconv.FormatInt(conv.GroupID, 10),
			"operator_id": strconv.FormatInt(operatorID, 10),
			"user_id":     strconv.FormatInt(userIDs[0], 10),
			"user_ids":    joinInt64(userIDs),
		},
	}
	envelope, err := events.NewEnvelope(eventType, strconv.FormatInt(conv.ID, 10), payload)
	if err != nil {
		return err
	}
	record, err := outbox.NewEvent("im", conv.ID, envelope)
	if err != nil {
		return err
	}
	return s.repo.SaveOutboxEvent(ctx, record)
}

// saveAgentNativeIMEvent 为文件、图片、语音等消息生成 Agent-Native IM 事件。
// 普通文本走 message.created；带附件的消息还需要附件引用，供 Agent 做文件入口处理。
func (s *messageServiceImpl) saveAgentNativeIMEvent(ctx context.Context, repo dao.MessageRepository, conv *model.Conversation, msg *model.Message, participantIDs []int64) error {
	if conv == nil || msg == nil {
		return nil
	}
	eventType, attachment, ok := agentNativeEventFromMessage(msg)
	if !ok {
		return nil
	}
	participantIDs = dedupeUserIDs(participantIDs)
	payload := events.IMEventPayload{
		EventType:        eventType,
		ConversationID:   msg.ConversationID,
		ConversationType: conv.Type,
		SenderID:         msg.SenderID,
		Content:          msg.Content,
		MsgType:          msg.MsgType,
		MsgID:            msg.ID,
		ReplyToID:        msg.ReplyToID,
		ParticipantIDs:   participantIDs,
		MentionUserIDs:   msg.MentionUserIDs,
		MentionAll:       msg.MentionAll,
		AttachmentRefs:   []events.AttachmentRef{attachment},
		Permission: events.PermissionContext{
			Scope:          conv.Type,
			VisibleUserIDs: participantIDs,
			CanReadFiles:   true,
			CanWrite:       true,
		},
		OccurredAt:     msg.CreatedAt.Format(time.RFC3339Nano),
		IdempotencyKey: fmt.Sprintf("%s:%d", eventType, msg.ID),
	}
	envelope, err := events.NewEnvelope(eventType, strconv.FormatInt(msg.ConversationID, 10), payload)
	if err != nil {
		return err
	}
	record, err := outbox.NewEvent("im", msg.ID, envelope)
	if err != nil {
		return err
	}
	return repo.SaveOutboxEvent(ctx, record)
}

// saveAgentNativeMessageStateEvent 把编辑、撤回等消息状态变化转换为统一 IM 事件。
// 这让 Agent 能感知上下文被修改，而不只处理新消息。
func (s *messageServiceImpl) saveAgentNativeMessageStateEvent(ctx context.Context, repo dao.MessageRepository, conv *model.Conversation, msg *model.Message, businessEventType string, actorID int64) error {
	if conv == nil || msg == nil {
		return nil
	}
	participants, err := repo.GetParticipants(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	participantIDs := userIDsFromParticipants(participants)
	envelopeType := imEnvelopeTypeForMessageEvent(businessEventType)
	if envelopeType == "" {
		return nil
	}
	payload := events.IMEventPayload{
		EventType:        businessEventType,
		ConversationID:   msg.ConversationID,
		ConversationType: conv.Type,
		SenderID:         actorID,
		Content:          msg.Content,
		MsgType:          msg.MsgType,
		MsgID:            msg.ID,
		ReplyToID:        msg.ReplyToID,
		ParticipantIDs:   participantIDs,
		MentionUserIDs:   msg.MentionUserIDs,
		MentionAll:       msg.MentionAll,
		Permission: events.PermissionContext{
			Scope:          conv.Type,
			VisibleUserIDs: participantIDs,
			CanReadFiles:   true,
			CanWrite:       true,
		},
		OccurredAt:     time.Now().Format(time.RFC3339Nano),
		IdempotencyKey: fmt.Sprintf("%s:%d", businessEventType, msg.ID),
		Metadata: map[string]string{
			"message_status": msg.Status,
		},
	}
	envelope, err := events.NewEnvelope(envelopeType, strconv.FormatInt(msg.ConversationID, 10), payload)
	if err != nil {
		return err
	}
	record, err := outbox.NewEvent("im", msg.ID, envelope)
	if err != nil {
		return err
	}
	return repo.SaveOutboxEvent(ctx, record)
}

// saveAgentNativeReadEvent 把已读游标变化写成统一 IM 事件。
// 目前主要用于审计和未来 Agent 在线状态/未读摘要判断。
func (s *messageServiceImpl) saveAgentNativeReadEvent(ctx context.Context, repo dao.MessageRepository, conv *model.Conversation, readerID, messageID int64, readAt time.Time) error {
	if conv == nil {
		return nil
	}
	participants, err := repo.GetParticipants(ctx, conv.ID)
	if err != nil {
		return err
	}
	participantIDs := userIDsFromParticipants(participants)
	payload := events.IMEventPayload{
		EventType:        events.EventTypeMessageRead,
		ConversationID:   conv.ID,
		ConversationType: conv.Type,
		SenderID:         readerID,
		MsgID:            messageID,
		ParticipantIDs:   participantIDs,
		Permission: events.PermissionContext{
			Scope:          conv.Type,
			VisibleUserIDs: participantIDs,
			CanReadFiles:   true,
			CanWrite:       true,
		},
		OccurredAt:     readAt.Format(time.RFC3339Nano),
		IdempotencyKey: fmt.Sprintf("%s:%d:%d", events.EventTypeMessageRead, readerID, messageID),
	}
	envelope, err := events.NewEnvelope(events.EventTypeIMMessageRead, strconv.FormatInt(conv.ID, 10), payload)
	if err != nil {
		return err
	}
	record, err := outbox.NewEvent("im", messageID, envelope)
	if err != nil {
		return err
	}
	return repo.SaveOutboxEvent(ctx, record)
}

// imEnvelopeTypeForMessageEvent 将传统消息事件名映射到 Agent-Native IM 事件名。
// 返回空字符串表示该事件暂不进入 Agent 原生流水线。
func imEnvelopeTypeForMessageEvent(eventType string) string {
	switch eventType {
	case events.EventTypeMessageEdited:
		return events.EventTypeIMMessageEdited
	case events.EventTypeMessageRecalled:
		return events.EventTypeIMMessageRecalled
	case events.EventTypeMessageRead:
		return events.EventTypeIMMessageRead
	default:
		return ""
	}
}

// agentNativeEventFromMessage 判断一条消息是否需要额外生成附件类 IM 事件。
// 文本消息不在这里处理，避免与 message.created 重复触发 Agent。
func agentNativeEventFromMessage(msg *model.Message) (string, events.AttachmentRef, bool) {
	if msg == nil {
		return "", events.AttachmentRef{}, false
	}
	switch msg.MsgType {
	case "file", "image":
		attachment := attachmentRefFromMessageContent(msg.Content)
		return events.EventTypeFileUploaded, attachment, true
	case "voice":
		attachment := attachmentRefFromMessageContent(msg.Content)
		return events.EventTypeVoiceTranscribed, attachment, true
	default:
		return "", events.AttachmentRef{}, false
	}
}

// attachmentRefFromMessageContent 从消息内容中提取附件引用。
// 兼容旧格式前缀（[file]/[img]/[voice]）和新的 JSON media payload。
func attachmentRefFromMessageContent(content string) events.AttachmentRef {
	trimmed := strings.TrimSpace(content)
	for _, prefix := range []string{"[file]", "[img]", "[voice]"} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			break
		}
	}
	var media mediaMessagePayload
	if strings.HasPrefix(trimmed, "{") {
		_ = json.Unmarshal([]byte(trimmed), &media)
	}
	fileID := media.FileID
	if fileID == 0 && media.ID != "" {
		if parsed, err := strconv.ParseInt(media.ID, 10, 64); err == nil {
			fileID = parsed
		}
	}
	name := media.Name
	if name == "" && media.URL != "" {
		name = filepath.Base(media.URL)
	}
	if name == "." || name == "/" || name == "\\" {
		name = ""
	}
	return events.AttachmentRef{
		FileID:      fileID,
		Name:        name,
		ContentType: media.ContentType,
		URL:         media.URL,
		Size:        media.Size,
		SHA256:      media.SHA256,
	}
}

// userIDsFromParticipants 将会话参与者模型压成去重后的用户 ID 列表。
func userIDsFromParticipants(participants []model.ConversationParticipant) []int64 {
	userIDs := make([]int64, 0, len(participants))
	for _, p := range participants {
		userIDs = append(userIDs, p.UserID)
	}
	return dedupeUserIDs(userIDs)
}
