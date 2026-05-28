// Package eventbus 抽象事件发布与消费能力。
// 生产环境使用 Kafka；单元测试或本地禁用 Kafka 时可以使用内存/空发布器，
// 但上层仍然走同一套 events.Envelope 契约，避免测试路径和生产路径割裂。
package eventbus

import (
	"ClaranAIM/pkg/events"
	"context"
	"sync"
)

// Publisher 是 Outbox Worker 所需的最小事件发布接口。
type Publisher interface {
	Publish(ctx context.Context, envelope events.Envelope) error
	Close() error
}

// Handler 处理消费者已经解码完成的一条事件 Envelope。
type Handler func(ctx context.Context, envelope events.Envelope) error

// NoopPublisher 接收事件但不真正投递。
// 当本地开发暂时关闭 Kafka 时，服务仍可启动并写入 Outbox，不会因为缺少 broker 阻塞主流程。
type NoopPublisher struct{}

// NewNoopPublisher 创建一个丢弃所有事件的发布器。
func NewNoopPublisher() *NoopPublisher {
	return &NoopPublisher{}
}

// Publish 丢弃事件并返回成功，用于 Kafka 未启用的降级路径。
func (p *NoopPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	return nil
}

// Close 对空发布器不需要释放资源。
func (p *NoopPublisher) Close() error {
	return nil
}

// MemoryPublisher 仅用于单元测试，记录已发布事件以便断言。
type MemoryPublisher struct {
	mu        sync.Mutex
	published []events.Envelope
}

// NewMemoryPublisher 创建测试用内存发布器。
func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{published: []events.Envelope{}}
}

// Publish 将事件记录到内存切片，便于测试断言。
func (p *MemoryPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, envelope)
	return nil
}

// Published 返回已记录事件的副本，避免调用方直接修改内部状态。
func (p *MemoryPublisher) Published() []events.Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]events.Envelope, len(p.published))
	copy(out, p.published)
	return out
}

// Reset 清空已记录事件，便于同一个测试对象复用。
func (p *MemoryPublisher) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = nil
}

// Close 对内存发布器不需要释放资源。
func (p *MemoryPublisher) Close() error {
	return nil
}
