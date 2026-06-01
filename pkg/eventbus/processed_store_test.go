package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"testing"
)

func TestReliableHandlerSkipsAlreadyProcessedEvent(t *testing.T) {
	store := NewMemoryReliabilityStore()
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{ConversationID: 10, MsgID: 99})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	if err := store.MarkProcessed(context.Background(), ConsumerEventKey("websocket-gateway", envelope), envelope); err != nil {
		t.Fatalf("MarkProcessed returned error: %v", err)
	}

	calls := 0
	handler := NewReliableHandler(store, "websocket-gateway", 3, func(ctx context.Context, got events.Envelope) error {
		calls++
		return nil
	})

	if err := handler(context.Background(), envelope); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0 for processed event", calls)
	}
}

func TestReliableHandlerMovesPermanentFailureToDLQ(t *testing.T) {
	store := NewMemoryReliabilityStore()
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{ConversationID: 10, MsgID: 99})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	boom := errors.New("decode permanent failure")
	handler := NewReliableHandler(store, "websocket-gateway", 1, func(ctx context.Context, got events.Envelope) error {
		return boom
	})

	err = handler(context.Background(), envelope)
	if err != nil {
		t.Fatalf("handler returned error = %v, want nil after DLQ handoff", err)
	}
	dead, ok := store.DeadLetter(ConsumerEventKey("websocket-gateway", envelope))
	if !ok {
		t.Fatal("expected dead letter record")
	}
	if dead.ErrorMessage != boom.Error() {
		t.Fatalf("dead letter error = %q, want %q", dead.ErrorMessage, boom.Error())
	}
}
