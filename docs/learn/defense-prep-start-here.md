# ClaranAIM 答辩准备启动文档

> 用途：明天继续准备答辩时，直接把这份文档发给 Codex，然后说“从这里开始按压力面训练我”。
>
> 当前状态：项目是 AI 辅助/vibe coding 做出来的，所以目标不是假装每一行都亲手写过，而是把系统链路、关键机制、源码入口、风险边界和可改进点真正变成自己的知识。

# 真实面试片段补全
```text
提问示例

为什么用sentinel不用redis cluster？（redis cluster更优秀）

kafka消息投递失败怎么办？（outbox）

等等
```
另外，我在事件这方面的知识和概念严重不足
## 0. 明天对话启动方式

明天可以直接这样开始：

```text
我们从 docs/learn/defense-prep-start-here.md 开始准备答辩。请按压力面方式训练我：
1. 先问我项目总览；
2. 然后按消息链路、Outbox、DTM、Redis、Agent runtime、RAG、GraphRAG、Milvus、Neo4j、管理与配置层逐个追问；
3. 每题我先答，你再指出漏洞并给推荐答案；
4. 如果我答不上来，请带我看源码。
```

## 1. 最重要的心态

这个项目可以承认是 AI 辅助开发，但不能让答辩变成“我让 AI 写了一个系统”。你要把表达改成：

- 我使用 AI 辅助完成了较大规模的工程搭建。
- 我的重点工作是系统设计、链路打通、功能验证和后续理解沉淀。
- 现在我正在把项目中的机制逐个补成自己的知识，包括消息可靠性、Agent 运行、RAG、图谱、缓存、服务治理等。

压力面里最危险的不是“不知道某个术语”，而是“无法解释自己项目里为什么用了这个设计”。所以准备方式应该是：

```text
项目功能点
-> 源码入口
-> 背后机制
-> 为什么这么设计
-> 失败场景
-> 当前不足
-> 可进阶方案
```

## 2. 两分钟项目总览

推荐开场：

> ClaranAIM 是一个 Agent-Native IM 系统，AIM = Agent + Instant Messaging。它不是在聊天软件旁边放一个 AI 聊天按钮，而是把 Agent 作为 IM 系统中的原生成员来设计。Agent 有真实系统用户身份，可以进入私聊和群聊，可以被 @，可以读取自己有权限看到的会话上下文，可以调用 Memory、RAG、Skill、MCP 工具，并且它的行为会进入审计和计费链路。
>
> 系统整体采用 Hertz API Gateway + Kitex RPC 微服务架构。IM 消息先由 msg-core-service 写成可靠消息事实，再通过事务 Outbox 发布到 Kafka，驱动 WebSocket 推送、Agent 调度、会话智能归档等后续链路。RAG、Memory、Knowledge、MCP 和 Admin 等服务围绕 IM 和 Agent 提供长期知识、工具调用和治理能力。

必须记住的核心观点：

- IM 是上下文入口、事件源、权限边界和协作现场。
- Agent 是真实系统用户，不是虚拟 sender。
- 消息先落库，再事件化。
- 高频事件可靠性靠 Outbox + Kafka。
- 低频跨服务补偿动作靠 DTM Saga。
- RAG/Memory/GraphRAG 是 Agent 的知识增强能力，但当前很多部分仍是 MVP 或有降级实现。

## 3. 服务地图

接入层：

- `cmd/api-gateway/main.go`：Hertz HTTP 网关启动。
- `internal/api-gateway/router/router.go`：HTTP 路由注册。
- `internal/api-gateway/client/rpc_client.go`：Kitex RPC client 初始化。
- `cmd/websocket-gateway/main.go`：WebSocket 网关启动。
- `internal/websocket-gateway/hub/hub.go`：连接管理。
- `internal/websocket-gateway/eventconsumer/message_consumer.go`：消费消息事件并推送。

IM 核心层：

- `user-service`：用户、好友、系统用户。
- `group-service`：群、成员、禁言、群事件。
- `msg-core-service`：会话、消息、用户级消息状态、Outbox。
- `msg-history-service`：历史读取和离线方向。
- `file-service`：文件元数据。

Agent 层：

- `agent-manager-service`：Agent 配置、系统用户、权限、路由、订阅、审计、调度、计费。
- `agent-runtime-service`：Eino Agent 运行、工具、Skill、长会话、工作目录安全。

知识增强层：

- `memory-service`：长期记忆、候选记忆、召回。
- `rag-service`：文档入库、向量检索、Hybrid Search、GraphRAG、CRAG/Self-RAG。
- `knowledge-service`：知识图谱查询、邻域、路径、审核候选。
- `conversation-intelligence-service`：会话摘要、决策、待办、主题、候选记忆。
- `mcp-gateway-service`：MCP 工具调用和追踪。

治理层：

- `settings-service`：LLM profile、Prompt、Skill、MCP Server 配置、密钥加密。
- `admin-service`：用户、群、文件、Agent、账单、审核、公告、审计、可观测入口。

基础设施：

- MySQL：业务事实源、Outbox、消费幂等表。
- Redis：缓存、在线状态、cache-aside、防穿透、防击穿锁。
- Kafka：事件总线。
- Etcd：服务发现。
- Milvus：向量数据库。
- Neo4j：图数据库。
- MinIO/本地目录：对象存储。
- DTM：低频跨服务 Saga。

## 4. IM CRUD 和高级 CRUD

### 4.1 普通 CRUD 不要只说增删改查

项目中很多 CRUD 不是简单表操作，而是带领域语义：

