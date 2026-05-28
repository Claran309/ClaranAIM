# 第 0 课：建立全局地图

## 学习目标

这一课先建立全局地图。学完后你应该能回答：

- ClaranAIM 为什么叫 AIM？
- 当前有哪些服务？
- 哪些服务属于 IM 基础层，哪些属于 Agent 增强层？
- Agent 对外命名和底层 bot 历史命名为什么会同时存在？
- 消息、事件、Agent、Memory、Settings 各自在哪条链路上？

## AIM 是什么

AIM = Agent + Instant Messaging。

这个项目的目标不是在聊天室旁边加一个 AI 对话框，而是让 Agent 成为 IM 的原生成员：

- 可以是系统用户。
- 可以被加好友。
- 可以进入群聊。
- 可以被 @。
- 可以按规则订阅 IM 事件。
- 可以读取自己有权看到的会话上下文。
- 可以调用 runtime、工具、Memory、RAG。
- 可以把回复写回普通消息系统。

## 当前服务地图

```text
前端 dist/
  | HTTP
  v
api-gateway
  | Kitex RPC / 内部 HTTP
  +--> user-service
  +--> group-service
  +--> msg-core-service
  +--> msg-history-service
  +--> file-service
  +--> agent-manager-service
  +--> agent-runtime-service
  +--> memory-service
  +--> settings-service

前端 dist/
  | WebSocket
  v
websocket-gateway
  ^ Kafka: claran.message.events

group-service / msg-core-service
  | event_outbox
  v
Kafka
  +--> websocket-gateway
  +--> msg-core-service group consumer
  +--> agent-manager-service AgentEventDispatcher
```

基础设施：

```text
MySQL  = 业务事实源 + event_outbox
Redis  = 缓存、在线状态、辅助状态
Kafka  = 事件分发
Etcd   = 服务发现
MinIO  = 对象存储
DTM    = 低频跨服务 Saga
```

## 服务职责速览

`api-gateway`：

- 对外 HTTP 入口。
- JWT、限流、参数绑定。
- 调 Kitex RPC。
- 调 memory/settings/msg-core 内部 HTTP client。
- 文件上传时处理 multipart 和存储。

`websocket-gateway`：

- 长连接。
- 消费消息事件。
- 推送在线用户。

`user-service`：

- 用户、登录、好友、好友分组。
- Agent 系统用户。

`group-service`：

- 群、成员、角色、禁言。
- 10 位群号。
- 自助入群。
- 发布群事件。

`msg-core-service`：

- 会话和消息事实源。
- 消息发送、编辑、撤回、已读、本地删除。
- 手动翻译。
- 写 `message.*` 和统一 IM 事件 outbox。

`msg-history-service`：

- 历史消息查询拆分方向。
- 为离线同步、冷数据、搜索演进预留。

`file-service`：

- 文件元数据。
- 文件内容由 api-gateway 写本地或 MinIO。

`agent-manager-service`：

- Agent 配置、权限、路由、订阅、审计、调度、计费。
- 消费 IM 事件并决定是否调用 Agent。

`agent-runtime-service`：

- Eino Agent 执行。
- 工具、Skill、长会话、RAG MVP。

`memory-service`：

- 可治理记忆事实。
- 用户可查看、编辑、删除。

`settings-service`：

- LLM profile。
- Prompt template。
- 翻译配置。

## 命名迁移要点

现在对外统一叫 Agent：

```text
/agent/create
/agent/chat
/agent/route/create
/agent/add-friend
```

但是内部还有历史 bot 命名：

```text
bot.thrift
kitex_gen/bot
bots
bot_routes
bot_permissions
```

学习时不要被名字绕住。看到 `Bot` 模型时，把它理解为“Agent 配置对象”。

## 三条主线

### 消息主线

```text
用户发消息
  -> msg-core-service 写 messages
  -> 写 event_outbox
  -> Kafka
  -> websocket-gateway 推送
```

### Agent 主线

```text
IM 事件
  -> agent-manager-service Dispatcher
  -> subscription rule 决策
  -> audit + dispatch record
  -> agent-runtime-service
  -> Agent 回复写回 msg-core
```

### Memory/Settings 主线

```text
settings-service
  -> LLM 预设和 Prompt
  -> 创建 Agent / 手动翻译

memory-service
  -> memory_facts
  -> Agent 调用前召回
  -> 用户治理记忆
```

## 本课检查

你应该能回答：

- 为什么 Agent 要作为真实系统用户？
- 为什么 `agent-manager-service` 和 `agent-runtime-service` 要拆开？
- 为什么 message 写入和 WebSocket 推送之间要有 Kafka？
- 为什么当前还有 bot 命名？
- memory 和 IM history 有什么区别？

## 动手任务

1. 画一张当前服务架构图。
2. 标出哪些服务写 MySQL，哪些服务只做接入。
3. 找出所有 `/agent/*` 路由。
4. 写一段话解释：AIM 和普通“AI 聊天按钮”有什么区别。

