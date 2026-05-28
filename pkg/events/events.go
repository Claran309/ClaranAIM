// Package events defines the shared event contract used by Kafka and the
// transactional outbox. Services should exchange domain events through these
// envelopes instead of inventing per-service wire formats.
package events

import (
	"ClaranAIM/pkg/idgen"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// TopicGroupEvents carries group membership and lifecycle events.
	TopicGroupEvents = "claran.group.events"
	// TopicMessageEvents carries message write, edit, recall and read events.
	TopicMessageEvents = "claran.message.events"
	// TopicAgentEvents carries Agent execution, tool and audit events.
	TopicAgentEvents = "claran.agent.events"
	// TopicIMEvents carries Agent-native IM events beyond the legacy message
	// and group topics. Existing consumers can keep using their historical
	// topics while Agent-native dispatchers subscribe to this unified contract.
	TopicIMEvents = "claran.im.events"

	// EventTypeGroupCreated is emitted after a group and initial members commit.
	EventTypeGroupCreated = "group.created"
	// EventTypeGroupMemberInvited is emitted after group membership expands.
	EventTypeGroupMemberInvited = "group.member_invited"
	// EventTypeGroupMemberKicked is emitted after a member is removed.
	EventTypeGroupMemberKicked = "group.member_kicked"
	// EventTypeGroupDeleted is emitted after a group is dissolved.
	EventTypeGroupDeleted = "group.deleted"

	// EventTypeMessageCreated is emitted after a message fact commits.
	EventTypeMessageCreated = "message.created"
	// EventTypeMessageEdited is emitted after message content changes.
	EventTypeMessageEdited = "message.edited"
	// EventTypeMessageRecalled is emitted after a message is recalled.
	EventTypeMessageRecalled = "message.recalled"
	// EventTypeMessageRead is emitted after a user's read cursor advances.
	EventTypeMessageRead = "message.read"
	// EventTypeIMMessageEdited carries message edit facts on the unified IM topic.
	EventTypeIMMessageEdited = "im.message.edited"
	// EventTypeIMMessageRecalled carries message recall facts on the unified IM topic.
	EventTypeIMMessageRecalled = "im.message.recalled"
	// EventTypeIMMessageRead carries read-receipt facts on the unified IM topic.
	EventTypeIMMessageRead = "im.message.read"
	// EventTypeReactionAdded is emitted after a user reacts to a message.
	EventTypeReactionAdded = "reaction.added"
	// EventTypeFileUploaded is emitted after a file becomes visible in a conversation.
	EventTypeFileUploaded = "file.uploaded"
	// EventTypeVoiceTranscribed is emitted after a voice message receives text.
	EventTypeVoiceTranscribed = "voice.transcribed"
	// EventTypeGroupMemberJoined is the Agent-native group join event name.
	EventTypeGroupMemberJoined = "group.member_joined"
	// EventTypeGroupMemberLeft is the Agent-native group leave event name.
	EventTypeGroupMemberLeft = "group.member_left"
	// EventTypeSystemNotice is emitted for auditable system notices.
	EventTypeSystemNotice = "system.notice"
	// EventTypeTaskChanged is emitted when an external or internal task changes.
	EventTypeTaskChanged = "task.changed"

	// EventTypeAgentInvoked is emitted when an Agent starts handling a request.
	EventTypeAgentInvoked = "agent.invoked"
	// EventTypeAgentCompleted is emitted when an Agent finishes successfully.
	EventTypeAgentCompleted = "agent.completed"
	// EventTypeAgentFailed is emitted when an Agent run fails.
	EventTypeAgentFailed = "agent.failed"
	// EventTypeAgentToolCalled is emitted for auditable tool calls.
	EventTypeAgentToolCalled = "agent.tool_called"
	// EventTypeAgentAuditRecorded is emitted after an Agent audit record is stored.
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

// NewEnvelope serializes a typed payload into the common event envelope.
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

// DecodeEnvelope parses raw Kafka/outbox bytes into an Envelope.
func DecodeEnvelope(data []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// DecodePayload decodes an envelope payload into a concrete event payload type.
func DecodePayload[T any](envelope Envelope) (T, error) {
	var payload T
	err := json.Unmarshal(envelope.Payload, &payload)
	return payload, err
}

// Topic maps an event type to the Kafka topic that should carry it.
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

// Bytes serializes the envelope for outbox storage or Kafka publication.
func (e Envelope) Bytes() ([]byte, error) {
	return json.Marshal(e)
}

// GroupCreatedPayload describes the initial group member snapshot.
type GroupCreatedPayload struct {
	GroupID   int64   `json:"group_id"`
	OwnerID   int64   `json:"owner_id"`
	MemberIDs []int64 `json:"member_ids"`
	Name      string  `json:"name"`
}

// GroupMemberInvitedPayload describes newly invited users and the full member snapshot.
type GroupMemberInvitedPayload struct {
	GroupID    int64   `json:"group_id"`
	OperatorID int64   `json:"operator_id"`
	UserIDs    []int64 `json:"user_ids"`
	MemberIDs  []int64 `json:"member_ids"`
}

// GroupMemberKickedPayload describes a removed user and the remaining member snapshot.
type GroupMemberKickedPayload struct {
	GroupID    int64   `json:"group_id"`
	OperatorID int64   `json:"operator_id"`
	UserID     int64   `json:"user_id"`
	MemberIDs  []int64 `json:"member_ids"`
}

// GroupDeletedPayload describes a dissolved group and users whose sidebars need refresh.
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

// IMEventPayload is the Agent-native event contract used by the dispatcher.
//
// It intentionally carries conversation routing, participant visibility,
// mentions, reply references, attachment references and idempotency metadata in
// one place so Agent consumers do not need to reverse-engineer context from
// several service-specific payloads.
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

// AttachmentRef gives Agent consumers enough information to decide whether a
// file-like event should be parsed, summarized, stored silently or ignored.
type AttachmentRef struct {
	FileID      int64  `json:"file_id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// PermissionContext describes the visibility boundary attached to an IM event.
// Downstream Agent code must still re-check permissions before loading full
// message/file bodies; this object is the fast routing hint and audit snapshot.
type PermissionContext struct {
	Scope          string  `json:"scope"`
	VisibleUserIDs []int64 `json:"visible_user_ids"`
	GroupRole      string  `json:"group_role"`
	CanReadFiles   bool    `json:"can_read_files"`
	CanWrite       bool    `json:"can_write"`
}

// AgentPayload describes runtime events for audit and async subscribers.
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

// WebSocketMessage wraps a message payload in the websocket-gateway protocol.
func (p MessagePayload) WebSocketMessage() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"type": p.Type,
		"data": p,
	})
}
