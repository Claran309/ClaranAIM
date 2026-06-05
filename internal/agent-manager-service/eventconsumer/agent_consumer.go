// Package eventconsumer 实现 agent-manager-service 侧的 Kafka 消费器。
package eventconsumer

import (
	"ClaranAIM/internal/agent-manager-service/dao"
	"ClaranAIM/internal/agent-manager-service/model"
	"ClaranAIM/internal/agent-manager-service/service"
	"ClaranAIM/kitex_gen/message"
	"ClaranAIM/kitex_gen/message/messageservice"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"context"
	"fmt"
	"log"
	"strings"
)

// agentDispatchHistoryLimit 控制 Agent 被 IM 事件触发时最多读取多少条历史消息。
// 过小会失去群聊上下文，过大会增加 LLM 输入成本；当前取 24，避免私聊长期会话把模型 prompt 撑爆。
const agentDispatchHistoryLimit int64 = 24

const (
	agentDispatchMessageRuneLimit = 240
	agentDispatchContextRuneLimit = 5000
)

// StartAgentMentionConsumer 启动兼容旧链路的 @Agent 消息事件分发器。
func StartAgentMentionConsumer(ctx context.Context, consumer *eventbus.KafkaConsumer, agentService service.AgentService, dispatchRepo dao.AgentDispatchRepository, messageClient messageservice.Client) {
	StartAgentEventDispatcherConsumer(ctx, consumer, agentService, dispatchRepo, nil, nil, messageClient)
}

// StartAgentEventDispatcherConsumer 基于一个 Kafka topic 启动 Agent 原生事件分发器。
//
// 迁移期间既可以消费旧的 message.created 事件，也可以消费新的统一 IM 事件，
// 等所有服务完成 Outbox payload 迁移后再逐步收敛到统一事件契约。
func StartAgentEventDispatcherConsumer(ctx context.Context, consumer *eventbus.KafkaConsumer, agentService service.AgentService, dispatchRepo dao.AgentDispatchRepository, subscriptionRepo dao.AgentSubscriptionRepository, auditRepo dao.AgentAuditRepository, messageClient messageservice.Client) {
	StartAgentEventDispatcherConsumerWithReliability(ctx, consumer, agentService, dispatchRepo, subscriptionRepo, auditRepo, messageClient, nil)
}

// StartAgentEventDispatcherConsumerWithReliability 基于一个 Kafka topic 启动带幂等/DLQ保护的 Agent 原生事件分发器。
func StartAgentEventDispatcherConsumerWithReliability(ctx context.Context, consumer *eventbus.KafkaConsumer, agentService service.AgentService, dispatchRepo dao.AgentDispatchRepository, subscriptionRepo dao.AgentSubscriptionRepository, auditRepo dao.AgentAuditRepository, messageClient messageservice.Client, reliability eventbus.ReliabilityStore) {
	StartAgentEventDispatcherConsumerWithOptions(ctx, consumer, agentService, dispatchRepo, subscriptionRepo, auditRepo, nil, messageClient, reliability)
}

// StartAgentEventDispatcherConsumerWithOptions 启动完整可配置的 Agent 事件消费者。
func StartAgentEventDispatcherConsumerWithOptions(ctx context.Context, consumer *eventbus.KafkaConsumer, agentService service.AgentService, dispatchRepo dao.AgentDispatchRepository, subscriptionRepo dao.AgentSubscriptionRepository, auditRepo dao.AgentAuditRepository, taskRepo dao.AgentTaskRepository, messageClient messageservice.Client, reliability eventbus.ReliabilityStore) {
	if consumer == nil || agentService == nil || dispatchRepo == nil || messageClient == nil {
		return
	}
	dispatcher := NewAgentEventDispatcher(agentService, dispatchRepo, subscriptionRepo, auditRepo, messageClient)
	dispatcher.SetTaskRepository(taskRepo)
	handler := eventbus.NewReliableHandler(reliability, "agent-manager-service", 5, func(ctx context.Context, envelope events.Envelope) error {
		return dispatcher.Handle(ctx, envelope)
	})
	go consumer.Run(ctx, handler)
}

// handleAgentMentionEvent 保留给旧测试和旧 consumer 的兼容入口。
// 新链路应直接创建 AgentEventDispatcher，让 @、文件、群成员等事件走同一套决策逻辑。
func handleAgentMentionEvent(ctx context.Context, envelope events.Envelope, agentService service.AgentService, dispatchRepo dao.AgentDispatchRepository, messageClient messageservice.Client) error {
	return NewAgentEventDispatcher(agentService, dispatchRepo, nil, nil, messageClient).Handle(ctx, envelope)
}

