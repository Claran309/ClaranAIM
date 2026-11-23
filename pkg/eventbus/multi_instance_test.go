package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// TestReliableHandlerClaimsOnceAcrossConcurrentInstances 模拟两个 consumer 实例并发消费同一事件。
func TestReliableHandlerClaimsOnceAcrossConcurrentInstances(t *testing.T) {
	store := NewMemoryReliabilityStore()
	var executed int64
	handlerA := NewReliableHandler(store, "websocket-gateway", 3, func(context.Context, events.Envelope) error {
		atomic.AddInt64(&executed, 1)
		return nil
	})
	handlerB := NewReliableHandler(store, "websocket-gateway", 3, func(context.Context, events.Envelope) error {
		atomic.AddInt64(&executed, 1)
		return nil
	})
	envelope := testEnvelope(t, "event-concurrent")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		handler := handlerA
		if i%2 == 1 {
			handler = handlerB
		}
		go func(h Handler) {
			defer wg.Done()
			if err := h(context.Background(), envelope); err != nil {
				t.Errorf("handler error = %v", err)
			}
		}(handler)
	}
	wg.Wait()

	if got := atomic.LoadInt64(&executed); got != 1 {
		t.Fatalf("executed = %d, want exactly 1 across instances", got)
	}
}

// TestReliableHandlerDoesNotClaimDeadLetter 验证死信事件不会被其他实例再次执行。
func TestReliableHandlerDoesNotClaimDeadLetter(t *testing.T) {
	store := NewMemoryReliabilityStore()
	var executed int64
	handler := NewReliableHandler(store, "consumer", 1, func(context.Context, events.Envelope) error {
		atomic.AddInt64(&executed, 1)
		return assertError("permanent")
	})
	envelope := testEnvelope(t, "event-dead-claim")
	if err := handler(context.Background(), envelope); err != nil {
		t.Fatalf("dead-letter delivery error = %v", err)
	}
	key := ConsumerEventKey("consumer", envelope)
	if _, ok := store.DeadLetter(key); !ok {
		t.Fatal("event should be dead letter")
	}
	if err := handler(context.Background(), envelope); err != nil {
		t.Fatalf("second delivery error = %v", err)
	}
	if got := atomic.LoadInt64(&executed); got != 1 {
		t.Fatalf("executed = %d, want 1", got)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

// TestMemoryStoreClaimStateTransitions 验证抢占 → 失败 → 重新抢占 → 成功的状态机。
func TestMemoryStoreClaimStateTransitions(t *testing.T) {
	store := NewMemoryReliabilityStore()
	ctx := context.Background()
	envelope := testEnvelope(t, "event-state")
	key := ConsumerEventKey("consumer", envelope)

	if ok, err := store.TryClaim(ctx, key, envelope); err != nil || !ok {
		t.Fatalf("first claim = (%v, %v), want true", ok, err)
	}
	if ok, err := store.TryClaim(ctx, key, envelope); err != nil || ok {
		t.Fatalf("second claim while processing = (%v, %v), want false", ok, err)
	}
	if _, err := store.RecordFailure(ctx, key, envelope, assertError("boom")); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}
	if ok, err := store.TryClaim(ctx, key, envelope); err != nil || !ok {
		t.Fatalf("claim after failure = (%v, %v), want true", ok, err)
	}
	if err := store.MarkProcessed(ctx, key, envelope); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}
	if ok, err := store.TryClaim(ctx, key, envelope); err != nil || ok {
		t.Fatalf("claim after processed = (%v, %v), want false", ok, err)
	}
}
