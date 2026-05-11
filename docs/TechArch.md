# ClaranAIM 技术实现原理

本文档从架构设计、数据流、核心机制三个维度，逐层拆解 ClaranAIM 第一阶段的完整技术实现逻辑。

---

## 一、整体架构

### 1.1 架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                         前端 (dist/)                             │
│              登录/注册 · 聊天 · 好友 · 群组                       │
└──────┬──────────────────────────────┬───────────────────────────┘
       │ HTTP (REST API)              │ WebSocket
       ▼                              ▼
┌──────────────┐              ┌──────────────────┐
│  api-gateway  │              │ websocket-gateway │
│  (Hertz :8080)│              │  (net/http :8081) │
│  JWT鉴权·路由  │              │  连接管理·消息推送  │
└──┬───┬───┬───┘              └────────┬─────────┘
   │   │   │                           │
   │   │   │  RPC (Kitex + TTHeader)   │ HTTP /push
   │   │   │  服务发现: Etcd            │
   ▼   ▼   ▼                           │
┌─────┐┌─────┐┌──────────┐             │
│user ││group││msg-core  │◄────────────┘
│svc  ││svc  ││svc       │  PushClient
│:9001││:9002││:9003     │
└──┬──┘└──┬──┘└──┬───────┘
   │      │      │
   │      │      ▼
   │      │  ┌──────────────┐
   │      │  │msg-history   │
   │      │  │svc :9004     │
   │      │  └──────┬───────┘
   ▼      ▼         ▼
┌─────────────────────────────┐
│     MySQL (Docker :3306)     │
│  users · friends · groups    │
│  messages · conversations    │
└─────────────────────────────┘
┌─────────────────────────────┐
│     Redis (Docker :6379)     │
│  缓存 · 在线状态              │
└─────────────────────────────┘
┌─────────────────────────────┐
│     Etcd (Docker :2379)      │
│  服务注册与发现               │
└─────────────────────────────┘
```

### 1.2 为什么是微服务而不是单体

| 维度 | 单体 | 微服务（当前选择） |
|------|------|-------------------|
| 表归属 | 所有表在一个数据库 | 每个服务管理自己的表，独立迁移 |
| 部署 | 改一行全量部署 | 改 user-service 不影响 msg-core-service |
| 扩容 | 整体扩容 | 消息服务压力大时只扩消息服务 |
| 故障隔离 | 一个模块崩溃全部崩溃 | 一个服务挂了不影响其他服务 |
| 技术选型 | 统一技术栈 | 未来 AI 服务可以用 Python |

### 1.3 各服务职责

| 服务 | 端口 | 框架 | 职责 |
|------|------|------|------|
| api-gateway | 8080 | Hertz | HTTP 入口，JWT 鉴权，路由分发，RPC 转发 |
| websocket-gateway | 8081 | net/http + gorilla/websocket | WebSocket 连接管理，实时消息推送 |
| user-service | 9001 | Kitex | 用户注册/登录/信息/好友/分组 |
| group-service | 9002 | Kitex | 群组 CRUD/成员管理/权限校验 |
| msg-core-service | 9003 | Kitex | 会话管理/消息发送/消息搜索 |
| msg-history-service | 9004 | Kitex | 消息历史归档/离线消息/已读未读 |

---

## 二、分层架构详解

每个后端服务严格遵循四层架构，以 user-service 为例：

```
cmd/user-service/main.go          ← 启动入口：加载配置→初始化DB/Redis→创建Kitex Server
internal/user-service/
  ├── handler/handler.go           ← RPC 入口层：接收 Thrift 请求，转换参数，调用 Service
  ├── service/service.go           ← 业务逻辑层：核心业务规则，缓存读写，调用 DAO
  ├── dao/dao.go                   ← 数据访问层：纯数据库操作，不包含业务逻辑
  └── model/model.go               ← 数据模型层：GORM 模型定义，对应数据库表
