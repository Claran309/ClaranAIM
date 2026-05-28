# ClaranAIM 可靠性与事件一致性技术文档

## 1. 文档目的

本文回答 ClaranAIM 在引入 Redis 与 Kafka 后最容易被误判的可靠性问题：

- Redis 已经扣减或修改成功，但 Kafka 还没推送，进程突然挂了怎么办？
- Kafka 会不会丢消息？如果丢了，业务数据怎么恢复？
- WAL 写在哪里？MySQL、Redis、Kafka 哪一个才是业务事实来源？
- 当前架构有哪些已知窗口，下一步应该如何补齐？

结论先行：

- 当前项目的核心事实来源应保持为 MySQL：用户、群、会话、消息、计费记录等最终以 MySQL 表为准。
- Redis 只适合做缓存、在线状态、短期锁和短期空值缓存；除非使用 Redis Stream + AOF + ACK + 重放机制，否则不要把 Redis 普通 Key 当业务 WAL。
- Kafka 是服务间事件总线和可重放提交日志，但只有“生产者收到 broker ack 之后”的消息才进入 Kafka 的可靠性边界。
- 当前代码已经为 `group.*` 和 `message.*` 接入 MySQL Transactional Outbox，覆盖“业务 DB 已提交，但 Kafka 发布前进程崩溃”的事件丢失窗口。
- DTM 已作为默认开启的分布式事务基础设施接入，但它不是 Outbox 的替代品。Outbox 继续负责高频事件可靠发布；DTM 更适合低频跨服务补偿事务，例如配额、计费、资源开通等需要明确正向/补偿分支的 Saga/TCC 场景。
- 下一步仍需补通用消费者幂等表 `processed_events`、对账任务和生产环境 Kafka 副本/acks 策略；当前 @Agent 分发链路已先用 `agent_dispatch_records` 与消息 `client_msg_id` 做专项幂等。

## 2. 当前架构事实

### 2.1 消息链路

当前发送消息的主路径是：

1. `msg-core-service` 校验会话、成员和群禁言状态。
2. 写入 `messages`、`message_user_states` 等 MySQL 表。
3. 删除或刷新相关 Redis 缓存。
4. 在同一 MySQL 事务内写入 `event_outbox(message.*)`。
5. Outbox worker 扫描 `event_outbox` 并发布 Kafka 事件到 `claran.message.events`。
6. `websocket-gateway` 消费事件并推送在线 WebSocket 连接。

这里的关键点是：消息事实已经写入 MySQL，Kafka 事件主要用于在线实时通知。即使在线推送事件丢失，客户端理论上仍可通过历史消息接口重新拉取已落库消息。但用户会感知到实时性问题，例如在线窗口没有立刻弹出消息。

### 2.2 群事件链路

当前群组事件主路径是：

1. `group-service` 在同一事务内写入 `groups`、`group_members` 和 `event_outbox(group.*)`。
2. Outbox worker 发布 `group.*` Kafka 事件到 `claran.group.events`。
3. `msg-core-service` 消费事件，同步群聊 conversation 和参与者。

这里的事件不仅是实时通知，还承担跨服务数据同步职责。如果 DB 已提交但 Kafka 事件没有成功进入 broker，`msg-core-service` 的群会话和参与者可能不会自动同步。虽然发送群消息时仍会同步查询 group-service 做权限兜底，但会话列表、参与者同步等体验可能出现缺口。

### 2.3 当前 Kafka 消费语义

`pkg/eventbus/kafka.go` 当前采用：

- `FetchMessage` 拉取消息。
- 事件解析失败时提交 offset，避免坏消息无限阻塞。
- handler 处理失败时不提交 offset，等待后续重试。
- handler 处理成功后提交 offset。

这接近 at-least-once 消费语义：成功进入 Kafka 的消息可能被重复处理，但正常情况下不应在 handler 失败后被静默跳过。

当前不足：

- 没有通用 `processed_events` 幂等表；@Agent 分发已有专用 `agent_dispatch_records`。
- 没有按 `event_id` 做消费去重。
- Kafka producer 当前 `RequiredAcks=RequireOne`，只等 leader ack，不等所有副本确认。单机开发环境可接受，生产环境建议改为 `RequireAll` 并配置合理副本数。

## 3. Redis、Kafka、MySQL 的职责边界