// AgentEventDispatcher 是 Agent-Native IM 的事件路由器。
//
// 它消费统一 IM 事件，匹配订阅规则，记录忽略/入库/触发等决策，
// 只有当事件确实被允许转化为 Agent 工作时才调用 runtime，避免群聊刷屏。
type AgentEventDispatcher struct {
	agentService     service.AgentService
	dispatchRepo     dao.AgentDispatchRepository
	subscriptionRepo dao.AgentSubscriptionRepository
	auditRepo        dao.AgentAuditRepository
	taskRepo         dao.AgentTaskRepository
	messageClient    messageservice.Client
}

// NewAgentEventDispatcher 装配 Agent 原生事件分发器。
//
// subscriptionRepo 和 auditRepo 在迁移期允许为空，这样旧的 @Agent consumer
// 仍然可以沿用同一套分发逻辑。
func NewAgentEventDispatcher(agentService service.AgentService, dispatchRepo dao.AgentDispatchRepository, subscriptionRepo dao.AgentSubscriptionRepository, auditRepo dao.AgentAuditRepository, messageClient messageservice.Client) *AgentEventDispatcher {
	return &AgentEventDispatcher{
		agentService:     agentService,
		dispatchRepo:     dispatchRepo,
		subscriptionRepo: subscriptionRepo,
		auditRepo:        auditRepo,
		messageClient:    messageClient,
	}
}

// SetTaskRepository 注入 Agent 任务仓储；未注入时保持旧同步执行行为。
func (d *AgentEventDispatcher) SetTaskRepository(taskRepo dao.AgentTaskRepository) {
	if d != nil {
		d.taskRepo = taskRepo
	}
}

// agentEvent 是 dispatcher 内部统一后的 IM 事件视图。
// 它把旧 MessagePayload 和新 IMEventPayload 压成同一个结构，后续决策不再关心 Kafka 来源 topic。
type agentEvent struct {
	EventType        string
	ConversationID   int64
	ConversationType string
	SenderID         int64
	Content          string
	MsgType          string
	MsgID            int64
	ReplyToID        int64
	ParticipantIDs   []int64
	MentionUserIDs   []int64
	MentionAll       bool
	AttachmentRefs   []events.AttachmentRef
	IdempotencyKey   string
	ClientMsgID      string
	Metadata         map[string]string
}

// agentDispatchDecision 表示某个 Agent 对当前事件的处理结果。
// Decision 可为 trigger、record 或 ignore；BotID 可来自订阅规则，AgentUserID 可来自 @ 或私聊参与者。
type agentDispatchDecision struct {
	BotID       int64
	AgentUserID int64
	Decision    string
	Reason      string
}