```

### 2.1 各层职责边界

**Handler 层**（RPC 入口）
- 接收 Kitex 生成的 Thrift 请求结构体
- 参数校验（非空检查等）
- 调用 Service 层获取结果
- 将结果转换为 Thrift 响应结构体返回
- **不包含**任何业务逻辑或数据库操作

**Service 层**（业务核心）
- 实现所有业务规则（如：不能添加自己为好友、私聊去重等）
- 管理缓存读写策略（先查缓存→缓存未命中查DB→回写缓存）
- 缓存失效策略（写操作时主动删除相关 Key）
- 调用 DAO 层进行数据持久化
- 调用外部服务（如 PushClient 推送消息）

**DAO 层**（数据访问）
- 定义 Repository 接口（面向接口编程，便于测试和替换实现）
- 纯数据库 CRUD 操作
- 使用 GORM 的 `WithContext` 支持请求级超时控制
- **不包含**任何业务判断逻辑

**Model 层**（数据模型）
- GORM 结构体定义，映射数据库表
- `TableName()` 方法指定表名
- JSON tag 控制序列化行为
- `json:"-"` 隐藏密码字段

### 2.2 依赖注入

所有层通过构造函数注入依赖，而非在内部创建：

```go
// main.go 中的组装过程
db := dao.InitDB(cfg.MySQL.DSN)
redisClient := redis.NewRedisClient(...)
repo := dao.NewUserRepo(db)
svc := service.NewUserService(repo, redisClient)
handler := handler.NewUserServiceImpl(svc)

// Kitex Server 注册 handler
server := kitex.NewServer(handler, ...)
```

这样做的好处：
- **可测试**：可以注入 mock 实现进行单元测试
- **可替换**：DAO 实现可以替换为其他数据库驱动
- **解耦**：每层只依赖接口，不依赖具体实现

---

## 三、核心数据流

### 3.1 用户注册完整流程

```
前端 POST /api/v1/user/register
  │
  ▼
api-gateway: UserHandler.Register()
  │  1. BindJSON 解析请求体 {username, password, nickname}
  │  2. 调用 RPC: UserClient.Register(ctx, &RegisterReq{...})
  │
  ▼
user-service: UserServiceImpl.Register()
  │  1. 调用 Service: svc.Register(ctx, username, password, nickname)
  │
  ▼
user-service: userServiceImpl.Register()
  │  1. 校验用户名和密码非空
  │  2. repo.GetUserByUsername() → 检查用户名是否已存在
  │  3. password.HashPassword(pwd) → bcrypt 加密（cost=10）
  │  4. 昵称为空时默认等于用户名
  │  5. repo.CreateUser(ctx, &User{...}) → 写入 MySQL
  │  6. redis.SetJSON("user:info:{id}", user, 15min) → 缓存用户信息
  │
  ▼
返回 RegisterResp{success: true, user_id: 1, msg: "注册成功"}
```

### 3.2 用户登录完整流程

```
前端 POST /api/v1/user/login
  │
  ▼
api-gateway: UserHandler.Login()
  │  调用 RPC: UserClient.Login(ctx, &LoginReq{...})
  │
  ▼
user-service: userServiceImpl.Login()
  │  1. repo.GetUserByUsername() → 查询用户记录
  │  2. password.CheckPassword(pwd, user.Password) → bcrypt 校验
  │  3. jwt.GenerateToken(secretKey, userID, username, expiration)
  │     → 生成 JWT（HS256 签名，payload 含 userID + username + 过期时间）
  │  4. user.Status = "online" → repo.UpdateUser() 更新状态
  │  5. redis.SetJSON("user:info:{id}", user, 15min) → 刷新用户缓存
  │  6. redis.Set("online:user:{id}", "1", 30min) → 设置在线标记
  │
  ▼
返回 LoginResp{success: true, token: "eyJ...", user_id: 1}
```

### 3.3 发送消息完整流程（最核心）

这是整个系统最复杂的数据流，涉及 4 个服务协作：

```
前端 POST /api/v1/message/send
  │  {conversation_id: 1, content: "你好", msg_type: "text"}
  │
  ▼
