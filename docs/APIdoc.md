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
| code    | int    | 0=成功, -1=服务器错误, 400=参数错误, 401=未认证, 403=无权限, 429=请求过于频繁 |
| message | string | 响应描述                                       |
| data    | object | 响应数据，失败时为 null                             |

### 认证机制

- 公开接口（注册/登录/刷新 Token）无需 Access Token
- 其余接口需在请求头携带：`Authorization: Bearer <access_token>`
- 登录会返回 `access_token` 和 `refresh_token`；`token` 字段保留为 `access_token` 的兼容别名
- Access Token 有效期由 `JWT_ACCESS_EXPIRATION` 控制，未配置时兼容读取 `JWT_EXPIRATION`
- Refresh Token 有效期由 `JWT_REFRESH_EXPIRATION` 控制
- JWT 载荷包含 `role=user/admin`。管理层接口统一挂载在 `/api/v1/admin/*`，需要 `role=admin`
- API 网关默认启用令牌桶限流。超过当前用户或来源 IP 的窗口配额时返回 HTTP 429，响应体为 `{"code":429,"message":"请求过于频繁，请稍后再试"}`。

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
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "user_id": 1,
    "role": "user",
    "msg": "登录成功"
  }
}
```

核心逻辑：

- bcrypt 校验密码
- 生成 Access/Refresh 两类 JWT（HS256 签名，包含 userID、username、role、token_type、过期时间）
- 更新用户状态为 online
- 缓存用户信息 + 在线状态到 Redis（key: `online:user:{id}`，TTL 30min）

***

### 1.2.1 刷新 Access Token

**POST** `/user/token/refresh`

无需 Access Token，但请求体必须携带有效 Refresh Token。

| 参数          | 类型     | 必填 | 说明            |
| ------------- | -------- | ---- | --------------- |
| refresh_token | string   | 是   | 登录返回的刷新令牌 |

请求示例：

```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
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
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "user_id": 1,
    "role": "user"
  }
}
```

核心逻辑：

- 只接受 `token_type=refresh` 的 JWT
- 使用 Refresh Token 中的 userID 定位用户，再通过 user-service 读取当前用户信息和当前 role 后重新签发 Access Token
- Refresh Token 中携带的 role 只作为令牌签发时的历史快照，不作为续签时的最终权限来源，避免用户角色变更后旧 Refresh Token 继续放大权限
- Refresh Token 无效或过期时返回 401

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
    "cover": "",
    "signature": "",
    "bio": "",
    "location": "",
    "website": "",
    "gender": "",
    "birthday": "",
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

**GET** `/user/batch?ids=1,2,3`

需要认证。

| 参数  | 类型     | 必填 | 说明                 |
| ---- | ------ | -- | ------------------ |
| ids  | string | 是  | 逗号分隔的用户ID列表，如 `1,2,3` |

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

| 参数        | 类型     | 必填 | 说明                  |
| ---------- | ------ | -- | ------------------- |
| nickname   | string | 否  | 昵称                  |
| email      | string | 否  | 邮箱                  |
| phone      | string | 否  | 手机号                 |
| avatar     | string | 否  | 头像URL               |
| cover      | string | 否  | 个人主页头图URL          |
| signature  | string | 否  | 个性签名，最长 120 字符     |
| bio        | string | 否  | 个人简介，最长 500 字符     |
| location   | string | 否  | 所在地                 |
| website    | string | 否  | 个人网站/主页            |
| gender     | string | 否  | 性别或展示身份            |
| birthday   | string | 否  | 生日，建议 `YYYY-MM-DD` |

请求示例：

```json
{
  "nickname": "NewAlice",
  "email": "alice@example.com",
  "avatar": "https://example.com/avatar.png",
  "cover": "https://example.com/cover.jpg",
  "signature": "今天也在认真聊天",
  "bio": "喜欢 IM、Agent 和好用的软件。",
  "location": "Shanghai",
  "website": "https://example.com",
  "gender": "保密",
  "birthday": "2000-01-01"
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

- 个人资料页使用覆盖式保存，允许提交空字符串清空签名、简介、头图等资料字段
- 写数据库成功后删除 `user:info:{id}` 缓存，下一次读取按 Cache-Aside 回源并重建缓存

***

### 1.7 更新用户头像

**POST** `/user/avatar`

需要认证。

| 参数     | 类型     | 必填 | 说明    |
| ------ | ------ | -- | ----- |
| avatar | string | 是  | 头像URL |

核心逻辑：

- 更新头像URL到数据库
- 写数据库成功后删除 `user:info:{id}` 缓存

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

### 1.11 修改好友备注/分组

**PUT** `/user/friend/remark`

需要认证。

| 参数        | 类型    | 必填 | 说明                 |
| ---------- | ------- | ---- | -------------------- |
| friend_id  | int64   | 是   | 好友用户ID           |
| group_id   | int64   | 否   | 新好友分组ID，0=默认 |
| remark     | string  | 否   | 新备注，可为空       |

核心逻辑：

- 只修改当前用户视角下的好友备注和分组
- 写数据库成功后删除当前用户好友列表缓存

***

### 1.12 创建好友分组

**POST** `/user/friend/group`

需要认证。

| 参数   | 类型     | 必填 | 说明   |
| ---- | ------ | -- | ---- |
| name | string | 是  | 分组名称 |

***

### 1.13 获取好友分组列表

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
| client_msg_id    | string   | 否  | 消息幂等键；普通前端可不传，内部 Agent 回复会使用稳定键防重复 |

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
5. 为所有参与者写入 `message_user_states`：发送者默认已投递且已读，接收者默认已投递未读
6. 更新会话的 updated_at 为消息创建时间
7. 缓存最近消息到 Redis（key: `conversation:recent:{id}`，TTL 10min）
8. 清除所有参与者的会话列表缓存
9. **WebSocket 实时推送**：获取会话所有参与者ID → 在同一数据库事务内写入 `event_outbox(message.created)` → Outbox worker 发布到 Kafka `claran.message.events` → websocket-gateway 消费事件 → Hub.Broadcast() → 所有在线参与者浏览器实时收到消息。

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
- 校验 message_id 必须属于当前 conversation，防止跨会话污染已读游标
- 同步更新 `message_user_states.read_at`，为未来单条已读回执和多端同步保留状态
- 会话列表的 unread_count 基于该游标用 SQL 统计，不再拉取大量消息到内存遍历
- 已读状态由接收消息者触发：用户打开会话后前端上报最新消息 ID，服务端把该用户读到的所有历史消息标记为已读

***

### 3.4 删除本地消息

**POST** `/message/delete-local`

需要认证。该接口对应常规 IM 的“删除我这边的聊天记录”，只影响当前登录用户自己的历史视图，不会删除 `messages` 中的服务端消息事实，也不会影响其他参与者。

| 参数              | 类型    | 必填 | 说明   |
| ---------------- | ----- | -- | ------ |
| conversation_id  | int64 | 是  | 会话ID |
| message_id       | int64 | 是  | 消息ID |

请求示例：

```json
{
  "conversation_id": 1,
  "message_id": 42
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "msg": "本地消息已删除"
  }
}
```

核心逻辑：

- 校验当前用户是会话参与者
- 校验消息存在且属于当前会话
- 写入 `message_user_states.local_deleted_at`
- 后续 `GET /message/history/:id` 会过滤当前用户已本地删除的消息
- 其他用户历史不受影响；如果要影响所有人，应使用“撤回消息”接口

***

### 3.5 编辑消息

**PUT** `/message/edit`

需要认证。仅消息发送者可编辑，已撤回消息不能编辑。

| 参数       | 类型     | 必填 | 说明     |
| ---------- | ------ | -- | -------- |
| message_id | int64  | 是  | 消息ID   |
| content    | string | 是  | 新消息内容 |

编辑成功后写入 `message_edit_records`，并通过 WebSocket 推送 `message_edited`。

***

### 3.6 撤回消息

**POST** `/message/recall`

需要认证。仅消息发送者可撤回，默认限时 2 分钟。

| 参数       | 类型    | 必填 | 说明   |
| ---------- | ----- | -- | ------ |
| message_id | int64 | 是  | 消息ID |

撤回成功后消息 `status` 更新为 `recalled`，正文清空，并通过 WebSocket 推送 `message_recalled`。

***

### 3.7 获取消息历史

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
        "read_count": 1,
        "recipient_count": 1,
        "is_read_by_me": true,
        "created_at": "2026-05-11 23:15:30"
      }
    ]
  }
}
```

核心逻辑：

- 按 ID 降序查询（最新在前），返回时反转为时间正序
- before_id > 0 时实现游标分页
- 按当前用户过滤 `message_user_states.local_deleted_at` 不为空的消息，实现本地删除不影响他人
- 每条消息返回已读回执统计：`read_count` 是除发送者外已读人数，`recipient_count` 是除发送者外接收人数，`is_read_by_me` 表示当前查询用户是否已读该消息
- 前端展示规则：自己发送的私聊消息显示“已读/未读”，自己发送的群聊消息显示“已读 x/y”

***

### 3.8 搜索消息

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

### 3.9 获取用户会话列表

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
    "file_url": "/files/image/a1b2c3d4-e5f6-7890-abcd-ef1234567890.jpg",
    "filename": "image.jpg",
    "size": 102400,
    "msg": "上传成功"
  }
}
```