// Handle 按 Agent 原生规则处理一条 Kafka/Outbox 事件信封。
func (d *AgentEventDispatcher) Handle(ctx context.Context, envelope events.Envelope) error {
	if d == nil || d.agentService == nil || d.dispatchRepo == nil || d.messageClient == nil {
		return nil
	}
	event, err := decodeAgentEvent(envelope)
	if err != nil {
		return err
	}
	if event == nil {
		return nil
	}
	log.Printf("Agent事件入口: event_id=%s type=%s conversation_id=%d conversation_type=%s sender_id=%d msg_id=%d client_msg_id=%s idempotency_key=%s participants=%v mentions=%v metadata=%v", envelope.EventID, event.EventType, event.ConversationID, event.ConversationType, event.SenderID, event.MsgID, event.ClientMsgID, event.IdempotencyKey, event.ParticipantIDs, event.MentionUserIDs, event.Metadata)
	if event.isNonTriggeringEvent() {
		log.Printf("Agent事件静默: non_triggering_event event_id=%s type=%s conversation_id=%d sender_id=%d msg_id=%d", envelope.EventID, event.EventType, event.ConversationID, event.SenderID, event.MsgID)
		return nil
	}
	if event.isAgentGenerated() {
		log.Printf("Agent事件静默: agent_generated event_id=%s type=%s conversation_id=%d sender_id=%d msg_id=%d client_msg_id=%s", envelope.EventID, event.EventType, event.ConversationID, event.SenderID, event.MsgID, event.ClientMsgID)
		return nil
	}
	if event.looksLikeAgentEcho() {
		log.Printf("Agent事件静默: agent_echo_marker event_id=%s type=%s conversation_id=%d sender_id=%d msg_id=%d client_msg_id=%s", envelope.EventID, event.EventType, event.ConversationID, event.SenderID, event.MsgID, event.ClientMsgID)
		return nil
	}
	if reason := d.agentEchoIgnoreReason(ctx, *event); reason != "" {
		log.Printf("Agent事件静默: %s event_id=%s type=%s conversation_id=%d sender_id=%d msg_id=%d client_msg_id=%s participants=%v mentions=%v", reason, envelope.EventID, event.EventType, event.ConversationID, event.SenderID, event.MsgID, event.ClientMsgID, event.ParticipantIDs, event.MentionUserIDs)
		_ = d.auditSilentAgentEcho(ctx, envelope, *event, reason)
		return nil
	}
	decisions, err := d.decide(ctx, *event)
	if err != nil {
		return err
	}
	if len(decisions) == 0 {
		return nil
	}
	for _, decision := range decisions {
		bot, err := d.resolveBot(ctx, decision)
		if err != nil {
			return err
		}
		if bot == nil || !bot.IsActive || bot.AgentUserID == event.SenderID {
			if bot != nil && bot.AgentUserID == event.SenderID {
				traceID := fmt.Sprintf("agent:%s:%d", event.dispatchEventID(envelope.EventID), bot.AgentUserID)
				_ = d.audit(ctx, envelope, *event, bot, "silent_agent_echo", "resolved_bot_sender_is_agent_user", traceID)
				log.Printf("Agent事件静默: resolved_bot_sender_is_agent_user event_id=%s type=%s conversation_id=%d sender_id=%d bot_id=%d agent_user_id=%d msg_id=%d client_msg_id=%s participants=%v metadata=%v", envelope.EventID, event.EventType, event.ConversationID, event.SenderID, bot.ID, bot.AgentUserID, event.MsgID, event.ClientMsgID, event.ParticipantIDs, event.Metadata)
			}
			continue
		}
		if event.sentByKnownAgent(bot) {
			traceID := fmt.Sprintf("agent:%s:%d", event.dispatchEventID(envelope.EventID), bot.AgentUserID)
			_ = d.audit(ctx, envelope, *event, bot, "silent_agent_echo", "event_sent_by_known_agent", traceID)
			log.Printf("Agent事件静默: event_sent_by_known_agent event_id=%s type=%s conversation_id=%d sender_id=%d bot_id=%d agent_user_id=%d msg_id=%d client_msg_id=%s participants=%v metadata=%v", envelope.EventID, event.EventType, event.ConversationID, event.SenderID, bot.ID, bot.AgentUserID, event.MsgID, event.ClientMsgID, event.ParticipantIDs, event.Metadata)
			continue
		}
		sourceEventID := event.dispatchEventID(envelope.EventID)
		traceID := fmt.Sprintf("agent:%s:%d", sourceEventID, bot.AgentUserID)
		if decision.Decision == "record" {
			_ = d.audit(ctx, envelope, *event, bot, "record", decision.Reason, traceID)
			continue
		}
		if decision.Decision != "trigger" {
			_ = d.audit(ctx, envelope, *event, bot, decision.Decision, decision.Reason, traceID)
			continue
		}
		_ = d.audit(ctx, envelope, *event, bot, "trigger", decision.Reason, traceID)
		shouldRun, err := d.dispatchRepo.Start(ctx, &model.AgentDispatchRecord{
			EventID:        sourceEventID,
			AgentUserID:    bot.AgentUserID,
			BotID:          bot.ID,
			EventType:      event.EventType,
			Decision:       "trigger",
			SourceEventID:  sourceEventID,
			AgentTraceID:   traceID,
			SourceMsgID:    event.MsgID,
			ConversationID: event.ConversationID,
			SenderID:       event.SenderID,
			Status:         "started",
		})
		if err != nil {
			return err
		}
		if !shouldRun {
			continue
		}
		_ = d.queueTask(ctx, bot, *event, sourceEventID, traceID)
		_ = d.markTaskRunning(ctx, sourceEventID)
		agentInput, err := buildAgentDispatchInput(ctx, d.messageClient, event.toMessagePayload(), bot.AgentUserID)
		if err != nil {
			_ = d.dispatchRepo.MarkFailed(ctx, sourceEventID, bot.AgentUserID, err.Error())
			_ = d.markTaskFailed(ctx, sourceEventID, err.Error())
			return err
		}
		result, err := d.agentService.ChatWithBot(ctx, bot.ID, event.SenderID, event.ConversationID, agentInput)
		if err != nil {
			_ = d.dispatchRepo.MarkFailed(ctx, sourceEventID, bot.AgentUserID, err.Error())
			_ = d.markTaskFailed(ctx, sourceEventID, err.Error())
			_ = d.audit(ctx, envelope, *event, bot, "failed", err.Error(), traceID)
			log.Printf("Agent事件响应失败 bot_id=%d msg_id=%d err=%v", bot.ID, event.MsgID, err)
			if isPermanentAgentDispatchError(err) {
				continue
			}
			return err
		}
		reply := ""
		if result != nil {
			reply = result.Reply
		}
		if strings.TrimSpace(reply) == "" {
			_ = d.dispatchRepo.MarkCompleted(ctx, sourceEventID, bot.AgentUserID, 0)
			_ = d.markTaskCompleted(ctx, sourceEventID)
			_ = d.audit(ctx, envelope, *event, bot, "silent", "Agent返回空回复，已静默完成", traceID)
			continue
		}
		clientMsgID := fmt.Sprintf("agent:%s:%d", sourceEventID, bot.AgentUserID)
		resp, err := d.messageClient.SendMessage(ctx, &message.SendMessageReq{
			ConversationId: event.ConversationID,
			SenderId:       bot.AgentUserID,
			Content:        reply,
			MsgType:        "text",
			ReplyToId:      event.MsgID,
			ClientMsgId:    clientMsgID,
		})
		if err != nil {
			_ = d.dispatchRepo.MarkFailed(ctx, sourceEventID, bot.AgentUserID, err.Error())
			_ = d.markTaskFailed(ctx, sourceEventID, err.Error())
			_ = d.audit(ctx, envelope, *event, bot, "failed", err.Error(), traceID)
			return err
		}
		if resp == nil || !resp.Success {
			msg := "msg-core-service返回空响应"
			if resp != nil && resp.GetMsg() != "" {
				msg = resp.GetMsg()
			}
			err := fmt.Errorf("Agent回复写入失败: %s", msg)
			_ = d.dispatchRepo.MarkFailed(ctx, sourceEventID, bot.AgentUserID, err.Error())
			_ = d.markTaskFailed(ctx, sourceEventID, err.Error())
			_ = d.audit(ctx, envelope, *event, bot, "failed", err.Error(), traceID)
			return err
		}
		if err := d.dispatchRepo.MarkCompleted(ctx, sourceEventID, bot.AgentUserID, resp.MsgId); err != nil {
			return err
		}
		_ = d.markTaskCompleted(ctx, sourceEventID)
		_ = d.audit(ctx, envelope, *event, bot, "completed", "Agent回复已写入消息事实表", traceID)
	}
	return nil
}