- 用户注册：密码 hash、系统用户、角色。
- 好友添加：双向关系，一边失败要回滚。
- 群创建：群资料 + 群成员 + 群会话，涉及 DTM。
- 消息发送：写消息事实 + 会话排序 + 用户状态 + Outbox。
- 本地删除：只影响当前用户视图，不删除全局消息。
- 撤回：全局消息状态变化。
- 编辑：改消息事实并追加编辑审计。
- 已读：更新会话读游标和消息级用户状态。

答辩时不要说“这里就是 CRUD”。要说“这是带一致性、权限和用户视图隔离的领域写操作”。

### 4.2 messages 与 message_user_states 为什么分开

源码：

- `internal/msg-core-service/model/model.go`
- `internal/msg-core-service/service/service.go`
- `internal/msg-core-service/dao/dao.go`

核心解释：

- `messages` 是全局消息事实：谁发的、哪个会话、内容、类型、是否撤回、是否编辑。
- `message_user_states` 是用户视角状态：是否投递、是否已读、是否本地删除。

为什么不能都放 `messages`？

- 同一条消息，A 已读、B 未读。
- A 本地删除，B 仍要能看到。
- 撤回是全局语义，应该进 `messages.status`。
- 本地删除是个人视图，应该进 `message_user_states.local_deleted_at`。

推荐回答：

> 这个拆分体现了 IM 里的“消息事实”和“用户本地视图”分离。全局事实放在 messages，用户状态放在 message_user_states。这样可以支持未读统计、已读回执、本地删除、多端同步，也避免一个用户的操作污染其他用户的历史。

## 5. 消息发送主链路

源码入口：

- `internal/api-gateway/router/router.go`：`POST /api/v1/message/send`
- `internal/api-gateway/handler/message_handler.go`
- `internal/msg-core-service/handler/handler.go`
- `internal/msg-core-service/service/service.go`：`SendMessageExt`
- `internal/msg-core-service/dao/dao.go`
- `pkg/events/events.go`
- `pkg/outbox/outbox.go`

流程：

```text
前端发送消息
-> api-gateway /message/send
-> Kitex 调 msg-core-service.SendMessage
-> SendMessageExt 校验内容、会话、参与者、群状态、禁言、引用消息
-> 创建 messages
-> 更新 conversations.updated_at
-> 为每个参与者 upsert message_user_states
-> 写 message.created Outbox
-> 对文件/图片/语音等写 Agent-Native IM Outbox
-> 事务提交
-> Outbox Worker 发布 Kafka
-> websocket-gateway 推送
-> agent-manager-service 可能触发 Agent
```

要讲清楚的点：

- 消息写入和事件记录在同一个事务里。
- WebSocket 推送不在事务里直接做。
- Kafka 发布失败不会导致消息丢失，因为 Outbox 表里还有 pending/retrying 事件。
- 事件可能重复，因此消费者和消息写入都要考虑幂等。

## 6. Outbox 具体实现

### 6.1 Outbox 解决什么问题

解决的问题：

```text
业务 DB 已提交
但 Kafka 还没发送
服务进程崩溃
```

如果直接在业务逻辑里：

```text
写 MySQL -> 发 Kafka
```

中间崩溃会导致消息已经存在，但下游不知道。

Outbox 做法：

```text
同一个 MySQL 事务：
  写业务表
  写 event_outbox

事务提交后：
  Worker 异步扫描 event_outbox
  发布 Kafka
  成功标记 published
  失败重试
```

### 6.2 源码入口

- `pkg/outbox/outbox.go`
- `internal/msg-core-service/service/service.go`：`saveMessageEvent`、`saveAgentNativeIMEvent`
- `internal/msg-core-service/dao/dao.go`：`SaveOutboxEvent`
- `cmd/msg-core-service/main.go`：启动 Outbox Worker
- `pkg/eventbus/kafka.go`：KafkaPublisher
- `pkg/events/events.go`：事件 Envelope 和 Topic 映射

### 6.3 Outbox 表字段

`pkg/outbox/outbox.go` 中 `Event`：

- `ID`：事件 ID。
- `AggregateType` / `AggregateID`：关联业务聚合，便于排查。
- `EventType`：如 `message.created`。
- `EventKey`：Kafka 分区 key。
- `Payload`：完整 `events.Envelope` JSON。
- `Status`：`pending` / `published` / `retrying` / `dead`。
- `RetryCount`：重试次数。
- `LastError`：最后错误。
- `NextRetryAt`：下次重试时间。
- `LockedUntil`：Worker 锁，降低并发重复发布。
- `PublishedAt`：发布时间。

### 6.4 Outbox Worker 原理

我的问题：问什么要设置locked_until?ProcessOne是怎么做的？

`outbox.Worker.ProcessOnce` 逻辑：

```text
FetchDue:
  找 status in pending/retrying
  next_retry_at <= now
  locked_until 为空或已过期
  设置 locked_until

逐条处理：
  解析 Envelope
  调 publisher.Publish
  成功 -> MarkPublished
  失败 -> MarkRetry 或 MarkDead
```

失败退避：

- `MarkRetry` 根据重试次数指数退避。
- 最大退避约 60 秒。
- 超过最大重试次数进入 `dead`。

### 6.5 Kafka 发布

源码：`pkg/eventbus/kafka.go`

关键点：

- `envelope.Topic()` 根据事件类型决定 topic。
- `envelope.Key` 作为 Kafka key，使同一聚合尽量同分区有序。
- `RequiredAcks = RequireOne`，可靠性主要由 Outbox 重试兜底。

### 6.6 Outbox 不能解决什么

必须主动知道边界：

- 不能保证 Kafka 消费者只消费一次。
- 不能保证下游业务没有重复副作用。
- 不能自动处理业务语义冲突。
- 不能替代消费者幂等。

所以项目里还需要：

- `processed_events` 表。
- `agent_dispatch_records` 唯一键。
- `messages.client_msg_id` 幂等键。

