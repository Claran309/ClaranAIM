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
