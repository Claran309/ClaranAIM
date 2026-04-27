## 开发优先级

### 第一阶段（2-3 周）：核心聊天功能

- 架构设计
- User Service（注册、登录、JWT）
- Chat Service（私聊、群聊、消息存储）
- WebSocket 网关（实时消息推送）

### 第二阶段（1-2 周）：Bot 集成

- Bot Service（接入你的 AmiyaAgent）
- Bot 会话管理

### 第三阶段（1-2 周）：社交功能

- Social Service（动态、点赞、评论）
- Notification Service（通知推送）

### 第四阶段：高级功能

- A2UI
- RAG 文档上传和检索
- MCP 工具集成
- 多 Bot 支持
- K8s 部署

## Bot改造方案（分阶段）

### 第一阶段：最小化 Bot 服务（1 周）
把你的 Agent 包装成 HTTP/WebSocket 服务：

```
aim-bot-service/
├── cmd/
│   └── main.go              # 服务入口
├── internal/
│   ├── agent/               # 复用你的 AmiyaAgent 代码
│   │   ├── agent.go
│   │   ├── tools.go
│   │   └── prompt.go
│   ├── service/
│   │   ├── bot_service.go   # Bot 业务逻辑
│   │   └── session_service.go
│   ├── handler/
│   │   ├── websocket.go     # WebSocket 处理
│   │   └── http.go          # HTTP 路由
│   └── storage/
│       └── session.go       # 数据库存储
├── docker-compose.yml
└── Dockerfile
```

#### WebSocket 处理器（替代 CLI 的 bufio.Reader）
```go 
// 伪代码
func (h *Handler) HandleMessage(ctx context.Context, userID, sessionID, content string) {
    // 调用你现有的 Agent
    messages := session.GetMessages()
    messages = append(messages, schema.UserMessage(content))
    
    events := runner.Run(ctx, messages)
    content, interruptInfo, err := getAssistantMsg(events)
    
    // 通过 WebSocket 流式发送
    h.sendToClient(userID, content)
}
```

#### 会话存储改成 PostgreSQL
```go
// 替代现在的 JSONL 文件存储
type SessionStore interface {
    GetSession(ctx context.Context, sessionID string) (*Session, error)
    SaveMessage(ctx context.Context, sessionID string, msg *schema.Message) error
    GetMessages(ctx context.Context, sessionID string) ([]*schema.Message, error)
}
```

#### Docker 编排
```yaml
version: '3'
services:
  bot-service:
    build: .
    ports:
      - "8080:8080"
    environment:
      - OPENAI_API_KEY=xxx
      - DATABASE_URL=postgres://...
    depends_on:
      - postgres
      - redis
  
  postgres:
    image: postgres:15
    environment:
      - POSTGRES_PASSWORD=xxx
  
  redis:
    image: redis:7
```

### 第二阶段：多 Bot 支持（1 周）
现在你只有阿米娅一个 Bot，扩展成多 Bot 框架：

```go
type BotConfig struct {
    ID       string
    Name     string
    Prompt   string
    Tools    []tool.BaseTool
    Model    model.BaseChatModel
}
type BotManager struct {
    bots map[string]*BotConfig
}
// 用户可以选择不同的 Bot
func (m *BotManager) GetBot(botID string) *BotConfig {
    return m.bots[botID]
}
```

### 第三阶段：RAG 知识库（1-2 周）
你已经有 RAG 工具了，只需要：

添加文档上传接口
用向量数据库（Milvus/Weaviate）存储 embeddings
改进 RAG 工具的检索逻辑

#### 把RAG改为由向量数据库支撑

### 第四阶段：MCP 工具集成（1 周）
MCP 其实就是标准化的工具调用协议。你现在的 Tool 系统已经很接近了，只需要：

定义 MCP 协议的 Tool Schema
实现 MCP 客户端调用外部工具

## 立即可做的事
建议你先做这个：

复制你的 AmiyaAgent 代码到新项目
用 gorilla/websocket 写一个简单的 WebSocket 服务器
把 CLI 的消息循环改成 WebSocket 事件处理
用 PostgreSQL 替换 JSONL 存储
Docker 打包
这样你就有了一个可以多用户使用的 Bot 服务，后续的 RAG、MCP、多 Bot 都是在这个基础上扩展。