## 7. Kafka 消费幂等和死信

源码：

- `pkg/eventbus/reliability.go`
- `pkg/eventbus/gorm_reliability.go`
- `cmd/msg-core-service/main.go`
- `cmd/agent-manager-service/main.go`

核心机制：

- 消费前根据 `consumer:event_id` 形成幂等 key。
- 成功处理后标记 `processed`。
- 失败记录 attempts。
- 超过阈值标记 dead。

推荐回答：

> Kafka 语义通常按至少一次投递理解，所以消费者必须做幂等。项目里通过 processed_events 做通用消费幂等；Agent 调度又额外通过 agent_dispatch_records 的 event_id + agent_user_id 唯一键保证同一个事件不会重复触发同一个 Agent。

## 8. DTM 实现在哪

### 8.1 DTM 在项目中的定位

Outbox 用于高频事件可靠发布。

DTM 用于低频、跨服务、需要显式补偿的业务动作。

本项目当前主要用于创建群聊：

```text
创建群资料和成员关系
+ 创建对应 msg-core 会话
```

这两个操作跨 `group-service` 和 `msg-core-service`，如果一个成功一个失败，需要补偿。

### 8.2 源码入口

DTM 封装：

- `pkg/dtm/dtm.go`

API 网关发起：

- `internal/api-gateway/handler/group_handler.go`
- 重点看 `createGroupWithDTM`

group-service 分支：

- `internal/group-service/dtmbranch/handler.go`
- `cmd/group-service/main.go`

msg-core-service 分支：

- `internal/msg-core-service/dtmbranch/handler.go`
- `internal/msg-core-service/service/service.go`
- `cmd/msg-core-service/main.go`

配置：

- `pkg/config/config.go`
- `config/*.yaml` 中 `dtm` 配置。

### 8.3 创建群的 Saga 流程

```text
api-gateway 预生成 group_id 和 conversation_id
-> NewSagaLocal
-> AddStep group-service /dtm/group/create
-> AddStep msg-core-service /dtm/message/group-conversation/create
-> Submit 到 DTM Server

如果后续步骤失败：
-> DTM 调用 create_compensate
-> group-service 删除群资料/成员
-> msg-core-service 删除群会话/参与者
```

为什么预生成 ID？

> DTM Saga 的分支之间不适合依赖上一个 HTTP 分支返回值继续传参，所以网关在提交 Saga 前预分配 group_id 和 conversation_id，再把同一组 ID 传给两个服务，保证分支可独立执行和补偿。

### 8.4 DTM 和 Outbox 的区别

| 维度 | Outbox | DTM Saga |
| --- | --- | --- |
| 解决问题 | 本地事务提交后可靠发事件 | 跨服务事务补偿 |
| 频率 | 高频 | 低频 |
| 典型场景 | 消息落库后发 Kafka | 创建群 + 创建会话 |
| 一致性 | 最终一致 | Saga 补偿一致 |
| 复杂度 | 较低 | 较高 |

推荐回答：

> 高频消息链路不适合走 DTM，因为消息发送量大，Saga 开销和复杂度太高。消息链路只需要保证消息事实和事件发布最终一致，所以用 Outbox。创建群这种低频跨服务动作，需要显式补偿，所以用 DTM Saga。

## 9. Redis 的高级应用

### 9.1 源码入口

- `pkg/cache/redis/redis.go`
- `pkg/cache/strategy.go`
- `internal/user-service/service/service.go`
- `internal/group-service/service/service.go`
- `internal/msg-core-service/service/service.go`

### 9.2 项目里 Redis 不只是简单缓存

已有能力：

- JSON cache-aside。
- TTL 抖动，防缓存雪崩。
- 空值缓存，防缓存穿透。
- Redis 分布式锁，防缓存击穿。
- Lua 释放锁，避免误删别人锁。
- 在线状态缓存。
- 会话列表缓存。
- 用户、好友、群资料、群成员缓存策略。

### 9.3 cache-aside 实现

`RedisClient.CacheAsideJSON` 流程：

```text
先读缓存
命中正常值 -> 返回
命中空值标记 -> 返回不存在
未命中 -> 抢 lock:cache:<hash>
  抢到锁 -> 查 DB -> 写缓存或写空值缓存
  没抢到 -> 短暂等待后重读缓存
```

关键点：

- 空值标记：`__CLARAN_CACHE_NULL__`
- 锁：`SETNX + TTL`
- 解锁：Lua 判断 token 再删除。

答辩可说：

> 项目对 Redis 做了一层 CacheAside 封装，不只是 get/set。它通过空值缓存防穿透，通过 TTL 抖动降低雪崩，通过 SETNX 锁降低热点 key 击穿风险，并用 Lua 保证释放锁时不会误删其他请求的锁。

### 9.4 当前 Redis 不足

- 不是所有服务都统一使用 `CacheAsideJSON`。
- 没有 Redis Cluster、主从、哨兵等高可用说明。
- 限流没有使用 Redis。
- 在线状态 TTL 还比较粗。
- 缓存一致性主要靠删除 key，缺少更系统的事件驱动失效。

进阶方案：

- 用 Redis Lua 实现分布式限流。
- 热点 key 加逻辑过期和异步刷新。
- 缓存失效走领域事件。
- 在线状态区分设备级、用户级、最后活跃时间。
- 增加缓存命中率、击穿锁等待次数、空值命中率等指标。

## 10. 限流的不足和进阶实现

### 10.1 当前实现

源码：

- `internal/api-gateway/middleware/middleware.go`
- `RateLimitMiddleware`
- `tokenBucketLimiter`

当前是单进程内存令牌桶：

- key 优先 `userID`。
- 未登录退化到 IP。
- 固定窗口重填。
- 每个 API Gateway 实例各自维护 map。

