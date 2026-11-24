package outbox

import (
	"ClaranAIM/pkg/events"
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
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

type orderPublisher struct {
	mu    sync.Mutex
	byKey map[string][]int64
}

func (p *orderPublisher) Publish(_ context.Context, envelope events.Envelope) error {
	id, _ := strconv.ParseInt(envelope.EventID, 10, 64)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byKey == nil {
		p.byKey = map[string][]int64{}
	}
	p.byKey[envelope.Key] = append(p.byKey[envelope.Key], id)
	// 放大发布耗时，确保并发分片真的交错执行。
	time.Sleep(2 * time.Millisecond)
	return nil
}

func (p *orderPublisher) Close() error { return nil }

func testEventWithKey(t *testing.T, id int64, key string) Event {
	t.Helper()
	envelope, err := events.NewEnvelope("message.created", key, map[string]string{"content": "hello"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	envelope.EventID = strconv.FormatInt(id, 10)
	record, err := NewEvent("message", id, envelope)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	return record
}

// TestWorkerConcurrentPublishKeepsPerKeyOrder 验证并发发布不会打乱同一会话的事件顺序。
func TestWorkerConcurrentPublishKeepsPerKeyOrder(t *testing.T) {
	t.Setenv("OUTBOX_CONCURRENCY", "4")
	t.Setenv("OUTBOX_BATCH_SIZE", "100")

	keys := []string{"conversation-a", "conversation-b", "conversation-c", "conversation-d"}
	var records []Event
	var id int64 = 1
	for i := 0; i < 10; i++ {
		for _, key := range keys {
			records = append(records, testEventWithKey(t, id, key))
			id++
		}
	}
	store := NewMemoryStore(records)
	publisher := &orderPublisher{}
	worker := NewWorker(store, publisher)

	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if len(publisher.byKey) != len(keys) {
		t.Fatalf("published keys = %d, want %d", len(publisher.byKey), len(keys))
	}
	for _, key := range keys {
		got := publisher.byKey[key]
		if len(got) != 10 {
			t.Fatalf("key %s published %d events, want 10", key, len(got))
		}
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Fatalf("key %s out of order: %v", key, got)
			}
		}
	}
}
