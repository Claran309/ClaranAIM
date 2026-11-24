package outbox

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryStore 是 Outbox Store 的测试实现，也可用于无数据库的本地演示。
type MemoryStore struct {
	mu     sync.Mutex
	events map[int64]Event
}

// NewMemoryStore 创建内存 Outbox 存储。
func NewMemoryStore(records []Event) *MemoryStore {
	store := &MemoryStore{events: map[int64]Event{}}
	for _, record := range records {
		store.events[record.ID] = record
	}
	return store
}

func (s *MemoryStore) FetchDue(ctx context.Context, limit int, lockFor time.Duration) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := make([]Event, 0)
	for _, record := range s.events {
		if record.Status != StatusPending && record.Status != StatusRetrying {
			continue
		}
		if record.NextRetryAt.After(now) {
			continue
		}
		out = append(out, record)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	// 与 GormStore 的 ORDER BY next_retry_at, id 保持一致，保证测试可复现。
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// CountBacklog 返回内存中待发布事件数量。
func (s *MemoryStore) CountBacklog(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for _, record := range s.events {
		if record.Status == StatusPending || record.Status == StatusRetrying {
			count++
		}
	}
	return count, nil
}

func (s *MemoryStore) MarkPublished(ctx context.Context, id int64) error {
	return s.update(id, func(record *Event) {
		now := time.Now()
		record.Status = StatusPublished
		record.PublishedAt = &now
		record.LockedUntil = nil
		record.LastError = ""
		record.UpdatedAt = now
	})
}

func (s *MemoryStore) MarkRetry(ctx context.Context, id int64, publishErr error) error {
	return s.update(id, func(record *Event) {
		record.Status = StatusRetrying
		record.RetryCount++
		record.LastError = publishErr.Error()
		record.NextRetryAt = time.Now().Add(backoff(record.RetryCount))
		record.LockedUntil = nil
		record.UpdatedAt = time.Now()
	})
}

func (s *MemoryStore) MarkDead(ctx context.Context, id int64, publishErr error) error {
	return s.update(id, func(record *Event) {
		record.Status = StatusDead
		record.LastError = publishErr.Error()
		record.LockedUntil = nil
		record.UpdatedAt = time.Now()
	})
}

func (s *MemoryStore) Requeue(ctx context.Context, id int64) error {
	return s.update(id, func(record *Event) {
		record.Status = StatusPending
		record.RetryCount = 0
		record.LastError = ""
		record.NextRetryAt = time.Now()
		record.LockedUntil = nil
		record.PublishedAt = nil
		record.UpdatedAt = time.Now()
	})
}

// Event 返回指定事件副本，便于测试断言。
func (s *MemoryStore) Event(id int64) Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events[id]
}

func (s *MemoryStore) update(id int64, mutate func(*Event)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.events[id]
	if !ok {
		return fmt.Errorf("outbox event %d not found", id)
	}
	mutate(&record)
	s.events[id] = record
	return nil
}