| 组件 | 当前推荐职责 | 不应该承担的职责 |
| --- | --- | --- |
| MySQL | 核心业务事实、事务一致性、Outbox WAL、补偿任务查询来源 | 高频在线心跳、临时锁、短期推送状态 |
| Redis | Cache-Aside 缓存、在线状态、分布式锁、短 TTL 空值缓存、热点保护 | 未持久化的核心消息事实、唯一可靠扣减记录、普通 Key 形式的事件 WAL |
| Kafka | 服务间事件、在线推送、异步后处理、可重放事件流 | 替代业务数据库、保存无限期业务真相、覆盖生产前崩溃窗口 |

一句话判断：

- 用户能否最终看到某条消息，应由 MySQL 决定。
- 用户是否立刻收到在线推送，可由 Kafka/WebSocket 决定。
- 某个缓存是否命中，不应该决定业务事实是否存在。

## 4. 关键故障场景分析

| 场景 | 当前结果 | 风险等级 | 推荐补救 |
| --- | --- | --- | --- |
| MySQL 消息写入成功，Kafka `message.created` 发布前服务崩溃 | `event_outbox` 中保留待发布事件，重启后 worker 补发 | 低 | 已接入 Outbox，继续补监控告警 |
| MySQL 群成员写入成功，Kafka `group.member_invited` 发布前服务崩溃 | `event_outbox` 中保留待发布事件，重启后 worker 补发 | 中 | 已接入 Outbox，仍建议提供按 group_id 对账同步任务 |
| Kafka 发布成功，消费者处理前崩溃 | offset 未提交，重启后会重试 | 中 | handler 保持幂等，通用链路写 `processed_events`；@Agent 分发写 `agent_dispatch_records` |
| 消费者完成副作用，但提交 offset 前崩溃 | 重启后重复消费，可能重复写入或重复推送 | 中 | 消费者按 `event_id` 去重，数据库写入使用唯一键或 upsert；Agent 回复使用 `client_msg_id` 复用已落库消息 |
| Redis 删除缓存成功，Kafka 发布失败 | 缓存一致性不受影响，但实时事件可能丢失 | 中 | 缓存删除与事件发布分开看；事件可靠性靠 Outbox |
| Redis 扣减成功，Kafka 未发布，服务崩溃 | 如果 Redis 是唯一扣减事实，则可能永久丢业务事件 | 高 | 不允许普通 Redis Key 承担最终扣减事实；改为 DB 事务 + Outbox，或 Redis Stream + AOF + 对账 |
| Kafka broker 暂不可用 | Outbox worker 发布失败后记录 retry_count/next_retry_at，稍后重试 | 中 | 增加告警和失败事件巡检 |
| Kafka topic 被误删、retention 到期或 broker 数据损坏 | Kafka 中事件不可再重放 | 高 | MySQL 业务表 + Outbox/审计表保留可重建事实，提供重建任务 |

## 5. 用户提出的几个深水区问题

### 5.1 Redis 扣减完了，但 Kafka 还没有推送，服务挂了怎么办？

要先区分“Redis 扣减”的业务含义。

如果 Redis 扣减只是缓存层优化，例如缓存里的未读数、会话列表计数、临时限流计数，那么服务挂了之后可以接受短期不一致。最终应通过 MySQL 的真实消息、已读游标和状态表重新计算。

如果 Redis 扣减代表真正的业务资产，例如余额、配额、付费 token、库存、消息投递额度，那么普通 Redis Key 不应作为唯一事实来源。否则会出现：

1. Redis 扣减成功。
2. 业务 DB 未写入，或 Kafka 事件未发布。
3. 服务崩溃。
4. 系统只剩一个已经变化的 Redis 值，但缺少可审计、可重放、可补偿的业务记录。

推荐做法：

- 首选：MySQL 事务内写业务变更和 `event_outbox`，提交成功后由 Outbox worker 发布 Kafka。
- 如果必须 Redis 先行：使用 Redis Lua 保证原子扣减，并同时写 Redis Stream；Redis 开启 AOF；消费者使用 consumer group ACK；后台定期把 Stream 与 MySQL 对账。这个复杂度明显高于 MySQL Outbox，当前项目没有必要优先走这条路。

### 5.2 Kafka 丢消息了怎么办？

Kafka 是否“丢消息”要分阶段看：