api-gateway: MessageHandler.SendMessage()
  │  1. 从 JWT 上下文获取 userID
  │  2. 调用 RPC: MessageClient.SendMessage(ctx, &SendMessageReq{...})
  │
  ▼
msg-core-service: messageServiceImpl.SendMessage()
  │
  │  ── 第1步：校验 ──
  │  1. 校验 content 非空
  │  2. repo.GetConversationByID() → 校验会话存在
  │
  │  ── 第2步：持久化 ──
  │  3. repo.CreateMessage(ctx, &Message{...}) → 写入 messages 表
  │
  │  ── 第3步：更新会话时间戳 ──
  │  4. conv.UpdatedAt = msg.CreatedAt
  │  5. repo.UpdateConversation(ctx, conv) → 更新 conversations 表
  │
  │  ── 第4步：缓存更新 ──
  │  6. redis.SetJSON("conversation:recent:{id}", msg, 10min) → 缓存最近消息
  │  7. 遍历所有参与者 → redis.Del("user:conversations:{uid}") → 清除会话列表缓存
  │
  │  ── 第5步：WebSocket 实时推送 ──
  │  8. repo.GetParticipants(conversationID) → 获取所有参与者 userID
  │  9. pushClient.PushMessage(targetUserIDs, pushData)
  │     │
  │     ▼
  │  push.PushClient.PushMessage()
  │     │  HTTP POST http://localhost:8081/push
  │     │  Body: {target_user_ids: [1,2], data: {type, conversation_id, sender_id, content, ...}}
  │     │
  │     ▼
  │  websocket-gateway: /push handler
  │     │  1. 解析推送请求
  │     │  2. 构造 WebSocket 消息: {type: "new_message", data: {...}}
  │     │  3. h.Broadcast(targetUserIDs, data)
  │     │
  │     ▼
  │  hub.Hub.Broadcast()
  │     │  1. 遍历 targetUserIDs
  │     │  2. 查找每个用户在 Hub.clients 中的所有连接
  │     │  3. 将消息写入每个连接的 Send channel
  │     │
  │     ▼
  │  WSClient.WritePump()
  │     │  从 Send channel 读取消息 → websocket.WriteMessage → 浏览器收到消息
  │     ▼
  │  前端 onmessage 回调 → 渲染新消息气泡
  │
  ▼
返回 SendMessageResp{success: true, msg_id: 42, send_time: "..."}
```

### 3.4 WebSocket 连接建立流程

```
前端: new WebSocket("ws://localhost:8081/ws?token=eyJ...")
  │
  ▼
websocket-gateway: WSHandler.ServeHTTP()
  │  1. 从 URL query 提取 token
  │  2. jwt.ParseToken(token) → 解析并验证 JWT
  │  3. Upgrader.Upgrade(w, r, nil) → HTTP 升级为 WebSocket
  │  4. 创建 WSClient 和 Hub.Client
  │  5. Hub.Register(client) → 注册到 Hub（按 userID 分组）
  │  6. redis.Set("online:user:{id}", "1", 30s) → 设置在线标记
  │  7. 启动两个协程：
  │     - go wsClient.ReadPump()  → 读取客户端消息 + 心跳检测
  │     - go wsClient.WritePump() → 推送消息 + 定时 ping
  │
  ▼
连接建立成功，进入长连接状态
```

---

## 四、核心机制详解

### 4.1 JWT 认证机制

**Token 生成**（登录时）：
```go
claims := Claims{
    UserID:   user.ID,        // 用户ID
    Username: user.Username,  // 用户名
    StandardClaims: jwt.StandardClaims{
        ExpiresAt: now.Add(168h).Unix(),  // 过期时间
        IssuedAt:  now.Unix(),            // 签发时间
        Issuer:    "ClaranAIM",           // 签发者
    },
}
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
signedToken := token.SignedString([]byte(secretKey))  // HMAC-SHA256 签名
```

**Token 验证**（每次请求时）：
```
api-gateway 中间件: JWTAuthMiddleware()
  1. 从 Authorization 头提取 "Bearer <token>"
  2. jwt.ParseToken(token) → 验证签名 + 检查过期时间
  3. 解析出 claims.UserID 和 claims.Username
  4. 存入请求上下文: c.Set("userID", claims.UserID)
  5. 后续 handler 通过 c.Get("userID") 获取当前用户ID
