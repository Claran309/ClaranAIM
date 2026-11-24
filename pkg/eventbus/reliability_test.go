package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"testing"
)

func testEnvelope(t *testing.T, eventID string) events.Envelope {
	t.Helper()
	envelope, err := events.NewEnvelope("message.created", "conversation-1", map[string]string{"content": "hello"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	if eventID != "" {
		envelope.EventID = eventID
	}
	return envelope
}

func TestReliableHandlerSkipsDuplicateDelivery(t *testing.T) {
	store := NewMemoryReliabilityStore()
	calls := 0
	handler := NewReliableHandler(store, "msg-core-service", 3, func(context.Context, events.Envelope) error {
		calls++
		return nil
	})
	envelope := testEnvelope(t, "event-1")

	for i := 0; i < 3; i++ {
		if err := handler(context.Background(), envelope); err != nil {
			t.Fatalf("delivery %d error = %v", i+1, err)
		}
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 for duplicate delivery", calls)
	}
	key := ConsumerEventKey("msg-core-service", envelope)
	if ok, err := store.IsProcessed(context.Background(), key); err != nil || !ok {
		t.Fatalf("IsProcessed() = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestReliableHandlerRetriesUntilSuccess(t *testing.T) {
	store := NewMemoryReliabilityStore()
	calls := 0
	handler := NewReliableHandler(store, "msg-core-service", 3, func(context.Context, events.Envelope) error {
		calls++
		if calls < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})
	envelope := testEnvelope(t, "event-retry")

	if err := handler(context.Background(), envelope); err == nil {
		t.Fatal("first delivery should return error for retry")
	}
	if err := handler(context.Background(), envelope); err == nil {
		t.Fatal("second delivery should return error for retry")
	}
	if err := handler(context.Background(), envelope); err != nil {
		t.Fatalf("third delivery error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("handler calls = %d, want 3", calls)
	}
	if err := handler(context.Background(), envelope); err != nil {
		t.Fatalf("duplicate after success error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("handler calls after duplicate = %d, want 3", calls)
	}
}

func TestReliableHandlerMovesToDeadLetterAfterMaxAttempts(t *testing.T) {
	store := NewMemoryReliabilityStore()
	calls := 0
	handler := NewReliableHandler(store, "msg-core-service", 2, func(context.Context, events.Envelope) error {
		calls++
		return errors.New("permanent failure")
	})
	envelope := testEnvelope(t, "event-dead")

	if err := handler(context.Background(), envelope); err == nil {
		t.Fatal("first delivery should return error")
	}
	if err := handler(context.Background(), envelope); err != nil {
		t.Fatalf("delivery reaching max attempts should return nil, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2", calls)
	}
	key := ConsumerEventKey("msg-core-service", envelope)
	record, ok := store.DeadLetter(key)
	if !ok {
		t.Fatal("event should be in dead letter")
	}
	if record.Attempts != 2 || record.State != DeliveryStateDead {
		t.Fatalf("dead letter record = %#v", record)
	}
}

func TestReliableHandlerWithoutStoreCallsNext(t *testing.T) {
	calls := 0
	handler := NewReliableHandler(nil, "consumer", 3, func(context.Context, events.Envelope) error {
		calls++
		return nil
	})
	if err := handler(context.Background(), testEnvelope(t, "event-nil-store")); err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestConsumerEventKeyFallsBackToTypeAndKey(t *testing.T) {
	envelope := testEnvelope(t, "")
	envelope.EventID = ""
	if got, want := ConsumerEventKey("consumer", envelope), "consumer:message.created:conversation-1"; got != want {
		t.Fatalf("ConsumerEventKey() = %q, want %q", got, want)
	}
}