- 生产者还没收到 broker ack：这条消息还不属于 Kafka 的可靠承诺范围，进程崩溃可能丢。
- broker 已 ack，但 topic 副本数不足或 `acks=1`：leader 写入后如果 leader 宕机且副本未同步，极端情况下仍可能丢。
- broker 已按生产配置可靠写入：消费者失败通常不会导致消息丢失，因为 offset 未提交会重试。
- topic retention 到期、topic 被删除、集群损坏：Kafka 中的旧事件仍可能不可用。

因此业务系统不能只说“用了 Kafka 就不丢”。正确说法是：

- Kafka 提供 broker ack 之后的事件日志能力。
- 应用需要 Outbox 覆盖 broker ack 之前的窗口。
- 消费者需要幂等覆盖重复消费窗口。
- 核心业务表需要能重建下游投影。

### 5.3 WAL 写到哪里？

当前项目里有三类“日志”容易混淆：

- MySQL redo log/binlog：数据库内部日志，保证 MySQL 行数据提交后的持久性和复制能力。
- Kafka commit log：Kafka topic 内部日志，保证已写入 Kafka 的消息可按 offset 消费。
- 应用层 WAL：业务服务自己记录“这个业务事实提交后必须发布哪些事件”。

当前 ClaranAIM 已经为群事件和消息事件加入应用层 WAL：MySQL `event_outbox` 表，并且和业务表写在同一个数据库事务里。

例如发送消息时，同一个事务内写：

- `messages`
- `message_user_states`
- `event_outbox(message.created)`

事务提交后，请求可以返回成功。后台 worker 扫描 `event_outbox`，发布 Kafka 成功后把 outbox 记录标记为 `published`。如果服务在提交后立刻崩溃，outbox 记录仍在 MySQL，重启后继续发布。

## 6. 推荐的 Transactional Outbox 设计

### 6.1 表结构

```sql
CREATE TABLE event_outbox (
  id BIGINT PRIMARY KEY,
  aggregate_type VARCHAR(50) NOT NULL,
  aggregate_id BIGINT NOT NULL,
  event_type VARCHAR(100) NOT NULL,
  event_key VARCHAR(100) NOT NULL,
  payload JSON NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_at DATETIME NOT NULL,
  locked_until DATETIME NULL,
  published_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uk_event_id (id),
  KEY idx_status_next_retry (status, next_retry_at),
  KEY idx_aggregate (aggregate_type, aggregate_id)
);
```

字段说明：

- `id`：事件 ID，使用项目统一雪花 ID。
- `aggregate_type`：聚合类型，例如 `message`、`group`、`billing`。
- `aggregate_id`：聚合根 ID，例如 message_id、group_id。
- `event_type`：事件类型，例如 `message.created`。
- `event_key`：Kafka 分区键，例如 conversation_id 或 group_id。
- `payload`：事件负载，内容应与 `pkg/events` 中的 payload 对齐。
- `status`：`pending/publishing/published/failed`。
- `retry_count` 与 `next_retry_at`：用于指数退避重试。
- `locked_until`：避免多个 worker 重复抢同一批事件。

### 6.2 消费者幂等表

```sql
CREATE TABLE processed_events (
  consumer_group VARCHAR(100) NOT NULL,
  event_id BIGINT NOT NULL,
  processed_at DATETIME NOT NULL,
  PRIMARY KEY (consumer_group, event_id)
);
```

消费者处理流程：

1. 收到 Kafka 事件。
2. 开启数据库事务。
3. 尝试插入 `(consumer_group, event_id)`。
4. 如果主键冲突，说明已处理过，直接提交 offset。
5. 如果插入成功，执行实际业务副作用。
6. 提交事务。
7. 提交 Kafka offset。

这样可以处理“副作用成功但 offset 未提交”导致的重复消费。

### 6.3 Outbox Worker 流程

```text
循环：
  1. 查询 status=pending 且 next_retry_at <= now 的事件
  2. 用 locked_until 抢占事件，避免多 worker 重复发布
  3. 发布到 Kafka，key=event_key，value=统一 Envelope JSON
  4. 发布成功后 status=published, published_at=now
  5. 发布失败后 retry_count+1, next_retry_at=now+backoff
  6. 超过最大重试仍失败时 status=failed，并告警
```

重要原则：

