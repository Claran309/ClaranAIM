# ClaranAIM API 文档

> 基础地址: `http://localhost:8080/api/v1`
> WebSocket 地址: `ws://localhost:8081/ws`
> 认证方式: Bearer Token (JWT)

***

## 通用说明

### 响应格式

所有接口统一返回以下 JSON 结构：

```json
{
  "code": 0,
  "message": "success",
  "data": { ... }
}
```

| 字段      | 类型     | 说明                                         |
| ------- | ------ | ------------------------------------------ |
| code    | int    | 0=成功, -1=服务器错误, 400=参数错误, 401=未认证, 403=无权限 |
| message | string | 响应描述                                       |
| data    | object | 响应数据，失败时为 null                             |

### 认证机制

- 公开接口（注册/登录）无需 Token
- 其余接口需在请求头携带：`Authorization: Bearer <token>`
- Token 有效期由环境变量 `JWT_EXPIRATION` 控制（默认 168 小时 = 7 天）

***

## 一、用户模块 (user-service)

### 1.1 用户注册

**POST** `/user/register`

无需认证。

| 参数       | 类型     | 必填 | 说明                  |
| -------- | ------ | -- | ------------------- |
| username | string | 是  | 用户名，唯一              |
| password | string | 是  | 密码                  |
| nickname | string | 否  | 昵称，为空时默认等于 username |

请求示例：

```json
{
  "username": "alice",
  "password": "123456",
  "nickname": "Alice"
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "user_id": 1,
    "msg": "注册成功"
  }
}
```

核心逻辑：

- 检查用户名是否已存在
- 使用 bcrypt 加密密码（cost=10）
- 昵称为空时默认等于用户名
- 注册成功后缓存用户信息到 Redis（key: `user:info:{id}`，TTL 15min）

***

### 1.2 用户登录

**POST** `/user/login`

无需认证。

| 参数       | 类型     | 必填 | 说明  |
| -------- | ------ | -- | --- |
| username | string | 是  | 用户名 |
| password | string | 是  | 密码  |

请求示例：

```json
{
  "username": "alice",
  "password": "123456"
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user_id": 1,
    "msg": "登录成功"
  }
}
```

核心逻辑：

- bcrypt 校验密码
- 生成 JWT Token（HS256 签名，包含 userID + username + 过期时间）
- 更新用户状态为 online
- 缓存用户信息 + 在线状态到 Redis（key: `online:user:{id}`，TTL 30min）

***

### 1.3 用户登出

**POST** `/user/logout`

需要认证。

核心逻辑：

- 更新用户状态为 offline
- 删除 Redis 在线状态缓存
- 前端断开 WebSocket 连接

***

### 1.4 获取用户信息

**GET** `/user/info`

需要认证。

无请求参数（从 JWT Token 中提取 userID）。

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1,
    "username": "alice",
    "nickname": "Alice",
    "avatar": "",
    "email": "",
    "phone": "",
    "status": "online",
    "created_at": "2026-05-11T22:50:00+08:00",
    "updated_at": "2026-05-11T22:50:00+08:00"
  }
}
```

核心逻辑：

- 优先从 Redis 缓存读取（key: `user:info:{id}`）
- 缓存未命中则查 MySQL，查到后回写缓存

***

### 1.5 批量获取用户信息

**POST** `/user/batch_info`

需要认证。

| 参数      | 类型       | 必填 | 说明          |
| ------- | -------- | -- | ----------- |
| user_ids | []int64 | 是  | 用户ID列表      |

请求示例：

```json
{
  "user_ids": [1, 2, 3]
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "users": [
      { "id": 1, "nickname": "Alice", "avatar": "", "status": "online" },
      { "id": 2, "nickname": "Bob", "avatar": "", "status": "offline" }
    ]
  }
}
```

核心逻辑：

- 批量查询用户信息，用于前端显示昵称和头像
- 优先从 Redis 缓存读取

***

### 1.6 更新用户信息

**PUT** `/user/info`

需要认证。

| 参数       | 类型     | 必填 | 说明   |
| -------- | ------ | -- | ---- |
| nickname | string | 否  | 新昵称  |
| email    | string | 否  | 新邮箱  |
| phone    | string | 否  | 新手机号 |

请求示例：

```json
{
  "nickname": "NewAlice",
  "email": "alice@example.com"
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "msg": "更新成功"
  }
}
```

核心逻辑：

- 只更新非空字段
- 更新后刷新 Redis 缓存（删除旧缓存 + 写入新缓存）

***

### 1.7 更新用户头像

**PUT** `/user/avatar`

需要认证。

| 参数     | 类型     | 必填 | 说明    |
| ------ | ------ | -- | ----- |
| avatar | string | 是  | 头像URL |

核心逻辑：

- 更新头像URL到数据库
- 刷新 Redis 缓存

***

### 1.8 添加好友

**POST** `/user/friend/add`

需要认证。

| 参数         | 类型     | 必填 | 说明            |
| ---------- | ------ | -- | ------------- |
| friend_id  | int64  | 是  | 好友用户ID        |
| group_id   | int64  | 否  | 好友分组ID，0=默认分组 |
| remark     | string | 否  | 好友备注          |

核心逻辑：

- 不能添加自己为好友
- 检查是否已是好友关系
- 双向添加好友（A→B 和 B→A 各一条记录）
- 清除双方好友列表缓存（key: `user:friends:{id}`）

***

### 1.9 删除好友

**POST** `/user/friend/delete`

需要认证。

| 参数         | 类型    | 必填 | 说明     |
| ---------- | ----- | -- | ------ |
| friend_id  | int64 | 是  | 好友用户ID |

核心逻辑：

- 双向删除好友关系
- 清除双方好友列表缓存

***

### 1.10 获取好友列表

**GET** `/user/friend/list`

需要认证。

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "friends": [
      {
        "id": 1,
        "user_id": 1,
        "friend_id": 2,
        "group_id": 0,
        "remark": "我的同事",
        "friend_name": "Bob",
        "friend_avatar": "",
        "friend_status": "online",
        "group_name": ""
      }
    ]
  }
}
```

