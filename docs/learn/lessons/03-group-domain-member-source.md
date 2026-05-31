# 第 3 课：群组领域与群号入群

## 学习目标

这一课学习 group-service。你要掌握：

- 群、群成员、群角色如何建模。
- 10 位群号和自助入群。
- 群成员事实和会话参与者为什么分开。
- 群事件如何同步到 msg-core-service。
- Agent 作为系统用户入群后为什么能复用普通群机制。

## 源码入口

重点阅读：

- `internal/group-service/model/model.go`
- `internal/group-service/dao/dao.go`
- `internal/group-service/service/service.go`
- `internal/group-service/dtmbranch/handler.go`
- `internal/api-gateway/handler/group_handler.go`
- `internal/msg-core-service/eventconsumer/group_consumer.go`
- `internal/msg-core-service/service/service.go`

## group-service 负责什么

group-service 是群成员事实源：

- 创建群。
- 生成 10 位群号。
- 查询群。
- 邀请成员。
- 自助入群。
- 踢出成员。
- 设置管理员。
- 禁言。
- 解散群。
- 发布群事件。
12345656432123323
它不负责保存消息。

## 10 位群号

当前群 ID 是 10 位数字群号：

```text
1000000000 到 9999999999
```

用户可以复制群号，通过：

```text
POST /group/join
```

加入群。

## 自助入群链路

```text
api-gateway.JoinGroupByID
  -> 校验 group_id 是 10 位
  -> 检查群存在
  -> 检查当前用户是否已是成员
  -> 调 group-service.InviteMember(groupID, currentUser, [currentUser])
```

group-service 识别：

```text
operatorID == userIDs[0]
```

这是 self-join，可以不要求管理员权限。

普通邀请仍然需要 owner/admin 权限。

## 群成员事实 vs 会话参与者

`group_members` 表示：

- 谁属于这个群。
- 谁是 owner/admin/member。
- 谁被禁言。

`conversation_participants` 表示：

- 谁能看到这个会话。
- 谁是推送目标。
- 谁有已读游标、草稿、置顶、通知设置。

它们不能简单合并。私聊没有 `group_members`，但也有 `conversation_participants`。

## 群事件同步

group-service 写完群事实后，会写 Outbox：

```text
group.created
group.member_invited
group.member_kicked
group.deleted
```

msg-core-service 消费 `claran.group.events`：

```text
group.created
  -> 创建/复用群聊 conversation
  -> 同步 conversation_participants

group.member_invited
  -> 增加 conversation_participants
  -> 生成 group.member_joined IM 事件

group.member_kicked
  -> 移除/更新参与者
  -> 生成 group.member_left IM 事件
```

这样群服务和消息服务解耦，不需要 api-gateway 手动拼两个服务的写操作。

## Agent 入群

Agent 是 user-service 里的系统用户，所以它入群不需要特殊模型：

```text
AgentUserID
  -> group_members.user_id
  -> conversation_participants.user_id
  -> messages.sender_id
```

这也是 Agent-native IM 的基础。

## 本课检查

你应该能回答：

- 群号为什么直接用 group id？
- self-join 和普通邀请有什么不同？
- group_members 和 conversation_participants 有什么区别？
- 群成员变化如何同步到消息服务？
- Agent 入群为什么不需要单独的 bot_members 表？

## 动手任务

1. 追踪 `/group/join`。
2. 追踪 `saveGroupEvent`。
3. 追踪 msg-core-service 的 group consumer。
4. 设计一个“群需要审批才能加入”的扩展方案。