- 业务请求线程不再直接承担强可靠 Kafka 发布，只负责写业务表和 outbox。
- worker 可以水平扩展，但抢锁必须基于数据库条件更新。
- Kafka 事件中的 `event_id` 应使用 outbox ID，这样生产、消费、排查全链路一致。

## 7. 针对当前模块的影响评估

### 7.1 msg-core-service

影响程度：高。

原因：

- 它负责消息落库、消息状态、会话更新时间、已读、编辑、撤回。
- `message.*` 事件目前用于 WebSocket 在线推送，未来还会用于搜索索引、AI 摘要、离线推送。

建议：

- 先将 `message.created/message.edited/message.recalled/message.read` 写入 Outbox。
- WebSocket 推送、搜索索引、离线通知都从 Kafka 消费。
- 消息历史仍从 MySQL 查询，不能依赖 Kafka 作为唯一历史来源。

### 7.2 group-service

影响程度：高。

原因：

- `group.*` 事件用于 msg-core-service 同步群聊会话和参与者。
- 这是跨服务投影同步，不只是在线通知。

建议：

- 群创建、邀请、踢人、解散必须写 Outbox。
- 提供对账任务：按 group-service 的 `groups/group_members` 重建 msg-core-service 的 group conversation 和 participants。

### 7.3 websocket-gateway

影响程度：中。

原因：

- 它只负责在线连接和推送，不保存核心消息事实。
- 重复推送可能造成前端重复显示。

建议：

- 前端按 `msg_id` 去重。
- WebSocket 事件保留 `event_id` 和 `msg_id`。
- reconnect 后通过历史接口补齐 missed messages，而不是依赖 WebSocket 事件永不丢。

### 7.4 user-service

影响程度：中。

原因：

- 当前用户、好友、分组主要是同步 RPC 查询。
- 如果未来好友关系变化也要触发通知、搜索索引或推荐系统，就需要事件化。

建议：

- 短期不必强行事件化所有用户操作。
- 对好友申请、好友新增、用户资料更新等需要下游订阅的动作，再逐步接入 Outbox。

### 7.5 agent-manager-service

影响程度：中到高。

原因：

- Bot 对话计费已经写 MySQL，但未来工具调用、RAG、长任务、多 Agent 协作都适合事件化。
- 计费不能依赖 Redis 或 Kafka 作为唯一事实。

建议：

- `billing_records` 继续同步写 MySQL。
- 后续如需成本告警、审计、账单汇总，可从 Outbox 发布 `billing.recorded`。
- 模型 usage 缺失时不要估算计费，应记录 `chat_usage_missing` 并告警。

## 8. Redis 缓存策略与可靠性关系

当前项目采用 Cache-Aside：

- 读：先读 Redis，未命中读 MySQL，再写 Redis。
- 写：先写 MySQL，成功后删除相关 Redis 缓存。
- TTL：增加随机抖动，降低缓存雪崩。
- 空值缓存：短 TTL 缓存不存在对象，降低缓存穿透。
- 分布式锁：热点 key 未命中时，用 Redis 锁降低缓存击穿。

这套策略解决的是读性能和缓存一致性，不解决事件可靠性。

特别注意：

- “写后删除缓存”不能替代 Outbox。
- “Redis 分布式锁”不能替代事务。
- “Redis AOF”只能提高 Redis 自身持久性，不自动提供跨 MySQL/Kafka 的一致性。
- 缓存删除失败可以靠 TTL 兜底；业务事件丢失不能只靠 TTL 兜底。

## 9. 推荐演进路线

### Phase A：文档与约束固定

- 明确 MySQL 是核心事实来源。
- 明确 Redis 不承载核心消息 WAL。
- 明确 Kafka 事件当前是 at-least-once 倾向；`group.*` 和 `message.*` 的生产前窗口已由 MySQL Transactional Outbox 覆盖，其他未来事件接入前仍需逐项补 Outbox。
- 在 API 和架构文档中标注当前可靠性边界。

### Phase B：Outbox 基础设施

- `event_outbox` 模型、DAO 和非破坏性 AutoMigrate 已完成。
- group-service 和 msg-core-service 已内置 outbox worker。
- 待新增通用 `processed_events` 模型、DAO 和迁移脚本；@Agent 专项幂等已通过 `agent_dispatch_records(event_id, agent_user_id)` 落地。
- Kafka producer 改为可配置 `RequiredAcks`，生产环境建议 `RequireAll`。