核心逻辑：

- 优先从 Redis 缓存读取（key: `user:friends:{id}`，TTL 5min）
- 缓存未命中则查 MySQL，关联查询好友用户信息和分组名称

***

### 1.11 创建好友分组

**POST** `/user/friend/group`

需要认证。

| 参数   | 类型     | 必填 | 说明   |
| ---- | ------ | -- | ---- |
| name | string | 是  | 分组名称 |

***

### 1.12 获取好友分组列表

**GET** `/user/friend/groups`

需要认证。

核心逻辑：

- 缓存 key: `user:friend_groups:{id}`，TTL 10min

***

## 二、群组模块 (group-service)

### 2.1 创建群组

**POST** `/group/create`

需要认证。

| 参数          | 类型       | 必填 | 说明         |
| ----------- | -------- | -- | ---------- |
| name        | string   | 是  | 群组名称       |
| member_ids  | []int64  | 否  | 初始成员用户ID列表 |

请求示例：

```json
{
  "name": "项目讨论组",
  "member_ids": [2, 3]
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "group_id": 1,
    "msg": "创建成功"
  }
}
```

核心逻辑：

- 创建者自动成为群主（role=owner）
- 其他成员角色为 member
- 群主不重复添加
- 清除群组相关 Redis 缓存

***

### 2.2 获取群组信息

**GET** `/group/:id`

需要认证。

| 参数 | 类型    | 必填      | 说明   |
| -- | ----- | ------- | ---- |
| id | int64 | 是（路径参数） | 群组ID |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "group": {
      "id": 1,
      "name": "项目讨论组",
      "avatar": "",
      "owner_id": 1,
      "announcement": "",
      "is_pinned": false,
      "created_at": "2026-05-11T22:50:00+08:00"
    }
  }
}
```

核心逻辑：

- 优先从 Redis 缓存读取（key: `group:info:{id}`，TTL 10min）

***

### 2.3 获取用户所在群组列表

**GET** `/group/list`

需要认证。

核心逻辑：

- 通过 JOIN group_members 表查询用户所在的所有群组
- 优先从 Redis 缓存读取（key: `user:groups:{id}`，TTL 5min）

***

### 2.4 邀请成员

**POST** `/group/invite`

需要认证。

| 参数        | 类型       | 必填 | 说明         |
| --------- | -------- | -- | ---------- |
| group_id  | int64    | 是  | 群组ID       |
| user_ids  | []int64  | 是  | 被邀请的用户ID列表 |

核心逻辑：

- 操作者必须是群主或管理员（owner/admin）
- 已在群中的用户不重复添加
- 新成员角色默认为 member
- 清除群组缓存

***

### 2.5 踢出成员

**POST** `/group/kick`

需要认证。

| 参数        | 类型    | 必填 | 说明       |
| --------- | ----- | -- | -------- |
| group_id  | int64 | 是  | 群组ID     |
| user_id   | int64 | 是  | 被踢出的用户ID |

核心逻辑：

- 操作者必须是群主或管理员
- 不能踢出群主
- 清除群组缓存

***

### 2.6 获取群组成员列表

**GET** `/group/:id/members`

需要认证。

| 参数 | 类型    | 必填      | 说明   |
| -- | ----- | ------- | ---- |
| id | int64 | 是（路径参数） | 群组ID |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "members": [
      {
        "id": 1,
        "group_id": 1,
        "user_id": 1,
        "role": "owner",
        "muted_until": null,
        "joined_at": "2026-05-11T22:50:00+08:00"
      }
    ]
  }
}
```

核心逻辑：

- 优先从 Redis 缓存读取（key: `group:members:{id}`，TTL 5min）

***

### 2.7 转让群主

**POST** `/group/transfer`

需要认证。

| 参数            | 类型    | 必填 | 说明        |
| ------------- | ----- | -- | --------- |
| group_id      | int64 | 是  | 群组ID      |
| new_owner_id  | int64 | 是  | 新群主的用户ID  |

请求示例：

```json
{
  "group_id": 1,
  "new_owner_id": 2
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "msg": "转让成功"
  }
}
```

核心逻辑：

- 仅当前群主可操作
- 新群主必须是群成员
- 原群主降级为普通成员（role=member）
- 清除群组相关缓存

***

### 2.8 更新群信息

**PUT** `/group/info`

需要认证。

| 参数            | 类型     | 必填 | 说明   |
| ------------- | ------ | -- | ---- |
| group_id      | int64  | 是  | 群组ID |
| name          | string | 否  | 群名称  |
| announcement  | string | 否  | 群公告  |

请求示例：

```json
{
  "group_id": 1,
  "name": "新群名称",
  "announcement": "本周五开会"
}
```

核心逻辑：

- 仅群主或管理员可操作
- 只更新非空字段
- 清除群组缓存

***

### 2.9 置顶/取消置顶群聊

**POST** `/group/pin`

需要认证。

| 参数        | 类型      | 必填 | 说明               |
| --------- | ------- | -- | ---------------- |
| group_id  | int64   | 是  | 群组ID             |
| is_pinned | bool    | 是  | true=置顶，false=取消 |

请求示例：

```json
{
  "group_id": 1,
  "is_pinned": true
}
```

