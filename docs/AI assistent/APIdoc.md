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
    "group_id": 1000000001,
    "msg": "创建成功"
  }
}
```

核心逻辑：

- 创建者自动成为群主（role=owner）
- 其他成员角色为 member
- 群组 ID 是 10 位数字群号，范围 `1000000000` 到 `9999999999`，可复制给其他用户通过群号加入
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
      "id": 1000000001,
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

### 2.4 通过群号加入群聊

**POST** `/group/join`

需要认证。

| 参数       | 类型    | 必填 | 说明        |
| -------- | ----- | -- | --------- |
| group_id | int64 | 是  | 10 位数字群号 |

请求示例：

```json
{
  "group_id": 1000000001
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "group_id": 1000000001,
    "group_name": "项目讨论组",
    "joined": true,
    "msg": "加入群聊成功"
  }
}
```

核心逻辑：

- 当前用户只能把自己加入群聊，不能借该接口邀请其他用户。
- 如果已经在群中，返回 success=true 且 joined=false。
- 成员变化仍通过 group-service outbox 事件同步到 msg-core-service 的群聊会话参与者。

***

### 2.5 邀请成员

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

### 2.6 踢出成员

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

### 3.10 手动翻译消息

**POST** `/message/translate`

需要认证。该接口不会自动翻译所有消息，只在用户点击“翻译”时触发。翻译能力落在 msg-core-service，并通过 settings-service 读取当前用户的翻译 LLM 预设和翻译 Prompt；如果用户未配置，则使用系统默认 LLM 配置。

| 参数            | 类型    | 必填 | 说明 |
| --------------- | ------- | ---- | ---- |
| message_id      | int64   | 是   | 要翻译的消息 ID |
| target_language | string  | 否   | 目标语言，默认 `中文` |
| force           | bool    | 否   | 是否跳过缓存强制重新翻译 |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "message_id": 49895688258818048,
    "target_language": "中文",
    "translated_text": "这里是译文",
    "cached": false
  }
}
```

核心逻辑：

- 校验当前用户是否能看到该消息，不能翻译无权限消息。
- 按 `user_id + message_id + target_language + source_hash` 缓存译文，消息内容未变化时直接返回缓存。
- 调用 OpenAI-compatible `/chat/completions` 接口，真实 URL/API Key/模型来自用户设置或系统默认配置。
- 翻译失败不会修改原消息，只返回错误给前端。

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

## 五、AI 助手模块 (agent-manager-service)

说明：面向业务和前端的接口统一使用 `/agent/*`。旧 `/bot/*` HTTP 兼容入口已删除；当前仍存在的 `kitex_gen/bot` 只是历史 Kitex 生成包名，不代表网关对外暴露 `/bot/*`。

Agent 配置新增运行参数：
- `context_message_limit`：会话总结、问答、洞察读取的最近消息条数，服务端默认 80，范围裁剪为 10-500。
- `memory_recall_limit`：每轮 Agent 对话召回的长期记忆条数，默认 12，最大 50。
- `max_output_tokens`：预留给模型输出长度控制，0 表示使用模型默认值。
- `temperature`：模型创造性参数，服务端裁剪在 0-2。
- `group_trigger_mode`：群聊触发方式，支持 `mention`、`keyword`、`command`、`all`、`silent`。
- `auto_reply_enabled`：是否允许 Agent 根据订阅规则自动回复；关闭后仍可手动运行 Agent。

### 5.1 创建 AI 助手

**POST** `/agent/create`

需要认证。

| 参数            | 类型     | 必填 | 说明                              |
| ------------- | ------ | -- | ------------------------------- |
| name          | string | 是  | 助手名称                            |
| type          | string | 是  | 类型：`internal`（平台内置 Agent）/ `custom`（自部署 Agent） |
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
| llm_profile_id | int64 | 否 | settings-service 中保存的 LLM 预设 ID；传入后优先使用该预设的 BaseURL、模型和 API Key |

请求示例：