### 10.2 不足

必须主动承认：

- 多实例部署下限流不全局一致。
- 服务重启后限流状态丢失。
- 固定窗口重填，不够平滑。
- 没有按接口维度、用户角色维度、风险等级维度限流。
- 没有对登录、注册、LLM 调用等高成本接口做单独策略。
- map 可能长期积累冷 key，需要清理机制。

### 10.3 进阶方案

可实现：

- Redis + Lua token bucket。
- 滑动窗口日志/滑动窗口计数。
- 分接口限流：`rate:{route}:{user/ip}`。
- 对 LLM/RAG/Agent 长任务做更严格配额。
- 管理员、普通用户、匿名用户不同配额。
- 返回 `Retry-After`。
- 接入 Prometheus 指标：限流命中次数、剩余 token、热点接口。

推荐回答：

> 当前限流是开发阶段的单实例网关限流，足够保护本地和演示环境，但生产多实例会失效。进阶我会改成 Redis Lua 令牌桶，按 route + user/ip 做分布式限流，并对登录、注册、Agent、RAG 这种高风险或高成本接口配置独立策略。

## 11. 服务降级与补全

项目里已经有一些降级：

- Kafka 未启用：服务仍能写 Outbox，但事件不会发布。
- msg-core 连接 group-service 失败：私聊可用，群聊部分校验降级。
- Redis 失败：服务仅用数据库。
- Milvus 失败：RAG 降级本地向量索引。
- Embedding 未配置：降级 hash embedding。
- RAG Router 未配置：降级规则路由。
- CRAG/Self-RAG LLM 未配置：降级规则评估。
- Neo4j 未启用：GraphRAG 降级内存图谱。
- CozeLoop 未配置：Agent 执行不受影响，只关闭追踪。

需要补全的降级：

- api-gateway 下游 RPC 失败时的更友好错误和熔断。
- Agent runtime 长时间失败时任务状态和重试队列。
- Kafka 不可用时 Outbox 积压监控和告警。
- RAG 外部模型失败时更明确的用户提示。
- Redis 不可用时缓存击穿风险监控。
- Milvus/Neo4j 降级模式应在 UI 和 admin 面板展示，避免用户误以为生产能力完整。

## 12. Agent：你目前最欠缺的部分

你说“除了 Agent 信息 CRUD，其他关于 Agent 运行基本一窍不通”，这块必须重点补。

### 12.1 Agent 分两层

Agent manager：

- 负责控制面。
- 源码：`internal/agent-manager-service`
- 管配置、系统用户、权限、路由、订阅、审计、调度、计费。

Agent runtime：

- 负责执行面。
- 源码：`internal/agent-runtime-service`
- 管模型调用、Eino DeepAgent、工具、Skill、长会话、工作目录安全。

推荐回答：

> manager 是控制面，runtime 是执行面。控制面需要稳定保存配置、权限、审计和计费；执行面依赖模型、工具和长任务，耗时更长、风险更高。拆开后可以分别设置超时、扩容和安全策略。

### 12.2 创建 Agent 时发生了什么

源码：

- `internal/agent-manager-service/service/service.go`
- `CreateBot`
- `createAgentUser`

流程：

```text
用户创建 Agent
-> agent-manager 校验配置
-> internal Agent 用平台默认模型配置
-> custom Agent 要求 API Key/BaseURL
-> 通过 user-service 创建 is_system=true 的系统用户
-> 写 bots 表
-> 写 bot_permissions owner 权限
-> 初始化 workspace root / skills dir
```

关键点：

- 表名还是 `bots`，但业务语义是 Agent。
- `AgentUserID` 是 Agent 在 IM 里的真实用户 ID。
- 创建者拥有 owner 权限。

### 12.3 Agent 被消息触发的流程

源码：

- `cmd/agent-manager-service/main.go`
- `internal/agent-manager-service/eventconsumer/agent_consumer.go`
- `AgentEventDispatcher.Handle`

流程：

```text
Kafka message.created 或 im event
-> decodeAgentEvent
-> 过滤非触发事件
-> 过滤 Agent 自己发出的回声
-> decide 判断触发谁
  私聊：默认触发私聊里的 Agent
  群聊：默认只 @ 触发
  订阅规则：keyword/command/all/record
-> resolveBot 找 Agent 配置
-> 写 agent_audit_records
-> 写 agent_dispatch_records 幂等记录
-> buildAgentDispatchInput 读取 Agent 可见历史消息
-> agentService.ChatWithBot
-> runtime.RunAgent
-> Agent 回复通过 msg-core-service 写回 messages
```

### 12.4 如何避免 Agent 自回声

源码：

- `agent_consumer.go`
- `isAgentGenerated`
- `looksLikeAgentEcho`
- `agentEchoIgnoreReason`
- `sentByKnownAgent`

机制：

- Agent 回复写回时 `client_msg_id` 使用 `agent:<sourceEventID>:<agentUserID>`。
- 事件 metadata 里可能标记 `agent_generated=true`。
- 如果 sender 是已知 Agent 系统用户，静默。
- 私聊中如果发送者就是 Agent，也静默。
- 如果 source event 已经在 `agent_dispatch_records` 处理过，不重复执行。

推荐回答：

> Agent 回复本身也会进入消息系统，所以必须防止再次触发。项目从多个层面做了防回声：client_msg_id 前缀、metadata 标记、sender 是否是 Agent 系统用户、私聊参与者判断，以及 agent_dispatch_records 幂等记录。

### 12.5 runtime 基础运行

源码：