// isAgentSender 判断事件发送者本身是否是 Agent 系统用户。
// 普通 IM 触发链必须忽略 Agent 自己发出的消息，避免 Agent 回复落库后再次触发自己或其他 Agent 形成回声。
func (d *AgentEventDispatcher) isAgentSender(ctx context.Context, senderID int64) bool {
	if d == nil || d.agentService == nil || senderID <= 0 {
		return false
	}
	bot, err := d.agentService.GetBotByAgentUserID(ctx, senderID)
	if err != nil {
		log.Printf("Agent sender lookup failed sender_id=%d err=%v", senderID, err)
		return false
	}
	if bot != nil && bot.AgentUserID == senderID {
		log.Printf("Agent sender lookup matched sender_id=%d bot_id=%d agent_user_id=%d", senderID, bot.ID, bot.AgentUserID)
		return true
	}
	log.Printf("Agent sender lookup not matched sender_id=%d", senderID)
	return false
}

func (d *AgentEventDispatcher) shouldIgnoreAgentEcho(ctx context.Context, event agentEvent) bool {
	return d.agentEchoIgnoreReason(ctx, event) != ""
}

func (d *AgentEventDispatcher) auditSilentAgentEcho(ctx context.Context, envelope events.Envelope, event agentEvent, reason string) error {
	if d == nil || d.auditRepo == nil || d.agentService == nil {
		return nil
	}
	var bot *model.Bot
	if event.SenderID > 0 {
		if resolved, err := d.agentService.GetBotByAgentUserID(ctx, event.SenderID); err == nil && resolved != nil {
			bot = resolved
		}
	}
	if bot == nil {
		for _, participantID := range event.ParticipantIDs {
			if participantID <= 0 {
				continue
			}
			resolved, err := d.agentService.GetBotByAgentUserID(ctx, participantID)
			if err == nil && resolved != nil {
				bot = resolved
				break
			}
		}
	}
	if bot == nil {
		return nil
	}
	traceID := fmt.Sprintf("agent:%s:%d", event.dispatchEventID(envelope.EventID), bot.AgentUserID)
	return d.audit(ctx, envelope, event, bot, "silent_agent_echo", reason, traceID)
}

func (d *AgentEventDispatcher) agentEchoIgnoreReason(ctx context.Context, event agentEvent) string {
	if event.isAgentGenerated() || event.looksLikeAgentEcho() {
		return "agent_echo_marker"
	}
	if d.isAgentSender(ctx, event.SenderID) {
		return "sender_is_agent_user"
	}
	if event.ConversationType != "private" {
		return ""
	}
	agentParticipants := 0
	for _, participantID := range event.ParticipantIDs {
		if participantID <= 0 {
			continue
		}
		if d.isAgentSender(ctx, participantID) {
			agentParticipants++
			if participantID == event.SenderID {
				return "private_sender_is_agent_participant"
			}
		}
	}
	if event.SenderID <= 0 && agentParticipants > 0 {
		return "private_missing_sender_with_agent_participant"
	}
	if event.SenderID <= 0 {
		return "private_missing_sender"
	}
	return ""
}