```json
{
  "name": "Claran Assistant",
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

- 创建 Agent 记录到 MySQL（bots 表）
- 创建或绑定一个真实系统用户作为 `agent_user_id`，Agent 在 IM 中以该用户身份被 @ 和发送消息；该用户 `is_system=true`，不能通过密码登录
- 创建者自动拥有 `owner` 权限
- internal/custom 配置都由 agent-manager-service 保存，执行交给 agent-runtime-service
- 如果传入 `llm_profile_id`，api-gateway 会调用 settings-service 读取该用户拥有的 LLM 预设，再把预设解析为 Agent 配置；前端不需要重复填写 API Key

***

### 5.2 更新 AI 助手

**PUT** `/agent/update`

需要认证。

| 参数            | 类型      | 必填 | 说明        |
| ------------- | ------- | -- | --------- |
| bot_id        | int64   | 是  | Agent ID  |
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
- 更新配置后 agent-runtime-service 下次调用会按新的配置快照创建/复用 Agent

***

### 5.3 获取 AI 助手详情

**GET** `/agent/:id`

需要认证。

| 参数 | 类型    | 必填      | 说明      |
| -- | ----- | ------- | ------- |
| id | int64 | 是（路径参数） | Agent ID |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "bot": {
      "id": 1,
      "name": "Claran Assistant",
      "type": "internal",
      "description": "项目助手",
      "model_name": "gpt-4o-mini",
      "base_url": "https://api.openai.com/v1",
      "system_prompt": "你是一个友好的技术助手...",
      "agent_user_id": 1000000001,
      "avatar": "/files/agent.png",
      "signature": "项目协作 Agent",
      "workspace_root": "storage/agent/files/1",
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

**GET** `/agent/list`

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
        "name": "Claran Assistant",
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

**DELETE** `/agent/delete`

需要认证。

| 参数     | 类型    | 必填 | 说明       |
| ------ | ----- | -- | -------- |
| bot_id | int64 | 是  | Agent ID |

核心逻辑：

- 仅 Bot 所有者可操作
- 同时删除关联的路由配置
- 清除 Agent 缓存

***

### 5.6 与 AI 助手对话

**POST** `/agent/chat`

需要认证。

| 参数              | 类型     | 必填 | 说明                |
| --------------- | ------ | -- | ----------------- |
| bot_id          | int64  | 是  | Agent ID          |
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
2. agent-manager-service 做权限、路由、计费入口校验
3. agent-runtime-service 获取或创建 Eino DeepAgent 并调用 Agent.Run()
4. 从 Eino `schema.Message.ResponseMeta.Usage` 读取模型返回的真实 Token 用量，并按模型单价计算费用
5. 记录计费信息到 billing_records 表
6. 如果通过群聊 @Agent 触发，agent-manager-service 消费 `message.created` Kafka 事件，调用 runtime 后用 Agent 的 `agent_user_id` 写回消息
7. @Agent 分发通过 `agent_dispatch_records(event_id, agent_user_id)` 记录状态，Agent 回复通过 msg-core-service `client_msg_id=agent:{event_id}:{agent_user_id}` 做消息幂等，Kafka 重投不会重复生成多条回复

说明：

- 当前计费严格以模型响应中的 usage 为准，不再按字符数估算。
- 如果模型或兼容接口没有返回 usage，系统会记录 `action=chat_usage_missing`，`input_tokens/output_tokens/token_count/cost` 均按 0 写入，避免用猜测值计费。

***

### 5.6.1 Agent 原生运行与会话理解接口

以下接口均需要认证，路径前缀为 `/api/v1`。

| 接口 | 方法 | 说明 |
| --- | --- | --- |
| `/agent/run` | POST | Agent 原生运行入口，参数与 `/agent/chat` 相同 |
| `/agent/summarize` | POST | 对指定会话生成总结 |
| `/agent/ask` | POST | 基于会话上下文问答 |
| `/agent/insights` | POST | 提取结论、分歧、风险、待办和负责人 |
| `/agent/reply-candidates` | POST | 生成可选回复候选 |
| `/agent/approvals` | GET | 查看当前用户的 Agent 工具确认记录 |
| `/agent/approval/confirm` | POST | 允许某次待确认操作继续执行 |
| `/agent/approval/reject` | POST | 拒绝某次待确认操作 |
| `/agent/add-friend` | POST | 创建者将 Agent 系统用户加为好友 |

任务接口请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| bot_id | int64 | 是 | Agent ID |
| conversation_id | int64 | 否 | 会话 ID |
| question | string | 否 | 问题或附加指令 |

当 `/agent/run` 或 `/agent/chat` 触发高风险工具策略时，网关会返回待确认状态：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "status": "pending_approval",
    "msg": "pending_user_approval",
    "approval_id": "appr_1001_1_...",
    "reply": "这个操作需要你确认后才能继续..."
  }
}
```

确认请求：

```json
{
  "approval_id": "appr_1001_1_...",
  "message": "允许执行，但不要删除文件"
}
```

说明：当前确认流是轻量 MVP，审批状态保存在 api-gateway 进程内。它已经能跑通 `agent-ask -> user-approve -> agent-act` 基础链路；生产级别应将审批记录、checkpoint/resume 状态和超时清理移动到 agent-runtime-service 或专用任务表中。

将 Agent 加为好友请求：

```json
{
  "bot_id": 1,
  "group_id": 0,
  "remark": "项目助手"
}
```

只有 Agent 创建者可以使用该便捷接口。它实际调用 user-service 添加 `agent_user_id` 为好友，因此 Agent 会像普通用户一样出现在好友列表并可被私聊。

***

### 5.6.1.1 Agent 记忆接口

以下接口均需要认证，路径前缀为 `/api/v1`。记忆默认只允许当前用户查看和管理自己的记录，用户画像和发言习惯默认 `visibility=private`。

| 接口 | 方法 | 说明 |
| --- | --- | --- |
| `/memory/list` | GET | 查看当前用户拥有的记忆 |
| `/memory/create` | POST | 手动创建一条记忆 |
| `/memory/:id` | PUT | 修改、启用或关闭一条记忆 |
| `/memory/:id` | DELETE | 删除一条记忆 |

查询参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| bot_id | int64 | 否 | 按 Agent/Bot 过滤 |
| user_id | int64 | 否 | 按用户过滤；默认当前用户 |
| group_id | int64 | 否 | 按群过滤 |
| conversation_id | int64 | 否 | 按会话过滤 |
| session_id | string | 否 | 按 Agent 长会话过滤 |
| scope | string | 否 | `user/group/conversation/session`，支持逗号分隔 |
| type | string | 否 | `preference/speaking_style/long_term_goal/group_profile/project_state/chat_summary/agent_run_summary` |
| include_disabled | bool | 否 | 是否包含已关闭记忆 |

创建/修改请求示例：

