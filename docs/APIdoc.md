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

### 1.3 获取用户信息

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

### 1.4 更新用户信息

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

### 1.5 添加好友

**POST** `/user/friend/add`

需要认证。

| 参数         | 类型     | 必填 | 说明            |
| ---------- | ------ | -- | ------------- |
| friend\_id | int64  | 是  | 好友用户ID        |
| group\_id  | int64  | 否  | 好友分组ID，0=默认分组 |
| remark     | string | 否  | 好友备注          |

请求示例：

```json
{
  "friend_id": 2,
  "group_id": 0,
  "remark": "我的同事"
}
```

核心逻辑：

- 不能添加自己为好友
- 检查是否已是好友关系
- 双向添加好友（A→B 和 B→A 各一条记录）
- 清除双方好友列表缓存（key: `user:friends:{id}`）

***

### 1.6 删除好友

**POST** `/user/friend/delete`

需要认证。

| 参数         | 类型    | 必填 | 说明     |
| ---------- | ----- | -- | ------ |
| friend\_id | int64 | 是  | 好友用户ID |

核心逻辑：

- 双向删除好友关系
- 清除双方好友列表缓存

***

### 1.7 获取好友列表

**GET** `/user/friend/list`

需要认证。

无请求参数。

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

### 1.8 创建好友分组

**POST** `/user/friend/group`

需要认证。

| 参数   | 类型     | 必填 | 说明   |
| ---- | ------ | -- | ---- |
| name | string | 是  | 分组名称 |

***

### 1.9 获取好友分组列表

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
| member\_ids | \[]int64 | 否  | 初始成员用户ID列表 |

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
      "created_at": "2026-05-11T22:50:00+08:00"
    }
  }
}
```

***

### 2.3 获取用户所在群组列表

**GET** `/group/list`

需要认证。

核心逻辑：

- 通过 JOIN group\_members 表查询用户所在的所有群组

***

### 2.4 邀请成员

**POST** `/group/invite`

需要认证。

| 参数        | 类型       | 必填 | 说明         |
| --------- | -------- | -- | ---------- |
| group\_id | int64    | 是  | 群组ID       |
| user\_ids | \[]int64 | 是  | 被邀请的用户ID列表 |

核心逻辑：

- 操作者必须是群主或管理员（owner/admin）
- 已在群中的用户不重复添加
- 新成员角色默认为 member

***

### 2.5 踢出成员

**POST** `/group/kick`

需要认证。

| 参数        | 类型    | 必填 | 说明       |
| --------- | ----- | -- | -------- |
| group\_id | int64 | 是  | 群组ID     |
| user\_id  | int64 | 是  | 被踢出的用户ID |

核心逻辑：

- 操作者必须是群主或管理员
- 不能踢出群主

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

***

## 三、消息模块 (msg-core-service)

### 3.1 创建会话

**POST** `/message/conversation`

需要认证。

| 参数               | 类型       | 必填 | 说明                               |
| ---------------- | -------- | -- | -------------------------------- |
| type             | string   | 是  | 会话类型：`private`(私聊) / `group`(群聊) |
| participant\_ids | \[]int64 | 是  | 参与者用户ID列表（至少2人）                  |

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
- 群聊不去重，每次创建新会话
- 创建会话后添加所有参与者到 conversation\_participants 表
- 清除所有参与者的会话列表缓存

***

### 3.2 发送消息

**POST** `/message/send`

需要认证。

| 参数               | 类型     | 必填 | 说明             |
| ---------------- | ------ | -- | -------------- |
| conversation\_id | int64  | 是  | 会话ID           |
| content          | string | 是  | 消息内容           |
| msg\_type        | string | 否  | 消息类型，默认 `text` |

请求示例：

```json
{
  "conversation_id": 1,
  "content": "你好！",
  "msg_type": "text"
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
2. 消息写入 MySQL（messages 表）
3. 更新会话的 updated\_at 为消息创建时间
4. 缓存最近消息到 Redis（key: `conversation:recent:{id}`，TTL 10min）
5. 清除所有参与者的会话列表缓存
6. **WebSocket 实时推送**：获取会话所有参与者ID → 调用 pushClient.PushMessage() → websocket-gateway 的 `/push` API → Hub.Broadcast() → 所有在线参与者浏览器实时收到消息

***

### 3.3 获取消息历史

**GET** `/message/history/:id`

需要认证。

| 参数         | 类型    | 必填      | 说明                   |
| ---------- | ----- | ------- | -------------------- |
| id         | int64 | 是（路径参数） | 会话ID                 |
| limit      | int64 | 否       | 每页条数，默认 50           |
| before\_id | int64 | 否       | 翻页游标，加载此ID之前的消息，默认 0 |

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
        "created_at": "2026-05-11 23:15:30"
      }
    ]
  }
}
```

核心逻辑：

- 按 ID 降序查询（最新在前），返回时反转为时间正序
- before\_id > 0 时实现游标分页

***

### 3.4 搜索消息

**GET** `/message/search`

需要认证。

| 参数      | 类型     | 必填 | 说明         |
| ------- | ------ | -- | ---------- |
| keyword | string | 是  | 搜索关键词      |
| limit   | int64  | 否  | 返回条数，默认 20 |

核心逻辑：

- 只搜索用户所在会话的消息
- 使用 MySQL LIKE 模糊匹配

***

### 3.5 获取用户会话列表

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
        "last_message_time": "2026-05-11 23:15:30"
      }
    ]
  }
}
```

