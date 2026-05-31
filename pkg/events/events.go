// Package events 定义 Kafka 与事务 Outbox 共用的事件契约。
// 各微服务之间只传递这里声明的统一 Envelope，避免每个服务私自拼接不同格式的消息，
// 也方便后续做幂等、审计、重放和 Agent 原生 IM 事件订阅。
package events

import (
	"ClaranAIM/pkg/idgen"
	"encoding/json"
	"fmt"
	"time"
)

// Topic 和 EventType 是 Kafka/Outbox 的跨服务协议。
// 新增事件时要同时维护 Envelope.Topic，否则 Outbox Worker 无法判断该把事件发到哪个 topic。
const (
	// TopicGroupEvents 承载群创建、解散、成员变更等群生命周期事件。
	TopicGroupEvents = "claran.group.events"
	// TopicMessageEvents 承载消息创建、编辑、撤回和已读事件。
	TopicMessageEvents = "claran.message.events"
	// TopicAgentEvents 承载 Agent 执行、工具调用和审计事件。
	TopicAgentEvents = "claran.agent.events"
	// TopicIMEvents 承载 Agent-Native IM 的统一事件流。
	// 旧消费者可以继续订阅消息/群聊历史 topic，Agent Dispatcher 则订阅这个统一入口，
	// 从而把消息、文件、群事件、系统通知等都看作可被 Agent 理解的事件。
	TopicIMEvents = "claran.im.events"

	// EventTypeGroupCreated 表示群资料和初始成员已经在业务库提交。
	EventTypeGroupCreated = "group.created"
	// EventTypeGroupMemberInvited 表示群成员被邀请并完成入群。
	EventTypeGroupMemberInvited = "group.member_invited"
	// EventTypeGroupMemberKicked 表示群成员被移出。
	EventTypeGroupMemberKicked = "group.member_kicked"
	// EventTypeGroupDeleted 表示群被解散，订阅方应刷新会话和成员状态。
	EventTypeGroupDeleted = "group.deleted"

	// EventTypeMessageCreated 表示一条消息事实已经落库。
	EventTypeMessageCreated = "message.created"
	// EventTypeMessageEdited 表示消息内容被编辑。
	EventTypeMessageEdited = "message.edited"
	// EventTypeMessageRecalled 表示消息被撤回。
	EventTypeMessageRecalled = "message.recalled"
	// EventTypeMessageRead 表示某个用户的会话已读游标推进。
	EventTypeMessageRead = "message.read"
	// EventTypeIMMessageEdited 是统一 IM 事件流里的消息编辑事件名。
	EventTypeIMMessageEdited = "im.message.edited"
	// EventTypeIMMessageRecalled 是统一 IM 事件流里的消息撤回事件名。
	EventTypeIMMessageRecalled = "im.message.recalled"
	// EventTypeIMMessageRead 是统一 IM 事件流里的已读回执事件名。
	EventTypeIMMessageRead = "im.message.read"
	// EventTypeReactionAdded 表示用户对消息添加了表情反应。
	EventTypeReactionAdded = "reaction.added"
	// EventTypeFileUploaded 表示文件已上传并在会话中可见。
	EventTypeFileUploaded = "file.uploaded"
	// EventTypeVoiceTranscribed 表示语音消息已经生成转写文本。
	EventTypeVoiceTranscribed = "voice.transcribed"
	// EventTypeGroupMemberJoined 是 Agent-Native 使用的群成员加入事件名。
	EventTypeGroupMemberJoined = "group.member_joined"
	// EventTypeGroupMemberLeft 是 Agent-Native 使用的群成员离开事件名。
	EventTypeGroupMemberLeft = "group.member_left"
	// EventTypeSystemNotice 表示需要进入审计链路的系统通知。
	EventTypeSystemNotice = "system.notice"
	// EventTypeTaskChanged 表示外部工单或内部任务状态发生变化。
	EventTypeTaskChanged = "task.changed"

	// EventTypeAgentInvoked 表示 Agent 开始处理一次请求或事件。
	EventTypeAgentInvoked = "agent.invoked"
	// EventTypeAgentCompleted 表示 Agent 执行成功结束。
	EventTypeAgentCompleted = "agent.completed"
	// EventTypeAgentFailed 表示 Agent 执行失败。
	EventTypeAgentFailed = "agent.failed"
	// EventTypeAgentToolCalled 表示 Agent 调用了需要审计的工具。
	EventTypeAgentToolCalled = "agent.tool_called"
	// EventTypeAgentAuditRecorded 表示 Agent 行为审计记录已经落库。
	EventTypeAgentAuditRecorded = "agent.audit_recorded"
)