核心逻辑：

1. 从 form-data 中读取文件
2. api-gateway 负责二进制流处理，生成对象名 `{file_type}/{uuid}{ext}`
3. 如果 MinIO 可用：上传到配置的 MinIO Bucket；`file_url` 记录为内部对象 URL，下载和预览仍由 api-gateway 代理
4. 如果 MinIO 不可用：保存到本地 `./storage/source/{file_type}/` 目录，`file_url` 记录为 `/files/{file_type}/{uuid}{ext}`
5. 调用 file-service 写入文件元数据到 MySQL（file_records 表）
6. 返回 `file_id` 和 `file_url` 供消息发送使用；前端下载应优先使用 `/file/download/:id`，图片/语音预览可使用 `/file/preview/:id`

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
    "file_url": "/files/image/xxx.jpg",
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

### 4.2.1 下载文件

**GET** `/file/download/:id`

需要认证。该接口由 api-gateway 读取本地文件或 MinIO 对象后返回二进制流，避免浏览器直接访问私有 MinIO Bucket。

### 4.2.2 预览文件

**GET** `/file/preview/:id`

需要认证。该接口使用 `inline` 方式返回文件流，主要用于图片和语音消息在会话中预览/播放。

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
| avatar        | string | 否  | Agent 作为用户展示时的头像 URL          |
| signature     | string | 否  | Agent 作为用户展示时的个性签名           |
| workspace_root | string | 否 | Agent 文件/代码工具允许使用的工作目录       |
| tool_policy   | string | 否  | 工具策略，默认 `safe`                  |

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
- 创建或绑定一个真实系统用户作为 `agent_user_id`，Agent 在 IM 中以该用户身份被 @ 和发送消息；该用户 `is_system=true`，不能通过密码登录
- 创建者自动拥有 `owner` 权限
- internal/custom 配置都由 bot-manager 保存，执行交给 bot-runtime-service

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
| avatar        | string  | 否  | 新头像 URL      |
| signature     | string  | 否  | 新个性签名       |
| workspace_root | string | 否  | 新工作目录       |
| tool_policy   | string  | 否  | 新工具策略       |
| is_active     | bool    | 否  | 是否启用      |