核心逻辑：

- 置顶后群聊在会话列表中置顶显示
- 清除群组缓存

***

### 2.10 禁言成员

**POST** `/group/mute`

需要认证。

| 参数                | 类型    | 必填 | 说明           |
| ----------------- | ----- | -- | ------------ |
| group_id          | int64 | 是  | 群组ID         |
| user_id           | int64 | 是  | 被禁言的用户ID     |
| duration_minutes  | int64 | 是  | 禁言时长（分钟）     |

请求示例：

```json
{
  "group_id": 1,
  "user_id": 3,
  "duration_minutes": 30
}
```

核心逻辑：

- 仅群主或管理员可操作
- 不能禁言群主
- 设置 muted_until = 当前时间 + duration_minutes
- 清除群成员缓存

***

### 2.11 解除禁言

**POST** `/group/unmute`

需要认证。

| 参数        | 类型    | 必填 | 说明        |
| --------- | ----- | -- | --------- |
| group_id  | int64 | 是  | 群组ID      |
| user_id   | int64 | 是  | 解除禁言的用户ID |

核心逻辑：

- 仅群主或管理员可操作
- 将 muted_until 设为 NULL
- 清除群成员缓存

***

### 2.12 设置成员角色

**POST** `/group/role`

需要认证。

| 参数        | 类型     | 必填 | 说明                          |
| --------- | ------ | -- | --------------------------- |
| group_id  | int64  | 是  | 群组ID                        |
| user_id   | int64  | 是  | 成员用户ID                      |
| role      | string | 是  | 角色：`admin`（管理员）/ `member`（成员） |

请求示例：

```json
{
  "group_id": 1,
  "user_id": 2,
  "role": "admin"
}
```

核心逻辑：

- 仅群主可操作
- 不能修改群主角色
- 清除群成员缓存

***

### 2.13 解散群组

**POST** `/group/delete`

需要认证。

| 参数        | 类型    | 必填 | 说明   |
| --------- | ----- | -- | ---- |
| group_id  | int64 | 是  | 群组ID |

核心逻辑：

- 仅群主可操作
- 删除群组及所有成员记录
- 清除群组相关缓存

***

## 三、消息模块 (msg-core-service)

### 3.1 创建会话

**POST** `/message/conversation`

需要认证。

| 参数               | 类型       | 必填 | 说明                               |
| ---------------- | -------- | -- | -------------------------------- |
| type             | string   | 是  | 会话类型：`private`(私聊) / `group`(群聊) |
| participant_ids  | []int64  | 是  | 参与者用户ID列表（至少2人）                  |
| group_id         | int64    | 群聊必填 | 群ID，群聊会话用于和 group-service 对齐成员关系 |

请求示例：

```json
{
  "type": "private",
  "participant_ids": [1, 2]
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "conversation_id": 1,
    "msg": "创建会话成功"
  }
}
```

核心逻辑：

- 私聊去重：如果两人已有私聊会话，直接返回已有会话ID
- 群聊按 group_id 复用会话，并同步新增成员到 conversation_participants 表
- 创建会话后添加所有参与者到 conversation_participants 表
- 清除所有参与者的会话列表缓存

***

### 3.2 发送消息

**POST** `/message/send`

需要认证。

| 参数               | 类型       | 必填 | 说明                           |
| ---------------- | -------- | -- | ---------------------------- |
| conversation_id  | int64    | 是  | 会话ID                         |
| content          | string   | 是  | 消息内容                         |
| msg_type         | string   | 否  | 默认 `text`，可选 `image`/`voice`/`file`/`broadcast` |
| reply_to_id      | int64    | 否  | 被引用/回复的消息ID                  |
| mention_user_ids | []int64  | 否  | @ 的用户ID列表                     |
| mention_all      | bool     | 否  | 是否 @ 所有人                      |

请求示例（文本消息）：

```json
{
  "conversation_id": 1,
  "content": "@2 收到，我回复这条",
  "msg_type": "text",
  "reply_to_id": 41,
  "mention_user_ids": [2],
  "mention_all": false
}
```

请求示例（图片消息）：

```json
{
  "conversation_id": 1,
  "content": "[img]%2Ffiles%2Fimage%2Fabc123.png|file-id|abc123.png[/img]",
  "msg_type": "image"
}
```

请求示例（文件消息）：

```json
{
  "conversation_id": 1,
  "content": "[file]%2Ffiles%2Ffile%2Fdoc.pdf|file-id|项目文档.pdf[/file]",
  "msg_type": "file"
}
```

请求示例（语音消息）：

```json
{
  "conversation_id": 1,
  "content": "[voice]%2Ffiles%2Fvoice%2Fvoice.webm|file-id|00:03 voice.webm[/voice]",
  "msg_type": "voice"
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "msg_id": 42,
    "msg": "发送成功",
    "send_time": "2026-05-11 23:15:30"
  }
}
```

核心逻辑（完整流程）：

1. 校验消息内容非空 + 会话存在
2. 校验发送者是会话参与者；群聊会额外校验群成员与禁言状态
3. 如果 reply_to_id > 0，校验引用消息属于当前会话
4. 消息写入 MySQL（messages 表），包含 reply_to_id、status、mention_user_ids、mention_all
5. 更新会话的 updated_at 为消息创建时间
6. 缓存最近消息到 Redis（key: `conversation:recent:{id}`，TTL 10min）
7. 清除所有参与者的会话列表缓存
8. **WebSocket 实时推送**：获取会话所有参与者ID → 调用 pushClient.PushMessage() → websocket-gateway 的 `/push` API → Hub.Broadcast() → 所有在线参与者浏览器实时收到消息

***