### Phase C：关键事件改造

优先级：

1. `group.*`：已接入 Outbox，因为它影响跨服务会话/参与者投影。
2. `message.created`：已接入 Outbox，因为它影响在线推送、离线通知、未来搜索索引和 AI 后处理。
3. `message.edited/recalled/read`：已接入 Outbox，保证多端状态事件可重放。
4. `billing.recorded`：用于审计和成本告警。

### Phase D：补偿与对账

- group-service 到 msg-core-service 的群成员对账。
- msg-core-service 消息表到搜索索引的重建。
- messages/message_user_states 到离线通知状态的对账。
- billing_records 到成本统计的重算。

## 10. 当前项目不建议做的事

- 不建议把消息先写普通 Redis List/Hash，再定时批量落 MySQL。这会把最核心的消息事实放到非事务缓存中，失败场景复杂且排查困难。
- 不建议让 api-gateway 承担跨服务业务编排。跨服务同步应通过 RPC 查询或事件总线完成，网关只负责协议转换、认证、参数校验和聚合返回。
- 不建议把 Kafka 当数据库。Kafka 可以重放事件，但 topic retention、误删、schema 演进都会影响长期事实保存。
- 不建议在 usage 缺失时估算 Token 费用。计费必须严格来自模型返回或供应商账单。
- 不建议把当前消息发送、群成员同步这类高频事件链路整体改成 DTM Saga。Saga 会引入更多分支接口、补偿语义和协调器依赖，而这里真正要解决的是“业务 DB 提交后事件可靠发布”，Outbox 更直接、更轻，也更容易重放和对账。

## 10.1 DTM 在当前项目中的合理位置

当前项目中 DTM 已经落到一个低频跨服务业务：创建群组时由 api-gateway 提交 Saga，先调用 group-service DTM 分支创建群元数据和成员，再调用 msg-core-service DTM 分支创建群聊 conversation 和参与者。api-gateway 会预生成 `group_id` 与 `conversation_id`，避免 Saga 分支之间依赖上一步 HTTP 返回值。

已接入的分支接口：

- group-service：`POST /dtm/group/create`，补偿接口 `POST /dtm/group/create_compensate`。
- msg-core-service：`POST /dtm/message/group-conversation/create`，补偿接口 `POST /dtm/message/group-conversation/create_compensate`。

这条链路适合 DTM 的原因是：建群是低频操作，涉及两个服务的业务事实，并且有清晰补偿语义。如果 msg-core-service 创建群聊会话失败，DTM 可以回调 group-service 补偿删除刚创建的群。

DTM 适合解决“一个业务动作必须跨多个服务完成，失败时还要按业务语义补偿”的问题。例如：

- Bot 或用户购买配额：扣余额、增加配额、写计费记录，其中某一步失败后需要补偿。
- 付费文件/知识库资源开通：创建订单、授权资源、通知用户，失败后撤销授权。
- 未来 Agent 长任务预占资源：冻结额度、创建任务、启动执行器，执行失败后释放冻结。

这类流程天然有明确的正向接口和补偿接口，可以用 Saga/TCC 建模。相反，IM 消息落库和在线推送的核心事实在 MySQL，Kafka 事件是后续投影和通知，使用 Outbox 就能覆盖核心崩溃窗口；强行改成 DTM 会让简单可靠的事件发布路径变得更重。

## 11. 最终建议

短期内，ClaranAIM 应保持：

- MySQL 同步落核心业务事实。
- Redis 做缓存、锁和在线状态。
- Kafka 做实时事件和服务间通信。
- 客户端重连后通过历史接口补齐 WebSocket 漏推消息。

中期补强：

- 用 MySQL Transactional Outbox 作为应用层 WAL。
- 用通用 `processed_events` 保证消费者幂等；对会产生消息副作用的链路，同时为目标事实表设计业务幂等键，例如 Agent 回复的 `client_msg_id`。
- 用对账任务修复投影和下游索引。

这样设计后，即使出现“Redis 已变更、Kafka 未推送、服务重启”“Kafka 暂时不可用”“消费者重复处理”等情况，系统也有明确的事实来源、重试入口和补偿路径，而不是依赖某一次请求内的内存状态或临时缓存状态。

