package outbox

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"testing"
)

type fakePublisher struct {
	err   error
	calls int
}

func (p *fakePublisher) Publish(_ context.Context, _ events.Envelope) error {
	p.calls++
	return p.err
}

func (p *fakePublisher) Close() error { return nil }

func testOutboxEvent(t *testing.T, id int64) Event {
	t.Helper()
	envelope, err := events.NewEnvelope("message.created", "conversation-1", map[string]string{"content": "hello"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	record, err := NewEvent("message", 1, envelope)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	record.ID = id
	return record
}

func TestWorkerPublishesPendingEvent(t *testing.T) {
	store := NewMemoryStore([]Event{testOutboxEvent(t, 1)})
	publisher := &fakePublisher{}
	worker := NewWorker(store, publisher)

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if publisher.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", publisher.calls)
	}
	if got := store.Event(1); got.Status != StatusPublished || got.PublishedAt == nil {
		t.Fatalf("event after publish = %#v", got)
	}
}

func TestWorkerMarksRetryOnPublishFailure(t *testing.T) {
	store := NewMemoryStore([]Event{testOutboxEvent(t, 2)})
	worker := NewWorker(store, &fakePublisher{err: errors.New("kafka unavailable")})

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	got := store.Event(2)
	if got.Status != StatusRetrying || got.RetryCount != 1 {
		t.Fatalf("event after failure = %#v, want retrying with 1 retry", got)
	}
	if got.LastError != "kafka unavailable" {
		t.Fatalf("LastError = %q", got.LastError)
	}
}

func TestWorkerMarksDeadAfterMaxRetries(t *testing.T) {
	store := NewMemoryStore([]Event{testOutboxEvent(t, 3)})
	worker := NewWorker(store, &fakePublisher{err: errors.New("permanent")})
	worker.SetMaxRetries(1)

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if got := store.Event(3); got.Status != StatusDead {
		t.Fatalf("event status = %q, want dead", got.Status)
	}
}

func TestWorkerMarksRetryForUndecodablePayload(t *testing.T) {
	record := testOutboxEvent(t, 4)
	record.Payload = "not-json"
	store := NewMemoryStore([]Event{record})
	worker := NewWorker(store, &fakePublisher{})

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	got := store.Event(4)
	if got.Status != StatusRetrying || got.RetryCount != 1 {
		t.Fatalf("event after decode failure = %#v", got)
	}
}