- `internal/agent-runtime-service/service/service.go`
- `internal/agent-runtime-service/agent/agent.go`
- `internal/agent-runtime-service/component/model.go`
- `internal/agent-runtime-service/component/workspace_sandbox.go`
- `internal/agent-runtime-service/agent/tools.go`
- `internal/agent-runtime-service/logic/*.go`

流程：

```text
agent-manager 调 runtime.RunAgent
-> runtime 校验 RuntimeBotConfig
-> 生成 session_id
-> 读取 JSONL 长会话历史
-> getOrCreateAgent
  -> 创建 OpenAI-compatible ChatModel
  -> 解析 workspace
  -> 创建 Eino DeepAgent
  -> 注入 tools
  -> 注入 Skill middleware
  -> 注入 SafeToolMiddleware
  -> 注入 WorkspaceSandbox
-> ag.Run
-> 收集 assistant 回复
-> 提取 token usage
-> 写入 session history
-> 返回 reply/usage/session_id
```

### 12.6 Eino DeepAgent 做什么

源码：`internal/agent-runtime-service/agent/agent.go`

关键点：

- 使用 `github.com/cloudwego/eino/adk/prebuilt/deep`。
- ChatModel 是 OpenAI-compatible。
- LocalBackend 提供本地文件/命令工具。
- `WorkspaceSandbox` 限制工作目录。
- `SafeToolMiddleware` 做工具安全策略。
- Skill middleware 从 `SKILL.md` 加载行为指令。
- 项目工具包括 RAG、WebSearch、MCP 等。
- `tool_policy` 控制工具模式：safe、readonly、approval_required、skill_only、disabled。

### 12.7 Agent runtime 的坑

- Agent 执行目前仍可能同步阻塞较久。
- 上下文压缩比较朴素。
- 长会话 JSONL 存储不是高并发生产级方案。
- Tool approval 主要靠中间件和提示词，还需要更强的服务端审批状态机。
- Agent 对文件、MCP、RAG 的权限边界还要继续收紧。
- 多 Agent 协作还没形成完整调度模型。

## 13. Memory Service

源码：

- `internal/memory-service/service/service.go`
- `internal/memory-service/model/model.go`
- `internal/agent-manager-service/service/memory_rpc.go`

核心概念：

- `memory_facts`：正式长期记忆。
- `memory_candidates`：候选记忆，等待接受/拒绝。
- Scope：`user` / `group` / `conversation` / `session`。
- Type：偏好、说话风格、长期目标、群画像、项目状态、聊天摘要、学习状态等。
- Visibility：private/shared。
- VectorStatus：pending/ready/disabled。

召回逻辑：

```text
输入 bot_id/user_id/query/context
-> 向量候选召回
-> MySQL 可见性回源
-> 按 scope 过滤
-> vector + importance + recency + scope 加权
-> 可选 LLM filter
-> 返回 ContextText 注入 Agent prompt
```

Memory 和聊天历史区别：

- 聊天历史是原始消息事实。
- Memory 是从历史或用户输入中沉淀出来的长期、有治理能力的事实。
- Memory 有 owner、scope、visibility、importance、confidence、enabled/expired 等治理字段。

推荐回答：

> Memory 不是消息历史缓存，而是可治理的长期事实。它会按用户、群、会话、session 范围隔离，召回时还会做可见性过滤和评分，避免不同用户或不同会话之间记忆串线。

## 14. Milvus 向量数据库

### 14.1 源码入口

- `internal/rag-service/service/vector.go`
- `cmd/rag-service/main.go`
- `internal/rag-service/service/embedding.go`

### 14.2 项目里怎么用 Milvus

`MilvusVectorIndex`：

- 连接 Milvus。
- 确保 collection 存在。
- 创建 schema：
  - `chunk_id`：主键。
  - `document_id`：文档 ID。
  - `vector`：float vector。
- 创建 COSINE index。
- Upsert chunk 向量。
- Search TopK。

RAG 入库时：

```text
文档 -> chunks
-> 对 child chunk 生成 embedding
-> vectorIndex.Upsert(chunk_id, vector, document_id)
```

搜索时：

```text
query -> embedding
-> vectorIndex.Search
-> 根据 chunk_id 回 MySQL 查 chunk/document
-> 和关键词/BM25 类得分融合
-> rerank
```

### 14.3 Milvus 降级

如果 Milvus 不可用：

- 使用 `LocalVectorIndex`。
- 内存 map 保存 `chunk_id -> vector`。
- 用 cosine 做本地 TopK。

必须承认：

> LocalVectorIndex 只适合开发和测试，不适合生产，因为它不持久化、不能多实例共享、数据规模和查询性能都有限。

## 15. Neo4j 图数据库

### 15.1 源码入口

- `internal/rag-service/graphstore/neo4j.go`
- `internal/rag-service/graphstore/store.go`
- `internal/rag-service/graphstore/memory.go`
- `cmd/rag-service/main.go`

### 15.2 项目里怎么用 Neo4j

图模型：

- Node：`Entity`
- Node：`Community`
- Relationship：`GRAPH_RELATION`

Neo4j schema：

- `Entity.id` 唯一约束。
- `(owner_id, canonical_key)` 索引。
- `(owner_id, name)` 索引。
- `Community.id` 唯一约束。
- `GRAPH_RELATION(owner_id, document_id)` 索引。

核心操作：

- `SaveEntity`：MERGE Entity。
- `SaveRelation`：MATCH source/target，再 MERGE 关系。
- `SaveCommunity`：MERGE Community。
- `ListGraph`：按 query 查 seed entity，再扩展邻居。
- `listDocumentGraph`：按 document_id 查文档子图，可扩展多跳邻居。
- `DeleteDocumentGraph`：删除文档对应关系和孤立节点。

### 15.3 Neo4j 降级

如果未启用 Neo4j：

