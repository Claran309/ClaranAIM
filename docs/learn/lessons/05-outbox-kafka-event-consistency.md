# 第 5 课：Outbox、Kafka、EventBus 与事件一致性

## 学习目标

这一课学习项目里的事件基础设施。你要掌握：

- 事件是什么。
- EventBus 抽象解决什么问题。
- Kafka 在项目中负责什么。
- Outbox 解决哪个崩溃窗口。
- 消费者为什么必须幂等。

## 源码入口

重点阅读：

- `pkg/events/events.go`
- `pkg/eventbus/eventbus.go`
- `pkg/eventbus/kafka.go`
- `pkg/outbox/outbox.go`
- `pkg/outbox/worker_test.go`
- `internal/msg-core-service/service/service.go`
- `internal/group-service/service/service.go`

## 事件是什么

事件是已经发生的业务事实。

示例：

```text
message.created
message.edited
group.created
group.member_invited
file.uploaded
voice.transcribed
agent.completed
```

不要把事件当命令。命令是“请创建消息”，事件是“消息已经创建”。

## Envelope

所有事件都用统一外壳：

```go
type Envelope struct {
    EventID    string
    Type       string
    Version    int
    Key        string
    OccurredAt string
    Payload    json.RawMessage
}
```

你可以理解为：

```text
Envelope = 快递盒
Type     = 快递类型
Key      = Kafka 分区键
Payload  = 业务内容
```

## Topic 分工

```text
claran.group.events
  -> 群生命周期事件

claran.message.events
  -> 消息实时推送事件

claran.im.events
  -> Agent-native IM 统一事件

claran.agent.events
  -> Agent 运行和审计事件
```

`Envelope.Topic()` 根据事件类型决定发往哪个 topic。

## EventBus 模式

EventBus 在项目中是一个抽象层：

```go
type Publisher interface {
    Publish(ctx context.Context, envelope events.Envelope) error
    Close() error
}
```

实现：

- `KafkaPublisher`
- `MemoryPublisher`
- `NoopPublisher`

好处：

- 业务和 Kafka client 解耦。
- 测试可以用内存 publisher。
- 后续换 MQ 成本更低。
- 所有事件统一走 Envelope。

在当前项目里，业务服务不是直接调用 Kafka，而是：

```text
业务事务写 event_outbox
  -> outbox.Worker
  -> eventbus.Publisher
  -> KafkaPublisher
```

## 为什么需要 Outbox

如果直接：

```text
写 MySQL
发 Kafka
```

会有崩溃窗口：

```text
MySQL 提交成功
服务在发 Kafka 前崩溃
事件丢失
```

Outbox 做法：

```text
同一事务：
  写业务表
  写 event_outbox

后台 worker：
  扫描 pending/retrying
  发布 Kafka
  成功标记 published
  失败标记 retrying
```

## event_outbox 关键字段

```text
aggregate_type
aggregate_id
event_type
event_key
payload
status
retry_count
last_error
next_retry_at
locked_until
published_at
```

`locked_until` 用来避免多个 worker 抢同一批事件。

## Kafka 消费语义

`KafkaConsumer.Run` 逻辑：

```text
FetchMessage
  -> DecodeEnvelope
  -> handler(ctx, envelope)
  -> handler 成功后 CommitMessages
```

如果 handler 失败，不提交 offset，后续会重试。

这意味着消费者会遇到重复事件。消费者必须幂等。

## 当前幂等策略

Agent 链路：

- `IMEventPayload.IdempotencyKey`
- `agent_dispatch_records(event_id, agent_user_id)`
- Agent 回复 `client_msg_id`

消息链路：

- 客户端发送用 `client_msg_id`。
- 前端渲染用 `id/msg_id/client_msg_id` 去重。

Outbox：

- 重试发布事件。
- 不保证消费者只处理一次。

## 本课检查

你应该能回答：

- 事件和命令有什么区别？
- EventBus 和 Kafka 是什么关系？
- Outbox 解决什么崩溃窗口？
- 为什么消费者必须幂等？
- `claran.message.events` 和 `claran.im.events` 有什么区别？

## 动手任务

1. 追踪 `outbox.NewEvent`。
2. 追踪 `KafkaPublisher.Publish`。
3. 追踪 `KafkaConsumer.Run`。
4. 设计一个 `processed_events` 表。

