# 2026-06-01 异步处理与缓存处理完善记录

## 背景

本次修改针对项目中异步事件链路和缓存策略的生产化缺口做补强，但按要求没有修改 Kafka 生产可靠性参数，`pkg/eventbus/kafka.go` 中 `RequiredAcks=RequireOne` 等开发期配置保持不变。

## 异步处理改动

### 1. Consumer 幂等与死信处理

新增 `pkg/eventbus/reliability.go` 和 `pkg/eventbus/gorm_reliability.go`：

- 新增 `ReliabilityStore` 抽象。
- 新增 `processed_events` 表模型 `GormDeliveryRecord`。
- 新增 `NewReliableHandler`，为 Kafka consumer 增加：
  - 已处理事件跳过。
  - 失败次数记录。
  - 超过阈值后写入 dead 状态并返回 nil，让 Kafka offset 可以提交，避免单条坏消息卡死分区。
- 新增 `MemoryReliabilityStore`，供 websocket-gateway 或测试环境使用。

已接入的消费者：

- `msg-core-service` 消费 `group.*` 事件时使用 MySQL `processed_events`。
- `agent-manager-service` 消费 message / IM 事件时使用 MySQL `processed_events`。
- `websocket-gateway` 消费消息事件时使用内存可靠性存储，避免重复推送和坏消息无限阻塞。

### 2. Outbox dead 状态与重放能力

扩展 `pkg/outbox/outbox.go`：

- 新增 `StatusDead`。
- `Worker` 新增 `maxRetries`，默认 10 次。
- Kafka 发布失败或 Outbox payload 解码失败超过重试上限后，事件进入 `dead`。
- 新增 `MarkDead` 和 `Requeue`：
  - `MarkDead`：停止普通 worker 自动扫描，保留错误原因。
  - `Requeue`：人工修复配置后可把事件重新放回 `pending`。

这解决了“无限 retry 但无人知道”和“坏事件卡住 Outbox 发布循环”的问题。

### 3. Agent 任务状态基础表

新增 `agent_tasks` 模型和仓储：

- `queued`
- `running`
- `completed`
- `failed`

当前 Agent consumer 仍同步调用 runtime，但会写入任务状态。这样后续可以无缝演进为 runtime 异步领取任务、心跳、取消、checkpoint 和后台重试。

## 缓存处理改动

### 1. 集中缓存策略

新增 `pkg/cache/strategy.go`，集中定义缓存 key、TTL、jitter 和空值缓存 TTL：

- `UserInfoPolicy`
- `UserFriendsPolicy`
- `FriendGroupsPolicy`
- `OnlineUserPolicy`
- `GroupInfoPolicy`
- `GroupMembersPolicy`
- `UserGroupsPolicy`
- `UserConversationsPolicy`
- `RecentConversationMessagesPolicy`

这样业务层不再散落 `user:xxx`、`group:xxx`、`conversation:xxx` magic string。

### 2. 核心服务接入统一策略

已替换的缓存路径：

- user-service
  - 用户资料缓存。
  - 好友列表缓存。
  - 好友分组缓存。
  - 在线状态缓存。
- group-service
  - 群资料缓存。
  - 群成员缓存。
  - 用户群列表缓存。
- msg-core-service
  - 用户会话列表缓存。
  - 最近消息缓存。

缓存仍坚持当前项目既定策略：

- 读路径：cache-aside。
- 不存在数据：短 TTL 空值缓存，防缓存穿透。
- 热点未命中：Redis 锁降低击穿。
- TTL：增加随机 jitter，降低雪崩。
- 写路径：数据库更新成功后删除相关缓存，下一次读取再回源重建。

## 未修改内容

按本次要求，未修改 Kafka 生产可靠性参数：

- 未把 `RequiredAcks` 改为 `RequireAll`。
- 未改 topic 副本数、`min.insync.replicas` 等 broker 级配置。
- 未调整 Kafka Writer 的重试和超时策略。

## 验证

已执行：

```powershell
$env:GOCACHE='D:\CodeStudy\GoProjects\src\ClaranAIM\.gocache'
go test ./...
```

结果：全量通过。

## 后续建议

1. 给 `processed_events` 和 Outbox dead 事件补管理接口与后台页面，支持查看、重放和跳过。
2. websocket-gateway 如果后续也接 MySQL 或轻量本地持久化，应把内存可靠性存储换成持久化存储。
3. Agent runtime 后续应从当前“同步执行但记录 task”演进为真正的任务领取模型。
4. Kafka 生产参数等你确认部署环境后再统一调整。