- 使用 `graphstore.MemoryStore`。
- 只适合本地开发。
- 进程重启数据丢失。
- 多实例不一致。

推荐回答：

> Neo4j 是生产级图谱后端，MemoryStore 是开发降级。项目启动时如果 Neo4j 启用会初始化约束和索引，否则明确日志提示 GraphRAG 使用内存图谱，不适合生产。

## 16. RAG 实现总览

源码：

- `internal/rag-service/service/service.go`
- `internal/rag-service/service/router.go`
- `internal/rag-service/service/reranker.go`
- `internal/rag-service/service/crag.go`
- `internal/rag-service/service/selfrag.go`
- `internal/rag-service/service/embedding.go`
- `internal/rag-service/dao/dao.go`
- `internal/rag-service/model/model.go`

### 16.1 入库链路

`IngestDocument` 流程：

```text
校验 owner_id/content
-> 创建 Document
-> buildHierarchicalChunks
  parent chunk
  child chunk
-> CreateDocumentWithChunks
-> child chunk 生成 embedding
-> vectorIndex.Upsert
-> 如果 source_type 支持，buildGraph
-> 返回 chunk/entity/relation count
```

需要理解的术语：

- Chunk：把长文档切分成小片段。
- Parent-child chunk：父块保留较大语义范围，子块用于精确检索。
- Embedding：文本向量化。
- Vector Search：按语义相似度召回。
- Keyword/BM25：按词面匹配召回。
- Hybrid Search：融合向量和关键词。
- Rerank：对候选重新排序。

### 16.2 搜索链路

`Search` 流程：

```text
校验 viewer_id/query
-> Adaptive Router 判断走 direct/project_rag/web_rag/memory_rag/tool_action
-> direct：直接回答
-> web/memory/tool：返回外部路线提示
-> project_rag：
   hybridRetrieve
   rerank
   CRAG evaluate
   GetGraph
   synthesizeAnswer
   Self-RAG judge
-> 返回 answer/sources/graph/self_check
```

### 16.3 Adaptive Router

源码：`internal/rag-service/service/router.go`

路由类型：

- `direct`：简单寒暄或无需知识库。
- `project_rag`：项目/文档知识库。
- `strict_rag`：高风险问题。
- `web_rag`：实时/最新信息。
- `memory_rag`：私有长期记忆。
- `tool_action`：动作请求，不应由 RAG 执行。

实现：

- 先规则分类。
- 规则不确定时调用 LLM Router。
- LLM 失败回退默认 project RAG。

### 16.4 CRAG

CRAG = Corrective RAG。

项目中作用：

- 对检索结果质量做评估。
- 如果相关性差，提示可能需要 web fallback 或补充资料。
- 有规则评估和 LLM 评估两套。

源码：

- `internal/rag-service/service/crag.go`

### 16.5 Self-RAG

Self-RAG 用于自检：

- 是否需要检索 Retrieve。
- 答案是否相关 IsRel。
- 答案是否被资料支持 IsSup。
- 答案是否有用 IsUse。

源码：

- `internal/rag-service/service/selfrag.go`

## 17. GraphRAG 和知识图谱

### 17.1 GraphRAG 做什么

普通 RAG 擅长找文本片段，但对多跳关系、实体之间影响、系统结构依赖不够直观。

GraphRAG 通过实体和关系构建知识图谱：

```text
文档 chunk
-> 抽取实体 Entity
-> 抽取关系 Relation
-> 合并同义实体 canonical_key
-> 过滤低质量实体/关系
-> 社区划分 Community
-> 查询时返回相关子图
```

### 17.2 源码入口

RAG 图谱构建：

- `internal/rag-service/service/service.go`
- 搜 `buildGraph`
- 搜 `graphExtractor`
- 搜 `ruleGraphExtractor`
- 搜 `LLMGraphExtractor`
- 搜 `community`
- 搜 `normalizeRelationType`
- 搜 `filterGraphForDisplay`

图谱存储：

- `internal/rag-service/graphstore/store.go`
- `internal/rag-service/graphstore/neo4j.go`
- `internal/rag-service/graphstore/memory.go`

图谱查询聚合：

- `internal/knowledge-service/service/service.go`
- `internal/knowledge-service/service/graph_view.go`
- `internal/knowledge-service/service/rag_source.go`
- `internal/knowledge-service/handler/handler.go`

### 17.3 GraphRAG 构建大致流程

```text
IngestDocument
-> buildGraph
-> 对 chunk 抽取实体和关系
   优先 LLM extractor
   失败降级 rule extractor
-> normalize entity type
-> canonical key 合并实体
-> relationAllowedByEntityTypes 过滤不合理关系
-> SaveEntity / SaveRelation
-> ListOwnerGraph
-> 社区划分和摘要
-> ReplaceOwnerCommunities
```

### 17.4 图谱审核

源码：

- `internal/knowledge-service/service/service.go`
- `internal/knowledge-service/dao/dao.go`
- `internal/knowledge-service/model/model.go`

功能：

- 用户可以创建图谱审核候选。
- item 可以是 node 或 edge。
- 可以 list candidates。
- 可以 approve/reject。

注意：

> 当前审核更多是治理记录，并不是完整的人审后自动重写图谱的闭环。答辩时不要夸大。

### 17.5 GraphRAG 的坑

- LLM 抽取实体/关系容易产生噪声。
- 规则抽取保底但准确率有限。
- 中文实体边界和同义合并困难。
- 图谱关系如果太泛化，会变成“万物 related_to”。
- Neo4j 未启用时内存图谱不适合生产。
- 社区划分目前是轻量/Leiden 风格，不要说成完整工业级 GraphRAG。
- 图谱查询结果需要权限过滤，不能跨 owner 泄露。

推荐回答：

