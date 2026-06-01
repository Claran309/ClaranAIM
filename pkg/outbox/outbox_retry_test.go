package outbox

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"testing"
)

func TestWorkerMarksDeadAfterMaxRetries(t *testing.T) {
	envelope, err := events.NewEnvelope(events.EventTypeMessageCreated, "10", events.MessagePayload{ConversationID: 10, MsgID: 99})
	if err != nil {
		t.Fatalf("NewEnvelope returned error: %v", err)
	}
	record, err := NewEvent("message", 99, envelope)
	if err != nil {
		t.Fatalf("NewEvent returned error: %v", err)
	}
	record.RetryCount = 2
	store := NewMemoryStore([]Event{record})
	worker := NewWorker(store, failingPublisher{err: errors.New("kafka unavailable")})
	worker.SetMaxRetries(3)

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce returned error: %v", err)
	}
	got := store.Event(record.ID)
	if got.Status != StatusDead {
		t.Fatalf("status = %s, want %s", got.Status, StatusDead)
	}
}

func TestMemoryStoreRequeueDeadEvent(t *testing.T) {
	record := Event{ID: 1, Status: StatusDead, RetryCount: 5, LastError: "boom"}
	store := NewMemoryStore([]Event{record})

	if err := store.Requeue(context.Background(), 1); err != nil {
		t.Fatalf("Requeue returned error: %v", err)
	}
	got := store.Event(1)
	if got.Status != StatusPending || got.RetryCount != 0 || got.LastError != "" {
		t.Fatalf("requeued event = %#v, want pending with clean retry state", got)
	}
}

type failingPublisher struct {
	err error
}

func (p failingPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	return p.err
}
