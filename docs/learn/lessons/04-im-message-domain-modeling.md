# 第 4 课：IM 消息领域建模

## 学习目标

这一课学习 msg-core-service。你要掌握：

- 消息事实和用户本地视图。
- 会话、参与者、消息、已读、编辑、撤回。
- 文件/图片/语音消息如何进入 IM。
- 手动翻译为什么放在 msg-core-service。
- 消息事件和 Agent-native IM 事件如何产生。

## 源码入口

重点阅读：

- `internal/msg-core-service/model/model.go`
- `internal/msg-core-service/dao/dao.go`
- `internal/msg-core-service/service/service.go`
- `internal/msg-core-service/service/translation.go`
- `internal/msg-core-service/service/translator_llm.go`
- `internal/msg-core-service/transport/http.go`
- `pkg/events/events.go`

## 核心模型

`conversations`：

- 会话事实。
- private/group。
- 群聊通过 `group_id` 对齐 group-service。

`conversation_participants`：

- 会话参与者。
- 已读游标。
- 草稿。
- 置顶。
- 通知设置。

`messages`：

- 消息服务端事实。
- sender、content、msg_type、reply_to、status、mention。

`message_user_states`：

- 某用户对某消息的视图。
- delivered/read/local_deleted。

`message_edit_records`：

- 编辑历史。

`message_translations`：

- 手动翻译结果缓存。
- 按用户、消息、目标语言、源内容 hash 复用。

## 消息事实和用户视图

核心思想：

```text
messages = 所有人共享的消息事实
message_user_states = 每个用户自己的消息视图
```

所以：

- 本地删除只影响自己。
- 撤回影响全局消息状态。
- 已读是每个用户自己的状态。
- 编辑要留审计。

## SendMessage 主链路

```text
1. 校验会话和发送者权限
2. 群聊时校验群成员和禁言
3. 使用 client_msg_id 做发送幂等
4. 写 messages
5. 写 message_user_states
6. 更新 conversation
7. 写 message.created Outbox
8. 写 Agent-native IM Outbox
9. 提交事务
```

注意：WebSocket 和 Agent 都不在这个事务里同步执行，它们消费事件。

## 文件、图片、语音消息

文件上传链路：

```text
api-gateway 上传文件
  -> file-service 保存元数据
  -> 前端发送 file_id/url/name 作为消息 content
  -> msg-core-service 保存消息引用
```

msg-core-service 根据 `msg_type` 生成 Agent-native 事件：

```text
image/file -> file.uploaded
voice      -> voice.transcribed
```

当前语音转写更多是事件语义预留，真实 ASR 仍是后续增强。

## 手动翻译

翻译入口：

```text
POST /message/translate
```

链路：

```text
api-gateway
  -> msg-core-service
  -> 校验用户能看该消息
  -> settings-service 读取翻译 Prompt/LLM 配置
  -> OpenAI-compatible chat completions
  -> message_translations 缓存
```

为什么不自动翻译？

- 每条消息都调用 LLM 成本高。
- 隐私扩散风险大。
- 延迟和限流影响聊天体验。
- 手动触发更可控。

## 消息事件

传统消息事件：

```text
claran.message.events
```

主要给：

- websocket-gateway。
- 兼容旧 Agent 触发。

Agent-native IM 事件：

```text
claran.im.events
```

给：

- agent-manager-service。
- 后续 RAG/Memory/Task 消费者。

## 本课检查

你应该能回答：

- 为什么本地删除不删除 `messages`？
- `client_msg_id` 解决什么问题？
- 文件为什么不直接存进消息表？
- 翻译为什么是手动触发？
- `message.created` 和 `file.uploaded` 分别服务什么消费者？

## 动手任务

1. 追踪一次文本消息发送。
2. 追踪一次文件消息发送。
3. 追踪 `/message/translate`。
4. 画出编辑消息时写哪些表、发哪些事件。