### 3.3 标记会话已读

**POST** `/message/read`

需要认证。

| 参数              | 类型    | 必填 | 说明                         |
| ---------------- | ----- | -- | ---------------------------- |
| conversation_id  | int64 | 是  | 会话ID                       |
| message_id       | int64 | 否  | 已读到的消息ID；为空时默认最新消息 |

请求示例：

```json
{
  "conversation_id": 1,
  "message_id": 42
}
```

核心逻辑：

- 更新 conversation_participants 的 `last_read_message_id` 和 `last_read_at`
- 会话列表的 unread_count 基于该游标计算

***

### 3.4 编辑消息

**PUT** `/message/edit`

需要认证。仅消息发送者可编辑，已撤回消息不能编辑。

| 参数       | 类型     | 必填 | 说明     |
| ---------- | ------ | -- | -------- |
| message_id | int64  | 是  | 消息ID   |
| content    | string | 是  | 新消息内容 |

编辑成功后写入 `message_edit_records`，并通过 WebSocket 推送 `message_edited`。

***

### 3.5 撤回消息

**POST** `/message/recall`

需要认证。仅消息发送者可撤回，默认限时 2 分钟。

| 参数       | 类型    | 必填 | 说明   |
| ---------- | ----- | -- | ------ |
| message_id | int64 | 是  | 消息ID |

撤回成功后消息 `status` 更新为 `recalled`，正文清空，并通过 WebSocket 推送 `message_recalled`。

***

### 3.6 获取消息历史

**GET** `/message/history/:id`

需要认证。

| 参数         | 类型    | 必填      | 说明                   |
| ---------- | ----- | ------- | -------------------- |
| id         | int64 | 是（路径参数） | 会话ID                 |
| limit      | int64 | 否       | 每页条数，默认 50           |
| before_id  | int64 | 否       | 翻页游标，加载此ID之前的消息，默认 0 |

请求示例：`GET /message/history/1?limit=50&before_id=0`

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "messages": [
      {
        "id": 1,
        "conversation_id": 1,
        "sender_id": 1,
        "content": "你好！",
        "msg_type": "text",
        "reply_to_id": 0,
        "status": "sent",
        "is_edited": false,
        "edited_at": "",
        "mention_user_ids": [],
        "mention_all": false,
        "created_at": "2026-05-11 23:15:30"
      }
    ]
  }
}
```

核心逻辑：

- 按 ID 降序查询（最新在前），返回时反转为时间正序
- before_id > 0 时实现游标分页

***

### 3.7 搜索消息

**GET** `/message/search`

需要认证。

| 参数              | 类型     | 必填 | 说明                    |
| --------------- | ------ | -- | --------------------- |
| keyword         | string | 是  | 搜索关键词                 |
| conversation_id | int64  | 否  | 会话ID，指定后仅在当前会话搜索     |
| limit           | int64  | 否  | 返回条数，默认 20            |
| start_at        | string | 否  | 起始时间，支持 `YYYY-MM-DD`、`YYYY-MM-DD HH:mm:ss`、RFC3339 |
| end_at          | string | 否  | 结束时间，格式同 start_at     |

核心逻辑：

- 指定 conversation_id 时仅搜索该会话的消息
- 未指定时搜索用户所在所有会话的消息
- start_at/end_at 用于时间范围过滤
- 使用 MySQL LIKE 模糊匹配

***

### 3.8 获取用户会话列表

**GET** `/message/conversations`

需要认证。

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "conversations": [
      {
        "conversation_id": 1,
        "type": "private",
        "last_message": "你好！",
        "last_message_time": "2026-05-11 23:15:30",
        "unread_count": 0,
        "participant_ids": [1, 2],
        "last_sender_id": 1,
        "group_id": 0,
        "is_deleted_group": false
      }
    ]
  }
}
```

核心逻辑：

- 优先从 Redis 缓存读取（key: `user:conversations:{id}`，TTL 5min）
- 查询用户参与的所有会话，附带最后一条消息
- 群聊已解散时保留会话项并标记 `is_deleted_group=true`

***

## 四、文件模块 (file-service)

### 4.1 上传文件

**POST** `/file/upload`

需要认证。请求格式为 `multipart/form-data`。

| 参数        | 类型     | 必填 | 说明                                    |
| --------- | ------ | -- | ------------------------------------- |
| file      | File   | 是  | 上传的文件                                 |
| file_type | string | 否  | 文件类型分类：`image`/`voice`/`file`，默认 `file` |

请求示例（curl）：

```bash
curl -X POST http://localhost:8080/api/v1/file/upload \
  -H "Authorization: Bearer <token>" \
  -F "file=@/path/to/image.jpg" \
  -F "file_type=image"
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "file_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "file_url": "http://localhost:9000/claran-files/image/a1b2c3d4-e5f6-7890-abcd-ef1234567890.jpg",
    "file_name": "image.jpg",
    "file_type": "image",
    "file_size": 102400,
    "msg": "上传成功"
  }
}
```

核心逻辑：

1. 从 form-data 中读取文件
2. 生成 UUID 作为文件ID
3. 如果 MinIO 可用：上传到 MinIO 对象存储（Bucket: `claran-files`）
4. 如果 MinIO 不可用：保存到本地 `./storage/{file_type}/` 目录
5. 写入文件记录到 MySQL（file_records 表）
6. 返回文件URL供消息发送使用

***

### 4.2 获取文件信息

**GET** `/file/:id`

需要认证。

