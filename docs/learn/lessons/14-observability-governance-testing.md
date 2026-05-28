# 第 14 课：可观测性、治理与测试

## 学习目标

这一课学习工程化。你要掌握：

- 当前已有治理能力。
- 哪些测试覆盖了关键边界。
- 事件驱动和 Agent 链路该如何测。
- 上线前最该补哪些指标。

## 源码入口

重点阅读：

- `pkg/governance/kitex.go`
- `pkg/logger/logger.go`
- `pkg/outbox/worker_test.go`
- `pkg/eventbus/eventbus_test.go`
- `pkg/events/events_test.go`
- `internal/agent-manager-service/eventconsumer/agent_consumer_test.go`
- `internal/user-service/service/profile_update_test.go`
- `internal/msg-core-service/service/*_test.go`

## 当前治理能力

已有：

- API Gateway 限流。
- Kitex timeout。
- circuit breaker。
- Agent 长运行 RPC 配置。
- Zap 结构化日志。
- Outbox 重试。
- Kafka consumer group。

仍需补：

- Prometheus。
- Jaeger。
- Grafana。
- ELK。
- consumer lag 监控。
- Agent trace 面板。

## 测试重点

用户：

- 系统用户不能密码登录。
- 好友反向写失败回滚。
- 删除好友反向失败返回错误。

消息：

- client_msg_id 幂等。
- 本地删除。
- 编辑/撤回/已读。
- 翻译缓存。

事件：

- Envelope topic 映射。
- Outbox 成功发布。
- Outbox 失败重试。
- EventBus MemoryPublisher。

Agent：

- mention 触发。
- subscription rule。
- record 不调用 runtime。
- 重复事件不重复执行。
- 回复写回失败处理。

## 指标建议

优先监控：

- Outbox pending 数量。
- Outbox retrying 数量。
- Kafka consumer lag。
- WebSocket 在线连接数。
- 消息发送 QPS。
- Agent dispatch 成功率。
- Agent runtime p95。
- Agent pending approval 数量。
- Memory 召回数量。
- 翻译调用失败率。
- 文件上传失败率。

## trace 字段

Agent 链路建议统一字段：

- `source_event_id`
- `agent_trace_id`
- `bot_id`
- `agent_user_id`
- `conversation_id`
- `sender_id`
- `event_type`
- `decision`
- `client_msg_id`

## 本课检查

你应该能回答：

- 为什么 Agent 长运行 RPC 和普通 RPC 超时不同？
- Outbox 失败重试怎么测？
- 好友双向关系怎么测？
- Agent dispatcher 幂等怎么测？
- 生产最先该监控哪 5 个指标？

## 动手任务

1. 为 `agent_record` 写测试草案。
2. 为 `file.uploaded` 重复消费写测试草案。
3. 设计 Agent trace 查询接口。
4. 给 Outbox dashboard 画字段。