func (d *AgentEventDispatcher) queueTask(ctx context.Context, bot *model.Bot, event agentEvent, sourceEventID, traceID string) error {
	if d.taskRepo == nil || bot == nil {
		return nil
	}
	return d.taskRepo.UpsertQueued(ctx, &model.AgentTask{
		BotID:          bot.ID,
		AgentUserID:    bot.AgentUserID,
		ConversationID: event.ConversationID,
		TriggerUserID:  event.SenderID,
		SourceEventID:  sourceEventID,
		TraceID:        traceID,
		EventType:      event.EventType,
		Status:         "queued",
	})
}

func (d *AgentEventDispatcher) markTaskRunning(ctx context.Context, sourceEventID string) error {
	if d.taskRepo == nil {
		return nil
	}
	return d.taskRepo.MarkRunning(ctx, sourceEventID)
}

func (d *AgentEventDispatcher) markTaskCompleted(ctx context.Context, sourceEventID string) error {
	if d.taskRepo == nil {
		return nil
	}
	return d.taskRepo.MarkCompleted(ctx, sourceEventID)
}

func (d *AgentEventDispatcher) markTaskFailed(ctx context.Context, sourceEventID, message string) error {
	if d.taskRepo == nil {
		return nil
	}
	return d.taskRepo.MarkFailed(ctx, sourceEventID, message)
}

// dispatchEventID 返回用于幂等去重的事件 ID。
// 统一 IM 事件优先使用 payload.idempotency_key；旧事件没有时退回 envelope.EventID。
func (e agentEvent) dispatchEventID(fallback string) string {
	if strings.TrimSpace(e.IdempotencyKey) != "" {
		return e.IdempotencyKey
	}
	return fallback
}

// decodeAgentEvent 兼容旧消息事件和新 Agent-Native IM 事件。
// 无法触发 Agent 的空消息会返回 nil，让 consumer 安静跳过而不是写失败记录。
func decodeAgentEvent(envelope events.Envelope) (*agentEvent, error) {
	switch envelope.Type {
	case events.EventTypeMessageCreated, events.EventTypeMessageEdited, events.EventTypeMessageRecalled:
		payload, err := events.DecodePayload[events.MessagePayload](envelope)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(payload.Content) == "" && len(payload.MentionUserIDs) == 0 {
			return nil, nil
		}
		if isMediaMessagePayload(payload.MsgType, payload.Content) {
			return nil, nil
		}
		return &agentEvent{
			EventType:        envelope.Type,
			ConversationID:   payload.ConversationID,
			ConversationType: payload.ConversationType,
			SenderID:         payload.SenderID,
			Content:          payload.Content,
			MsgType:          payload.MsgType,
			MsgID:            payload.MsgID,
			ReplyToID:        payload.ReplyToID,
			ParticipantIDs:   payload.ParticipantIDs,
			MentionUserIDs:   payload.MentionUserIDs,
			MentionAll:       payload.MentionAll,
			IdempotencyKey:   envelope.EventID,
			ClientMsgID:      payload.ClientMsgID,
			Metadata:         map[string]string{},
		}, nil
	case events.EventTypeIMMessageEdited, events.EventTypeIMMessageRecalled, events.EventTypeIMMessageRead, events.EventTypeReactionAdded, events.EventTypeFileUploaded, events.EventTypeVoiceTranscribed, events.EventTypeGroupMemberJoined, events.EventTypeGroupMemberLeft, events.EventTypeSystemNotice, events.EventTypeTaskChanged:
		payload, err := events.DecodePayload[events.IMEventPayload](envelope)
		if err != nil {
			return nil, err
		}
		if payload.EventType == "" {
			payload.EventType = envelope.Type
		}
		return &agentEvent{
			EventType:        payload.EventType,
			ConversationID:   payload.ConversationID,
			ConversationType: payload.ConversationType,
			SenderID:         payload.SenderID,
			Content:          payload.Content,
			MsgType:          payload.MsgType,
			MsgID:            payload.MsgID,
			ReplyToID:        payload.ReplyToID,
			ParticipantIDs:   payload.ParticipantIDs,
			MentionUserIDs:   payload.MentionUserIDs,
			MentionAll:       payload.MentionAll,
			AttachmentRefs:   payload.AttachmentRefs,
			IdempotencyKey:   payload.IdempotencyKey,
			ClientMsgID:      payload.Metadata["client_msg_id"],
			Metadata:         payload.Metadata,
		}, nil
	default:
		return nil, nil
	}
}

func (e agentEvent) isAgentGenerated() bool {
	if e.Metadata != nil {
		if strings.EqualFold(strings.TrimSpace(e.Metadata["agent_generated"]), "true") || strings.EqualFold(strings.TrimSpace(e.Metadata["source"]), "agent") {
			return true
		}
	}
	clientMsgID := strings.TrimSpace(e.ClientMsgID)
	if strings.HasPrefix(clientMsgID, "agent:") {
		return true
	}
	idempotencyKey := strings.TrimSpace(e.IdempotencyKey)
	if strings.HasPrefix(idempotencyKey, "agent:") {
		return true
	}
	return false
}