核心逻辑：

- `owner/admin` 可操作，创建者可给其他用户授予 Agent 权限
- 更新配置后 bot-runtime-service 下次调用会按新的配置快照创建/复用 Agent

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
      "agent_user_id": 1000000001,
      "avatar": "/files/agent.png",
      "signature": "项目协作 Agent",
      "workspace_root": "storage/agent/workspaces/1",
      "tool_policy": "safe",
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
2. bot-manager-service 做权限、路由、计费入口校验
3. bot-runtime-service 获取或创建 Eino DeepAgent 并调用 Agent.Run()
4. 从 Eino `schema.Message.ResponseMeta.Usage` 读取模型返回的真实 Token 用量，并按模型单价计算费用
5. 记录计费信息到 billing_records 表
6. 如果通过群聊 @Agent 触发，bot-manager-service 消费 `message.created` Kafka 事件，调用 runtime 后用 Agent 的 `agent_user_id` 写回消息
7. @Agent 分发通过 `agent_dispatch_records(event_id, agent_user_id)` 记录状态，Agent 回复通过 msg-core-service `client_msg_id=agent:{event_id}:{agent_user_id}` 做消息幂等，Kafka 重投不会重复生成多条回复

说明：

- 当前计费严格以模型响应中的 usage 为准，不再按字符数估算。
- 如果模型或兼容接口没有返回 usage，系统会记录 `action=chat_usage_missing`，`input_tokens/output_tokens/token_count/cost` 均按 0 写入，避免用猜测值计费。