| 参数 | 类型     | 必填      | 说明   |
| -- | ------ | ------- | ---- |
| id | string | 是（路径参数） | 文件ID |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "file_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "file_url": "http://localhost:9000/claran-files/image/xxx.jpg",
    "file_name": "image.jpg",
    "file_type": "image",
    "file_size": 102400,
    "content_type": "image/jpeg",
    "uploader_id": 1,
    "created_at": "2026-05-12 10:30:00"
  }
}
```

***

### 4.3 删除文件

**DELETE** `/file/:id`

需要认证。

| 参数 | 类型     | 必填      | 说明   |
| -- | ------ | ------- | ---- |
| id | string | 是（路径参数） | 文件ID |

核心逻辑：

- 仅文件上传者可删除
- 同时删除 MinIO/本地存储中的文件和数据库记录

***

### 4.4 获取文件列表

**GET** `/file/list`

需要认证。

| 参数        | 类型     | 必填 | 说明                       |
| --------- | ------ | -- | ------------------------ |
| file_type | string | 否  | 按类型筛选：image/voice/file  |
| limit     | int64  | 否  | 每页条数，默认 20               |
| offset    | int64  | 否  | 偏移量，默认 0                 |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "files": [
      {
        "file_id": "xxx",
        "file_name": "image.jpg",
        "file_type": "image",
        "file_size": 102400,
        "created_at": "2026-05-12 10:30:00"
      }
    ],
    "total": 15
  }
}
```

***

## 五、AI 助手模块 (bot-manager-service)

### 5.1 创建 AI 助手

**POST** `/bot/create`

需要认证。

| 参数            | 类型     | 必填 | 说明                              |
| ------------- | ------ | -- | ------------------------------- |
| name          | string | 是  | 助手名称                            |
| type          | string | 是  | 类型：`internal`（内部Bot）/ `custom`（自部署Bot） |
| description   | string | 否  | 助手描述                            |
| model_name    | string | 是  | LLM 模型名称（如 gpt-4o-mini）        |
| api_key       | string | 否  | LLM API Key                     |
| base_url      | string | 否  | LLM API Base URL                |
| system_prompt | string | 否  | 系统提示词                           |
| skills_dir    | string | 否  | 技能目录路径                          |
| agent_root    | string | 否  | Agent 根目录路径                     |

请求示例：

```json
{
  "name": "Amiya",
  "type": "internal",
  "description": "项目助手，帮助解答技术问题",
  "model_name": "gpt-4o-mini",
  "api_key": "sk-xxx",
  "base_url": "https://api.openai.com/v1",
  "system_prompt": "你是一个友好的技术助手，帮助用户解决编程问题。"
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "bot_id": 1,
    "msg": "创建成功"
  }
}
```

核心逻辑：

- 创建 Bot 记录到 MySQL（bots 表）
- internal 类型使用内置 Agent 框架（eino adk）
- custom 类型支持自部署 Bot 接入

***

### 5.2 更新 AI 助手

**PUT** `/bot/update`

需要认证。

| 参数            | 类型      | 必填 | 说明        |
| ------------- | ------- | -- | --------- |
| bot_id        | int64   | 是  | Bot ID    |
| name          | string  | 否  | 新名称       |
| description   | string  | 否  | 新描述       |
| model_name    | string  | 否  | 新模型名称     |
| api_key       | string  | 否  | 新 API Key |
| base_url      | string  | 否  | 新 Base URL |
| system_prompt | string  | 否  | 新系统提示词    |
| skills_dir    | string  | 否  | 新技能目录     |
| agent_root    | string  | 否  | 新 Agent 根目录 |
| is_active     | bool    | 否  | 是否启用      |

核心逻辑：

- 仅 Bot 所有者可操作
- 更新后清除 Agent 缓存（下次对话时重新创建 Agent）

***

### 5.3 获取 AI 助手详情

**GET** `/bot/:id`

需要认证。

| 参数 | 类型    | 必填      | 说明      |
| -- | ----- | ------- | ------- |
| id | int64 | 是（路径参数） | Bot ID  |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "bot": {
      "id": 1,
      "name": "Amiya",
      "type": "internal",
      "description": "项目助手",
      "model_name": "gpt-4o-mini",
      "base_url": "https://api.openai.com/v1",
      "system_prompt": "你是一个友好的技术助手...",
      "owner_id": 1,
      "is_active": true,
      "created_at": "2026-05-12 10:00:00",
      "updated_at": "2026-05-12 10:00:00"
    }
  }
}
```

***

### 5.4 获取 AI 助手列表

**GET** `/bot/list`

需要认证。

| 参数   | 类型     | 必填 | 说明                           |
| ---- | ------ | -- | ---------------------------- |
| type | string | 否  | 按类型筛选：internal/custom，空=全部 |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "bots": [
      {
        "id": 1,
        "name": "Amiya",
        "type": "internal",
        "description": "项目助手",
        "model_name": "gpt-4o-mini",
        "is_active": true,
        "created_at": "2026-05-12 10:00:00"
      }
    ]
  }
}
```

***

### 5.5 删除 AI 助手

**DELETE** `/bot/delete`

需要认证。

| 参数     | 类型    | 必填 | 说明       |
| ------ | ----- | -- | -------- |
| bot_id | int64 | 是  | Bot ID   |

核心逻辑：

- 仅 Bot 所有者可操作
- 同时删除关联的路由配置
- 清除 Agent 缓存

***

### 5.6 与 AI 助手对话

**POST** `/bot/chat`

需要认证。

| 参数              | 类型     | 必填 | 说明                |
| --------------- | ------ | -- | ----------------- |
| bot_id          | int64  | 是  | Bot ID            |
| message         | string | 是  | 用户消息              |
| conversation_id | int64  | 否  | 关联的会话ID，0=不关联     |

请求示例：

