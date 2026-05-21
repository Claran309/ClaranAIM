# ClaranAIM

ClaranAIM 是一个面向多人在线场景的 AIM 系统：AIM = Agent + Instant Messaging。它不是简单地在聊天室里加一个 AI 按钮，而是把 Agent 智能体嵌入真实 IM 会话、群聊协作、文件流转、语音消息、知识库和工具调用中，让 IM 从“消息容器”升级为“人和 Agent 共处的协作现场”。

## Agent-Native AIM

ClaranAIM 的 A 不应该只被理解为 AI ChatBot，而应该进一步被定义为 Agent。AIM 的核心命题是：在 IM 这个天然承载工作、生活、社交和组织协作的场域中，嵌入能理解上下文、调用工具、沉淀知识、辅助决策并主动执行任务的智能体。换句话说，IM 提供真实语境、关系网络和事件流，Agent 在其中获得“工作现场”。

传统 AI 聊天通常发生在孤立对话框里，用户需要手动复制上下文、描述背景、整理材料，再把结果搬回实际工作流。AIM 的方向相反：让 Agent 原生存在于会话、群聊、文件、语音、任务和知识库之间。Agent 不只是回答问题，而是在 IM 中观察、理解、总结、提醒、协作、执行。

## 项目结构

```text
ClaranAIM/
├── cmd/                                   # 各服务启动入口
│   ├── api-gateway/                       # Hertz HTTP 网关，默认 :8080
│   ├── websocket-gateway/                 # WebSocket 网关，默认 :8081
│   ├── user-service/                      # 用户、登录、好友 RPC 服务
│   ├── group-service/                     # 群组、群成员、公告、禁言 RPC 服务
│   ├── msg-core-service/                  # 会话、消息写入、推送、Outbox RPC 服务
│   ├── msg-history-service/               # 历史消息查询 RPC 服务
│   ├── file-service/                      # 文件元数据 RPC 服务
│   ├── bot-manager-service/               # Agent 配置、权限、计费、调度管理服务
│   ├── bot-runtime-service/               # Agent 运行时、长会话、工具调用服务
│   ├── memory-service/                    # 记忆服务预留入口
│   ├── rag-service/                       # RAG 服务预留入口
│   └── msg-filter-service/                # 消息审核/过滤服务预留入口
├── internal/                              # 各服务内部实现，外部包不应直接依赖
│   ├── api-gateway/                       # HTTP handler、RPC client、中间件、路由
│   ├── websocket-gateway/                 # WebSocket Hub、连接处理、客户端读写循环
│   ├── user-service/                      # 用户模型、DAO、好友和个人资料业务
│   ├── group-service/                     # 群模型、DAO、群权限、DTM 分支接口
│   ├── msg-core-service/                  # 会话、消息、参与者、Outbox、DTM 分支接口
│   ├── msg-history-service/               # 历史消息与离线消息查询
│   ├── file-service/                      # 文件元数据、MinIO/本地存储适配
│   ├── bot-manager-service/               # Agent 配置、权限、Kafka 消费、计费、路由
│   │   ├── dao/                           # Bot、权限、路由、计费、调度记录持久化
│   │   ├── eventconsumer/                 # Kafka message.created 的 @Agent 分发消费
│   │   ├── handler/                       # Kitex RPC handler
│   │   ├── model/                         # Bot、权限、路由、计费模型
│   │   └── service/                       # Agent 管理、权限校验、runtime 调用
│   ├── bot-runtime-service/               # Agent 执行侧实现
│   │   ├── agent/                         # Eino Agent、工具、安全策略
│   │   ├── component/                     # JSONL 长会话存储等运行时组件
│   │   ├── graphTool/                     # 图/工具调用相关封装
│   │   ├── handler/                       # bot-runtime Kitex RPC handler
│   │   ├── logic/                         # 运行时辅助逻辑
│   │   └── service/                       # RunAgent、总结、问答、insights、sessions
│   ├── memory-service/                    # 记忆服务预留实现
│   ├── rag-service/                       # RAG 服务预留实现
│   ├── msg-filter-service/                # 消息过滤/审核预留实现
│   └── msg-service/                       # 消息服务旧/兼容目录
├── pkg/                                   # 跨服务公共包
│   ├── cache/redis/                       # Redis 客户端与缓存辅助
│   ├── config/                            # Viper 配置加载、环境变量覆盖
│   ├── dtm/                               # DTM Saga/TCC 辅助封装
│   ├── eventbus/                          # Kafka 生产/消费封装
│   ├── events/                            # Kafka topic 与事件结构
│   ├── governance/                        # Kitex 超时、熔断、限流配置
│   ├── health/                            # 服务启动健康检查日志
│   ├── idgen/                             # 雪花 ID 与 10 位用户 UID 生成
│   ├── jwt/                               # access token / refresh token
│   ├── logger/                            # Zap 日志，本地 INFO/ERR 文件输出
│   ├── outbox/                            # 事务 Outbox 事实表与事件发布
│   ├── password/                          # bcrypt 密码哈希
│   └── response/                          # HTTP 统一响应结构
├── idl/                                   # Thrift IDL 定义
│   ├── user.thrift
│   ├── group.thrift
│   ├── message.thrift
│   ├── file.thrift
│   ├── bot.thrift
│   └── bot_runtime.thrift
├── kitex_gen/                             # Kitex 根据 IDL 生成的 Go 代码
│   ├── user/
│   ├── group/
│   ├── message/
│   ├── file/
│   ├── bot/
│   └── bot_runtime/
├── config/                                # 各服务 YAML 配置
│   ├── config.yaml                        # 默认/示例公共配置
│   ├── api-gateway.yaml
│   ├── websocket-gateway.yaml
│   ├── user-service.yaml
│   ├── group-service.yaml
│   ├── msg-core-service.yaml
│   ├── msg-history-service.yaml
│   ├── file-service.yaml
│   ├── bot-manager-service.yaml
│   └── bot-runtime-service.yaml
├── dist/                                  # 前端静态页面
│   ├── index.html
│   ├── css/
│   └── js/
├── docs/                                  # 项目文档
│   ├── APIdoc.md                          # HTTP/RPC/API 文档
│   ├── TechArch.md                        # 技术架构说明
│   ├── ReliabilityAndEventConsistency.md  # Kafka/Outbox/DTM 一致性说明
│   ├── ChatHistoryWithAI.md               # AI 会话历史设计记录
│   └── plan.md                            # 阶段计划
├── scripts/                               # 本地启动脚本
│   └── start.bat
├── storage/                               # 本地运行时数据，不应提交业务内容
│   ├── bot/                               # Agent 相关本地数据
│   └── source/                            # 本地文件存储目录
├── logs/                                  # 本地日志目录，ERR 按日期集中归档
├── docker-compose.yaml                    # MySQL、Redis、Etcd、MinIO、Kafka、DTM
├── .env.example                           # 环境变量示例
├── go.mod / go.sum                        # Go module 依赖
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

## to fix list

- 继续扫描其他 Agent 边界：运行中断、工具审批、长任务排队、调用失败后的用户可见状态。
- Agent 真正流式输出需要 runtime/API 网关增加 streaming/SSE/WebSocket 事件通道。目前前端已补“真实思考耗时 + 执行中状态”，但不是 token 级流式输出。


## future to fix

- 管理层
- 文本翻译功能：需要查询相应api或用llm
- 多端消息同步怎么实现？
- 离线推送与上线同步怎么实现？
- 消息本地存储与云端漫游是否实现？怎么实现？
- 审核机制：针对图片视频等媒体，将这些文件放入管理层的特定页面，由管理人员负责统一验收等

- 把bot更改作为用户实例，创建后可以正常加好友（有特殊id）和邀请群聊。可以在私聊/群聊里对话，记忆机制等就像现在这样就行，也就是说不同机器人的记忆独立，对不同用户的记忆独立，但是同一机器人在不同会话中对同一用户的记忆保持
- 添加表情包功能，可以把图片快速保存为表情包并快速发送
- 搜索历史消息支持按时间搜索

- 汪神的知识图谱