```json
{
  "bot_id": 1,
  "scope": "user",
  "type": "preference",
  "title": "回答偏好",
  "content": "用户喜欢中文、简短、直接的回答。",
  "visibility": "private",
  "enabled": true
}
```

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "memory": {
      "id": 49895688258818048,
      "bot_id": 1,
      "user_id": 1000000001,
      "scope": "user",
      "type": "preference",
      "content": "用户喜欢中文、简短、直接的回答。",
      "visibility": "private",
      "enabled": true,
      "vector_status": "pending"
    }
  }
}
```

说明：

- `memory_facts` 是 Phase4 的基础事实记忆表，先不依赖向量库。
- Agent 调用 runtime 前会按 `bot_id + user_id + conversation_id + session_id` 召回可用记忆并注入上下文。
- Agent 成功回复后会写入一条 `scope=conversation,type=agent_run_summary` 的私有运行摘要。
- `vector_status/embedding_ref` 是向量化预留字段，当前不会真正写入向量数据库。

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

### 5.6.3 Agent-Native IM 事件与订阅

Agent 原生入口优先走 Kafka/Outbox，而不是前端 HTTP 按钮。agent-manager-service 当前同时消费：

| Topic | 说明 |
| --- | --- |
| `claran.message.events` | 兼容现有 `message.created/edited/recalled/read` 消息事件 |
| `claran.im.events` | 新增统一 IM 事件，承载文件、语音、表情、系统通知、任务变化等 Agent 原生事件 |

统一 IM 事件 payload：

```json
{
  "event_type": "file.uploaded",
  "conversation_id": 10,
  "conversation_type": "group",
  "sender_id": 1001,
  "content": "",
  "msg_type": "file",
  "msg_id": 99,
  "reply_to_id": 0,
  "participant_ids": [1001, 1002, 2001],
  "mention_user_ids": [2001],
  "mention_all": false,
  "attachment_refs": [
    {
      "file_id": 3001,
      "name": "error.png",
      "content_type": "image/png",
      "url": "/files/3001",
      "size": 12345,
      "sha256": ""
    }
  ],
  "permission_context": {
    "scope": "group",
    "visible_user_ids": [1001, 1002, 2001],
    "group_role": "member",
    "can_read_files": true,
    "can_write": true
  },
  "occurred_at": "2026-05-27T10:00:00+08:00",
  "idempotency_key": "file.uploaded:3001",
  "metadata": {}
}
```

事件分发规则由 `agent_subscription_rules` 保存。Dispatcher 支持三类决策：

| 决策 | 行为 |
| --- | --- |
| `ignore` | 不运行 Agent，也不打扰会话 |
| `record` | 只写审计/后续入库事实，不回复用户 |
| `trigger` | 构建上下文，调用 runtime，并通过 msg-core-service 用 Agent 系统用户身份回写消息 |

当前默认策略：

- 私聊 Agent：普通消息默认触发。
- 群聊 Agent：默认只响应 @Agent；也可通过订阅规则配置关键词、命令、全量监听或静默记录。
- Agent 回复使用 `client_msg_id=agent:{event_id}:{agent_user_id}` 做消息幂等，dispatch 表记录 `source_event_id` 和 `agent_trace_id`。
- 每次触发、静默记录、失败和完成都会写入 `agent_audit_records`，便于排查“为什么 Agent 没反应/为什么响应了”。

消息事实源生产规则：

- 普通文本消息仍写入 `claran.message.events` 的 `message.created`，用于兼容现有推送、未读和 Agent @ 触发。
- 消息编辑、撤回、已读会继续写入 legacy `claran.message.events`，同时由 msg-core-service 额外写入 unified IM outbox；Kafka envelope 类型分别是 `im.message.edited`、`im.message.recalled`、`im.message.read`，payload 内 `event_type` 保持业务语义 `message.edited/message.recalled/message.read`，便于订阅规则匹配。
- `file`、`image` 消息除 `message.created` 外，会在同一个数据库事务内额外写入 `claran.im.events` 的 `file.uploaded` outbox 事件。
- `voice` 消息除 `message.created` 外，会额外写入 `claran.im.events` 的 `voice.transcribed` 事件信封；当前阶段先携带语音附件引用，真实 ASR 文本由后续文件/语音处理服务补齐。
- 群成员邀请/移除仍先由 group-service 发布 `group.member_invited/group.member_kicked`，msg-core-service 消费后同步会话参与者，并在知道真实 `conversation_id` 后再写入 `group.member_joined/group.member_left` unified IM 事件。
- Dispatcher 构建 Agent 上下文时，会把附件引用注入提示词，格式包含 `file_id/name/content_type/url/size`，保证 Agent 至少知道“有哪个文件进入了会话事件流”。
- Dispatcher 幂等优先使用 payload 中的 `idempotency_key`，缺失时退回 Kafka envelope `event_id`；Agent 回复的 `client_msg_id` 同样使用这个 dispatch key，避免同一业务事件重复投递时重复生成 Agent 回复。

Agent 触发规则管理入口：`/agent/route/*` 用于管理 Agent 订阅规则。旧 `/bot/route/*` HTTP 兼容入口已删除。agent-manager-service 会把以下 route 类型镜像到 `agent_subscription_rules`：

| route_type | route_pattern 含义 | Dispatcher 行为 |
| --- | --- | --- |
| `agent_keyword` | 关键词，如 `报错` | 群聊消息包含关键词时触发 Agent 回复 |
| `agent_command` | 命令前缀，如 `/amiya` | 消息以前缀开头时触发 Agent 回复 |
| `agent_record` | 事件类型，如 `file.uploaded` | 静默记录，不主动回复 |

删除对应 route 时，会同步删除由该 route 生成的订阅规则。

最小 Action Card 协议：

Agent 任务返回 JSON 时，前端会识别 `cards`、`action_cards` 或 `actions` 数组并渲染为结构化卡片：

```json
{
  "summary": "已识别一个待办",
  "cards": [
    {
      "version": "1.0",
      "type": "task",
      "title": "创建联调任务",
      "summary": "周五前完成接口联调，负责人是用户1002。",
      "source": "conversation:10/message:99",
      "status": "pending",
      "actions": [
        {"type": "confirm", "label": "确认创建"},
        {"type": "reject", "label": "忽略"}
      ]
    }
  ]
}
```

当前 Action Card 是前端展示 MVP，按钮先记录本地提示；持久化 `action_id`、审批状态、服务端回调和权限校验继续放到结构化卡片协议阶段。

***

### 5.7 创建路由规则

**POST** `/agent/route/create`

需要认证。

| 参数            | 类型     | 必填 | 说明                                |
| ------------- | ------ | -- | --------------------------------- |
| bot_id        | int64  | 是  | Agent ID                          |
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

- 路由规则用于 Agent 事件分发，当用户消息或 IM 事件匹配规则时触发 Agent
- keyword 类型：消息包含关键词即匹配
- regex 类型：使用正则表达式匹配

***

### 5.8 获取路由规则列表

**GET** `/agent/:id/routes`

需要认证。

| 参数 | 类型    | 必填      | 说明      |
| -- | ----- | ------- | ------- |
| id | int64 | 是（路径参数） | Agent ID |

***

### 5.9 删除路由规则

**DELETE** `/agent/route/delete`

需要认证。

| 参数       | 类型    | 必填 | 说明      |
| -------- | ----- | -- | ------- |
| route_id | int64 | 是  | 路由规则 ID |

***

### 5.10 获取计费记录

**GET** `/agent/:id/billing`

需要认证。

| 参数     | 类型    | 必填 | 说明      |
| ------ | ----- | -- | ------- |
| id     | int64 | 是（路径参数） | Agent ID |
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

## 六、系统设置模块 (settings-service)

settings-service 保存用户级系统设置。本阶段已落地 LLM 预设、Prompt 模板和 Agent Skill 配置：用户可以预设多个 OpenAI-compatible BaseURL、API Key、模型名和用途，在创建 Agent 或手动翻译时复用；也可以上传全局 Skill 或单个 Agent 专属 Skill。

### 6.1 获取 LLM 预设列表

**GET** `/settings/llm-profiles?usage_type=agent`

需要认证。

| 参数       | 类型   | 必填 | 说明 |
| ---------- | ------ | ---- | ---- |
| usage_type | string | 否   | 过滤用途：`agent`、`translation`、`rag_router`、`general` |

响应中的 `api_key` 会被隐藏，只返回 `has_api_key`。

### 6.2 保存 LLM 预设

**POST** `/settings/llm-profiles`

需要认证。

| 参数           | 类型   | 必填 | 说明 |
| -------------- | ------ | ---- | ---- |
| id             | int64  | 否   | 传入则更新，不传则创建 |
| name           | string | 是   | 预设名称 |
| provider       | string | 否   | 供应商标识，如 `openai-compatible` |
| usage_type     | string | 否   | 用途：`agent`、`translation`、`rag_router`、`general`。`rag_router` 表示 RAG 检索前判断是否需要知识库的小模型 |
| base_url       | string | 是   | API Base URL |
| api_key        | string | 否   | API Key |
| api_key_action | string | 否   | `set`、`keep`、`clear`，更新时默认 `keep` |
| model_name     | string | 是   | 模型名称 |
| enabled        | bool   | 否   | 是否启用 |
| is_default     | bool   | 否   | 是否作为该用途默认预设 |

### 6.3 删除 LLM 预设

**DELETE** `/settings/llm-profiles/:id`

需要认证。只能删除当前用户自己的预设。

说明：

- 用户保存用途为 `rag_router` 的 LLM 预设并设为默认后，`rag-service` 会在该用户执行 RAG 检索时优先使用它作为 RAG Router 小模型。
- 若用户没有配置 `rag_router`，或该预设不可用，RAG 会回退项目内置小模型。
- 项目内置小模型默认读取 `.env` 的 `RAG_ROUTER_*`；如果 `RAG_ROUTER_API_KEY` / `RAG_ROUTER_BASE_URL` 为空，则使用当前项目默认 LLM 的 `LLM_DEFAULT_API_KEY` / `LLM_DEFAULT_BASE_URL`。

### 6.4 获取 Prompt 模板

**GET** `/settings/prompts`

需要认证。当前前端使用 `translation` 类型保存翻译 Prompt，后续可扩展总结、代码审查、回复候选等 Prompt。

### 6.5 保存 Prompt 模板

**POST** `/settings/prompts`

需要认证。

| 参数       | 类型   | 必填 | 说明 |
| ---------- | ------ | ---- | ---- |
| id         | int64  | 否   | 传入则更新，不传则创建 |
| type       | string | 是   | Prompt 类型，如 `translation` |
| name       | string | 是   | 模板名称 |
| content    | string | 是   | Prompt 内容 |
| enabled    | bool   | 否   | 是否启用 |
| is_default | bool   | 否   | 是否作为默认模板 |

### 6.6 获取 Agent Skill 列表

**GET** `/settings/skills?scope=global&agent_id=0`

需要认证。用于查询当前用户上传的全局 Skill 或某个 Agent 的专属 Skill。

| 参数     | 类型   | 必填 | 说明 |
| -------- | ------ | ---- | ---- |
| scope    | string | 否   | `global` 表示全局 Skill，`agent` 表示 Agent 专属 Skill，留空返回当前用户全部启用 Skill |
| agent_id | int64  | 否   | 查询某个 Agent 的专属 Skill；查询全局 Skill 时传 0 |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "skills": [
      {
        "id": 10001,
        "owner_id": 1000000001,
        "agent_id": 0,
        "scope": "global",
        "name": "代码审查",
        "description": "为 Agent 增加代码审查流程能力",
        "skills_dir": "D:/CodeStudy/GoProjects/src/ClaranAIM/storage/agent/skills/global/1000000001/skill_4",
        "entry_file": "SKILL.md",
        "source_type": "package",
        "is_default": true,
        "enabled": true
      }
    ]
  }
}
```

### 6.7 上传 Agent Skill

**POST** `/settings/skills/upload`

需要认证。Content-Type 为 `multipart/form-data`。支持三类上传：

- 单个 `SKILL.md`
- `.zip` Skill 包，包根目录必须包含 `SKILL.md`
- 浏览器文件夹上传，文件夹内必须包含 `SKILL.md`

| 表单字段    | 类型   | 必填 | 说明 |
| ----------- | ------ | ---- | ---- |
| file        | file   | 是   | 一个或多个文件；zip 会由网关解包 |
| name        | string | 否   | Skill 名称；为空时自动推断 |
| description | string | 否   | Skill 说明 |
| scope       | string | 否   | `global` 或 `agent`，默认 `global` |
| agent_id    | int64  | 否   | `scope=agent` 时必填 |
| is_default  | bool   | 否   | 是否作为默认 Skill |

安全规则：

- 上传包最大 5MB，最多 80 个文件
- Skill 包必须包含根目录 `SKILL.md`
- 禁止绝对路径和 `../` 路径穿越
- 文件落盘目录由 settings-service 生成，浏览器不能直接指定 runtime 读取目录
- 全局 Skill 默认写入 `storage/agent/skills/global/{owner_id}/{skill_name}`，Agent 专属 Skill 写入 `storage/agent/skills/agents/{agent_id}/{skill_name}`

### 6.8 删除 Agent Skill

**DELETE** `/settings/skills/:id`

需要认证。只能删除当前用户自己的 Skill 元数据。已落盘文件默认保留，避免误删正在运行中的 Agent Skill 目录。

***

## 七、RAG 知识库模块 (rag-service)

rag-service 负责知识库文档入库、分层切片、embedding、Hybrid Search、GraphRAG indexing 和原始子图查询。浏览器只访问 api-gateway；api-gateway 通过 Kitex RPC 调用 rag-service。当前索引使用 parent/child 两层 chunk：检索只搜索 child 小块，命中后按 `parent_chunk_id` 聚合，回答上下文返回 parent 摘要、parent 正文或命中的 child 摘录。Hybrid Search 使用 Dense 向量召回 + BM25 稀疏召回，并通过 RRF（Reciprocal Rank Fusion）融合排名；RRF top30 会进入模型 reranker，由模型读取 query + chunk 后重新给相关性分数，再输出最终 topK。reranker 未配置或调用失败时降级本地轻量 rerank。

### 7.1 写入知识文本

**POST** `/rag/ingest`

需要认证。

| 参数 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| title | string | 否 | 文档标题，缺省时取正文第一行 |
| content | string | 是 | 知识正文 |
| source | string | 否 | 来源说明，如 manual、url、file |
| source_type | string | 否 | 来源类型，如 `text`、`markdown`、`conversation`、`go` |
| visibility | string | 否 | `private` / `group` / `public`，默认 private |
| group_id | int64 | 否 | 群知识范围 |
| conversation_id | int64 | 否 | 会话知识范围 |

核心逻辑：

- 只允许当前登录用户作为 owner 写入知识。
- rag-service 将正文切成 parent/child 分层 chunk，只为 child 小块生成 embedding 并写入向量索引；parent 大块留作上下文、摘要和来源展示。
- 已配置 GLM embedding 时优先调用 GLM `embedding-3`；未配置或调用失败时降级为本地 hash embedding，保证入库不中断。

### 7.2 上传知识库文件

**POST** `/rag/upload`

需要认证，请求类型为 `multipart/form-data`。

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| file | file 或 file[] | 是 | 支持 `txt`、`md`、`markdown`、`pdf`、`docx`、图片、常见代码和配置文件，单个文件最大 20MB |
| title | string | 否 | 文档标题；多文件上传时为空更合理，会自动使用文件名 |
| visibility | string | 否 | `private` / `group` / `public`，默认 private |
| group_id | int64 | 否 | 群知识范围 |
| conversation_id | int64 | 否 | 会话知识范围 |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "files": [
      {
        "success": true,
        "file_name": "project.md",
        "chunk_count": 3,
        "entity_count": 8,
        "relation_count": 7,
        "msg": "写入成功"
      }
    ]
  }
}
```

文件解析边界：

- `txt/md/markdown` 要求 UTF-8 编码；Markdown 会按 `#` 文档标题、`##` parent chunk、`###`/段落 child chunk 分层。
- `go/js/ts/py/java/c/cpp/rs/sql/json/yaml` 等代码或配置文件要求 UTF-8 编码；Go 代码会按声明结构切分，函数前注释会随函数进入同一 parent chunk。其他语言代码当前可上传入库，但先走通用文本分片。
- `pdf` 优先使用本地轻量文本抽取；扫描件 PDF 在 api-gateway 配置 OCR provider 后会调用 GLM-OCR 兜底解析。
- `png/jpg/jpeg/webp/bmp/gif/tif/tiff` 图片文件在 api-gateway 配置 OCR provider 后可直接上传入库。
- `docx` 当前读取正文 XML 文本；复杂表格、批注、页眉页脚和图片 OCR 不在本阶段范围。
- 多文件上传按文件返回结果，单个文件失败不会阻断其他文件。

### 7.3 RAG 检索

**POST** `/rag/search`

需要认证。

| 参数 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| query | string | 是 | 用户问题 |
| mode | string | 否 | `adaptive` / `hybrid` / `graphrag` / `text_to_sql`，默认 adaptive |
| limit | int | 否 | 返回来源数量，默认 8，最大 20 |
| group_id | int64 | 否 | 群知识范围 |
| conversation_id | int64 | 否 | 会话知识范围 |

返回包含 `answer`、`sources`、`graph_nodes`、`graph_edges`、`route`、`crag_action`、`self_check`。

`crag_action` 说明：

- `correct`：CRAG evaluator 判断内部资料相关、覆盖充分、足够具体且无明显冲突，可直接使用内部资料回答。
- `incorrect`：内部资料明显不相关或覆盖不足，应 Web/询问用户搜索兜底。
- `ambiguous`：内部资料部分可用，但覆盖不足、过于泛化或存在冲突，应内部 + 外部合并。

CRAG evaluator 会评估 `Relevance`、`Coverage`、`Specificity`、`Conflict` 四项；当前结果会写入 `self_check.note`，例如：

```json
{
  "label": "ambiguous",
  "score": 0.56,
  "relevance": 0.72,
  "coverage": 0.45,
  "specificity": 0.62,
  "conflict": 0.10,
  "reason": "资料提到了 Agent 调度，但没有解释 event_id 和 agent_user_id 的业务含义"
}
```

`sources` 说明：

- `chunk_id` 默认是 parent chunk ID，而不是实际被向量/BM25 命中的 child chunk ID。
- `content` 优先返回 parent summary + parent 正文；当 parent 正文过长时，返回 parent summary + 命中 child 摘录，避免把超长上下文塞进 prompt。
- `reason` 会包含 `child_chunk_id=` 和 `parent_chunk_id=`，用于追踪“小块命中、大块返回”的链路；模型精排生效时还会包含 `model_rerank=`。

Adaptive RAG Router 说明：

- `mode=adaptive` 时，服务先执行 Router / Classifier，再决定是否检索以及走哪条路线。
- Router 采用规则 + LLM 混合：明显问候、实时最新、当前项目/代码、私有记忆、动作请求、高风险问题优先走规则；规则不确定时才调用 LLM Router。
- Router 输出 `route`、`complexity`、`need_retrieve`、`sources`、`strategy`、`retrieval_source` 和改写后的 `query`。这些信息目前写入 `self_check.note`。
- 支持路线：`direct`、`project_rag`、`strict_rag`、`web_rag`、`memory_rag`、`tool_action`。
- LLM 只做结构化判断，不执行 Milvus/Web/数据库/工具调用；真正检索由 rag-service 按当前用户权限上下文执行。

### 7.4 查询知识图谱

**GET** `/rag/graph?query=&limit=80`

需要认证。

返回当前用户可见的 GraphRAG 原始子图，包括节点、关系和社区摘要。该接口面向 RAG/GraphRAG 能力调试和内部数据读取；前端知识图谱可视化页面统一使用 `/knowledge/*`，由 knowledge-service 在原始子图之上补充过滤、详情聚合和展示属性。

### 7.5 文档列表

**GET** `/rag/documents?limit=20&offset=0`

需要认证。

返回当前用户可见的知识文档列表。

### 7.6 Embedding 环境变量

| 环境变量 | 说明 |
| ---- | ---- |
| `RAG_EMBEDDING_PROVIDER` | `glm` 表示调用 GLM embedding；其他值走本地 hash embedding |
| `RAG_EMBEDDING_URL` | GLM embedding 接口地址 |
| `RAG_EMBEDDING_API_KEY` | GLM API Key，只放本地 `.env`，不要提交仓库 |
| `RAG_EMBEDDING_MODEL` | 模型名，当前为 `embedding-3` |
| `RAG_EMBEDDING_DIMENSION` | 传给 GLM 的 dimensions；0 表示不传，使用模型默认维度 |
| `RAG_EMBEDDING_DIM` | 项目内部向量索引维度，默认 256 |
| `RAG_ROUTER_PROVIDER` | `rule` 使用本地规则；`llm` 使用小模型判断是否需要 RAG |
| `RAG_ROUTER_BASE_URL` | 项目内置 Router 的 OpenAI-compatible BaseURL；为空时回退 `LLM_DEFAULT_BASE_URL` |
| `RAG_ROUTER_API_KEY` | 项目内置 Router 的 API Key，只放本地 `.env`；为空时回退 `LLM_DEFAULT_API_KEY` |
| `RAG_ROUTER_MODEL` | Router 小模型名，例如 `glm-4-flash`；为空时回退 `LLM_DEFAULT_MODEL` |
| `RAG_RERANK_PROVIDER` | `glm` 表示启用 GLM rerank；未配置时使用本地轻量 rerank |
| `RAG_RERANK_URL` | GLM rerank 接口地址 |
| `RAG_RERANK_API_KEY` | GLM rerank API Key，只放本地 `.env`，不要提交仓库 |
| `RAG_RERANK_MODEL` | rerank 模型名，当前为 `rerank` |
| `DOCUMENT_OCR_PROVIDER` | `glm` 表示启用 GLM-OCR 文档解析 |
| `DOCUMENT_OCR_URL` | GLM-OCR layout_parsing 接口地址 |
| `DOCUMENT_OCR_API_KEY` | GLM-OCR API Key，只放本地 `.env`，不要提交仓库 |
| `DOCUMENT_OCR_MODEL` | OCR 模型名，当前为 `glm-ocr` |

Self-RAG 说明：

- `Retrieve` 现在由 Self-RAG Retrieve 判断器输出结构化 JSON；配置 LLM router 后，小模型会先决定是否需要检索、检索源和改写后的检索 query。
- LLM 只做判断，不执行 Milvus/Web/数据库工具；真正检索由 rag-service 代码按当前用户权限上下文执行。
- `IsRel`、`IsSup`、`IsUse` 由 Self-RAG judge 在 rerank、CRAG 和 answer synthesis 后判断；小模型不可用时降级规则判断。
- 用户级 `rag_router` 默认预设优先级高于项目内置 Router，便于用户用自己的 API Key、BaseURL 和小模型控制 RAG 成本。
- 如果判断为无需检索，接口返回 `route=direct`、`crag_action=skip_vector`，不会访问向量库。
- Router 调用失败时自动降级为本地规则，不阻断检索。

***

## 八、知识图谱可视化模块 (knowledge-service)

knowledge-service 负责知识图谱后端查询和前端可视化视图模型。它不负责文档入库、embedding、GraphRAG 实体抽取或关系抽取；这些仍归 rag-service。knowledge-service 会读取 rag-service 的 GraphRAG 子图，整理成前端画布需要的节点、边、社区、统计信息、颜色、大小、度数、详情和邻居列表。

当前状态说明：

- HTTP 入口已接入 api-gateway：`/api/v1/knowledge/*`。
- `idl/knowledge.thrift` 已生成 `kitex_gen/knowledge`，并落地独立 `cmd/knowledge-service` Kitex RPC 进程。
- api-gateway 通过 `pkg/knowledgeclient.RPCClient` 调用 knowledge-service；knowledge-service 再通过 rag-service RPC 读取 GraphRAG 子图，避免网关直接 import 其他服务的 `internal` 包。

### 8.1 查询可视化图谱

**GET** `/knowledge/graph?query=&types=&relations=&community_id=0&hops=1&limit=160`

需要认证。

| 参数 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| query | string | 否 | 搜索关键词，传给底层 GraphRAG 子图查询 |
| types | string | 否 | 实体类型过滤，逗号分隔，如 `Service,DatabaseTable` |
| relations | string | 否 | 关系类型过滤，逗号分隔，如 `WRITES,CALLS` |
| community_id | int64 | 否 | 社区过滤，0 表示不过滤 |
| hops | int | 否 | 邻居扩展层数，支持 1 或 2，默认 1 |
| limit | int | 否 | 最大节点/子图规模，默认 160 |

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "nodes": [
      {
        "id": 1,
        "name": "msg-core-service",
        "type": "Service",
        "summary": "消息核心服务，负责会话、消息写入和 Outbox",
        "community_id": 10,
        "score": 0.91,
        "degree": 3,
        "size": 56,
        "color": "#0f766e"
      }
    ],
    "edges": [
      {
        "id": 11,
        "source_id": 1,
        "target_id": 2,
        "relation": "WRITES",
        "description": "WRITES · 发送消息时写入 event_outbox",
        "weight": 0.88,
        "evidence": "发送消息时同事务写入 messages 和 event_outbox",
        "color": "#16a34a"
      }
    ],
    "communities": [
      {
        "id": 10,
        "name": "IM 消息链路",
        "summary": "描述消息写入、事件发布和在线推送流程",
        "level": 1,
        "color": "#0f766e"
      }
    ],
    "stats": {
      "node_count": 1,
      "edge_count": 1,
      "community_count": 1,
      "types": ["Service"],
      "relations": ["WRITES"]
    }
  }
}
```

前端使用说明：

- G6 可用时使用 AntV G6 渲染拖拽、缩放、悬浮、节点大小、社区颜色和关系连线。
- G6 CDN 不可用时自动降级为 SVG 图谱，仍保留搜索、过滤、节点/关系点击详情。
- 节点颜色优先使用社区颜色；没有社区时按实体类型着色。
- 节点大小由 degree 和 score 计算，关系颜色由关系类型计算。

### 8.2 查询节点详情

**GET** `/knowledge/node/:id?query=&limit=160`

需要认证。

返回节点本身、相邻节点和所有相关关系。节点不存在、或当前用户没有权限看到该节点时，`success=false`，`msg` 说明原因。

### 8.3 查询关系详情

**GET** `/knowledge/edge/:id?query=&limit=160`

需要认证。

返回关系本身、source 节点、target 节点和 evidence 原文证据。关系不存在、或当前用户没有权限看到该关系时，`success=false`，`msg` 说明原因。

### 8.4 查询节点邻域子图

**GET** `/knowledge/node/:id/neighborhood?query=&types=&relations=&community_id=0&hops=1&limit=160`

需要认证。

返回以指定节点为中心的一跳或多跳邻域子图。`hops` 默认 1，最大按后端策略裁剪为 3，避免一次性展开过大的图。响应结构与 `/knowledge/graph` 相同。

### 8.5 查询两节点最短路径

**GET** `/knowledge/path?source_id=1&target_id=2&query=&limit=160`

需要认证。

返回两个实体之间的最短可见路径，用于前端路径高亮和链路解释。

响应示例：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "node_ids": [2, 1, 3],
    "edge_ids": [101, 102],
    "nodes": [],
    "edges": []
  }
}
```

***

## 九、WebSocket 网关 (websocket-gateway)

### 9.1 建立 WebSocket 连接

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

### 9.2 消息推送接口（内部兼容接口）

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

### 9.3 Kafka 事件主题（内部）

Kafka 默认开启，默认 broker 为 `127.0.0.1:9092`。本地如暂时不启动 Kafka，可设置 `KAFKA_ENABLED=false`；业务请求仍会把待发布事件写入 `event_outbox`，等 Kafka 恢复并启用 worker 后继续投递。

| Topic | 生产者 | 消费者 | 说明 |
| ----- | ------ | ------ | ---- |
| `claran.group.events` | group-service Outbox worker | msg-core-service | 群创建、邀请、踢人、解散事件，用于同步群聊会话和参与者 |
| `claran.message.events` | msg-core-service Outbox worker | websocket-gateway、agent-manager-service | 新消息、编辑、撤回、已读事件，用于在线 WebSocket 推送和兼容 Agent @ 分发 |
| `claran.im.events` | 各 IM 事件生产者 | agent-manager-service | Agent-Native IM 统一事件，承载表情、文件、语音转写、群成员、系统通知、任务变化等事件 |
| `claran.agent.events` | agent-manager-service / agent-runtime-service | 审计、成本监控、异步后处理消费者 | Agent 运行、完成、失败、工具调用和审计事件 |

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

### 9.4 查询在线用户

**GET** `http://localhost:8081/online`

响应示例：

```json
{
  "online_users": [1, 2, 5]
}
```

***

### 9.5 查询用户是否在线

**GET** `http://localhost:8081/is_online?user_id=1`

响应示例：

```json
{
  "user_id": 1,
  "online": true
}
```

***

## 十、数据库表结构

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
| id           | bigint PK    | 10 位数字群号       |
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

### bots 表 (agent-manager-service)

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

### bot_routes 表 (agent-manager-service)

| 字段            | 类型           | 说明                                |
| ------------- | ------------ | --------------------------------- |
| id            | bigint PK    | 雪花ID                               |
| bot_id        | bigint       | Agent ID，索引                         |
| route_pattern | varchar(255) | 路由匹配模式                            |
| route_type    | varchar(50)  | 路由类型：keyword/regex               |
| priority      | int          | 优先级，默认 0                          |
| created_at    | datetime     | 创建时间                              |

### billing_records 表 (agent-manager-service)

| 字段            | 类型           | 说明       |
| ------------- | ------------ | -------- |
| id            | bigint PK    | 雪花ID      |
| bot_id        | bigint       | Agent ID，索引 |
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

## 十一、Redis 缓存 Key 规范

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

## 十二、前端页面功能映射

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
| AI助手按钮（🤖）        | GET /agent/list → POST /agent/chat                          | 打开AI助手面板进行对话      |
| "搜索"按钮           | GET /message/search                                     | 搜索当前会话消息          |
| "详情"按钮           | GET /message/history/:id                                | 查看会话详情            |
| 个人信息编辑           | PUT /user/info                                          | 编辑昵称、头像、头图、签名、简介等资料 |
| AI助手管理面板          | POST /agent/create → GET /agent/list → POST /agent/chat      | 创建/管理/对话AI助手      |
| AI助手路由管理          | POST /agent/route/create → GET /agent/:id/routes            | 配置消息路由规则          |
| AI助手计费查询          | GET /agent/:id/billing                                    | 查看Token用量和费用      |
| 知识图谱页面            | GET /knowledge/graph → GET /knowledge/node/:id → GET /knowledge/edge/:id | 搜索、过滤、拖拽查看知识图谱和证据来源 |
| WebSocket 连接     | ws://localhost:8081/ws?token=xxx                        | 登录后自动建立，接收实时消息    |

***

## 十三、多媒体消息格式规范

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

## 十四、服务端口一览

| 服务                  | 端口   | 协议       | 说明         |
| ------------------- | ---- | -------- | ---------- |
| user-service        | 9001 | Thrift RPC | 用户服务       |
| group-service       | 9002 | Thrift RPC | 群组服务       |
| msg-core-service    | 9003 | Thrift RPC | 消息核心服务     |
| msg-history-service | 9004 | Thrift RPC | 消息历史服务     |
| file-service        | 9005 | Thrift RPC | 文件服务       |
| agent-manager-service | 9006 | Thrift RPC | AI助手管理服务   |
| agent-runtime-service | 9007 | Thrift RPC | Agent 长会话、工具调用、事件执行 |
| memory-service      | 9008 | Thrift RPC | 用户/群/会话长期记忆 |
| settings-service    | 9009 | Thrift RPC | LLM 预设、Prompt 模板、Agent Skill 配置 |
| rag-service         | 9012 | Thrift RPC | 知识库、Hybrid RAG、GraphRAG 子图 |
| knowledge-service   | 9013 | Thrift RPC | 知识图谱查询、过滤、详情和可视化视图 |
| api-gateway         | 8080 | HTTP      | API 网关     |
| websocket-gateway   | 8081 | HTTP/WS   | WebSocket 网关 |
| Kafka               | 9092 | TCP       | 事件总线       |
| Kafka Controller    | 9093 | TCP       | Kafka KRaft 控制器 |
| DTM HTTP            | 36789 | HTTP      | 分布式事务协调器 |
| DTM gRPC            | 36790 | gRPC      | 分布式事务协调器 |
| MinIO               | 9000 | HTTP      | 对象存储       |
| MinIO Console       | 9009 | HTTP      | MinIO 管理界面；若和 settings-service 本机端口冲突，可在 Docker 映射中改为 9011 |
| MySQL               | 3306 | TCP       | 数据库        |
| Redis               | 6379 | TCP       | 缓存         |
| Etcd                | 2379 | gRPC      | 服务注册与发现    |