```json
{
  "bot_id": 1,
  "message": "如何用Go实现WebSocket？",
  "conversation_id": 5
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "reply": "在Go中实现WebSocket，推荐使用gorilla/websocket库...",
    "input_tokens": 0,
    "output_tokens": 0,
    "cost": 0.0012
  }
}
```

核心逻辑：

1. 检查 Bot 是否存在且启用
2. 获取或创建 Agent 实例（缓存到 agentCache）
3. 调用 Agent.Run() 执行对话
4. 估算 Token 用量和费用
5. 记录计费信息到 billing_records 表
6. 如果关联了会话ID，AI 回复也会推送到聊天界面

***

### 5.7 创建路由规则

**POST** `/bot/route/create`

需要认证。

| 参数            | 类型     | 必填 | 说明                                |
| ------------- | ------ | -- | --------------------------------- |
| bot_id        | int64  | 是  | Bot ID                            |
| route_pattern | string | 是  | 路由匹配模式                            |
| route_type    | string | 是  | 路由类型：`keyword`（关键词）/ `regex`（正则） |
| priority      | int64  | 否  | 优先级，默认 0，数值越大优先级越高               |

请求示例：

```json
{
  "bot_id": 1,
  "route_pattern": "帮助|help",
  "route_type": "keyword",
  "priority": 10
}
```

核心逻辑：

- 路由规则用于消息分发，当用户消息匹配路由模式时自动转发给对应 Bot
- keyword 类型：消息包含关键词即匹配
- regex 类型：使用正则表达式匹配

***

### 5.8 获取路由规则列表

**GET** `/bot/:id/routes`

需要认证。

| 参数 | 类型    | 必填      | 说明      |
| -- | ----- | ------- | ------- |
| id | int64 | 是（路径参数） | Bot ID  |

***

### 5.9 删除路由规则

**DELETE** `/bot/route/delete`

需要认证。

| 参数       | 类型    | 必填 | 说明      |
| -------- | ----- | -- | ------- |
| route_id | int64 | 是  | 路由规则 ID |

***

### 5.10 获取计费记录

**GET** `/bot/:id/billing`

需要认证。

| 参数     | 类型    | 必填 | 说明      |
| ------ | ----- | -- | ------- |
| id     | int64 | 是（路径参数） | Bot ID  |
| limit  | int64 | 否  | 每页条数，默认 20 |
| offset | int64 | 否  | 偏移量，默认 0   |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "records": [
      {
        "id": 1,
        "bot_id": 1,
        "user_id": 1,
        "conversation_id": 5,
        "input_tokens": 150,
        "output_tokens": 320,
        "cost": 0.0012,
        "model_name": "gpt-4o-mini",
        "created_at": "2026-05-12 10:30:00"
      }
    ],
    "total": 42
  }
}
```

***

## 六、WebSocket 网关 (websocket-gateway)

### 6.1 建立 WebSocket 连接

**GET** `ws://localhost:8081/ws?token=<JWT_TOKEN>`

连接流程：

1. 客户端携带 JWT Token 作为 URL 参数发起 WebSocket 升级请求
2. 服务端解析 Token 验证身份
3. 升级 HTTP 连接为 WebSocket 长连接
4. 将客户端注册到 Hub（按 userID 分组管理）
5. 在 Redis 中设置在线状态（key: `online:user:{id}`，TTL 30s）
6. 启动读写协程：ReadPump（读取客户端消息 + 心跳检测）+ WritePump（推送消息 + 定时 ping）

客户端接收消息格式：

```json
{
  "type": "new_message",
  "data": {
    "type": "new_message",
    "conversation_id": 1,
    "sender_id": 2,
    "content": "你好！",
    "msg_type": "text",
    "msg_id": 42,
    "created_at": "2026-05-11 23:15:30"
  }
}
```

心跳机制：

- 服务端每 54 秒发送 ping 帧
- 客户端需回复 pong 帧（浏览器自动处理）
- 60 秒无 pong 响应则断开连接

断线重连：

- 前端在 onclose 事件后 3 秒自动重连
- 重连时携带最新 Token

***

### 6.2 消息推送接口（内部）

**POST** `http://localhost:8081/push`

供后端服务调用，不对外暴露。

| 参数                    | 类型       | 必填 | 说明                    |
| --------------------- | -------- | -- | --------------------- |
| target_user_ids       | []int64  | 是  | 目标用户ID列表              |
| data.type             | string   | 是  | 消息类型（如 `new_message`） |
| data.conversation_id  | int64    | 是  | 会话ID                  |
| data.sender_id        | int64    | 是  | 发送者ID                 |
| data.content          | string   | 是  | 消息内容                  |
| data.msg_type         | string   | 是  | 消息类型                  |
| data.msg_id           | int64    | 是  | 消息ID                  |
| data.created_at       | string   | 是  | 消息创建时间                |

***

### 6.3 查询在线用户

**GET** `http://localhost:8081/online`

响应示例：

```json
{
  "online_users": [1, 2, 5]
}
```

***

### 6.4 查询用户是否在线

**GET** `http://localhost:8081/is_online?user_id=1`

响应示例：

```json
{
  "user_id": 1,
  "online": true
}
```

***

## 七、数据库表结构

### users 表 (user-service)

| 字段          | 类型           | 说明                |
| ----------- | ------------ | ----------------- |
| id          | bigint PK    | 自增主键              |
| username    | varchar(50)  | 用户名，唯一索引          |
| password    | varchar(255) | bcrypt 加密后的密码     |
| nickname    | varchar(50)  | 昵称                |
| avatar      | varchar(255) | 头像URL             |
| email       | varchar(100) | 邮箱                |
| phone       | varchar(20)  | 手机号               |
| status      | varchar(20)  | 状态：online/offline |
| created_at  | datetime     | 创建时间              |
| updated_at  | datetime     | 更新时间              |

