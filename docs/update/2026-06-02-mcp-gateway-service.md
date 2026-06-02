# 2026-06-02 MCP Gateway 与远程 MCP 接入

## 背景

本次目标是把 Agent 可调用工具从零散的 runtime 本地工具，收敛到独立 `mcp-gateway-service`。MCP Gateway 负责把内部服务能力和用户自定义远程 MCP Server 暴露为标准工具，Agent runtime 只调用 MCP Gateway，不直接跨服务拼装下游能力。

## 已完成内容

### 1. 新增 mcp-gateway-service

- 新增 `idl/mcp_gateway.thrift` 和 Kitex 生成代码。
- 新增 `cmd/mcp-gateway-service/main.go`，默认监听 `127.0.0.1:9016`。
- 新增 `config/mcp-gateway-service.yaml`。
- 新增 `internal/mcp-gateway-service`：
  - `handler`：Kitex RPC 入口。
  - `service`：工具发现、工具调用、远程 MCP JSON-RPC 调用、审计记录。
  - `dao/model`：`mcp_tool_call_traces` 工具调用审计表。

### 2. 内置 MCP 工具

MCP Gateway 已内置 5 个 Agent 工具：

- `web_search`：调用 `web-search-service`，查询外部实时资料、官方文档、价格、版本、API。
- `search_memory`：调用 `memory-service`，召回用户/会话/Agent 长期记忆。
- `search_knowledge`：调用 `rag-service`，查项目文档、文件知识库和 RAG chunks。
- `query_knowledge_graph`：调用 `knowledge-service`，查实体、关系、子图、社区摘要。
- `summarize_conversation`：调用 `conversation-intelligence-service`，手动触发会话摘要/决策/任务/主题/候选记忆归档。

### 3. 远程 MCP Server 配置

`settings-service` 新增 `mcp_server_configs` 配置模型和 RPC：

- `SaveMCPServer`
- `ListMCPServers`
- `ResolveMCPServers`
- `DeleteMCPServer`

支持作用域：

- `global`
- `user`
- `agent`
- `conversation`

支持传输配置：

- `streamable_http`
- `sse`
- `stdio`

当前运行时只执行 HTTP/SSE JSON-RPC MVP；`stdio` 只保存配置，不在服务端执行。

支持安全配置：

- Secret 列表接口脱敏，只在 `ResolveMCPServers` 服务间调用中返回。
- `allow_tools_json` / `deny_tools_json` 过滤远程工具。
- `headers_json` 注入远程请求头。
- `auth_type` 支持 `bearer`、`api_key`、`basic`。
- `trust_level=low` 的远程工具标记 `requires_approval=true`。

### 4. Agent runtime 接入

`agent-runtime-service` 已连接 `mcp-gateway-service`，并把标准工具注册为 Eino 工具：

- `web_search`
- `search_memory`
- `search_knowledge`
- `query_knowledge_graph`
- `summarize_conversation`
- `list_mcp_tools`
- `call_mcp_tool`

每次工具调用都会传入：

- `user_id`
- `agent_id`
- `conversation_id`

这些字段用于 MCP Server 解析、下游权限裁剪和工具调用审计。

用户自定义的远程 MCP 工具不会在 Agent 启动时静态注册为固定工具名，而是通过通用 MCP 工具调用：

- Agent 先调用 `list_mcp_tools`，查看当前用户、Agent 和会话上下文可用的内置工具与远程工具。
- Agent 再调用 `call_mcp_tool`，传入 `tool_name` 和 `arguments_json` 执行对应工具。
- `arguments_json` 必须是合法 JSON 对象；非法 JSON 会在 Agent runtime 或 MCP Gateway 入口直接返回错误，不会静默变成空参数。

这样用户在系统设置中新增的 HTTP/SSE 远程 MCP Server，可以被前端调试入口使用，也可以被 Agent 在运行中发现和调用。

### 5. API Gateway 与前端

新增 HTTP 管理入口：

- `GET /api/v1/settings/mcp-servers`
- `POST /api/v1/settings/mcp-servers`
- `DELETE /api/v1/settings/mcp-servers/:id`
- `GET /api/v1/mcp/tools`
- `POST /api/v1/mcp/call`
- `GET /api/v1/mcp/traces`
- `GET /api/v1/mcp/traces/:trace_id`

前端系统设置新增 `MCP 工具` 标签页：

- 配置外部 MCP Server。
- 查看当前上下文可用 MCP 工具。
- 查看 MCP 工具调用审计。

### 6. 启动脚本

`scripts/start.bat` 已加入 `mcp-gateway-service`，启动顺序调整为：

1. 基础 IM / Memory / Settings / WebSearch
2. RAG / Knowledge / Conversation Intelligence
3. MCP Gateway
4. Agent Runtime
5. Agent Manager
6. API Gateway / WebSocket Gateway

这样 Agent runtime 初始化时可以发现 MCP Gateway。

## 当前限制

- 远程 MCP 当前是 HTTP/SSE JSON-RPC MVP，未实现完整 MCP session initialize、流式消息和 session 生命周期。
- `stdio` MCP 只保存配置，不执行命令。后续需要沙箱、审批、命令 allowlist、资源限制和审计后才能启用。
- 低信任远程工具只标记 `requires_approval=true`，完整持久化审批流仍待补。
- 工具调用审计已记录 trace，但还没有统一配额、失败重试、死信重放和工具市场。
- 内置工具的下游能力依赖对应服务在线，例如 `search_memory` 依赖 `memory-service`，`search_knowledge` 依赖 `rag-service`。

## 验证

已执行：

```bash
go test ./pkg/mcpclient ./internal/api-gateway/... ./cmd/api-gateway ./internal/mcp-gateway-service/... ./cmd/mcp-gateway-service ./internal/settings-service/... ./internal/agent-runtime-service/... ./cmd/agent-runtime-service
```

验证结果：通过。

覆盖重点：

- `settings-service` MCP 配置保存、列表脱敏、Resolve 返回 Secret，并覆盖 global + agent 作用域。
- `mcp-gateway-service` 内置工具列表。
- 远程 MCP `tools/list` 和 `tools/call` JSON-RPC 调用。
- Agent runtime 标准 MCP 工具注册、上下文注入、远程工具发现与通用调用。
- Agent runtime 与 MCP Gateway 均会拒绝非法 `arguments_json`，避免用户自定义工具被错误地用空参数调用。
- api-gateway MCP 配置、工具发现、调用和审计 HTTP handler 编译通过。