```

**WebSocket 认证**（特殊处理）：
- WebSocket 无法设置 HTTP Header，因此 Token 通过 URL 参数传递
- `ws://localhost:8081/ws?token=<JWT_TOKEN>`
- 服务端在 Upgrade 之前验证 Token

### 4.2 密码安全机制

```
注册时:
  明文密码 → bcrypt.GenerateFromPassword(password, cost=10) → $2a$10$...哈希值
  存入数据库的 password 字段是 60 字符的 bcrypt 哈希

登录时:
  明文密码 + 数据库中的哈希值 → bcrypt.CompareHashAndPassword()
  → 匹配返回 true，不匹配返回 false

为什么用 bcrypt 而不是 MD5/SHA256:
  - bcrypt 自带盐值（salt），无需额外存储
  - cost 参数控制计算复杂度，抵抗暴力破解
  - 每次生成的哈希值不同，防止彩虹表攻击
```

### 4.3 RPC 通信机制

**为什么用 Kitex + Thrift 而不是 gRPC + Protobuf**：
- Kitex 是字节跳动开源的高性能 RPC 框架，对 Thrift 协议有深度优化
- Thrift IDL 语法更简洁，适合快速定义服务接口
- TTHeader 传输协议提供更高效的序列化和反序列化

**IDL 定义 → 代码生成**：
```
idl/user.thrift → kitex_gen/user/
  ├── userservice/client.go    ← RPC 客户端桩代码
  ├── userservice/server.go    ← RPC 服务端桩代码
  └── user/model.go            ← 请求/响应结构体

idl/group.thrift → kitex_gen/group/...
idl/message.thrift → kitex_gen/message/...
```

**RPC 客户端配置**（api-gateway 中）：
```go
UserClient, _ = userservice.NewClient("user-service",
    client.WithResolver(etcdResolver),              // Etcd 服务发现
    client.WithTransportProtocol(transport.TTHeader), // TTHeader 协议
)
```

**RPC 服务端配置**（各服务 main.go 中）：
```go
svr := userservice.NewServer(handler,
    server.WithServiceAddr(addr),
    server.WithRegistry(etcdRegistry),              // Etcd 服务注册
    kitexutil.WithServerMiddlewares(...),            // TTHeader meta handler
)
```

**服务发现流程**：
```
1. user-service 启动 → 向 Etcd 注册: key="/kitex/registry/user-service", value="127.0.0.1:9001"
2. api-gateway 启动 → 创建 EtcdResolver
3. RPC 调用时 → Resolver 从 Etcd 查询 "user-service" 的地址 → 建立连接 → 发送请求
4. Etcd 保持租约心跳 → 服务下线时租约过期自动注销
```

### 4.4 WebSocket 实时推送机制

**Hub 设计**（核心数据结构）：
```go
type Hub struct {
    clients    map[int64]map[*Client]bool  // userID → 该用户的所有连接
    broadcast  chan *BroadcastMessage       // 广播消息通道
    register   chan *Client                 // 注册通道
    unregister chan *Client                 // 注销通道
    mu         sync.RWMutex                // 读写锁保护 clients
}
```

**为什么 clients 是 `map[int64]map[*Client]bool` 而不是 `map[int64]*Client`**：
- 一个用户可能打开多个浏览器标签页/设备
- 每个标签页/设备对应一个 WebSocket 连接
- 所以一个 userID 可以对应多个 Client

