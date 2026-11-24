package outbox

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreCountsPendingAndRetryingBacklog(t *testing.T) {
	records := []Event{
		{ID: 1, Status: StatusPending, NextRetryAt: time.Now()},
		{ID: 2, Status: StatusRetrying, NextRetryAt: time.Now()},
		{ID: 3, Status: StatusPublished},
		{ID: 4, Status: StatusDead},
	}
	store := NewMemoryStore(records)
	count, err := store.CountBacklog(context.Background())
	if err != nil {
		t.Fatalf("CountBacklog() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("backlog = %d, want 2 pending+retrying", count)
	}
}

func TestWorkerProcessesBacklogWithoutError(t *testing.T) {
	store := NewMemoryStore([]Event{testOutboxEvent(t, 11)})
	worker := NewWorker(store, &fakePublisher{})
	if err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	count, err := store.CountBacklog(context.Background())
	if err != nil {
		t.Fatalf("CountBacklog() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("backlog after publish = %d, want 0", count)
	}
}