func (e agentEvent) looksLikeAgentEcho() bool {
	if strings.HasPrefix(strings.TrimSpace(e.ClientMsgID), "agent:") || strings.HasPrefix(strings.TrimSpace(e.IdempotencyKey), "agent:") {
		return true
	}
	if e.Metadata != nil {
		source := strings.ToLower(strings.TrimSpace(e.Metadata["source"]))
		if source == "agent" || source == "agent-runtime" || source == "agent-manager" {
			return true
		}
	}
	return false
}

func (e agentEvent) isNonTriggeringEvent() bool {
	switch strings.TrimSpace(e.EventType) {
	case events.EventTypeMessageRead, events.EventTypeIMMessageRead, events.EventTypeReactionAdded:
		return true
	default:
		return false
	}
}

func (e agentEvent) sentByKnownAgent(bot *model.Bot) bool {
	if bot == nil {
		return false
	}
	// 兼容早期 IM 事件/历史库错配：Agent 回复事件的 sender_id 可能落成 bot.id，
	// 而不是后来的 agent_user_id。命中后必须静默，否则私聊会把 Agent 自己的回复再次触发。
	if bot.ID > 0 && e.SenderID == bot.ID {
		return true
	}
	if bot.AgentUserID <= 0 {
		return false
	}
	if e.SenderID == bot.AgentUserID {
		return true
	}
	if strings.Contains(strings.TrimSpace(e.ClientMsgID), fmt.Sprintf(":%d", bot.AgentUserID)) && strings.HasPrefix(strings.TrimSpace(e.ClientMsgID), "agent:") {
		return true
	}
	for _, participantID := range e.ParticipantIDs {
		if participantID == bot.AgentUserID && e.ConversationType == "private" && e.SenderID <= 0 {
			return true
		}
	}
	return false
}

func isMediaMessagePayload(msgType, content string) bool {
	switch strings.ToLower(strings.TrimSpace(msgType)) {
	case "image", "file", "voice":
		return true
	}
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "[img]") || strings.HasPrefix(trimmed, "[file]") || strings.HasPrefix(trimmed, "[voice]")
}

// toMessagePayload 把统一事件降级成旧 MessagePayload 形状。
// 这样上下文构建和 @ 目标识别可以在迁移期复用原有 helper。
func (e agentEvent) toMessagePayload() events.MessagePayload {
	content := e.Content
	if strings.TrimSpace(content) == "" && len(e.AttachmentRefs) > 0 {
		parts := make([]string, 0, len(e.AttachmentRefs))
		for _, ref := range e.AttachmentRefs {
			name := strings.TrimSpace(ref.Name)
			if name == "" {
				name = fmt.Sprintf("file:%d", ref.FileID)
			}
			hint := ""
			if strings.HasPrefix(strings.ToLower(ref.ContentType), "image/") {
				hint = " 可使用图片OCR/识别能力解释图片内容。"
			}
			parts = append(parts, fmt.Sprintf("[附件 file_id=%d name=%s content_type=%s url=%s size=%d]%s", ref.FileID, name, ref.ContentType, ref.URL, ref.Size, hint))
		}
		content = strings.Join(parts, " ")
	}
	return events.MessagePayload{
		Type:             e.EventType,
		ConversationID:   e.ConversationID,
		ConversationType: e.ConversationType,
		SenderID:         e.SenderID,
		Content:          content,
		MsgType:          e.MsgType,
		MsgID:            e.MsgID,
		ReplyToID:        e.ReplyToID,
		MentionUserIDs:   e.MentionUserIDs,
		MentionAll:       e.MentionAll,
		ParticipantIDs:   e.ParticipantIDs,
	}
}

// decide 根据私聊/@默认规则和订阅规则合并出 Agent 决策。
// 私聊 Agent 默认触发；群聊默认只 @ 触发，额外的关键词/命令/静默记录由订阅规则补充。
func (d *AgentEventDispatcher) decide(ctx context.Context, event agentEvent) ([]agentDispatchDecision, error) {
	decisions := make([]agentDispatchDecision, 0)
	defaultTargets := agentTargetsFromMessage(event.toMessagePayload())
	for _, agentUserID := range defaultTargets {
		decisions = append(decisions, agentDispatchDecision{AgentUserID: agentUserID, Decision: "trigger", Reason: defaultTriggerReason(event)})
	}
	if d.subscriptionRepo != nil {
		rules, err := d.subscriptionRepo.ListActiveRules(ctx, event.ConversationID, event.EventType)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			if !ruleMatchesEvent(rule, event) {
				continue
			}
			action := strings.TrimSpace(rule.Action)
			if action == "" {
				action = "trigger"
			}
			if rule.Silent {
				action = "record"
			}
			decisions = append(decisions, agentDispatchDecision{
				BotID:       rule.BotID,
				AgentUserID: rule.AgentUserID,
				Decision:    action,
				Reason:      fmt.Sprintf("subscription:%s", strings.TrimSpace(rule.TriggerMode)),
			})
		}
	}
	return mergeAgentDecisions(decisions), nil
}

