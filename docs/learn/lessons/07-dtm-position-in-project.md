# 第 7 课：DTM 在项目中的位置

## 学习目标

这一课学习 DTM 与 Outbox 的边界。你已经学过分布式事务基础，所以重点是判断：

- 什么流程适合 DTM。
- 什么流程适合 Outbox。
- 为什么高频消息发送不用 Saga。
- 创建群组为什么可以用 DTM 协调。

## 源码入口

重点阅读：

- `pkg/dtm/dtm.go`
- `internal/group-service/dtmbranch/handler.go`
- `internal/msg-core-service/dtmbranch/handler.go`
- `docs/ReliabilityAndEventConsistency.md`
- `docs/plan.md`

## DTM 和 Outbox 的区别

DTM：

```text
协调多个服务的低频业务动作
每个分支有正向和补偿
适合创建群组、购买配额、初始化空间
```

Outbox：

```text
保证单服务事务提交后事件最终发布
适合消息发送、群成员事件、Agent 事件
```

## 为什么消息发送不用 DTM

消息发送是高频路径。

如果用 Saga 串：

```text
写消息
推 WebSocket
触发 Agent
RAG 入库
Memory 抽取
```

会让一条消息变得极慢，而且补偿语义很怪：Agent 失败不应该导致用户消息撤销。

所以：

```text
消息事实提交成功 = 发消息成功
后续副作用 = Outbox + Kafka
```

## 什么适合 DTM

例如创建群组：

```text
group-service 创建群和成员
msg-core-service 创建群聊 conversation
```

两边都要成功，否则需要补偿。

这类低频、边界清楚、可补偿的动作更适合 DTM。

## Agent 场景中的 DTM

可能适合：

- 创建企业空间 + 默认 Agent。
- 购买 Agent 配额。
- 创建 Agent + 创建系统用户的强补偿版本。

不适合：

- 每次 Agent 回复。
- 每次工具调用。
- 每次 Memory 写入。
- 每次 RAG 检索。

## 本课检查

你应该能回答：

- Outbox 和 DTM 分别解决什么问题？
- 为什么消息发送不走 DTM？
- 什么是 Saga 补偿？
- Agent 哪些动作可能适合 DTM？

## 动手任务

1. 画出创建群组 Saga。
2. 设计购买 Agent 配额 Saga。
3. 写出消息发送选择 Outbox 的理由。