核心逻辑：

- 优先从 Redis 缓存读取（key: `user:conversations:{id}`，TTL 5min）
- 查询用户参与的所有会话，附带最后一条消息

***

## 四、WebSocket 网关 (websocket-gateway)

### 4.1 建立 WebSocket 连接

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

### 4.2 消息推送接口（内部）

**POST** `http://localhost:8081/push`

供后端服务调用，不对外暴露。

| 参数                    | 类型       | 必填 | 说明                    |
| --------------------- | -------- | -- | --------------------- |
| target\_user\_ids     | \[]int64 | 是  | 目标用户ID列表              |
| data.type             | string   | 是  | 消息类型（如 `new_message`） |
| data.conversation\_id | int64    | 是  | 会话ID                  |
| data.sender\_id       | int64    | 是  | 发送者ID                 |
| data.content          | string   | 是  | 消息内容                  |
| data.msg\_type        | string   | 是  | 消息类型                  |
| data.msg\_id          | int64    | 是  | 消息ID                  |
| data.created\_at      | string   | 是  | 消息创建时间                |

***

### 4.3 查询在线用户

**GET** `http://localhost:8081/online`

响应示例：

```json
{
  "online_users": [1, 2, 5]
}
```

***

### 4.4 查询用户是否在线

**GET** `http://localhost:8081/is_online?user_id=1`

响应示例：

```json
{
  "user_id": 1,
  "online": true
}
```

***

## 五、数据库表结构

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
| created\_at | datetime     | 创建时间              |
| updated\_at | datetime     | 更新时间              |

### friends 表 (user-service)

| 字段          | 类型          | 说明        |
| ----------- | ----------- | --------- |
| id          | bigint PK   | 自增主键      |
| user\_id    | bigint      | 用户ID，索引   |
| friend\_id  | bigint      | 好友ID，索引   |
| group\_id   | bigint      | 好友分组ID，索引 |
| remark      | varchar(50) | 备注        |
| created\_at | datetime    | 创建时间      |

### friend\_groups 表 (user-service)

| 字段          | 类型          | 说明      |
| ----------- | ----------- | ------- |
| id          | bigint PK   | 自增主键    |
| user\_id    | bigint      | 用户ID，索引 |
| name        | varchar(50) | 分组名称    |
| created\_at | datetime    | 创建时间    |

### groups 表 (group-service)

| 字段           | 类型           | 说明      |
| ------------ | ------------ | ------- |
| id           | bigint PK    | 自增主键    |
| name         | varchar(100) | 群组名称    |
| avatar       | varchar(255) | 群头像     |
| owner\_id    | bigint       | 群主ID，索引 |
| announcement | text         | 群公告     |
| created\_at  | datetime     | 创建时间    |
| updated\_at  | datetime     | 更新时间    |

