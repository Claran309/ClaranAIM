# 第 11 课：Memory 与当前轻量 RAG

## 学习目标

这一课把 Memory 和当前 RAG MVP 放在一起学。你要掌握：

- memory-service 保存什么。
- memory、IM history、RAG 的区别。
- agent-manager 如何在调用 runtime 前召回记忆。
- 当前 RAG MVP 的实现边界。

## 源码入口

重点阅读：

- `internal/memory-service/model/model.go`
- `internal/memory-service/dao/dao.go`
- `internal/memory-service/service/service.go`
- `internal/memory-service/transport/http.go`
- `pkg/memoryclient`
- `internal/agent-manager-service/service/service.go`
- `internal/agent-runtime-service/graphTool/rag.go`

## Memory 是什么

Memory 保存的是可治理事实，不是聊天记录原文。

例如：

- 用户偏好。
- 用户表达风格。
- 长期目标。
- 群背景。
- 项目状态。
- 会话摘要。
- Agent 运行摘要。

表：

```text
memory_facts
```

关键字段：

- `BotID`
- `UserID`
- `OwnerUserID`
- `GroupID`
- `ConversationID`
- `SessionID`
- `Scope`
- `Type`
- `Visibility`
- `Enabled`
- `VectorStatus`
- `EmbeddingRef`

## Memory 与 IM History

IM History：

- 原始消息。
- 来自 msg-core/msg-history。
- 是事实记录。
- 按权限查询。

Memory：

- 从消息或 Agent 运行中提炼出的事实。
- 可编辑、可删除、可关闭。
- 是 Agent 个性化上下文。

不要把 memory 当成聊天历史的替代品。

## Memory 与 RAG

RAG：

- 面向文档/知识片段检索。
- 需要 chunk、embedding、索引、引用。

Memory：

- 面向用户/群/会话的长期事实。
- 重点是治理和个性化。

未来二者可能都向量化，但语义不同。

## 用户治理

api-gateway 暴露：

```text
GET    /memory/list
POST   /memory/create
PUT    /memory/:id
DELETE /memory/:id
```

这很重要：Agent 记忆不能变成黑箱。用户必须能看、改、删。

## 当前轻量 RAG

runtime 中的 RAG MVP 大致是：

```text
读取文档
切分 chunk
LLM 评分
TopK
返回引用上下文
```

它没有：

- 向量数据库。
- 独立 rag-service。
- 文档入库任务。
- 权限索引。
- reranker。

适合学习 RAG 流程，但不是最终方案。

## 本课检查

你应该能回答：

- memory_facts 保存什么？
- memory 和聊天历史有什么区别？
- 为什么用户必须能删除记忆？
- 当前 RAG MVP 为什么不适合大规模？
- `VectorStatus` 和 `EmbeddingRef` 暗示了什么演进方向？

## 动手任务

1. 设计一条用户偏好 memory。
2. 设计一条群背景 memory。
3. 画出 Agent 调用前召回 memory 的链路。
4. 把当前 RAG MVP 画成 DAG。