### friends 表 (user-service)

| 字段          | 类型          | 说明        |
| ----------- | ----------- | --------- |
| id          | bigint PK   | 自增主键      |
| user_id     | bigint      | 用户ID，索引   |
| friend_id   | bigint      | 好友ID，索引   |
| group_id    | bigint      | 好友分组ID，索引 |
| remark      | varchar(50) | 备注        |
| created_at  | datetime    | 创建时间      |

### friend_groups 表 (user-service)

| 字段          | 类型          | 说明      |
| ----------- | ----------- | ------- |
| id          | bigint PK   | 自增主键    |
| user_id     | bigint      | 用户ID，索引 |
| name        | varchar(50) | 分组名称    |
| created_at  | datetime    | 创建时间    |

### groups 表 (group-service)

| 字段           | 类型           | 说明             |
| ------------ | ------------ | -------------- |
| id           | bigint PK    | 自增主键           |
| name         | varchar(100) | 群组名称           |
| avatar       | varchar(255) | 群头像            |
| owner_id     | bigint       | 群主ID，索引        |
| announcement | text         | 群公告            |
| is_pinned    | bool         | 是否置顶，默认 false  |
| created_at   | datetime     | 创建时间           |
| updated_at   | datetime     | 更新时间           |

### group_members 表 (group-service)

| 字段           | 类型          | 说明                    |
| ------------ | ----------- | --------------------- |
| id           | bigint PK   | 自增主键                  |
| group_id     | bigint      | 群组ID，索引               |
| user_id      | bigint      | 用户ID，索引               |
| role         | varchar(20) | 角色：owner/admin/member |
| muted_until  | datetime    | 禁言截止时间（可空）            |
| joined_at    | datetime    | 加入时间                  |

### conversations 表 (msg-core-service)

| 字段          | 类型          | 说明                 |
| ----------- | ----------- | ------------------ |
| id          | bigint PK   | 自增主键               |
| type        | varchar(20) | 会话类型：private/group |
| created_at  | datetime    | 创建时间               |
| updated_at  | datetime    | 更新时间（最后消息时间）       |

### conversation_participants 表 (msg-core-service)

| 字段               | 类型        | 说明      |
| ---------------- | --------- | ------- |
| id               | bigint PK | 自增主键    |
| conversation_id  | bigint    | 会话ID，索引 |
| user_id          | bigint    | 用户ID，索引 |
| joined_at        | datetime  | 加入时间    |

### messages 表 (msg-core-service)

| 字段               | 类型          | 说明                                  |
| ---------------- | ----------- | ----------------------------------- |
| id               | bigint PK   | 自增主键                                |
| conversation_id  | bigint      | 会话ID，索引                             |
| sender_id        | bigint      | 发送者ID，索引                            |
| content          | text        | 消息内容                                |
| msg_type         | varchar(20) | 消息类型：text/image/voice/file         |
| created_at       | datetime    | 创建时间                                |

### file_records 表 (file-service)

| 字段           | 类型           | 说明                          |
| ------------ | ------------ | --------------------------- |
| id           | varchar(36) PK | 文件ID（UUID）                  |
| file_name    | varchar(255) | 原始文件名                        |
| file_type    | varchar(20)  | 文件分类：image/voice/file       |
| file_size    | bigint       | 文件大小（字节）                    |
| content_type | varchar(100) | MIME 类型                     |
| file_url     | varchar(500) | 文件访问URL                      |
| storage_path | varchar(500) | 存储路径（MinIO object key 或本地路径） |
| storage_type | varchar(10)  | 存储类型：minio/local            |
| uploader_id  | bigint       | 上传者ID，索引                     |
| created_at   | datetime     | 创建时间                        |

### bots 表 (bot-manager-service)

| 字段            | 类型           | 说明                                    |
| ------------- | ------------ | ------------------------------------- |
| id            | bigint PK    | 自增主键                                  |
| name          | varchar(100) | 助手名称                                  |
| type          | varchar(20)  | 类型：internal/custom                   |
| description   | text         | 助手描述                                  |
| model_name    | varchar(100) | LLM 模型名称                              |
| api_key       | varchar(255) | LLM API Key                           |
| base_url      | varchar(255) | LLM API Base URL                      |
| system_prompt | text         | 系统提示词                                 |
| skills_dir    | varchar(255) | 技能目录路径                                |
| agent_root    | varchar(255) | Agent 根目录路径                           |
| owner_id      | bigint       | 所有者ID，索引                              |
| is_active     | bool         | 是否启用，默认 true                          |
| created_at    | datetime     | 创建时间                                  |
| updated_at    | datetime     | 更新时间                                  |

### bot_routes 表 (bot-manager-service)

| 字段            | 类型           | 说明                                |
| ------------- | ------------ | --------------------------------- |
| id            | bigint PK    | 自增主键                              |
| bot_id        | bigint       | Bot ID，索引                         |
| route_pattern | varchar(255) | 路由匹配模式                            |
| route_type    | varchar(50)  | 路由类型：keyword/regex               |
| priority      | int          | 优先级，默认 0                          |
| created_at    | datetime     | 创建时间                              |

### billing_records 表 (bot-manager-service)

| 字段            | 类型           | 说明       |
| ------------- | ------------ | -------- |
| id            | bigint PK    | 自增主键     |
| bot_id        | bigint       | Bot ID，索引 |
| user_id       | bigint       | 用户ID，索引  |
| action        | varchar(50)  | 操作类型     |
| token_count   | bigint       | Token 数量 |
| cost          | double       | 费用       |
| created_at    | datetime     | 创建时间     |