***

### 5.6.1 Agent 原生运行与会话理解接口

以下接口均需要认证，路径前缀为 `/api/v1`。

| 接口 | 方法 | 说明 |
| --- | --- | --- |
| `/agent/run` | POST | Agent 原生运行入口，参数与 `/bot/chat` 相同 |
| `/agent/summarize` | POST | 对指定会话生成总结 |
| `/agent/ask` | POST | 基于会话上下文问答 |
| `/agent/insights` | POST | 提取结论、分歧、风险、待办和负责人 |
| `/agent/reply-candidates` | POST | 生成可选回复候选 |

任务接口请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| bot_id | int64 | 是 | Agent/Bot ID |
| conversation_id | int64 | 否 | 会话 ID |
| question | string | 否 | 问题或附加指令 |

***

### 5.6.2 Agent 权限接口

| 接口 | 方法 | 说明 |
| --- | --- | --- |
| `/agent/permission/grant` | POST | 授予其他用户 Agent 权限 |
| `/agent/permission/revoke` | POST | 撤销其他用户 Agent 权限 |
| `/agent/:id/permissions` | GET | 查看 Agent 权限列表 |

角色说明：

| 角色 | 权限 |
| --- | --- |
| owner | 创建者，拥有全部权限，不可被撤销 |
| admin | 可修改配置、授权、运行 Agent |
| operator | 可运行 Agent、查看运行结果 |
| viewer | 只读基础信息和权限列表 |

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

### 6.2 消息推送接口（内部兼容接口）

**POST** `http://localhost:8081/push`

供后端服务调用，不对外暴露。当前实时推送主路径已经迁移为 MySQL Outbox + Kafka `message.*` 事件；该接口仅作为旧链路兼容入口，不再是 msg-core-service 的主要降级路径。

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

### 6.3 Kafka 事件主题（内部）

Kafka 默认开启，默认 broker 为 `127.0.0.1:9092`。本地如暂时不启动 Kafka，可设置 `KAFKA_ENABLED=false`；业务请求仍会把待发布事件写入 `event_outbox`，等 Kafka 恢复并启用 worker 后继续投递。

| Topic | 生产者 | 消费者 | 说明 |
| ----- | ------ | ------ | ---- |
| `claran.group.events` | group-service Outbox worker | msg-core-service | 群创建、邀请、踢人、解散事件，用于同步群聊会话和参与者 |
| `claran.message.events` | msg-core-service Outbox worker | websocket-gateway | 新消息、编辑、撤回、已读事件，用于在线 WebSocket 推送 |

可靠性边界：

- 当前 Kafka 事件用于服务间通知和在线实时推送，MySQL 仍是消息、群成员、会话等核心事实的最终来源。
- `group.*` 和 `message.*` 事件已经通过 `event_outbox` 与业务写入同事务提交，可覆盖“业务数据库已提交，但进程在 Kafka 发布前崩溃”的窗口。
- Kafka 写入成功后，消费者处理成功才提交 offset，因此消费者异常时可重试；当前仍待补 `processed_events` 幂等表，用于处理“消费者副作用成功但 offset 未提交”后的重复消费。

事件统一外壳：

