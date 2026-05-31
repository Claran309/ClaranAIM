# 项目结构与技术栈解析

本文件按目录、服务、模块和核心运作方式讲解。你读代码时优先看 `cmd/`、`internal/`、`pkg/`、`docs/`，生成代码 `kitex_gen/` 只在查 RPC 字段时看。

## 顶层目录

```text
cmd/                         各服务启动入口
internal/                    各服务内部实现
idl/                         Thrift IDL
kitex_gen/                   Kitex 生成代码
pkg/                         跨服务公共包
docs/                        项目文档
dist/                        前端静态资源
config/                      各服务配置
storage/                     本地运行时和文件数据
learn/                       学习资料，已加入 .gitignore
```

## 技术栈

后端：

- Go
- Hertz
- Kitex
- GORM
- Redis
- Kafka
- Etcd
- DTM
- MinIO / 本地文件存储
- Eino

前端：

- 原生静态前端。
- WebSocket。
- Markdown 渲染。
- Agent 面板。
- Action Card MVP。
- 记忆与设置管理入口。

## cmd 服务入口

当前服务入口包括：

- `cmd/api-gateway`
- `cmd/websocket-gateway`
- `cmd/user-service`
- `cmd/group-service`
- `cmd/msg-core-service`
- `cmd/msg-history-service`
- `cmd/file-service`
- `cmd/agent-manager-service`
- `cmd/agent-runtime-service`
- `cmd/memory-service`
- `cmd/settings-service`
- `cmd/rag-service`
- `cmd/msg-filter-service`

读每个服务先看 `main.go`，它会告诉你该服务依赖哪些数据库、缓存、RPC client、Kafka consumer 或 outbox worker。

## internal/api-gateway

核心职责：

- 注册 `/api/v1/*` HTTP 路由。
- JWT 鉴权。
- 用户/IP 限流。
- 参数绑定和统一响应。
- 调用 Kitex RPC；浏览器入口和少量外部协议回调才保留 HTTP。
- 文件上传时直接写本地/MinIO，然后调用 file-service 存元数据。

当前重要路由：

- `/user/*`
- `/group/*`
- `/message/*`
- `/file/*`
- `/agent/*`
- `/memory/*`
- `/settings/*`

新近细节：

- 分页参数统一使用 `parsePositiveLimit`，限制最大 limit。
- 游标和可选 ID 使用 `parseNonNegativeQueryInt64`。
- `/agent/*` 是唯一对外 Agent 入口，旧 `/bot/*` 不再作为 HTTP 兼容入口。

## internal/user-service

职责：

- 普通用户注册登录。
- Refresh Token。
- 用户资料。
- 好友与好友分组。
- 系统用户。

新近细节：

- 登录更新在线状态失败会返回错误，不再静默忽略。
- 添加好友是双向关系；反向关系写入失败时会回滚正向关系并返回错误。
- 删除好友时反向删除失败会返回错误。

这些变化让好友关系的一致性比旧版更明确。

## internal/group-service

职责：

- 群资料。
- 群成员。
- 群角色。
- 禁言。
- 10 位群号。
- 自助入群。
- 群事件 Outbox。

关键点：

- `groups.id` 是 10 位群号。
- `/group/join` 只允许当前用户把自己加入群。
- 群成员变化通过 `claran.group.events` 被 msg-core-service 消费，用于同步会话参与者。

## internal/msg-core-service

职责：

- 会话。
- 参与者。
- 消息事实。
- 用户消息状态。
- 编辑、撤回、已读。
- 翻译。
- 消息 Outbox。
- Agent-native IM Outbox。

关键点：

- `messages` 是服务端消息事实。
- `message_user_states` 是用户本地视图。
- 文件/图片/语音消息会生成统一 IM 事件。
- 编辑、撤回、已读也会生成统一 IM 事件。
- 手动翻译通过 settings-service 获取 Prompt 和模型配置。

## internal/msg-history-service

职责：

- 历史消息查询拆分方向。
- 为未来离线消息、冷数据、搜索和归档做准备。

当前学习重点是理解为什么历史查询适合从 msg-core 拆出：写路径和读路径压力不同。

## internal/file-service

职责：

- 文件元数据。
- 文件列表。
- 文件删除。
- 与 api-gateway 的文件上传/下载配合。

二进制不直接放消息表。消息里保存文件引用，file-service 管元数据，MinIO/本地目录管文件内容。

## internal/websocket-gateway

职责：

- WebSocket 连接。
- 用户多端连接管理。
- 消费 `claran.message.events`。
- 推送在线用户。

前端新近细节：

- 根据消息 ID 或 `client_msg_id` 做去重，避免乐观渲染和 WebSocket 回推导致重复显示。

## internal/agent-manager-service

职责：

- Agent 配置。
- Agent 系统用户。
- 权限。
- Agent 触发规则。
- 订阅规则。
- 审计记录。
- 调度记录。
- 计费。
- 调用 runtime。

历史兼容：

- 模型名仍叫 `Bot`。
- 表名仍叫 `bots`。
- 生成代码仍叫 `bot`。

业务语义：

- 都按 Agent 理解。

核心模型：

- `Bot`
- `BotPermission`
- `BotRoute`
- `AgentSubscriptionRule`
- `AgentAuditRecord`
- `AgentDispatchRecord`
- `BillingRecord`

## internal/agent-runtime-service

职责：

- Eino Agent 运行。
- Tool/Skill 加载。
- RAG MVP。
- 长会话 JSONL。
- 安全中间件。
- 会话级任务：总结、问答、洞察、候选回复。

它不负责 Agent 管理权限，也不直接管理 IM 消息事实。

## internal/memory-service

职责：

- `memory_facts`。
- 用户、群、会话、session 范围记忆。
- 私有/共享可见性。
- 用户可查看、编辑、删除。
- 向量化状态预留。

核心字段：

- `Scope`
- `Type`
- `Visibility`
- `VectorStatus`
- `EmbeddingRef`

## internal/settings-service

职责：

- LLM profile。
- Prompt template。
- 翻译 Prompt。
- 创建 Agent 时解析用户保存的模型配置。

核心模型：

- `llm_profiles`
- `prompt_templates`

## pkg 公共包

重点：

- `pkg/events`：事件契约、topic、payload。
- `pkg/eventbus`：Kafka 发布/消费抽象。
- `pkg/outbox`：事务 Outbox。
- `pkg/governance`：Kitex 治理。
- `pkg/settingsclient`：settings-service 的 Kitex RPC 适配器，向调用方暴露稳定 Service 接口。
- `pkg/memoryclient`：memory-service 的 Kitex RPC 适配器，向调用方暴露稳定 Service 接口。
- `pkg/idgen`：雪花 ID、10 位 UID/群号。

## docs 文档

建议同步读：

- `docs/TechArch.md`
- `docs/APIdoc.md`
- `docs/plan.md`
- `docs/ReliabilityAndEventConsistency.md`
- `docs/consideration.md`
