# Conversation Intelligence Service 更新记录

## 背景

聊天记录适合做长期 RAG，但不适合把每条消息都直接写入向量库。短消息噪声大、上下文依赖强、权限复杂，单条 embedding 往往没有意义。

本次新增 `conversation-intelligence-service`，负责把当前用户可见的聊天窗口提炼成长期可用的会话知识和候选记忆。

## 已实现

- 新增 `idl/conversation_intelligence.thrift` 和 Kitex RPC 服务。
- 新增 `cmd/conversation-intelligence-service`，默认端口 `127.0.0.1:9015`。
- 新增 `internal/conversation-intelligence-service`：
  - `DigestJob` 归档任务。
  - `ConversationArtifact` 归档产物。
  - `ConversationActivityCursor` 活跃会话调度游标。
  - MySQL AutoMigrate。
  - 从 `msg-core-service.GetHistory` 拉取当前 viewer 可见消息窗口。
  - 过滤无价值短消息。
  - 规则版 extractor 提炼摘要、决策、任务、主题、金句和候选记忆。
  - 摘要/主题块写入 `rag-service`。
  - 候选记忆写入 `memory-service` pending 区。
- Kafka 开启时消费 `claran.message.events` 与 `claran.im.events`，推进活跃会话游标。
- Scheduler 按 `CONVERSATION_INTELLIGENCE_SCHEDULE_INTERVAL_SECONDS` 周期扫描活跃游标，满足消息数阈值或时间窗口后自动归档。
- memory-service 允许 `bot_id=0` 的系统级候选记忆，用于 IM 原生聊天归档，不强制绑定某个 Agent。
- 新增 `pkg/conversationintelclient`，供网关或其他服务通过稳定客户端调用。
- api-gateway 新增接口：
  - `POST /api/v1/conversation-intelligence/jobs`
  - `POST /api/v1/conversation-intelligence/jobs/:id/process`
  - `GET /api/v1/conversation-intelligence/artifacts`
- 启动脚本加入 `conversation-intelligence-service`。
- `.env.example` 增加窗口大小、最小有效消息数和调度参数。

## 当前数据流

```text
消息/IM 事件 或 用户/Agent 请求归档
  -> conversation-intelligence-service 记录活跃游标 或 api-gateway 手动建 job
  -> Scheduler / ProcessDigestJob
  -> msg-core-service.GetHistory(viewer_id)
  -> 清洗低价值消息
  -> 提炼 conversation_summary / decision / task / topic / quote / memory_candidate
  -> MySQL 保存 artifacts
  -> rag-service.IngestDocument 保存摘要/主题
  -> memory-service.CreateCandidate 保存 pending 记忆候选
```

## 重要边界

- 不全量 RAG 每一条消息。
- 短期上下文仍由 Agent/网关从 `msg-core-service` 读取最近 N 条。
- 长期聊天 RAG 的对象是会话摘要、主题块、决策、任务和高价值内容。
- `viewer_id` 由 JWT 登录态决定，网关不会信任请求体中的 viewer_id。
- 当前 `start_time/end_time` 已进入任务模型，但消息窗口仍依赖 msg-core 的历史读取能力，后续需要补按时间范围拉取 RPC。
- 自动归档会按事件参与者分别记录 viewer 视角，后续窗口读取仍走 msg-core 可见性校验。

## 当前限制

- Extractor 是规则版 MVP，尚未接入 LLM 提炼器。
- 尚未实现归档失败重试队列和前端归档状态页。
- 自动归档已支持时间窗口和消息数阈值，但活跃会话来源依赖 Kafka 事件；如果 Kafka 关闭，只能通过 HTTP 手动创建/处理 job。
- msg-core 尚未提供按 start_time/end_time 精确拉取历史消息的 RPC，因此当前窗口边界仍以最近 N 条和 before_id 为主。

## 验证

- `go test ./internal/conversation-intelligence-service/... ./pkg/conversationintelclient ./cmd/conversation-intelligence-service ./pkg/config`
- `go test ./internal/api-gateway/handler`
- `go test ./internal/memory-service/service`

- `go test ./...`