**Hub.Run() 事件循环**：
```
for {
    select {
    case client := <-register:     // 新连接 → 加入 clients
    case client := <-unregister:   // 断开 → 从 clients 移除 + close(Send)
    case msg := <-broadcast:       // 推送消息 → 遍历目标用户的所有连接 → 写入 Send channel
    }
}
```

**消息推送链路**：
```
msg-core-service.SendMessage()
  → pushClient.PushMessage(targetUserIDs, data)
    → HTTP POST /push
      → websocket-gateway 解析请求
        → hub.Broadcast(targetUserIDs, jsonData)
          → 遍历每个目标用户的所有连接
            → client.Send <- data
              → WritePump() 从 Send 读取
                → conn.NextWriter(TextMessage).Write(data)
                  → 浏览器 onmessage 回调
```

**心跳保活**：
```
WritePump:
  每 54 秒发送 Ping 帧 → conn.WriteMessage(PingMessage, nil)

ReadPump:
  设置 ReadDeadline = 60 秒
  收到 Pong → 重置 ReadDeadline
  60 秒无 Pong → 断开连接 → 触发 Unregister
```

**在线状态同步**：
```
WebSocket 连接建立时:
  redis.Set("online:user:{id}", "1", 30s)

定时同步（每 10 秒）:
  遍历 Hub 中的在线用户 → 刷新 Redis TTL

WebSocket 断开时:
  Hub.Unregister → 从内存移除
  Redis 中的 key 会在 30s TTL 后自动过期
```

### 4.5 Redis 缓存策略

**缓存模式：Cache-Aside（旁路缓存）**

```
读取:
  1. 查 Redis → 命中 → 直接返回
  2. 查 Redis → 未命中 → 查 MySQL → 写入 Redis → 返回

写入:
  1. 写 MySQL
  2. 删除 Redis 缓存（而非更新）
  3. 依赖 TTL 作为兜底（防止删除失败导致脏数据）

为什么是删除缓存而不是更新缓存:
  - 删除更简单，不需要考虑并发更新的时序问题
  - 下次读取时自然会从 DB 加载最新数据
  - TTL 兜底保证最终一致性
```

**各服务缓存详情**：

user-service:
| 操作 | 缓存 Key | 策略 |
|------|---------|------|
| 注册 | `user:info:{id}` | 写入缓存（TTL 15min） |
| 登录 | `user:info:{id}` + `online:user:{id}` | 刷新用户缓存 + 设置在线标记（TTL 30min） |
| 获取用户信息 | `user:info:{id}` | 先查缓存，未命中查 DB 回写 |
| 更新用户信息 | `user:info:{id}` | 更新 DB + 刷新缓存 |
| 获取好友列表 | `user:friends:{id}` | 先查缓存（TTL 5min），未命中查 DB 回写 |
| 添加/删除好友 | `user:friends:{uid}` | 双向删除双方好友缓存 |
| 获取好友分组 | `user:friend_groups:{id}` | 先查缓存（TTL 10min），未命中查 DB 回写 |
| 创建好友分组 | `user:friend_groups:{id}` | 删除缓存 |

msg-core-service:
| 操作 | 缓存 Key | 策略 |
|------|---------|------|
| 获取会话列表 | `user:conversations:{id}` | 先查缓存（TTL 5min），未命中查 DB 回写 |
| 发送消息 | `conversation:recent:{id}` + `user:conversations:{uid}` | 缓存最近消息 + 清除所有参与者的会话列表缓存 |
| 创建会话 | `user:conversations:{uid}` | 清除所有参与者的会话列表缓存 |

### 4.6 数据库设计原则

**每个服务独立管理自己的表**：
- user-service 管理 users、friends、friend_groups 表
- group-service 管理 groups、group_members 表
- msg-core-service 管理 conversations、conversation_participants、messages 表
- msg-history-service 管理 message_history、offline_messages 表

**启动时自动迁移**：
```go
func InitDB(dsn string) (*gorm.DB, error) {
    db, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{})
    
    for _, m := range models {
        if db.Migrator().HasTable(m) {
            db.Migrator().DropTable(m)  // 开发阶段：删表重建
        }
        db.AutoMigrate(m)               // 根据模型自动建表
    }
    return db, nil
}
```

