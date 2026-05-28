# 第 6 课：WebSocket 实时推送与前端补偿

## 学习目标

这一课学习实时推送。你要掌握：

- websocket-gateway 和 api-gateway 为什么分开。
- WebSocket 推送为什么不是事实源。
- Kafka 消息事件如何推给前端。
- 前端如何去重和补偿。
- 最新前端 Markdown 表格与 Action Card 解析增强。

## 源码入口

重点阅读：

- `internal/websocket-gateway/hub/hub.go`
- `internal/websocket-gateway/handler/ws_handler.go`
- `internal/websocket-gateway/eventconsumer/message_consumer.go`
- `cmd/websocket-gateway/main.go`
- `dist/js/app.js`

## WebSocket 网关职责

websocket-gateway 负责：

- WebSocket 握手和认证。
- 用户多连接管理。
- 消费 `claran.message.events`。
- 推送在线用户。

它不负责：

- 写消息。
- 保存历史。
- 做最终可靠投递。
- 运行 Agent。

## 推送链路

```text
msg-core-service
  -> event_outbox(message.created)
  -> outbox worker
  -> Kafka claran.message.events
  -> websocket-gateway consumer
  -> Hub.Broadcast(TargetUserIDs)
  -> 前端
```

如果用户离线，WebSocket 推送自然失败或不存在。用户上线后通过历史消息接口补齐。

## 前端去重

最新前端增加了 `messageIdentity`：

```text
优先 id/msg_id
其次 client_msg_id
```

原因：

- 前端可能乐观渲染用户刚发的消息。
- WebSocket 后面又推回来一次。
- Kafka 也可能重复。

所以渲染时要去重，不然会显示两条。

## Markdown 表格

前端 Markdown 渲染现在支持表格：

```markdown
| 字段 | 说明 |
| --- | --- |
| id | 消息 ID |
```

这对 Agent 回复很重要，因为 Agent 经常输出结构化总结。

## Action Card 解析增强

前端现在可以解析：

```text
cards
action_cards
actionCards
action_decisions
decisions
actions
```

并且可以处理：

- 直接 JSON 对象。
- JSON 字符串。
- ```json fenced code block。

这解决了模型经常把结构化结果包在 Markdown 代码块里的问题。

## 好友分组折叠

前端新增好友分组折叠状态：

```text
claran_friend_group_collapsed
```

这和会话分组折叠类似，属于体验层增强。

## 本课检查

你应该能回答：

- WebSocket 为什么不作为事实源？
- 前端为什么要用 message id/client_msg_id 去重？
- 离线用户如何补消息？
- Action Card 为什么要兼容多种字段名？

## 动手任务

1. 追踪 `StartMessageEventConsumer`。
2. 追踪 `appendMessage` 的去重逻辑。
3. 构造一个 Markdown 表格消息，看前端如何渲染。
4. 构造一个 fenced JSON Action Card，看前端如何解析。

