# ClaranAIM 课程大纲

这套课程按 4 周设计。你已经熟悉 Go 微服务和 Eino 基础，所以前 3 课主要建立地图；第 4-10 课是核心；第 11-15 课进入 Memory、RAG、MCP 和工程化。

## 第 0 课：建立全局地图

目标：

- 说清 AIM = Agent + Instant Messaging。
- 画出当前 12+ 服务和基础设施关系。
- 理解 Agent 对外命名已迁移，底层表/IDL 仍有历史 bot 命名。

重点：

- `README.md`
- `docs/TechArch.md`
- `docs/plan.md`
- `cmd/*/main.go`

## 第 1 课：HTTP 网关与 RPC/HTTP 内部门面

目标：

- 理解 api-gateway 的职责边界。
- 理解 Kitex RPC 和内部 HTTP client 并存。
- 掌握 `/agent/*`、`/memory/*`、`/settings/*` 的入口。

重点：

- `internal/api-gateway/router/router.go`
- `internal/api-gateway/handler/*`
- `internal/api-gateway/client/rpc_client.go`
- `kitex_gen/settings/settingsservice`
- `kitex_gen/memory/memoryservice`

## 第 2 课：用户、好友与 Agent 系统用户

目标：

- 理解普通用户、系统用户、Agent 配置对象的区别。
- 理解好友双向关系和失败回滚。
- 理解 Agent 作为真实用户的收益。

重点：

- `internal/user-service/service/service.go`
- `internal/user-service/model/model.go`
- `internal/agent-manager-service/service/service.go`

## 第 3 课：群组领域与群号入群

目标：

- 理解 group-service。
- 理解 10 位群号、自助入群、群成员事实源。
- 理解群事件如何同步到 msg-core。

重点：

- `internal/group-service/service/service.go`
- `internal/msg-core-service/eventconsumer/group_consumer.go`

## 第 4 课：IM 消息领域建模

目标：

- 深入理解 msg-core-service。
- 区分消息事实和用户本地视图。
- 理解文件消息、翻译、编辑、撤回、已读。

重点：

- `internal/msg-core-service/model/model.go`
- `internal/msg-core-service/service/service.go`

## 第 5 课：Outbox、Kafka、EventBus 与事件一致性

目标：

- 掌握 EventBus 抽象、Kafka 消费、Outbox 可靠发布。
- 理解 `claran.message.events`、`claran.group.events`、`claran.im.events`。

重点：

- `pkg/events`
- `pkg/eventbus`
- `pkg/outbox`

## 第 6 课：WebSocket 实时推送与前端补偿

目标：

- 理解 WebSocket 网关和历史拉取补偿。
- 理解前端消息去重、Markdown 表格、Action Card 解析增强。

重点：

- `internal/websocket-gateway`
- `dist/js/app.js`

## 第 7 课：DTM 在项目中的位置

目标：

- 区分 DTM 和 Outbox。
- 理解创建群组这类低频跨服务流程为何适合 Saga。

重点：

- `pkg/dtm`
- `internal/group-service/dtmbranch`
- `internal/msg-core-service/dtmbranch`

## 第 8 课：Agent 管理服务

目标：

- 理解 agent-manager-service。
- 理解 route -> subscription rule。
- 理解权限、计费、审计、dispatch record。

重点：

- `internal/agent-manager-service/model`
- `internal/agent-manager-service/service`
- `internal/agent-manager-service/dao`

## 第 9 课：Agent-native 事件分发

目标：

- 理解 Agent Event Dispatcher。
- 理解默认触发、订阅规则、record/trigger、幂等和审计。

重点：

- `internal/agent-manager-service/eventconsumer/agent_consumer.go`
- `pkg/events/events.go`

## 第 10 课：Agent Runtime 与 Eino DeepAgent

目标：

- 理解 agent-runtime-service。
- 理解工具、Skill、长会话、审批 MVP。

重点：

- `internal/agent-runtime-service`
- `internal/api-gateway/handler/agent_handler.go`

## 第 11 课：Memory 与当前轻量 RAG

目标：

- 理解 memory-service。
- 理解 runtime 内当前 RAG MVP。
- 区分 memory、IM history、RAG。

重点：

- `internal/memory-service`
- `internal/agent-runtime-service/graphTool/rag.go`

## 第 12 课：向量数据库版 RAG 设计

目标：

- 设计独立 rag-service。
- 理解 `file.uploaded` 到知识入库链路。

重点：

- `cmd/rag-service`
- `internal/rag-service`
- `pkg/events`

## 第 13 课：MCP 与工具治理设计

目标：

- 理解 MCP 应放在 runtime/MCP Gateway 层。
- 设计 ToolPolicy、审批、审计。

重点：

- `internal/agent-runtime-service/agent/tools.go`
- `internal/agent-runtime-service/component/middleware.go`

## 第 14 课：可观测性、治理与测试

目标：

- 理解当前治理能力。
- 设计事件、Agent、Memory、Settings 的测试和指标。

重点：

- `pkg/governance`
- `pkg/logger`
- `*_test.go`

## 第 15 课：最终综合项目

目标：

- 选择一个方向做真实设计：RAG、MCP、Action Card、Memory、审计面板。

交付：

- 数据流图。
- 表结构/RPC。
- 权限、幂等、失败重试。
- 测试用例。

## 第 16 课：File Service 与多媒体消息

目标：

- 理解 file-service 为什么只管元数据。
- 理解 api-gateway 为什么处理 multipart 和文件存储。
- 理解图片、文件、语音消息如何通过 msg-core 进入 IM 事件流。

重点：

- `internal/api-gateway/handler/file_handler.go`
- `internal/file-service`
- `internal/msg-core-service/service/service.go`

## 第 17 课：Msg History 与消息读模型

目标：

- 理解 msg-history-service 的拆分意义。
- 理解写模型和读模型为什么可以分开。
- 为离线消息、搜索、冷数据归档建立设计视角。

重点：

- `cmd/msg-history-service`
- `internal/msg-history-service`
- `internal/msg-core-service`

## 第 18 课：Settings Service 与 LLM 配置中心

目标：

- 理解 settings-service 如何保存 LLM 预设和 Prompt 模板。
- 理解创建 Agent 时 `llm_profile_id` 如何解析。
- 理解手动翻译为何依赖 settings-service。

重点：

- `internal/settings-service`
- `kitex_gen/settings/settingsservice`
- `internal/api-gateway/handler/agent_handler.go`
- `internal/msg-core-service/service/translation.go`

## 第 19 课：Memory Service 深入

目标：

- 深入理解 `memory_facts`。
- 理解 scope/type/visibility/vector_status。
- 理解 Agent 调用前如何召回记忆，以及用户如何治理记忆。

重点：

- `internal/memory-service`
- `kitex_gen/memory/memoryservice`
- `internal/agent-manager-service/service/service.go`

## 第 20 课：Action Card 与审批闭环

目标：

- 理解当前前端 Action Card MVP。
- 理解 gateway 内存审批的边界。
- 设计生产级持久化审批和卡片操作幂等。

重点：

- `dist/js/app.js`
- `internal/api-gateway/handler/agent_handler.go`
- `internal/agent-runtime-service`
