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
	Type           string  `json:"type"`
	ConversationID int64   `json:"conversation_id"`
	SenderID       int64   `json:"sender_id"`
	Content        string  `json:"content"`
	MsgType        string  `json:"msg_type"`
	MsgID          int64   `json:"msg_id"`
	CreatedAt      string  `json:"created_at"`
	ReplyToID      int64   `json:"reply_to_id"`
	Status         string  `json:"status"`
	IsEdited       bool    `json:"is_edited"`
	EditedAt       string  `json:"edited_at"`
	MentionUserIDs []int64 `json:"mention_user_ids"`
	MentionAll     bool    `json:"mention_all"`
	UserID         int64   `json:"user_id"`
	TargetUserIDs  []int64 `json:"target_user_ids"`
}

// WebSocketMessage wraps a message payload in the websocket-gateway protocol.
func (p MessagePayload) WebSocketMessage() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"type": p.Type,
		"data": p,
	})
}
