# ClaranAIM 学习入口

更新时间：2026-05-28

这份 `learn/` 目录是按你的背景定制的项目学习材料。你已经具备 Go 后端、CRUD、MySQL、Redis、Kitex/Hertz、微服务、服务治理、DTM、可观测性和 Eino Agent 基础，所以这里不会把通用概念讲得很慢，而是聚焦 ClaranAIM 当前最有价值的设计：AIM = Agent + Instant Messaging。

## 当前项目学习重点

- IM 领域建模：用户、好友、群组、会话、消息事实、用户本地消息视图、历史查询、文件与翻译。
- 事件一致性：MySQL 事实源、Transactional Outbox、Kafka、eventbus、消费者幂等。
- Agent-native IM：Agent 是真实系统用户，能被加好友、入群、被 @，也能通过订阅规则观察 IM 事件。
- 统一 IM 事件流：`claran.im.events` 承载文件、语音、已读、编辑、撤回、群成员、系统通知和任务事件。
- Agent 管理面与运行时：`agent-manager-service` 管配置、权限、订阅、审计、计费；`agent-runtime-service` 管 Eino Agent、工具、Skill、长会话。
- Memory 与 Settings：`memory-service` 管可治理的长期记忆，`settings-service` 管 LLM 预设和 Prompt 模板。
- RAG/MCP 进阶：当前 RAG 是 MVP，真正的向量 RAG、MCP Gateway、Action Card 持久化仍是后续重点。

## 推荐阅读顺序

1. 先读 `01-project-overview.md`，建立项目全景。
2. 再读 `02-project-structure-and-tech-stack.md`，把目录、服务、模块和关键链路对上。
3. 读 `03-course-outline.md`，了解课程路线。
4. 按 `lessons/` 从第 0 课推进到第 15 课。
5. 读 `UPDATE-2026-05-28.md`，了解本次重新扫描后学习资料改了什么。

## 逐课讲义

- [第 0 课：建立全局地图](./lessons/00-global-map.md)
- [第 1 课：HTTP 网关与 RPC/HTTP 内部门面](./lessons/01-api-gateway-rpc-boundary.md)
- [第 2 课：用户、好友与 Agent 系统用户](./lessons/02-user-friend-system-user.md)
- [第 3 课：群组领域与群号入群](./lessons/03-group-domain-member-source.md)
- [第 4 课：IM 消息领域建模](./lessons/04-im-message-domain-modeling.md)
- [第 5 课：Outbox、Kafka、EventBus 与事件一致性](./lessons/05-outbox-kafka-event-consistency.md)
- [第 6 课：WebSocket 实时推送与前端补偿](./lessons/06-websocket-realtime-push.md)
- [第 7 课：DTM 在项目中的位置](./lessons/07-dtm-position-in-project.md)
- [第 8 课：Agent 管理服务](./lessons/08-bot-manager-service.md)
- [第 9 课：Agent-native 事件分发](./lessons/09-agent-mention-dispatch.md)
- [第 10 课：Agent Runtime 与 Eino DeepAgent](./lessons/10-bot-runtime-eino-deepagent.md)
- [第 11 课：Memory 与当前轻量 RAG](./lessons/11-lightweight-rag.md)
- [第 12 课：向量数据库版 RAG 设计](./lessons/12-vector-database-rag-design.md)
- [第 13 课：MCP 与工具治理设计](./lessons/13-mcp-integration-design.md)
- [第 14 课：可观测性、治理与测试](./lessons/14-observability-governance-testing.md)
- [第 15 课：最终综合项目](./lessons/15-final-capstone.md)
- [第 16 课：File Service 与多媒体消息](./lessons/16-file-service-media.md)
- [第 17 课：Msg History 与消息读模型](./lessons/17-msg-history-read-model.md)
- [第 18 课：Settings Service 与 LLM 配置中心](./lessons/18-settings-service-llm-profiles.md)
- [第 19 课：Memory Service 深入](./lessons/19-memory-service-deep-dive.md)
- [第 20 课：Action Card 与审批闭环](./lessons/20-action-card-approval-flow.md)

## 你已经看到 lesson3 时的提醒

这次 lesson3 之前也有变化，主要是命名、服务边界和一些可靠性细节。建议至少快速重读：

- `01-project-overview.md`
- `02-project-structure-and-tech-stack.md`
- `lessons/00-global-map.md`
- `lessons/01-api-gateway-rpc-boundary.md`
- `lessons/02-user-friend-system-user.md`

特别是：旧 `bot-manager-service` 表述已经整体更新为 `agent-manager-service`，并补入 `memory-service`、`settings-service`、`file-service`、`msg-history-service`。

## 新增专题课

新增服务不再强行塞进原 0-15 课，而是从第 16 课开始作为专题扩展。你可以先按 0-15 建主线，再按需要学习 16-20：

- 想理解文件上传、图片/语音/文件消息：读第 16 课。
- 想理解历史消息、离线同步、读写分离方向：读第 17 课。
- 想理解用户模型配置、Prompt、翻译配置：读第 18 课。
- 想深入 Agent 记忆治理：读第 19 课。
- 想把 Agent 工具审批和前端卡片做成闭环：读第 20 课。