### group\_members 表 (group-service)

| 字段           | 类型          | 说明                    |
| ------------ | ----------- | --------------------- |
| id           | bigint PK   | 自增主键                  |
| group\_id    | bigint      | 群组ID，索引               |
| user\_id     | bigint      | 用户ID，索引               |
| role         | varchar(20) | 角色：owner/admin/member |
| muted\_until | datetime    | 禁言截止时间（可空）            |
| joined\_at   | datetime    | 加入时间                  |

### conversations 表 (msg-core-service)

| 字段          | 类型          | 说明                 |
| ----------- | ----------- | ------------------ |
| id          | bigint PK   | 自增主键               |
| type        | varchar(20) | 会话类型：private/group |
| created\_at | datetime    | 创建时间               |
| updated\_at | datetime    | 更新时间（最后消息时间）       |

### conversation\_participants 表 (msg-core-service)

| 字段               | 类型        | 说明      |
| ---------------- | --------- | ------- |
| id               | bigint PK | 自增主键    |
| conversation\_id | bigint    | 会话ID，索引 |
| user\_id         | bigint    | 用户ID，索引 |
| joined\_at       | datetime  | 加入时间    |

### messages 表 (msg-core-service)

| 字段               | 类型          | 说明        |
| ---------------- | ----------- | --------- |
| id               | bigint PK   | 自增主键      |
| conversation\_id | bigint      | 会话ID，索引   |
| sender\_id       | bigint      | 发送者ID，索引  |
| content          | text        | 消息内容      |
| msg\_type        | varchar(20) | 消息类型：text |
| created\_at      | datetime    | 创建时间      |

***

## 六、Redis 缓存 Key 规范

| Key 模式                     | 服务                        | TTL         | 说明        |
| -------------------------- | ------------------------- | ----------- | --------- |
| `user:info:{id}`           | user-service              | 15min       | 用户信息 JSON |
| `user:friends:{id}`        | user-service              | 5min        | 好友列表 JSON |
| `user:friend_groups:{id}`  | user-service              | 10min       | 好友分组 JSON |
| `online:user:{id}`         | user-service / ws-gateway | 30min / 30s | 在线状态标记    |
| `user:conversations:{id}`  | msg-core-service          | 5min        | 会话列表 JSON |
| `conversation:recent:{id}` | msg-core-service          | 10min       | 最近消息 JSON |

缓存失效策略：写操作时主动删除相关 Key，依赖 TTL 作为兜底。

***

## 七、前端页面功能映射

| 页面/按钮        | 对应接口                                                  | 说明             |
| ------------ | ----------------------------------------------------- | -------------- |
| 登录表单         | POST /user/login                                      | 输入用户名+密码登录     |
| 注册表单         | POST /user/register                                   | 输入用户名+密码+昵称注册  |
| 侧边栏-会话列表     | GET /message/conversations                            | 加载所有会话         |
| 侧边栏-好友列表     | GET /user/friend/list                                 | 加载好友列表         |
| 侧边栏-群组列表     | GET /group/list                                       | 加载群组列表         |
| 好友"聊天"按钮     | POST /message/conversation → GET /message/history/:id | 创建私聊会话并打开      |
| 群组"进入"按钮     | GET /group/:id/members → POST /message/conversation   | 获取群成员后创建群聊会话   |
| "+新会话"按钮     | POST /message/conversation                            | 弹窗选择类型和参与者     |
| "+添加好友"按钮    | POST /user/friend/add                                 | 弹窗输入好友ID       |
| "+创建群组"按钮    | POST /group/create                                    | 弹窗输入群名和成员      |
| 消息输入框        | POST /message/send                                    | 发送文本消息         |
| "详情"按钮       | GET /message/history/:id + GET /message/search        | 查看会话详情和搜索消息    |
| WebSocket 连接 | ws\://localhost:8081/ws?token=xxx                     | 登录后自动建立，接收实时消息 |

