# 第 10 课：Agent Runtime 与 Eino DeepAgent

## 学习目标

这一课学习 `agent-runtime-service`。你要掌握：

- runtime 和 manager 的边界。
- Eino Agent 如何被构建和运行。
- 工具、Skill、RAG MVP、长会话如何接入。
- 用户审批 MVP 如何从网关转回 runtime。

## 源码入口

重点阅读：

- `cmd/agent-runtime-service/main.go`
- `internal/agent-runtime-service/service/service.go`
- `internal/agent-runtime-service/agent/agent.go`
- `internal/agent-runtime-service/agent/tools.go`
- `internal/agent-runtime-service/component/middleware.go`
- `internal/agent-runtime-service/component/memory.go`
- `internal/agent-runtime-service/graphTool/rag.go`
- `internal/api-gateway/handler/agent_handler.go`

## runtime 负责什么

runtime 负责：

- Eino Agent 执行。
- 长会话。
- Tool/Skill。
- RAG MVP。
- 总结、问答、洞察、候选回复。
- 返回 usage。

runtime 不负责：

- Agent 权限。
- Agent 订阅规则。
- Agent 审计表。
- IM 消息事实。

## manager 到 runtime

```text
agent-manager-service
  -> 校验 Agent 配置和权限
  -> 准备 runtime config
  -> 调 agent-runtime-service
  -> 记录计费
```

runtime 是执行面，manager 是管理面。

## 长会话

runtime 使用 session key 和 JSONL 存储维护基础上下文。

网关提供：

```text
GET /agent/:id/sessions
```

查询前先通过 agent-manager-service 做权限检查，避免 session 泄露。

## 工具与安全

runtime 的工具能力包括：

- web search。
- answer_from_document/RAG。
- 本地工具。
- Skill。

安全重点：

- workspace root。
- ToolPolicy。
- SafeToolMiddleware。
- 高风险动作审批。

## 审批 MVP

当前审批记录在 api-gateway 进程内存中：

```text
agent-ask
  -> runtime 返回 pending_user_approval
  -> gateway 保存 approval
  -> 前端展示确认
  -> /agent/approval/confirm
  -> gateway 组织确认消息继续调用 runtime
```

这是 MVP。生产化应迁移到 runtime 或 manager 的持久化 checkpoint。

## RAG MVP

当前 RAG 在 runtime 内：

```text
读取文档
切分
LLM 评分
TopK
返回上下文
```

不是真正向量数据库 RAG，适合学习流程，不适合大规模知识库。

## 本课检查

你应该能回答：

- manager 和 runtime 为什么拆开？
- session 为什么要做权限检查？
- 审批 MVP 的不足是什么？
- ToolPolicy 应该约束什么？
- 当前 RAG MVP 和向量 RAG 有什么区别？

## 动手任务

1. 追踪 `/agent/chat`。
2. 追踪 `/agent/approval/confirm`。
3. 追踪 runtime RAG tool。
4. 设计一个持久化审批表。