**为什么开发阶段用 DropTable + AutoMigrate**：
- 开发阶段模型频繁变更，直接 AutoMigrate 无法处理字段删除/类型变更
- 生产环境应使用迁移工具（如 golang-migrate）做增量迁移

**好友关系是双向的**：
```
A 添加 B 为好友:
  friends 表插入两条记录:
    {user_id: A, friend_id: B, remark: "我的同事"}
    {user_id: B, friend_id: A, remark: ""}

A 删除 B:
  删除两条记录:
    WHERE user_id=A AND friend_id=B
    WHERE user_id=B AND friend_id=A
```

**私聊会话去重**：
```sql
SELECT * FROM conversations
WHERE type = 'private'
AND id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ?)
AND id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ?)
```
- 两个子查询取交集，找到同时包含两个用户的私聊会话
- 如果已存在则直接返回，不创建新的

### 4.7 配置管理机制

**三层配置加载**：
```
1. YAML 配置文件 (config/*.yaml)
   └── 定义所有配置项的结构和默认值

2. .env 环境变量文件
   └── 存放敏感信息（DSN、密码、密钥），不提交到 Git

3. 系统环境变量
   └── 最高优先级，用于生产环境覆盖
```

**加载流程**：
```go
func Load(configPath string) (*Config, error) {
    godotenv.Load()                    // 1. 加载 .env
    viper.AutomaticEnv()               // 2. 允许环境变量覆盖
    viper.SetConfigFile(configPath)
    viper.ReadInConfig()               // 3. 读取 YAML
    viper.Unmarshal(&cfg)              // 4. 反序列化
    applyEnvOverrides(&cfg)            // 5. 环境变量覆盖敏感字段
    return &cfg, nil
}
```

**环境变量覆盖优先级**：
```
系统环境变量 > .env 文件 > YAML 配置文件
```

---

## 五、API 网关设计

### 5.1 路由分组

```go
// 公开接口（无需认证）
public := r.Group("/api/v1")
public.POST("/user/register", ...)
public.POST("/user/login", ...)

// 认证接口（需要 JWT）
auth := r.Group("/api/v1")
auth.Use(middleware.JWTAuthMiddleware())
auth.GET("/user/info", ...)
auth.POST("/message/send", ...)
```

### 5.2 CORS 中间件

```go
func CORSMiddleware() app.HandlerFunc {
    // 设置允许的 Origin、Methods、Headers
    // OPTIONS 请求直接返回 204（预检请求）
    // 其他请求放行
}
```

为什么需要 CORS：
- 前端运行在 `http://localhost:5500`（Live Server）
- API 网关运行在 `http://localhost:8080`
- 浏览器同源策略会阻止跨域请求
- CORS 中间件告诉浏览器"允许跨域"

### 5.3 统一响应格式

```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data"`
}
```

- `code=0`：成功
- `code=-1`：服务器内部错误
- `code=400`：参数错误
- `code=401`：未认证
- `code=403`：无权限

---

## 六、前端实现原理

### 6.1 技术选型

- 纯 HTML/CSS/JavaScript（无框架依赖）
- 目的：零构建步骤，直接浏览器打开即可测试

### 6.2 页面结构

```
index.html
  ├── 登录/注册页面（默认显示）
  └── 聊天主页面（登录后显示）
       ├── 左侧边栏
       │    ├── 会话列表
       │    ├── 好友列表
       │    └── 群组列表
       └── 右侧聊天区域
            ├── 消息列表
            └── 输入框
```

### 6.3 WebSocket 集成

```javascript
// 登录成功后建立 WebSocket
const ws = new WebSocket(`ws://localhost:8081/ws?token=${token}`);

// 接收消息
ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    if (msg.type === 'new_message') {
        // 判断是否是当前会话的消息
        if (msg.data.conversation_id === currentConversationId) {
            renderMessage(msg.data);  // 直接渲染
        } else {
            showNotification(msg.data);  // 显示通知
        }
    }
};