```json
{
  "event_id": "734...",
  "type": "message.created",
  "version": 1,
  "key": "conversation_id",
  "occurred_at": "2026-05-17T12:00:00+08:00",
  "payload": {}
}
```

### 6.4 查询在线用户

**GET** `http://localhost:8081/online`

响应示例：

```json
{
  "online_users": [1, 2, 5]
}
```

***

### 6.5 查询用户是否在线

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

### ID 生成规则

- 用户 ID：10 位数字 UID，便于用户复制并添加好友。
- 其他业务主键：使用 64 位雪花 ID，避免依赖 MySQL 自增。
- 各服务启动只执行非破坏性 AutoMigrate。开发阶段新增字段会自动补列；字段删除、类型变更等复杂场景应使用增量迁移脚本，不在启动流程中删表。

### users 表 (user-service)

| 字段          | 类型           | 说明                |
| ----------- | ------------ | ----------------- |
| id          | bigint PK    | 10 位数字 UID         |
| username    | varchar(50)  | 用户名，唯一索引          |
| password    | varchar(255) | bcrypt 加密后的密码     |
| nickname    | varchar(50)  | 昵称                |
| avatar      | varchar(255) | 头像URL             |
| cover       | varchar(255) | 个人主页头图URL       |
| signature   | varchar(120) | 个性签名             |
| bio         | varchar(500) | 个人简介             |
| location    | varchar(80)  | 所在地              |
| website     | varchar(255) | 个人网站/主页         |
| gender      | varchar(20)  | 性别或展示身份         |
| birthday    | varchar(20)  | 生日，YYYY-MM-DD     |
| email       | varchar(100) | 邮箱                |
| phone       | varchar(20)  | 手机号               |
| role        | varchar(20)  | 系统角色：user/admin，用于管理层接口鉴权 |
| status      | varchar(20)  | 状态：online/offline |
| created_at  | datetime     | 创建时间              |
| updated_at  | datetime     | 更新时间              |

### friends 表 (user-service)

| 字段          | 类型          | 说明        |
| ----------- | ----------- | --------- |
| id          | bigint PK   | 雪花ID       |
| user_id     | bigint      | 用户ID，索引   |
| friend_id   | bigint      | 好友ID，索引   |
| group_id    | bigint      | 好友分组ID，索引 |
| remark      | varchar(50) | 备注        |
| created_at  | datetime    | 创建时间      |

### friend_groups 表 (user-service)

| 字段          | 类型          | 说明      |
| ----------- | ----------- | ------- |
| id          | bigint PK   | 雪花ID     |
| user_id     | bigint      | 用户ID，索引 |
| name        | varchar(50) | 分组名称    |
| created_at  | datetime    | 创建时间    |

### groups 表 (group-service)

| 字段           | 类型           | 说明             |
| ------------ | ------------ | -------------- |
| id           | bigint PK    | 雪花ID            |
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
| id           | bigint PK   | 雪花ID                   |
| group_id     | bigint      | 群组ID，索引               |
| user_id      | bigint      | 用户ID，索引               |
| role         | varchar(20) | 角色：owner/admin/member |
| muted_until  | datetime    | 禁言截止时间（可空）            |
| joined_at    | datetime    | 加入时间                  |

### conversations 表 (msg-core-service)

| 字段          | 类型          | 说明                 |
| ----------- | ----------- | ------------------ |
| id          | bigint PK   | 雪花ID                |
| type        | varchar(20) | 会话类型：private/group |
| group_id    | bigint      | 群组ID，私聊为 0，群聊用于和 group-service 对齐成员关系 |
| created_at  | datetime    | 创建时间               |
| updated_at  | datetime    | 更新时间（最后消息时间）       |

### conversation_participants 表 (msg-core-service)

| 字段               | 类型        | 说明      |
| ---------------- | --------- | ------- |
| id               | bigint PK | 雪花ID     |
| conversation_id  | bigint    | 会话ID，索引 |
| user_id          | bigint    | 用户ID，索引 |
| last_read_message_id | bigint | 用户在该会话的已读游标 |
| last_read_at     | datetime  | 最近一次标记已读时间，可空 |
| draft            | text      | 多端共享草稿 |
| is_pinned        | bool      | 多端共享置顶 |
| notify_enabled   | bool      | 多端共享通知设置 |
| joined_at        | datetime  | 加入时间    |

