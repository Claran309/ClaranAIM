# 2026-06-03 运行端口与 Agent 鉴权修复

## 背景

本次排查来自本地启动日志中的三类现象：

- `api-gateway:8080`、`websocket-gateway:8081`、`rag-service:9112`、`knowledge-service:9113`、`web-search-service:9114` 出现 `bind: Only one usage of each socket address`。
- Agent 事件响应失败，runtime 返回 `401 Unauthorized / 身份验证失败`。
- `GetBotByAgentUserID` 对普通用户查询时打印 `record not found`，容易被误判为业务错误。

## 根因

- 端口绑定失败不是服务内部崩溃，而是同一端口已有旧进程在监听；再次执行 `go run` 会启动第二个实例，第二个实例自然绑定失败。
- 报错 Agent 是 `internal` 类型，但数据库中保存的是创建时写入的旧平台 API Key。后续 `.env` 改了 `LLM_DEFAULT_API_KEY` 后，历史 Agent 仍然使用旧快照，导致模型供应商返回 401。
- 普通用户消息进入 Agent 事件分发器时，会先判断发送者是否是 Agent 系统用户。普通用户查不到 `bots.agent_user_id` 是正常路径，但 GORM `First` 会把它打印成 `record not found`。

## 已完成

- `scripts/start.bat` 增加端口监听检查：端口已监听时跳过该服务，避免重复拉起导致 bind FATAL。
- `scripts/start.bat` 补齐近期新增服务启动入口：conversation-intelligence、mcp-gateway、admin、RAG、knowledge、web-search 等。
- agent-manager-service 启动时注入当前平台默认 LLM 配置。
- internal Agent 在运行前使用当前默认 API Key/BaseURL，避免旧 Agent 因保存了过期 key 而持续 401。
- Agent 事件分发器将 `401 Unauthorized`、`身份验证失败`、`invalid_api_key` 等识别为永久配置错误，记录失败后不再交给 Kafka 反复重试。
- 媒体消息的 `message.created` 事件会跳过 Agent 触发，由 `file.uploaded` 事件统一负责图片/文件触发，避免同一图片触发两轮 Agent。
- Agent 事件分发器继续忽略任意 Agent 系统用户发送的消息，防止 Agent 回复自己的消息形成回声。
- `GetBotByAgentUserID` 改为 `Find + Limit(1)`，普通用户不是 Agent 时不再输出 GORM `record not found` 噪声。

## 验证

- `go test ./internal/agent-manager-service/service`
- `go test ./internal/agent-manager-service/eventconsumer`
- `go test ./internal/agent-manager-service/dao`
- `node --check dist/js/app.js`
- `node --check dist/js/api.js`

## 运维说明

- 如果仍看到 bind FATAL，优先执行端口检查确认是否已有实例：
  - PowerShell: `Get-NetTCPConnection -State Listen | ? LocalPort -in 8080,8081,9112,9113,9114`
- 如果 Agent 仍返回 401，优先检查：
  - `.env` 中 `LLM_DEFAULT_API_KEY` 是否有效；
  - Agent 是否为 `custom` 类型。`custom` Agent 使用自己的 key，不会被平台默认 key 覆盖；
  - 模型名是否与供应商实际开放模型一致。
