# 第 15 课：最终综合项目

## 学习目标

这一课把项目串起来。你可以选择一个方向做真实设计或实现，目标是同时覆盖：

- IM 事实源。
- Outbox/Kafka。
- Agent-native 事件。
- Memory/RAG/Tool。
- 权限、幂等、审计。

## 方向一：Agent 审计面板

最贴近当前代码。

目标：

- 查询 `agent_audit_records`。
- 查询 `agent_dispatch_records`。
- 按 trace id 串事件、决策、runtime、回复。
- 展示为什么触发、为什么沉默、为什么失败。

交付：

- API。
- 查询条件。
- 前端列表。
- trace 详情。
- 测试。

## 方向二：持久化审批与 Action Card

目标：

- 把当前 gateway 内存 approval 迁移到持久化。
- Action Card 点击走后端幂等。
- 支持确认、拒绝、超时。

交付：

- approval 表。
- action_id / idempotency_key。
- 权限校验。
- 卡片状态更新。
- 审计。

## 方向三：完整 rag-service MVP

目标：

- 消费 `file.uploaded`。
- 解析文件。
- chunk。
- embedding。
- 向量检索。
- runtime RAG tool 调用 rag-service。

交付：

- rag_documents。
- rag_chunks。
- rag_ingest_jobs。
- SearchKnowledge RPC/HTTP。
- 权限过滤。

## 方向四：Memory 自动抽取

目标：

- 从 Agent 运行结果或 IM 事件中抽取 memory candidate。
- 用户确认后写入 `memory_facts`。
- 支持私有/共享可见性。

交付：

- candidate 表。
- 确认 API。
- 用户治理页面。
- Agent 调用前召回。

## 方向五：MCP Gateway

目标：

- 统一管理 MCP server。
- runtime 通过 Gateway 获取工具。
- 工具调用有权限、审批、审计。

交付：

- MCP server 配置。
- ToolPolicy。
- Tool audit。
- approval_required 链路。

## 推荐路线

如果你已经学到 lesson3，建议后续按这个优先级做：

1. 先做 Agent 审计面板。
2. 再做持久化审批与 Action Card。
3. 再做完整 rag-service MVP。
4. 最后做 MCP Gateway。

原因：

- 审计面板最贴当前 Agent-native 代码。
- 审批和 Action Card 能把前端体验闭环补齐。
- RAG 能补你还没学的向量数据库。
- MCP 是工具生态的下一阶段。

## 最终设计模板

```text
1. 背景
2. 当前代码入口
3. 数据流图
4. 表结构
5. API/RPC
6. 权限
7. 幂等
8. 失败重试
9. 审计
10. 测试
```

## 本课检查

你应该能回答：

- 你的方案哪个服务是事实源？
- 哪些副作用走 Outbox？
- 幂等键是什么？
- 谁有权限执行？
- 失败后如何重试或补偿？

