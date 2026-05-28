package events

import (
	"encoding/json"
	"testing"
)

func TestNewEnvelopePreservesTypeKeyAndPayload(t *testing.T) {
	payload := GroupCreatedPayload{
		GroupID:   10,
		OwnerID:   1,
		MemberIDs: []int64{1, 2, 3},
		Name:      "team",
	}

	envelope, err := NewEnvelope(EventTypeGroupCreated, "10", payload)
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	if envelope.EventID == "" {
		t.Fatal("expected event id to be generated")
	}
	if envelope.Type != EventTypeGroupCreated {
		t.Fatalf("type = %q, want %q", envelope.Type, EventTypeGroupCreated)
	}
	if envelope.Topic() != TopicGroupEvents {
		t.Fatalf("topic = %q, want %q", envelope.Topic(), TopicGroupEvents)
	}

	var decoded GroupCreatedPayload
	if err := json.Unmarshal(envelope.Payload, &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded.GroupID != payload.GroupID || decoded.OwnerID != payload.OwnerID || len(decoded.MemberIDs) != 3 {
		t.Fatalf("decoded payload = %#v, want %#v", decoded, payload)
	}
}

func TestMessagePayloadProducesWebSocketEnvelope(t *testing.T) {
	payload := MessagePayload{
		Type:           "new_message",
		ConversationID: 20,
		SenderID:       1,
		Content:        "hello",
		MsgType:        "text",
		MsgID:          30,
		CreatedAt:      "2026-05-17 12:00:00",
		TargetUserIDs:  []int64{1, 2},
	}

	wsMsg, err := payload.WebSocketMessage()
	if err != nil {
		t.Fatalf("WebSocketMessage returned error: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(wsMsg, &decoded); err != nil {
		t.Fatalf("decode websocket message: %v", err)
	}
	if string(decoded["type"]) != `"new_message"` {
		t.Fatalf("websocket type = %s, want new_message", decoded["type"])
	}
	if len(decoded["data"]) == 0 {
		t.Fatal("expected websocket data payload")
	}
}

func TestIMEventPayloadCoversAgentNativeEventContract(t *testing.T) {
	payload := IMEventPayload{
		EventType:        EventTypeFileUploaded,
		ConversationID:   20,
		ConversationType: "group",
		SenderID:         1001,
		ParticipantIDs:   []int64{1001, 1002, 2001},
		MentionUserIDs:   []int64{2001},
		ReplyToID:        99,
		AttachmentRefs: []AttachmentRef{
			{FileID: 3001, Name: "error.png", ContentType: "image/png", URL: "/files/3001"},
		},
		Permission: PermissionContext{
			VisibleUserIDs: []int64{1001, 1002, 2001},
			Scope:          "group",
		},
		OccurredAt:     "2026-05-27T10:00:00+08:00",
		IdempotencyKey: "file.uploaded:3001",
	}

	envelope, err := NewEnvelope(EventTypeFileUploaded, "20", payload)
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	if envelope.Topic() != TopicIMEvents {
		t.Fatalf("topic = %q, want %q", envelope.Topic(), TopicIMEvents)
	}

	decoded, err := DecodePayload[IMEventPayload](envelope)
	if err != nil {
		t.Fatalf("DecodePayload returned error: %v", err)
	}
	if decoded.IdempotencyKey == "" || decoded.ConversationID == 0 || decoded.SenderID == 0 {
		t.Fatalf("decoded payload misses required routing fields: %#v", decoded)
	}
	if len(decoded.AttachmentRefs) != 1 || decoded.AttachmentRefs[0].FileID != 3001 {
		t.Fatalf("attachment refs = %#v, want uploaded file reference", decoded.AttachmentRefs)
	}
	if decoded.Permission.Scope != "group" || len(decoded.Permission.VisibleUserIDs) != 3 {
		t.Fatalf("permission context = %#v, want group visibility", decoded.Permission)
	}
}

func TestIMMessageEnvelopeTypesRouteToUnifiedIMTopic(t *testing.T) {
	for _, eventType := range []string{EventTypeIMMessageEdited, EventTypeIMMessageRecalled, EventTypeIMMessageRead} {
		envelope, err := NewEnvelope(eventType, "20", IMEventPayload{EventType: EventTypeMessageEdited, ConversationID: 20})
		if err != nil {
			t.Fatalf("NewEnvelope(%s) returned error: %v", eventType, err)
		}
		if envelope.Topic() != TopicIMEvents {
			t.Fatalf("topic for %s = %q, want %q", eventType, envelope.Topic(), TopicIMEvents)
		}
	}
}
