# ClaranAIM 项目概览

ClaranAIM 是一个 AIM 系统：Agent + Instant Messaging。它不是“聊天 + 一个 AI 按钮”，而是把 Agent 作为真实 IM 参与者放进用户、好友、群聊、文件、语音、知识、记忆和工具调用里。

## 一句话理解

ClaranAIM = IM 基础设施 + 事件驱动架构 + Agent 管理面 + Agent 运行时 + Memory/RAG/Tool 演进。

## 当前服务分层

接入层：

- `api-gateway`：Hertz HTTP 网关，暴露 `/api/v1/*`，负责 JWT、限流、参数绑定、RPC/内部 HTTP 调用。
- `websocket-gateway`：WebSocket 网关，消费消息事件，把实时消息推给在线用户。
- `dist/`：前端静态资源，包含聊天、Agent 面板、记忆管理、设置、Action Card MVP。

IM 业务层：

- `user-service`：用户、好友、好友分组、系统用户、登录状态。
- `group-service`：群、群成员、角色、禁言、群号、自助入群。
- `msg-core-service`：会话、消息写入、消息状态、编辑、撤回、已读、翻译、Outbox。
- `msg-history-service`：历史消息查询服务，面向后续离线和冷数据拆分。
- `file-service`：文件元数据，文件二进制由网关写本地或 MinIO。

Agent 平台层：

- `agent-manager-service`：Agent 配置、权限、触发规则、订阅规则、审计、调度记录、计费。
- `agent-runtime-service`：Eino Agent 运行、工具调用、Skill、长会话、RAG MVP。
- `memory-service`：可治理记忆事实，支持用户/群/会话/session 范围。
- `settings-service`：LLM 预设、Prompt 模板，供创建 Agent 和翻译使用。
- `rag-service`：预留的向量 RAG 服务。
- `msg-filter-service`：预留的审核/过滤服务。

基础设施：

- MySQL：业务事实源和 Outbox。
- Redis：缓存、在线状态、辅助状态。
- Kafka：事件总线。
- Etcd：服务发现。
- MinIO：对象存储。
- DTM：低频跨服务 Saga。

## 当前关键主线

### 1. 消息发送主线

```text
前端
  -> api-gateway /message/send
  -> msg-core-service
  -> MySQL 写消息事实
  -> event_outbox 写 message.created 和 Agent-native IM 事件
  -> Outbox worker 发布 Kafka
  -> websocket-gateway 推送在线用户
  -> agent-manager-service 判断是否触发 Agent
```

这里你要抓住一句话：消息先成为数据库事实，再通过 Outbox 变成事件。

### 2. Agent 原生事件主线

```text
msg-core-service / group-service 写业务事实
  -> 事件进入 Kafka
  -> agent-manager-service AgentEventDispatcher 消费
  -> mention / private / subscription rule 决策
  -> audit record
  -> dispatch record 幂等
  -> 调 agent-runtime-service
  -> Agent 以系统用户身份写回 msg-core-service
```

Agent 不是一个虚拟 sender。Agent 在 user-service 中是 `is_system=true` 的真实用户，因此可以被加好友、入群、被 @，也能作为消息发送者。

### 3. Memory 主线

```text
用户/Agent/会话产生可沉淀事实
  -> memory-service 写 memory_facts
  -> agent-manager 调 runtime 前召回相关记忆
  -> runtime 注入 Agent 上下文
  -> 用户可通过 /memory/* 查看、编辑、删除
```

当前 memory 是 MySQL MVP，`vector_status/embedding_ref` 为未来向量化预留。

### 4. Settings 与翻译主线

```text
用户保存 LLM profile / Prompt 模板
  -> 创建 Agent 时 api-gateway 解析 LLM profile
  -> 写入 Agent 配置

用户手动翻译消息
  -> api-gateway /message/translate
  -> msg-core-service
  -> settings-service 读取翻译配置
  -> 调 OpenAI-compatible 模型
  -> 按消息 hash 缓存译文
```

当前不做自动翻译，避免每条消息都进入 LLM 调用路径。

## 当前命名状态

对外统一叫 Agent：

- HTTP 路由使用 `/agent/*`。
- 前端展示为 Agent/智能助手。
- 服务目录是 `agent-manager-service` 和 `agent-runtime-service`。

但历史兼容仍存在：

- IDL 仍有 `bot.thrift`。
- 生成代码仍在 `kitex_gen/bot`。
- 数据表仍有 `bots`、`bot_routes`、`bot_permissions`。

学习时要把它理解成：底层历史命名还没完全迁移，但业务语义已经是 Agent。

## 你应该重点深学

1. msg-core-service 的消息事实模型。
2. eventbus + outbox + Kafka 的可靠事件发布。
3. `claran.message.events` 和 `claran.im.events` 的分工。
4. Agent Event Dispatcher 的决策、审计、幂等。
5. agent-manager 和 agent-runtime 的边界。
6. memory-service 和 settings-service 如何作为独立内部服务接入。
7. RAG/MCP/Action Card 的后续设计空间。