// Envelope 是所有 Kafka 域事件的统一外壳。
// Type/Version 用于消费者按事件类型和版本分发，Key 用作 Kafka 分区键，Payload
// 保持 RawMessage 以便不同业务事件拥有自己的结构。
type Envelope struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	Version    int             `json:"version"`
	Key        string          `json:"key"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

// NewEnvelope 将具体业务 payload 序列化为统一事件 Envelope。
// 这里同时生成全局事件 ID 和发生时间，调用方只需要提供事件类型、分区 key 和业务载荷。
func NewEnvelope(eventType, key string, payload interface{}) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	eventID, err := idgen.NextID()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID:    fmt.Sprintf("%d", eventID),
		Type:       eventType,
		Version:    1,
		Key:        key,
		OccurredAt: time.Now().Format(time.RFC3339Nano),
		Payload:    data,
	}, nil
}

// DecodeEnvelope 将 Kafka 或 Outbox 中保存的原始字节解析成统一事件 Envelope。
func DecodeEnvelope(data []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// DecodePayload 将 Envelope.Payload 反序列化为具体业务结构体。
func DecodePayload[T any](envelope Envelope) (T, error) {
	var payload T
	err := json.Unmarshal(envelope.Payload, &payload)
	return payload, err
}

// Topic 根据事件类型选择 Kafka topic。
// Outbox 发布器依赖这个映射决定消息发往哪里，因此新增事件类型时必须同步维护这里。
func (e Envelope) Topic() string {
	switch e.Type {
	case EventTypeGroupCreated, EventTypeGroupMemberInvited, EventTypeGroupMemberKicked, EventTypeGroupDeleted:
		return TopicGroupEvents
	case EventTypeMessageCreated, EventTypeMessageEdited, EventTypeMessageRecalled, EventTypeMessageRead:
		return TopicMessageEvents
	case EventTypeAgentInvoked, EventTypeAgentCompleted, EventTypeAgentFailed, EventTypeAgentToolCalled, EventTypeAgentAuditRecorded:
		return TopicAgentEvents
	case EventTypeIMMessageEdited, EventTypeIMMessageRecalled, EventTypeIMMessageRead, EventTypeReactionAdded, EventTypeFileUploaded, EventTypeVoiceTranscribed, EventTypeGroupMemberJoined, EventTypeGroupMemberLeft, EventTypeSystemNotice, EventTypeTaskChanged:
		return TopicIMEvents
	default:
		return ""
	}
}

// Bytes 将 Envelope 序列化为 Outbox 可存储、Kafka 可发送的 JSON 字节。
func (e Envelope) Bytes() ([]byte, error) {
	return json.Marshal(e)
}

// GroupCreatedPayload 描述群创建完成时的初始成员快照。
type GroupCreatedPayload struct {
	GroupID   int64   `json:"group_id"`
	OwnerID   int64   `json:"owner_id"`
	MemberIDs []int64 `json:"member_ids"`
	Name      string  `json:"name"`
}

// GroupMemberInvitedPayload 描述新增成员以及变更后的完整成员快照。
type GroupMemberInvitedPayload struct {
	GroupID    int64   `json:"group_id"`
	OperatorID int64   `json:"operator_id"`
	UserIDs    []int64 `json:"user_ids"`
	MemberIDs  []int64 `json:"member_ids"`
}

// GroupMemberKickedPayload 描述被移除成员以及剩余成员快照。
type GroupMemberKickedPayload struct {
	GroupID    int64   `json:"group_id"`
	OperatorID int64   `json:"operator_id"`
	UserID     int64   `json:"user_id"`
	MemberIDs  []int64 `json:"member_ids"`
}

// GroupDeletedPayload 描述被解散的群和需要刷新侧边栏的成员列表。
type GroupDeletedPayload struct {
	GroupID    int64   `json:"group_id"`
	OperatorID int64   `json:"operator_id"`
	MemberIDs  []int64 `json:"member_ids"`
}

// MessagePayload 兼容 websocket-gateway 当前推送协议，同时补充 TargetUserIDs
// 让网关无需查询消息服务即可按具体用户连接广播。
type MessagePayload struct {
	Type             string  `json:"type"`
	ConversationID   int64   `json:"conversation_id"`
	ConversationType string  `json:"conversation_type"`
	SenderID         int64   `json:"sender_id"`
	Content          string  `json:"content"`
	MsgType          string  `json:"msg_type"`
	MsgID            int64   `json:"msg_id"`
	CreatedAt        string  `json:"created_at"`
	ReplyToID        int64   `json:"reply_to_id"`
	Status           string  `json:"status"`
	IsEdited         bool    `json:"is_edited"`
	EditedAt         string  `json:"edited_at"`
	MentionUserIDs   []int64 `json:"mention_user_ids"`
	MentionAll       bool    `json:"mention_all"`
	UserID           int64   `json:"user_id"`
	TargetUserIDs    []int64 `json:"target_user_ids"`
	ParticipantIDs   []int64 `json:"participant_ids"`
}

// IMEventPayload 是 Agent Dispatcher 消费的 Agent-Native 统一 IM 事件契约。
// 它把会话路由、参与者可见性、@、引用、附件、权限上下文和幂等键放在同一份载荷里，
// 避免 Agent 消费者再从多个服务的私有 payload 中反推上下文。
type IMEventPayload struct {
	EventType        string            `json:"event_type"`
	ConversationID   int64             `json:"conversation_id"`
	ConversationType string            `json:"conversation_type"`
	SenderID         int64             `json:"sender_id"`
	Content          string            `json:"content"`
	MsgType          string            `json:"msg_type"`
	MsgID            int64             `json:"msg_id"`
	ReplyToID        int64             `json:"reply_to_id"`
	ParticipantIDs   []int64           `json:"participant_ids"`
	MentionUserIDs   []int64           `json:"mention_user_ids"`
	MentionAll       bool              `json:"mention_all"`
	AttachmentRefs   []AttachmentRef   `json:"attachment_refs"`
	Permission       PermissionContext `json:"permission_context"`
	OccurredAt       string            `json:"occurred_at"`
	IdempotencyKey   string            `json:"idempotency_key"`
	Metadata         map[string]string `json:"metadata"`
}

// AttachmentRef 是文件类事件的轻量引用。
// Agent 可以先基于类型、大小、哈希和 URL 判断是否解析、总结、静默入库或忽略，
// 真正读取文件内容前仍要重新做权限校验。
type AttachmentRef struct {
	FileID      int64  `json:"file_id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// PermissionContext 描述事件触发时的可见性边界。
// 这个结构只作为快速路由提示和审计快照，下游 Agent 在读取完整消息、文件或知识片段前
// 仍然必须按当前用户和 Agent 身份重新检查权限。
type PermissionContext struct {
	Scope          string  `json:"scope"`
	VisibleUserIDs []int64 `json:"visible_user_ids"`
	GroupRole      string  `json:"group_role"`
	CanReadFiles   bool    `json:"can_read_files"`
	CanWrite       bool    `json:"can_write"`
}

// AgentPayload 描述 Agent 运行态事件，供审计、前端状态展示和异步消费者使用。
type AgentPayload struct {
	BotID          int64  `json:"bot_id"`
	AgentUserID    int64  `json:"agent_user_id"`
	UserID         int64  `json:"user_id"`
	ConversationID int64  `json:"conversation_id"`
	SessionID      string `json:"session_id"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
}

// WebSocketMessage 将消息 payload 包装成 websocket-gateway 当前前端协议格式。
func (p MessagePayload) WebSocketMessage() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"type": p.Type,
		"data": p,
	})
}