// resolveBot 根据决策中的 BotID 或 Agent 系统用户 ID 找到 Bot 配置。
// 订阅规则通常带 BotID，普通 @ 事件通常只知道被 @ 的 agent_user_id。
func (d *AgentEventDispatcher) resolveBot(ctx context.Context, decision agentDispatchDecision) (*model.Bot, error) {
	if decision.BotID > 0 {
		bot, err := d.agentService.GetBot(ctx, decision.BotID)
		if err != nil || bot != nil {
			return bot, err
		}
	}
	if decision.AgentUserID > 0 {
		return d.agentService.GetBotByAgentUserID(ctx, decision.AgentUserID)
	}
	return nil, nil
}

// audit 记录事件、决策、原因和 traceID。
// auditRepo 允许为空，便于迁移期旧 consumer 复用 dispatcher 而不强依赖审计表。
func (d *AgentEventDispatcher) audit(ctx context.Context, envelope events.Envelope, event agentEvent, bot *model.Bot, decision, reason, traceID string) error {
	if d.auditRepo == nil || bot == nil {
		return nil
	}
	return d.auditRepo.Create(ctx, &model.AgentAuditRecord{
		EventID:        envelope.EventID,
		EventType:      event.EventType,
		BotID:          bot.ID,
		AgentUserID:    bot.AgentUserID,
		ConversationID: event.ConversationID,
		SenderID:       event.SenderID,
		Decision:       decision,
		Reason:         reason,
		TraceID:        traceID,
		SourceMsgID:    event.MsgID,
	})
}

// defaultTriggerReason 标注默认触发来源，方便审计时区分私聊自动触发和群聊 @ 触发。
func defaultTriggerReason(event agentEvent) string {
	if event.ConversationType == "private" {
		return "private_default"
	}
	return "mention"
}

// ruleMatchesEvent 判断订阅规则是否命中当前事件。
// TriggerMode 支持 all、keyword、command、mention；未知模式按关键词兜底处理。
func ruleMatchesEvent(rule model.AgentSubscriptionRule, event agentEvent) bool {
	if rule.ConversationType != "" && rule.ConversationType != event.ConversationType {
		return false
	}
	mode := strings.TrimSpace(rule.TriggerMode)
	if mode == "" {
		mode = "mention"
	}
	switch mode {
	case "all":
		return true
	case "keyword":
		return containsAnyCSVToken(event.Content, rule.Keywords)
	case "command":
		prefix := strings.TrimSpace(rule.CommandPrefix)
		return prefix != "" && strings.HasPrefix(strings.TrimSpace(event.Content), prefix)
	case "mention":
		return containsInt64(event.MentionUserIDs, rule.AgentUserID)
	default:
		return containsAnyCSVToken(event.Content, rule.Keywords)
	}
}

// mergeAgentDecisions 合并同一 Agent 的多条规则命中结果。
// 如果同时命中 record 和 trigger，优先 trigger，避免被静默规则压掉显式 @ 或命令。
func mergeAgentDecisions(decisions []agentDispatchDecision) []agentDispatchDecision {
	merged := make(map[int64]agentDispatchDecision)
	order := make([]int64, 0, len(decisions))
	for _, decision := range decisions {
		key := decision.AgentUserID
		if key == 0 {
			key = -decision.BotID
		}
		if key == 0 {
			continue
		}
		existing, ok := merged[key]
		if !ok {
			merged[key] = decision
			order = append(order, key)
			continue
		}
		if decisionRank(decision.Decision) > decisionRank(existing.Decision) {
			merged[key] = decision
		}
	}
	result := make([]agentDispatchDecision, 0, len(order))
	for _, key := range order {
		result = append(result, merged[key])
	}
	return result
}

// decisionRank 定义决策优先级：触发执行 > 静默记录 > 忽略/未知。
func decisionRank(decision string) int {
	switch decision {
	case "trigger":
		return 3
	case "record":
		return 2
	default:
		return 1
	}
}

// containsAnyCSVToken 用逗号分隔关键词做大小写不敏感包含匹配。
// 这是订阅规则的轻量 MVP，后续可替换为更强的命令解析或意图分类。
func containsAnyCSVToken(text, csv string) bool {
	text = strings.ToLower(text)
	for _, part := range strings.Split(csv, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		if token != "" && strings.Contains(text, token) {
			return true
		}
	}
	return false
}

