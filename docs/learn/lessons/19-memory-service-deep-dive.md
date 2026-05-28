# 第 19 课：Memory Service 深入

## 学习目标

这一课深入 memory-service。你要掌握：

- `memory_facts` 的字段含义。
- scope、type、visibility 的组合方式。
- Agent 调用前如何召回记忆。
- 用户治理记忆为什么是必要能力。
- 未来向量化记忆如何演进。

## 源码入口

重点阅读：

- `internal/memory-service/model/model.go`
- `internal/memory-service/dao/dao.go`
- `internal/memory-service/service/service.go`
- `internal/memory-service/transport/http.go`
- `pkg/memoryclient`
- `internal/api-gateway/handler/memory_handler.go`
- `internal/agent-manager-service/service/service.go`

## memory_facts 字段

关键字段：

```text
BotID
UserID
OwnerUserID
GroupID
ConversationID
SessionID
Scope
Type
Title
Content
Source
Visibility
Enabled
VectorStatus
EmbeddingRef
Confidence
LastUsedAt
```

## Scope

```text
user
group
conversation
session
```

含义：

- user：用户长期偏好。
- group：群背景。
- conversation：某会话上下文。
- session：某次 Agent 长会话。

## Type

```text
preference
speaking_style
long_term_goal
group_profile
project_state
chat_summary
agent_run_summary
```

这些类型帮助 Agent 区分记忆用途，而不是把所有东西都塞成一段文本。

## Visibility

```text
private
shared
```

默认私有很重要。用户画像、发言风格、长期目标不应默认暴露给群或管理员。

## VectorStatus

当前预留：

```text
pending
disabled
ready
```

这说明 memory-service 未来可以接 embedding，但现在仍是 MySQL MVP。

## 用户治理

用户必须能：

- 查看记忆。
- 创建记忆。
- 编辑记忆。
- 删除记忆。
- 禁用记忆。

否则 Agent 记忆会变成不可控黑箱。

## 本课检查

你应该能回答：

- Memory 和 RAG 的区别是什么？
- scope 为什么不能只有 user？
- speaking_style 为什么默认应私有？
- VectorStatus 有什么演进意义？

## 动手任务

1. 写一条 private user preference。
2. 写一条 shared group_profile。
3. 设计记忆自动抽取候选表。
4. 设计用户删除记忆后的 Agent 行为。