> GraphRAG 在项目里主要用于补足普通 chunk 检索对实体关系和多跳问题的不足。它会从文档中抽实体和关系，保存到图谱后端，并在查询时返回相关子图和社区摘要。但当前图谱质量仍然依赖抽取质量，规则抽取和 LLM 抽取都有噪声，所以项目里也做了过滤、归一化、审核候选和降级。

## 18. Knowledge Service 和 RAG Service 的关系

区别：

- `rag-service` 负责知识入库、embedding、检索、GraphRAG indexing。
- `knowledge-service` 不负责入库，不负责 embedding。
- `knowledge-service` 负责面向前端的图谱视图、节点详情、边详情、邻域、路径、审核候选。

源码：

- `internal/knowledge-service/service/service.go` 文件开头注释已经说明边界。

推荐回答：

> RAG service 是知识构建和检索服务，Knowledge service 是知识图谱查询和治理服务。这样拆分后，GraphRAG 的构建逻辑集中在 rag-service，而前端图谱交互和审核逻辑由 knowledge-service 聚合。

## 19. Conversation Intelligence

源码：

- `internal/conversation-intelligence-service/service/service.go`
- `internal/conversation-intelligence-service/service/scheduler.go`
- `internal/conversation-intelligence-service/service/sinks.go`
- `internal/conversation-intelligence-service/eventconsumer/activity_consumer.go`

功能：

- 记录会话活动。
- 创建 digest job。
- 拉取当前 viewer 可见消息窗口。
- 过滤有价值消息。
- 提取 summary、decision、task、topic、quote、memory candidate。
- 摘要/主题归档到 RAG。
- 用户画像类内容写入 Memory candidate。

关键点：

- 归档时按 viewer 权限读取消息。
- 候选记忆不是直接变成正式记忆，需要治理。
- 当前默认有规则提取器，生产可替换 LLM extractor。

## 20. Settings Service 配置层

需要重点看：

- `internal/settings-service/service/service.go`
- `internal/settings-service/service/secrets.go`
- `internal/settings-service/model/model.go`
- `internal/api-gateway/handler/settings_handler.go`

能力：

- LLM Profiles。
- Prompt Templates。
- Skill 上传、读取、更新、删除。
- MCP Server 配置。
- 密钥加密存储。

答辩可讲：

> settings-service 把模型供应商、Prompt、Skill、MCP Server 配置从业务服务中抽出来，避免每个服务都自己保存密钥和提示词。Agent 创建、翻译、RAG Router 等能力都可以读取 settings 中的配置。

要注意：

- `.env` 里有真实配置风险，答辩或提交时要注意密钥治理。
- 密钥加密是必要点，要能解释 AES-GCM 或类似方案的价值。

## 21. Admin Service 管理层

源码：

- `internal/admin-service/service/service.go`
- `internal/admin-service/handler/handler.go`
- `internal/api-gateway/router/router.go` 中 `/api/v1/admin`
- `internal/api-gateway/middleware/middleware.go` 中 `RequireRole`

能力：

- Dashboard。
- 用户管理。
- 群管理。
- 文件管理。
- Agent 管理。
- Billing。
- Reviews。
- MCP traces。
- Observability links。
- Notices。
- Audit logs。

答辩可讲：

> admin-service 是治理聚合层，不拥有所有领域事实，而是面向管理台提供聚合查询和治理入口。API Gateway 通过 JWT + RequireRole("admin") 限制访问。

## 22. MCP Gateway

源码：

- `internal/mcp-gateway-service/service/service.go`
- `internal/mcp-gateway-service/model/model.go`
- `internal/mcp-gateway-service/handler/handler.go`
- `internal/agent-runtime-service/logic/mcp_tool.go`

你要先掌握的点：

- MCP 是 Agent 调外部工具的一种协议层。
- mcp-gateway 负责工具列表、调用、trace。
- Agent runtime 通过 logic 层把 MCP 工具接入工具调用链。
- 工具调用需要审计和权限控制。

当前可以承认：

> MCP 在项目里已经有网关、配置和调用追踪雏形，但工具权限、审批、失败重试和工具结果可信边界还可以继续加强。

## 23. 可观测性

源码：

- `pkg/observability/observability.go`
- `internal/api-gateway/middleware/middleware.go`
- `deployment/docker/observability`

能力：

- HTTP request metrics。
- Business event/duration/gauge。
- OTel trace。
- Prometheus/Grafana/Jaeger/ELK 配置。

答辩可讲：

> 可观测性主要用于排查微服务链路、Outbox 积压、Agent/RAG 耗时、HTTP 错误率。当前已经接入 OTel 和 Prometheus，但业务维度告警还可以进一步完善，比如 Outbox dead 数、RAG 降级次数、Agent 执行失败率。

## 24. 项目亮点

可以主动讲的亮点：

1. Agent-Native IM 设计，而不是简单 AI 聊天框。
2. Agent 使用真实系统用户身份进入 IM。
3. 消息事实和用户视图分离。
4. Outbox + Kafka 保证消息事件最终可靠发布。
5. DTM Saga 用于低频跨服务补偿。
6. Agent manager/runtime 控制面和执行面拆分。
7. Agent 调度有防自回声和幂等。
8. Memory 有 scope、visibility、candidate 治理。
9. RAG 支持 Adaptive Router、Hybrid、Rerank、CRAG、Self-RAG。
10. GraphRAG 支持 Neo4j 和内存降级。
11. Redis 封装了 cache-aside、防穿透、防击穿、TTL 抖动。
12. 管理台和 settings-service 体现治理意识。
13. 可观测性栈比较完整。

## 25. 项目坑点和不能乱吹的地方

必须记住，别被追问打穿：