// containsInt64 判断 @ 列表中是否包含指定 Agent 用户 ID。
func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// agentTargetsFromMessage 推导默认目标 Agent。
// 私聊取除发送者外的会话参与者；群聊只取 mention_user_ids，避免每条群消息都触发 Agent。
func agentTargetsFromMessage(payload events.MessagePayload) []int64 {
	targets := make([]int64, 0)
	if payload.ConversationType == "private" {
		for _, participantID := range payload.ParticipantIDs {
			if participantID > 0 && participantID != payload.SenderID {
				targets = append(targets, participantID)
			}
		}
		return dedupePositiveIDs(targets)
	}
	return dedupePositiveIDs(payload.MentionUserIDs)
}

// buildAgentDispatchInput 为 runtime 构造带 IM 上下文的用户输入。
// 历史消息以 Agent 用户身份读取，确保上下文裁剪和普通用户可见性保持一致。
func buildAgentDispatchInput(ctx context.Context, messageClient messageservice.Client, payload events.MessagePayload, agentUserID int64) (string, error) {
	contextText := ""
	if messageClient != nil && payload.ConversationID > 0 && agentUserID > 0 {
		resp, err := messageClient.GetHistory(ctx, &message.GetHistoryReq{
			ConversationId: payload.ConversationID,
			UserId:         agentUserID,
			Limit:          agentDispatchHistoryLimit,
		})
		if err != nil {
			return "", fmt.Errorf("读取Agent可见会话上下文失败: %w", err)
		}
		if resp != nil && resp.Success {
			contextText = formatMessagesForAgentContext(resp.Messages)
		} else if resp != nil && resp.Msg != "" {
			return "", fmt.Errorf("读取Agent可见会话上下文失败: %s", resp.Msg)
		}
	}
	if contextText == "" {
		contextText = "（当前没有读取到历史消息。请基于用户这条消息本身回答，并说明上下文很少。）"
	}
	return fmt.Sprintf("你是 ClaranAIM 中的原生 Agent 成员，本轮输入来自 IM 事件流，而不是孤立聊天按钮。\n\n处理原则：\n1. 先阅读会话材料，再判断用户真正要你做什么。\n2. 如果材料很少或内容没有价值，也要直接说明这些消息基本没有有效信息，而不是拒绝总结。\n3. 群聊场景必须结合群聊上下文、引用关系和当前触发消息回答；不要只使用你和触发用户的长期记忆。\n4. 只使用你作为 Agent 用户有权看到的内容；不要猜测不可见消息、文件或知识库。\n5. 输出优先面向当前 IM 会话，可用 Markdown，但不要把 JSON 当作直接回复，除非用户明确要求机器可读 JSON。\n\n事件信息：\n- event_type: %s\n- conversation_id: %d\n- conversation_type: %s\n- sender_id: %d\n- reply_to_id: %d\n\n当前触发内容：\n%s\n\n会话材料说明：下面是 msg-core-service 从当前会话读取到的、Agent 用户有权看到的历史消息，按时间从旧到新排列；它们是本轮回答的主要事实来源。\n\n会话材料：\n%s", payload.Type, payload.ConversationID, payload.ConversationType, payload.SenderID, payload.ReplyToID, truncateRunes(strings.TrimSpace(payload.Content), agentDispatchMessageRuneLimit), contextText), nil
}

// formatMessagesForAgentContext 将消息历史压缩为适合 LLM 阅读的文本窗口。
// 单条消息和整体上下文都有限制，避免附件描述、长文或历史错误报告把 Agent 输入撑爆。
func formatMessagesForAgentContext(messages []*message.Message) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			content = fmt.Sprintf("[%s消息]", msg.MsgType)
		}
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.ReplaceAll(content, "\n", " ")
		content = truncateRunes(content, agentDispatchMessageRuneLimit)
		line := fmt.Sprintf("- [%s] 用户%d: %s\n", msg.CreatedAt, msg.SenderId, content)
		if len([]rune(b.String()+line)) > agentDispatchContextRuneLimit {
			break
		}
		b.WriteString(line)
	}
	return strings.TrimSpace(b.String())
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// dedupePositiveIDs 过滤空 ID 并保持原顺序，用于 @ 目标和私聊参与者去重。
func dedupePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// isPermanentAgentDispatchError 判断错误是否不值得 Kafka 重试。
// 配置缺失、权限不足和 Agent 停用这类问题重试也不会恢复，直接记录失败即可。
func isPermanentAgentDispatchError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	permanentHints := []string{
		"无权操作该Agent",
		"Agent权限不足",
		"bot不存在",
		"bot已停用",
		"bot未配置API Key",
		"bot未配置Base URL",
		"Agent未配置API Key",
		"Agent未配置Base URL",
		"Agent未配置模型",
		"401 Unauthorized",
		"status code: 401",
		"身份验证失败",
		"invalid api key",
		"invalid_api_key",
		"unauthorized",
		"Prompt exceeds max length",
		"context length",
		"maximum context",
	}
	for _, hint := range permanentHints {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(hint)) {
			return true
		}
	}
	return false
}
