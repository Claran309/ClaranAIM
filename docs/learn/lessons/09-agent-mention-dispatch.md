# 第 9 课：Agent-native 事件分发

## 学习目标

这一课学习 `AgentEventDispatcher`。你要掌握：

- 它消费哪些 topic。
- 它如何把事件转换为 Agent 工作。
- 默认触发和订阅规则如何合并。
- `record` 和 `trigger` 的区别。
- 审计、调度幂等、回复幂等如何协作。

## 源码入口

重点阅读：

- `internal/agent-manager-service/eventconsumer/agent_consumer.go`
- `internal/agent-manager-service/model/model.go`
- `internal/agent-manager-service/dao/dao.go`
- `pkg/events/events.go`
- `internal/msg-core-service/service/service.go`

## Dispatcher 启动

agent-manager-service 会消费：

```text
claran.message.events
claran.im.events
```

迁移期保留双 topic：

- 旧 message events 兼容 WebSocket 和旧 @Agent。
- 新 IM events 承载 Agent-native 统一事件。

## decodeAgentEvent

传统消息事件解析成 `MessagePayload`：

```text
message.created
message.edited
message.recalled
```

统一 IM 事件解析成 `IMEventPayload`：

```text
im.message.edited
im.message.recalled
im.message.read
file.uploaded
voice.transcribed
group.member_joined
group.member_left
system.notice
task.changed
```

## 默认触发

私聊：

```text
如果 Agent 是另一个参与者，默认 trigger
```

群聊：

```text
默认只响应 MentionUserIDs 中的 Agent
```

这能避免群聊里每条消息都叫醒 Agent。

## 订阅规则触发

`agent_subscription_rules` 支持：

- all
- keyword
- command
- mention

动作：

- `trigger`
- `record`

如果规则 `Silent=true`，则强制转为 `record`。

## 决策合并

同一个 Agent 可能被多个规则命中。合并时优先级：

```text
trigger > record > 其他
```

这样如果一个规则说记录，另一个规则说触发，最终会触发。

## record

record 只写审计，不调用 runtime，不回复。

适合：

- 文件上传静默记录。
- 群成员变化记录。
- 未来 Memory/RAG 入库。

## trigger

trigger 会：

```text
写 audit
写 dispatch record
读取 Agent 可见历史
调用 agent-runtime-service
以 AgentUserID 写回 msg-core-service
标记 completed
```

## 幂等

业务事件 ID：

```text
优先 IMEventPayload.IdempotencyKey
否则 envelope.EventID
```

调度幂等：

```text
agent_dispatch_records(event_id, agent_user_id)
```

回复幂等：

```text
client_msg_id = agent:{sourceEventID}:{agentUserID}
```

## Agent 上下文

Dispatcher 调 runtime 前，会用 Agent 用户身份读取最近历史：

```text
GetHistory(conversation_id, user_id=agent_user_id, limit=80)
```

这很关键：Agent 只能基于自己有权看到的消息回答。

## 本课检查

你应该能回答：

- 为什么要同时消费 message topic 和 im topic？
- record 和 trigger 有什么区别？
- 私聊和群聊默认触发规则不同在哪里？
- 为什么读历史要用 AgentUserID？
- 三层幂等分别防什么？

## 动手任务

1. 画出 @Agent 触发链路。
2. 画出 `file.uploaded -> record` 链路。
3. 设计 `agent_keyword` 的匹配测试。
4. 设计重复 Kafka 事件的幂等测试。

