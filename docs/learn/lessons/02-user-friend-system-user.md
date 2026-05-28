# 第 2 课：用户、好友与 Agent 系统用户

## 学习目标

这一课学习 user-service。你已经会用户 CRUD，所以重点看：

- 普通用户、系统用户、Agent 配置对象的区别。
- 好友双向关系的一致性。
- 最新登录和好友错误处理变化。
- Agent 为什么必须是系统用户。

## 源码入口

重点阅读：

- `internal/user-service/model/model.go`
- `internal/user-service/dao/dao.go`
- `internal/user-service/service/service.go`
- `internal/user-service/service/profile_update_test.go`
- `internal/agent-manager-service/service/service.go`
- `internal/api-gateway/handler/agent_handler.go`

## 三个概念

普通用户：

- 能注册登录。
- 有密码。
- 有资料。
- 有好友。
- 能发消息。

系统用户：

- `IsSystem = true`。
- 不能通过密码登录。
- 可作为消息 sender。
- Agent 会绑定一个系统用户。

Agent 配置对象：

- 存在 `bots` 表。
- 业务上表示 Agent。
- 保存模型、Prompt、工具策略、工作目录、owner。
- 通过 `agent_user_id` 绑定系统用户。

## 为什么 Agent 要是系统用户

如果 Agent 只是一个 `bot_id`，会带来很多额外分支：

- 消息 sender 无法统一。
- @ 列表要单独支持 Bot。
- 群成员要单独支持 Bot。
- 历史消息展示要特殊处理。
- 已读、回复、引用、撤回都要特殊处理。

Agent 作为系统用户后：

- Agent 回复就是普通消息。
- Agent 可以被加好友。
- Agent 可以进入群。
- Agent 可以被 @。
- 前端可以按用户头像和昵称渲染。

## 登录状态变化

当前登录逻辑里，用户密码校验成功后会更新在线状态。

最新变化：

```text
如果 UpdateUser 更新 online 状态失败
  -> 直接返回错误
```

旧逻辑可能忽略这个错误。现在更严格，因为登录状态影响：

- 好友在线展示。
- WebSocket 体验。
- 用户资料缓存。

## 好友双向关系

添加好友时会写两条关系：

```text
A -> B
B -> A
```

最新变化：

```text
正向 A->B 写成功
反向 B->A 写失败
  -> 回滚 A->B
  -> 返回 “添加反向好友关系失败”
```

删除好友时：

```text
删除 A->B
删除 B->A
```

最新变化：

```text
如果反向删除失败
  -> 返回错误
```

这比静默忽略更安全。因为好友关系是 IM 的基础事实，如果只删一半，后续会出现：

- A 看不到 B，B 还能看到 A。
- 私聊入口不一致。
- 缓存刷新不一致。

## 缓存

常见缓存：

- `user:info:{id}`
- `user:friends:{id}`
- `user:friend_groups:{id}`
- `online:user:{id}`

写操作后要删除相关缓存，让下一次读取回源。

## Agent 加好友

网关有：

```text
POST /agent/add-friend
```

大致流程：

```text
当前用户请求把某个 Agent 加为好友
  -> api-gateway 获取 Agent 配置
  -> 检查当前用户是 Agent owner
  -> 取 AgentUserID
  -> 调 user-service AddFriend
```

这一步体现了 Agent 作为真实用户的价值。

## 本课检查

你应该能回答：

- 普通用户、系统用户、Agent 配置对象有什么区别？
- 为什么系统用户不能密码登录？
- 好友关系为什么要双向写？
- 反向好友写入失败为什么要回滚？
- Agent 加好友最终落到哪个服务？

## 动手任务

1. 追踪 `AddFriend` 的正向/反向写入。
2. 阅读新增的好友失败测试。
3. 画出创建 Agent 时系统用户和 Agent 配置的关系。
4. 思考：删除 Agent 后，历史消息里的 sender 应该如何展示？