### messages 表 (msg-core-service)

| 字段               | 类型          | 说明                                  |
| ---------------- | ----------- | ----------------------------------- |
| id               | bigint PK   | 雪花ID                                 |
| conversation_id  | bigint      | 会话ID，索引                             |
| sender_id        | bigint      | 发送者ID，索引                            |
| content          | text        | 消息内容                                |
| msg_type         | varchar(20) | 消息类型：text/image/voice/file         |
| reply_to_id      | bigint      | 引用/回复的源消息ID，默认 0               |
| status           | varchar(20) | 消息状态：sent/recalled/deleted          |
| is_edited        | bool        | 是否被编辑过                            |
| edited_at        | datetime    | 最近编辑时间，可空                        |
| mention_user_ids | text        | @ 用户 ID 列表的 JSON 持久化文本           |
| mention_all      | bool        | 是否 @所有人                            |
| created_at       | datetime    | 创建时间                                |

### message_user_states 表 (msg-core-service)

| 字段               | 类型        | 说明 |
| ---------------- | --------- | ---- |
| id               | bigint PK | 雪花ID |
| conversation_id  | bigint    | 会话ID，索引 |
| message_id       | bigint    | 消息ID，与 user_id 组成唯一约束 |
| user_id          | bigint    | 用户ID，与 message_id 组成唯一约束 |
| delivered_at     | datetime  | 服务端认为该用户可获得此消息的时间 |
| read_at          | datetime  | 该用户读取该消息的时间 |
| local_deleted_at | datetime  | 该用户本地删除/隐藏该消息的时间 |
| created_at       | datetime  | 创建时间 |
| updated_at       | datetime  | 更新时间 |

说明：

- `messages` 是服务端消息事实，保存消息是否存在、正文、发送者、撤回和编辑状态。
- `message_user_states` 是用户视图状态，保存某个用户对某条消息的投递、已读、本地删除状态。
- 用户删除本地消息只写 `local_deleted_at`，不会删除 `messages`，因此不会影响其他参与者。

### file_records 表 (file-service)

| 字段           | 类型           | 说明                          |
| ------------ | ------------ | --------------------------- |
| id           | bigint PK    | 雪花ID，内部记录主键                |
| file_id      | varchar(64)  | 对外文件ID（UUID），唯一索引          |
| file_name    | varchar(255) | 原始文件名                        |
| file_type    | varchar(20)  | 文件分类：image/voice/file       |
| file_size    | bigint       | 文件大小（字节）                    |
| content_type | varchar(100) | MIME 类型                     |
| file_url     | varchar(500) | 文件访问URL                      |
| uploader_id  | bigint       | 上传者ID，索引                     |
| created_at   | datetime     | 创建时间                        |

### message_edit_records 表 (msg-core-service)

| 字段            | 类型        | 说明                 |
| --------------- | ----------- | -------------------- |
| id              | bigint PK   | 雪花ID               |
| message_id      | bigint      | 被编辑的消息ID，索引   |
| conversation_id | bigint      | 会话ID，索引          |
| editor_id       | bigint      | 编辑者用户ID，索引     |
| old_content     | text        | 编辑前内容            |
| new_content     | text        | 编辑后内容            |
| created_at      | datetime    | 编辑记录创建时间       |

### event_outbox 表 (group-service/msg-core-service)

| 字段            | 类型         | 说明                                      |
| --------------- | ------------ | ----------------------------------------- |
| id              | bigint PK    | 雪花ID，同时作为事件追踪ID                 |
| aggregate_type  | varchar(50)  | 聚合类型，如 message/group                 |
| aggregate_id    | bigint       | 聚合ID，如 message_id/group_id             |
| event_type      | varchar(100) | 事件类型，如 message.created/group.created |
| event_key       | varchar(100) | Kafka 分区键，如 conversation_id/group_id  |
| payload         | json         | 事件 Envelope JSON                         |
| status          | varchar(20)  | pending/retrying/published                 |
| retry_count     | int          | 发布重试次数                               |
| last_error      | text         | 最近一次发布失败原因                       |
| next_retry_at   | datetime     | 下一次可重试时间                           |
| locked_until    | datetime     | Worker 抢占锁过期时间，可空                |
| published_at    | datetime     | 发布成功时间，可空                         |
| created_at      | datetime     | 创建时间                                  |
| updated_at      | datetime     | 更新时间                                  |

