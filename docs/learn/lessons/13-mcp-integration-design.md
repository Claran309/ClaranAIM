# 第 13 课：MCP 与工具治理设计

## 学习目标

这一课学习 MCP 如何接入。你要掌握：

- MCP 应该放在哪一层。
- MCP tool 如何变成 Agent tool。
- ToolPolicy、审批、审计如何保护工具调用。
- MCP Gateway 为什么可能需要独立服务。

## 源码入口

重点阅读：

- `internal/agent-runtime-service/agent/tools.go`
- `internal/agent-runtime-service/component/middleware.go`
- `internal/agent-runtime-service/agent/agent.go`
- `internal/api-gateway/handler/agent_handler.go`
- `docs/plan.md`

## MCP 放在哪里

MCP 本质是工具生态接入，不属于 IM 消息事实。

推荐边界：

```text
agent-manager-service
  -> 保存 MCP 配置和权限策略

agent-runtime-service / MCP Gateway
  -> 连接 MCP server
  -> 转成 Eino tools
  -> 执行工具
  -> 返回结果
```

## 为什么可能需要 MCP Gateway

当工具越来越多时，runtime 直接连所有 MCP server 会变复杂。

MCP Gateway 可以负责：

- 工具注册。
- 权限校验。
- secret 管理。
- 调用审计。
- 超时和重试。
- 统一日志。

runtime 只把它当一个工具提供方。

## ToolPolicy

常见策略：

- `safe`
- `readonly`
- `approval_required`
- `disabled`

高风险工具不能只靠 prompt 约束，必须在工具调用边界校验。

## 审批闭环

当前项目已有审批 MVP：

```text
runtime 返回 pending_user_approval
gateway 保存内存 approval
用户确认
gateway 再次调用 runtime
```

未来应持久化：

- approval_id。
- action_id。
- tool_name。
- input 摘要。
- operator。
- status。
- expires_at。
- trace_id。

## Action Card

MCP 高风险动作很适合用 Action Card：

```text
Agent 想创建任务
  -> 返回审批卡
  -> 用户点击确认
  -> 后端校验权限和幂等
  -> 执行工具
  -> 更新卡片状态
```

前端当前已经能解析多种 cards/actions 字段，但服务端持久化卡片操作仍是后续任务。

## 本课检查

你应该能回答：

- MCP 为什么不应接在 msg-core-service？
- MCP Gateway 解决什么问题？
- `approval_required` 和 Action Card 如何配合？
- 工具调用审计应记录哪些字段？

## 动手任务

1. 设计 `agent_mcp_servers`。
2. 设计 `agent_tool_audit_records`。
3. 设计一个创建工单工具的审批流程。