// 断线重连
ws.onclose = () => {
    setTimeout(() => connectWebSocket(token), 3000);
};
```

### 6.4 消息去重

```javascript
const sentMessageIds = new Set();

// 发送消息时记录 ID
function onSendSuccess(msgId) {
    sentMessageIds.add(msgId);
}

// 收到 WebSocket 推送时检查
ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    if (sentMessageIds.has(msg.data.msg_id)) {
        sentMessageIds.delete(msg.data.msg_id);
        return;  // 跳过自己发送的消息（已在发送时渲染）
    }
    renderMessage(msg.data);
};
```

为什么需要去重：
- 发送消息时前端会立即渲染（乐观更新）
- 同时 WebSocket 也会推送这条消息回来
- 不去重会导致消息显示两次

---

## 七、Docker 基础设施

### 7.1 docker-compose.yaml

```yaml
services:
  mysql:        # 数据库 - 端口 3306
  redis:        # 缓存 - 端口 6379
  etcd:         # 服务注册发现 - 端口 2379
```

### 7.2 服务连接方式

所有服务通过 `localhost:{port}` 连接 Docker 容器：
- MySQL: `claran:chr070309@tcp(localhost:3306)/ClaranAIM?charset=utf8mb4&parseTime=True&loc=Local`
- Redis: `localhost:6379`，无密码，DB 0
- Etcd: `http://localhost:2379`

---

## 八、启动流程

### 8.1 完整启动顺序

```
1. docker-compose up -d          → 启动 MySQL、Redis、Etcd
2. 等待 MySQL 初始化完成（约 10 秒）
3. scripts/start.bat             → 依次启动 5 个后端服务
   ├── user-service (:9001)      → 自动建表 + 注册到 Etcd
   ├── group-service (:9002)     → 自动建表 + 注册到 Etcd
   ├── msg-core-service (:9003)  → 自动建表 + 注册到 Etcd
   ├── msg-history-service (:9004) → 自动建表 + 注册到 Etcd
   ├── api-gateway (:8080)       → 初始化 RPC 客户端（从 Etcd 发现服务）
   └── websocket-gateway (:8081) → 启动 Hub 事件循环
4. 浏览器打开 dist/index.html    → 前端页面
```

### 8.2 为什么有启动顺序要求

- api-gateway 依赖 Etcd 中的服务注册信息
- 如果 user-service 还没注册到 Etcd，api-gateway 的 RPC 调用会失败
- start.bat 中通过 `timeout /t 3` 在每个服务之间等待 3 秒

---

## 九、安全设计

| 安全措施 | 实现位置 | 说明 |
|---------|---------|------|
| 密码加密 | pkg/password | bcrypt，cost=10，自带盐值 |
| JWT 签名 | pkg/jwt | HMAC-SHA256，密钥从环境变量读取 |
| 敏感信息隔离 | .env + Viper | DSN/密钥不硬编码在代码中 |
| JWT 中间件 | api-gateway/middleware | 所有业务接口强制认证 |
| WebSocket 认证 | websocket-gateway/handler | 连接时验证 Token |
| CORS 限制 | api-gateway/middleware | 开发阶段允许所有来源 |
| SQL 注入防护 | GORM | 参数化查询，不拼接 SQL |

---

## 十、性能设计考量

| 设计点 | 当前实现 | 未来优化方向 |
|-------|---------|------------|
| 消息存储 | 同步写 MySQL | 异步写（先写 Redis，再批量落盘） |
| 消息推送 | HTTP 调用 /push | Redis Pub/Sub 替代 HTTP |
| 会话列表 | 每次查 DB 拼装 | Redis Sorted Set 维护 |
| 在线状态 | Redis String + TTL | Redis Set + 定时刷新 |
| 历史消息分页 | 游标分页（before_id） | 已实现，无需优化 |
| 批量用户查询 | 逐个查缓存 | Pipeline 批量查询 Redis |