### bots 表 (bot-manager-service)

| 字段            | 类型           | 说明                                    |
| ------------- | ------------ | ------------------------------------- |
| id            | bigint PK    | 雪花ID                                   |
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
| id            | bigint PK    | 雪花ID                               |
| bot_id        | bigint       | Bot ID，索引                         |
| route_pattern | varchar(255) | 路由匹配模式                            |
| route_type    | varchar(50)  | 路由类型：keyword/regex               |
| priority      | int          | 优先级，默认 0                          |
| created_at    | datetime     | 创建时间                              |

### billing_records 表 (bot-manager-service)

| 字段            | 类型           | 说明       |
| ------------- | ------------ | -------- |
| id            | bigint PK    | 雪花ID      |
| bot_id        | bigint       | Bot ID，索引 |
| user_id       | bigint       | 用户ID，索引  |
| conversation_id | bigint     | 关联会话ID，0 表示不关联具体 IM 会话 |
| action        | varchar(50)  | 操作类型     |
| token_count   | bigint       | 输入 + 输出 Token 总数 |
| input_tokens  | bigint       | 模型响应 usage.prompt_tokens |
| output_tokens | bigint       | 模型响应 usage.completion_tokens |
| cost          | double       | 费用       |
| model_name    | varchar(100) | 计费使用的模型名称 |
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

缓存失效策略：写操作时主动删除相关 Key，依赖 TTL 作为兜底。缓存写入使用随机 TTL 抖动防止雪崩；对象不存在时写短 TTL 空值缓存防穿透；热点 Key 未命中时使用 Redis 分布式锁防击穿。

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
| 个人信息编辑           | PUT /user/info                                          | 编辑昵称、头像、头图、签名、简介等资料 |
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
| image    | 图片消息 | 推荐 JSON：`{"id":"file_id","url":"file_url","name":"image.jpg"}`；兼容旧格式 `[img]{file_url}[/img]` |
| voice    | 语音消息 | 推荐 JSON：`{"id":"file_id","url":"file_url","name":"voice.webm"}`；兼容旧格式 `[voice]{文件名}[/voice]` |
| file     | 文件消息 | 推荐 JSON：`{"id":"file_id","url":"file_url","name":"report.pdf"}`；兼容旧格式 `[file]{文件名}[/file]` |
| broadcast | 广播消息 | 当前仅作为消息类型存储和推送，尚未实现管理员投放、目标筛选、强提醒等完整广播产品流程 |

### 前端渲染规则

- JSON 中有 `id` 时，前端使用 `/api/v1/file/preview/:id` 预览图片/语音，使用 `/api/v1/file/download/:id` 下载文件。
- JSON 中只有 `url` 时，前端按 `file_url` 渲染；本地 `/files/...` 地址由 api-gateway 静态代理。
- 旧格式 `[img]...[/img]`、`[voice]...[/voice]`、`[file]...[/file]` 仍保留解析兼容。

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
| Kafka               | 9092 | TCP       | 事件总线       |
| Kafka Controller    | 9093 | TCP       | Kafka KRaft 控制器 |
| DTM HTTP            | 36789 | HTTP      | 分布式事务协调器 |
| DTM gRPC            | 36790 | gRPC      | 分布式事务协调器 |
| MinIO               | 9000 | HTTP      | 对象存储       |
| MinIO Console       | 9009 | HTTP      | MinIO 管理界面 |
| MySQL               | 3306 | TCP       | 数据库        |
| Redis               | 6379 | TCP       | 缓存         |
| Etcd                | 2379 | gRPC      | 服务注册与发现    |
