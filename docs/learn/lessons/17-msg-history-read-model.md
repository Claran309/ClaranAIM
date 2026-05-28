# 第 17 课：Msg History 与消息读模型

## 学习目标

这一课专题学习历史消息服务。你要掌握：

- 为什么消息写入和历史查询可以拆开。
- msg-history-service 当前的定位。
- 离线同步、搜索、冷数据归档如何从这里演进。
- WebSocket 推送和历史拉取如何配合。

## 源码入口

重点阅读：

- `cmd/msg-history-service/main.go`
- `internal/msg-history-service/model/model.go`
- `internal/msg-history-service/dao/dao.go`
- `internal/msg-history-service/service/service.go`
- `internal/msg-history-service/handler/handler.go`
- `internal/msg-core-service/model/model.go`
- `internal/msg-core-service/service/service.go`

## 写模型和读模型

msg-core-service 当前是写模型核心：

- 创建会话。
- 发送消息。
- 编辑、撤回、已读。
- 写 Outbox。

msg-history-service 是读模型拆分方向：

- 历史查询。
- 离线消息。
- 搜索。
- 冷热分层。
- 归档。

读写分离的意义：

- 写路径保持短。
- 读路径可以做更多索引。
- 历史消息可以冷热分层。
- 搜索和离线补偿不挤压发送消息路径。

## WebSocket 与历史补偿

WebSocket 只负责实时通知。

可靠补偿靠历史查询：

```text
用户在线
  -> WebSocket 实时收到

用户离线/断网/网关重启
  -> 重新上线
  -> 按 conversation_id + before_id/游标 拉历史
```

这也是为什么消息事实必须在 MySQL，而不是只存在 WebSocket 推送里。

## 未来离线同步设计

可以引入：

- per-user 拉取游标。
- ack。
- 重试。
- 离线消息队列。
- 多端同步状态。

但不要把所有东西都塞进 WebSocket 网关。网关是推送层，不是消息事实层。

## 本课检查

你应该能回答：

- msg-core-service 和 msg-history-service 的边界是什么？
- 为什么 WebSocket 丢消息不可怕？
- 历史查询为什么适合独立服务？
- 搜索索引应该同步写还是异步建？

## 动手任务

1. 设计一个离线补偿接口。
2. 设计一个按时间范围搜索消息的索引方案。
3. 画出用户重连后的消息同步流程。