***

## 八、Redis 缓存 Key 规范

| Key 模式                      | 服务                        | TTL         | 说明        |
| --------------------------- | ------------------------- | ----------- | --------- |
| `user:info:{id}`            | user-service              | 15min       | 用户信息 JSON |
| `user:friends:{id}`         | user-service              | 5min        | 好友列表 JSON |
| `user:friend_groups:{id}`   | user-service              | 10min       | 好友分组 JSON |
| `online:user:{id}`          | user-service / ws-gateway | 30min / 30s | 在线状态标记    |
| `user:conversations:{id}`   | msg-core-service          | 5min        | 会话列表 JSON |
| `conversation:recent:{id}`  | msg-core-service          | 10min       | 最近消息 JSON |
| `group:info:{id}`           | group-service             | 10min       | 群组信息 JSON |
| `user:groups:{id}`          | group-service             | 5min        | 用户群组列表 JSON |
| `group:members:{id}`        | group-service             | 5min        | 群成员列表 JSON |

缓存失效策略：写操作时主动删除相关 Key，依赖 TTL 作为兜底。

***

## 九、前端页面功能映射

| 页面/按钮            | 对应接口                                                    | 说明                |
| ---------------- | ------------------------------------------------------- | ----------------- |
| 登录表单             | POST /user/login                                        | 输入用户名+密码登录        |
| 注册表单             | POST /user/register                                     | 输入用户名+密码+昵称注册     |
| 侧边栏-会话列表         | GET /message/conversations                              | 加载所有会话            |
| 侧边栏-好友列表         | GET /user/friend/list                                   | 加载好友列表            |
| 侧边栏-群组列表         | GET /group/list                                         | 加载群组列表            |
| 好友"聊天"按钮         | POST /message/conversation → GET /message/history/:id   | 创建私聊会话并打开         |
| 群组"进入"按钮         | GET /group/:id/members → POST /message/conversation     | 获取群成员后创建群聊会话      |
| 群组"管理"按钮         | GET /group/:id → 管理弹窗                                   | 群信息编辑/转让/置顶/解散    |
| 群成员"禁言"按钮        | POST /group/mute                                        | 设置禁言时长            |
| 群成员"角色"按钮        | POST /group/role                                        | 设置管理员/成员          |
| 群成员"移除"按钮        | POST /group/kick                                        | 踢出群成员             |
| "+新会话"按钮         | POST /message/conversation                              | 弹窗选择类型和参与者        |
| "+添加好友"按钮        | POST /user/friend/add                                   | 弹窗输入好友ID          |
| "+创建群组"按钮        | POST /group/create                                      | 弹窗输入群名和成员         |
| 消息输入框             | POST /message/send                                      | 发送文本消息            |
| 附件按钮（📎）         | POST /file/upload → POST /message/send                  | 上传文件并发送多媒体消息      |
| AI助手按钮（🤖）        | GET /bot/list → POST /bot/chat                          | 打开AI助手面板进行对话      |
| "搜索"按钮           | GET /message/search                                     | 搜索当前会话消息          |
| "详情"按钮           | GET /message/history/:id                                | 查看会话详情            |
| 个人信息编辑           | PUT /user/info + PUT /user/avatar                       | 编辑昵称/邮箱/手机/头像     |
| AI助手管理面板          | POST /bot/create → GET /bot/list → POST /bot/chat      | 创建/管理/对话AI助手      |
| AI助手路由管理          | POST /bot/route/create → GET /bot/:id/routes            | 配置消息路由规则          |
| AI助手计费查询          | GET /bot/:id/billing                                    | 查看Token用量和费用      |
| WebSocket 连接     | ws://localhost:8081/ws?token=xxx                        | 登录后自动建立，接收实时消息    |

***

## 十、多媒体消息格式规范

### 消息类型

| msg_type | 说明   | content 格式                                    |
| -------- | ---- | --------------------------------------------- |
| text     | 文本消息 | 纯文本内容                                         |
| image    | 图片消息 | `[img]{file_url}[/img]`                        |
| voice    | 语音消息 | `[voice]{文件名}[/voice]`                         |
| file     | 文件消息 | `[file]{文件名}[/file]`                           |

### 前端渲染规则

- `[img]...[/img]`：渲染为可点击的图片，点击在新窗口打开
- `[voice]...[/voice]`：渲染为语音消息卡片（🎤 图标 + 文件名）
- `[file]...[/file]`：渲染为文件消息卡片（📎 图标 + 文件名）

***

## 十一、服务端口一览

| 服务                  | 端口   | 协议       | 说明         |
| ------------------- | ---- | -------- | ---------- |
| user-service        | 9001 | Thrift RPC | 用户服务       |
| group-service       | 9002 | Thrift RPC | 群组服务       |
| msg-core-service    | 9003 | Thrift RPC | 消息核心服务     |
| msg-history-service | 9004 | Thrift RPC | 消息历史服务     |
| file-service        | 9005 | Thrift RPC | 文件服务       |
| bot-manager-service | 9006 | Thrift RPC | AI助手管理服务   |
| api-gateway         | 8080 | HTTP      | API 网关     |
| websocket-gateway   | 8081 | HTTP/WS   | WebSocket 网关 |
| MinIO               | 9000 | HTTP      | 对象存储       |
| MinIO Console       | 9009 | HTTP      | MinIO 管理界面 |
| MySQL               | 3306 | TCP       | 数据库        |
| Redis               | 6379 | TCP       | 缓存         |
| Etcd                | 2379 | gRPC      | 服务注册与发现    |
