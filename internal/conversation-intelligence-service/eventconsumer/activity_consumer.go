// Package eventconsumer 消费 IM/消息事件，推动会话智能归档调度游标。
package eventconsumer

import (
	"ClaranAIM/internal/conversation-intelligence-service/service"
	"ClaranAIM/pkg/eventbus"
	"ClaranAIM/pkg/events"
	"context"
	"encoding/json"
	"log"
	"time"
)

// StartConversationActivityConsumer 启动聊天活动消费者。
// 每条消息不会直接入库为 RAG，而是先推进 conversation_activity_cursors；
// 后续 scheduler 按时间窗口或累计消息数生成 digest job。
func StartConversationActivityConsumer(ctx context.Context, consumer *eventbus.KafkaConsumer, svc service.ConversationIntelligenceService) {
	if consumer == nil || svc == nil {
		return
	}
	go consumer.Run(ctx, func(ctx context.Context, envelope events.Envelope) error {
		return recordConversationActivity(ctx, svc, envelope)
	})
}

func recordConversationActivity(ctx context.Context, svc service.ConversationIntelligenceService, envelope events.Envelope) error {
	event, ok, err := decodeConversationActivityEvent(envelope)
	if err != nil || !ok {
		return err
	}
	for _, viewerID := range event.ParticipantIDs {
		if viewerID <= 0 {
			continue
		}
		if err := svc.RecordActivity(ctx, service.ConversationActivityInput{
			ConversationID:    event.ConversationID,
			ViewerID:          viewerID,
			MessageID:         event.MessageID,
			MessageCountDelta: 1,
			OccurredAt:        event.OccurredAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

type conversationActivityEvent struct {
	ConversationID int64
	MessageID      int64
	ParticipantIDs []int64
	OccurredAt     time.Time
}

func decodeConversationActivityEvent(envelope events.Envelope) (conversationActivityEvent, bool, error) {
	switch envelope.Type {
	case events.EventTypeMessageCreated:
		payload, err := events.DecodePayload[events.MessagePayload](envelope)
		if err != nil {
			return conversationActivityEvent{}, false, err
		}
		return conversationActivityEvent{
			ConversationID: payload.ConversationID,
			MessageID:      payload.MsgID,
			ParticipantIDs: payload.ParticipantIDs,
			OccurredAt:     parseEnvelopeTime(envelope.OccurredAt),
		}, payload.ConversationID > 0 && payload.MsgID > 0, nil
	case events.EventTypeFileUploaded, events.EventTypeVoiceTranscribed, events.EventTypeIMMessageEdited, events.EventTypeIMMessageRecalled, events.EventTypeReactionAdded, events.EventTypeSystemNotice, events.EventTypeTaskChanged:
		payload, err := events.DecodePayload[events.IMEventPayload](envelope)
		if err != nil {
			return conversationActivityEvent{}, false, err
		}
		return conversationActivityEvent{
			ConversationID: payload.ConversationID,
			MessageID:      payload.MsgID,
			ParticipantIDs: payload.ParticipantIDs,
			OccurredAt:     parseEnvelopeTime(firstNonEmpty(payload.OccurredAt, envelope.OccurredAt)),
		}, payload.ConversationID > 0, nil
	default:
		return conversationActivityEvent{}, false, nil
	}
}

func parseEnvelopeTime(value string) time.Time {
	if value == "" {
		return time.Now()
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	var raw struct {
		OccurredAt string `json:"occurred_at"`
	}
	if err := json.Unmarshal([]byte(value), &raw); err == nil && raw.OccurredAt != "" {
		return parseEnvelopeTime(raw.OccurredAt)
	}
	log.Printf("conversation-intelligence事件时间解析失败: %s", value)
	return time.Now()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