- Agent 模块 README 已明确说还是 MVP。
- `bot` 历史命名尚未完全迁移。
- Docker compose 展示环境 README 里说还未完全测试。
- RAG 很多能力有规则兜底，不是全量工业级。
- Milvus/Neo4j 不启用时是本地/内存降级，不适合生产。
- Agent runtime 仍可能同步长耗时。
- 多 Agent 协作没有完整实现。
- Tool approval 还不够严谨。
- 前端是原生 JS，工程化不足。
- GORM AutoMigrate 适合开发，不适合严肃生产迁移。
- 限流是单进程内存版，多实例失效。
- Redis 缓存一致性还可以更系统化。
- Outbox 能解决发布可靠性，但不能替代消费者幂等。

推荐表达：

> 我不会把这个项目包装成已经工业级成熟。它更像一个完整系统雏形，主链路、服务边界和治理思路已经打通，但在 Agent 上下文管理、工具审批、异步任务化、RAG 质量评估、生产级部署和多实例治理上还有不少技术债。

## 26. 面试官可能按项目追问的问题

### 项目总览

1. 这个项目和普通 IM + AI 助手有什么区别？
2. 为什么 Agent 要是真实用户？
3. 为什么还存在 bot 命名？
4. 你认为项目最大亮点是什么？
5. 你认为项目最不成熟的地方是什么？

### 消息链路

1. messages 和 message_user_states 为什么分表？
2. 本地删除和撤回有什么区别？
3. 已读游标和 message_user_states.read_at 是否重复？
4. 群成员变化后，消息服务如何同步参与者？
5. 如果用户猜 conversation_id 发消息怎么办？

### Outbox/Kafka

1. 为什么不用写库后直接 WebSocket 推送？
2. Outbox 解决了什么故障窗口？
3. Worker 重复发布怎么办？
4. Kafka 重复消费怎么办？
5. Outbox 进入 dead 后怎么办？
6. Kafka 挂了，用户发消息会失败吗？

### DTM

1. 为什么创建群要用 DTM？
2. 为什么消息发送不用 DTM？
3. Saga 补偿和事务回滚有什么区别？
4. DTM 分支为什么要预生成 ID？
5. 补偿接口如何保证幂等？

### Redis

1. 项目 Redis 用在哪些地方？
2. 什么是缓存穿透、击穿、雪崩？
3. 你的代码怎么防这些问题？
4. Lua 解锁解决什么问题？
5. Redis 挂了项目还能运行吗？

### 限流

1. 当前限流怎么实现？
2. 多实例下有什么问题？
3. 如何用 Redis 改造成分布式限流？
4. 哪些接口应该单独限流？
5. 限流指标怎么做？

### Agent

1. manager 和 runtime 为什么拆？
2. Agent 创建时为什么要创建系统用户？
3. 群聊里 Agent 为什么默认只 @ 触发？
4. 如何避免 Agent 自己回复再次触发自己？
5. Agent 如何拿到会话上下文？
6. Agent 如何调用工具？
7. Skill 是怎么注入的？
8. workspace sandbox 解决什么风险？
9. token usage 和 billing 怎么记录？
10. Agent 当前最大技术债是什么？

### Memory/RAG

1. Memory 和聊天历史有什么区别？
2. Memory scope 怎么防止串线？
3. RAG 入库流程是什么？
4. Hybrid Search 是什么？
5. Rerank 解决什么问题？
6. Adaptive Router 判断什么？
7. CRAG 和 Self-RAG 分别是什么？
8. Milvus 不可用怎么办？
9. Embedding 没配置怎么办？

### GraphRAG/Knowledge

1. 为什么需要 GraphRAG？
2. 实体和关系怎么抽取？
3. Neo4j 存了什么？
4. MemoryStore 降级有什么问题？
5. 知识图谱审核是不是会改图谱？
6. knowledge-service 和 rag-service 边界是什么？

### 管理和配置

1. settings-service 管什么？
2. 密钥为什么不能散落在各服务？
3. admin-service 是领域服务还是聚合服务？
4. JWT 里 role 怎么用于 admin 鉴权？
5. 可观测性如何帮助排查故障？

## 27. 明天建议学习顺序

第一天不要贪多，建议顺序：

1. 项目总览，练 2 分钟讲解。
2. 消息发送链路，必须讲到源码。
3. Outbox + Kafka，必须能画出故障窗口。
4. DTM 创建群链路，必须能说清为什么不用在消息链路。
5. Redis 和限流，准备“不足 + 进阶方案”。
6. Agent manager/runtime，先把运行主链路讲清楚。
7. RAG 入库和搜索。
8. GraphRAG/Milvus/Neo4j。
9. 管理层、配置层、MCP、可观测性。
10. 总结项目亮点和技术债。

## 28. 明天第一轮训练从这里开始

第一题：

> 请你用 2 分钟介绍 ClaranAIM。注意，我不是要听 README，我要听你怎么理解这个系统，以及它和普通 AI 聊天助手的本质区别。

第二题：

> 你说消息先落库再事件化。请你从 `/message/send` 开始，把消息写入、Outbox、Kafka、WebSocket、Agent 触发整条链路讲清楚，并指出每一步对应的源码文件。

第三题：

> 如果 Kafka 挂了，用户发消息会发生什么？如果 Kafka 恢复了，事件怎么补发？如果补发重复了，怎么保证不会重复触发 Agent 或重复写消息？

第四题：

> 你说 Agent 是真实系统用户。那 Agent 的身份、权限、触发、运行、回复写回、防自回声分别在哪里实现？

第五题：

> 你项目里 RAG、GraphRAG、Milvus、Neo4j 到底哪些是生产级实现，哪些是降级或 MVP？不要泛泛讲概念，结合源码说。

把这五题打透，答辩的地基就稳了。
