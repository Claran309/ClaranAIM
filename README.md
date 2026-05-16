# ClaranAIM

ClaranAIM 是一个面向多人在线场景的 AIM 系统：AIM = Agent + Instant Messaging。它不是简单地在聊天室里加一个 AI 按钮，而是把 Agent 智能体嵌入真实 IM 会话、群聊协作、文件流转、语音消息、知识库和工具调用中，让 IM 从“消息容器”升级为“人和 Agent 共处的协作现场”。

## Agent-Native AIM

ClaranAIM 的 A 不应该只被理解为 AI ChatBot，而应该进一步被定义为 Agent。AIM 的核心命题是：在 IM 这个天然承载工作、生活、社交和组织协作的场域中，嵌入能理解上下文、调用工具、沉淀知识、辅助决策并主动执行任务的智能体。换句话说，IM 提供真实语境、关系网络和事件流，Agent 在其中获得“工作现场”。

传统 AI 聊天通常发生在孤立对话框里，用户需要手动复制上下文、描述背景、整理材料，再把结果搬回实际工作流。AIM 的方向相反：让 Agent 原生存在于会话、群聊、文件、语音、任务和知识库之间。Agent 不只是回答问题，而是在 IM 中观察、理解、总结、提醒、协作、执行。

## 项目结构

```text
ClaranAIM/
├── cmd/                           # 各服务入口
│   ├── api-gateway/               # Hertz HTTP 网关，默认 :8080
│   ├── websocket-gateway/         # WebSocket 网关，默认 :8081
│   ├── user-service/              # 用户服务
│   ├── group-service/             # 群组服务
│   ├── msg-core-service/          # 消息核心服务
│   ├── msg-history-service/       # 消息历史服务
│   ├── file-service/              # 文件服务
│   └── bot-manager-service/       # Bot 管理服务
├── internal/
│   ├── api-gateway/               # HTTP handler、RPC client、中间件、路由
│   ├── websocket-gateway/         # WebSocket Hub、连接处理、客户端读写循环
│   ├── user-service/              # 用户、好友、好友分组
│   ├── group-service/             # 群、群成员、角色、禁言、公告
│   ├── msg-core-service/          # 会话、消息、参与者、WebSocket 推送
│   ├── msg-history-service/       # 历史消息查询
│   ├── file-service/              # 文件元数据、MinIO/本地存储
│   └── bot-manager-service/       # Bot 配置、Agent、工具、记忆、计费
├── pkg/                           # 公共包：配置、JWT、Redis、日志、响应等
├── kitex_gen/                     # Kitex/Thrift 生成代码
├── idl/                           # Thrift IDL 定义
├── config/                        # 各服务配置
├── dist/                          # 前端静态文件
├── docs/                          # 项目文档
├── scripts/                       # 启动与代码生成脚本
├── docker-compose.yaml            # MySQL、Redis、Etcd、MinIO
└── README.md
```

## 快速启动

```bash
# 1. 配置环境变量
cp .env.example .env

# 2. 启动基础设施
docker-compose up -d

# 3. 启动所有服务
# Windows:
scripts\start.bat

# 4. 打开前端
# 浏览器打开 dist/index.html
```

## 注意事项

- 缓存逻辑主要集成在 service 层，而不是 DAO 层。
- 文件二进制存储在本地目录或 MinIO，消息表只保存媒体引用。
- 群成员关系由 group-service 管理，消息推送目标由 msg-core-service 的 conversation_participants 管理。
- Agent 能力未来应拆分为管理面 bot-manager-service 和执行面 bot-runtime-service。

## to fix list

- 暂无

## future to fix

- 把bot更改作为用户实例，创建后可以正常加好友（有特殊id）和邀请群聊。可以在私聊/群聊里对话，记忆机制等就像现在这样就行，也就是说不同机器人的记忆独立，对不同用户的记忆独立，但是同一机器人在不同会话中对同一用户的记忆保持
