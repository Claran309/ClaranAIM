// Package eventbus abstracts event publication and consumption.
//
// Production uses Kafka; tests can use memory/no-op publishers while still
// exercising the same event envelope contract.
package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"sync"
)

// Publisher is the minimal publish/close contract used by outbox workers.
type Publisher interface {
	Publish(ctx context.Context, envelope events.Envelope) error
	Close() error
}

// Handler processes one decoded event envelope from a consumer.
type Handler func(ctx context.Context, envelope events.Envelope) error

// NoopPublisher accepts events and drops them. It is useful when Kafka is
// disabled but services still need to run and write outbox rows.
type NoopPublisher struct{}

// NewNoopPublisher creates a drop-all publisher.
func NewNoopPublisher() *NoopPublisher {
	return &NoopPublisher{}
}

// Publish drops the event and returns success.
func (p *NoopPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	return nil
}

// Close releases no resources for the no-op publisher.
func (p *NoopPublisher) Close() error {
	return nil
}

// MemoryPublisher 仅用于单元测试，记录已发布事件以便断言。
type MemoryPublisher struct {
	mu        sync.Mutex
	published []events.Envelope
}

// NewMemoryPublisher creates an in-memory publisher for tests.
func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{published: []events.Envelope{}}
}

// Publish records an event in memory.
func (p *MemoryPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, envelope)
	return nil
}

// Published returns a copy of all recorded events.
func (p *MemoryPublisher) Published() []events.Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]events.Envelope, len(p.published))
	copy(out, p.published)
	return out
}

// Reset clears recorded events.
func (p *MemoryPublisher) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = nil
}

// Close releases no resources for the memory publisher.
func (p *MemoryPublisher) Close() error {
	return nil
}
