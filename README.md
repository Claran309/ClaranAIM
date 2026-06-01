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
│   ├── agent-manager-service/             # Agent 配置、权限、计费、调度管理服务
│   ├── agent-runtime-service/             # Agent 运行时、长会话、工具调用服务
│   ├── memory-service/                    # 记忆服务启动入口，负责 memory_facts 迁移/健康检查
│   ├── settings-service/                  # 用户系统设置、LLM 预设、Prompt 模板服务
│   ├── rag-service/                       # RAG 入库、检索、GraphRAG indexing 服务
│   ├── knowledge-service/                 # 知识图谱查询、过滤、详情和可视化 RPC 服务
│   └── msg-filter-service/                # 消息审核/过滤服务预留入口
├── internal/                              # 各服务内部实现，外部包不应直接依赖
│   ├── api-gateway/                       # HTTP handler、RPC client、中间件、路由
│   ├── websocket-gateway/                 # WebSocket Hub、连接处理、客户端读写循环
│   ├── user-service/                      # 用户模型、DAO、好友和个人资料业务
│   ├── group-service/                     # 群模型、DAO、群权限、DTM 分支接口
│   ├── msg-core-service/                  # 会话、消息、参与者、Outbox、DTM 分支接口
│   ├── msg-history-service/               # 历史消息与离线消息查询
│   ├── file-service/                      # 文件元数据、MinIO/本地存储适配
│   ├── agent-manager-service/             # Agent 配置、权限、Kafka 消费、计费、路由
│   │   ├── dao/                           # Agent、权限、路由、计费、订阅、审计、调度记录持久化
│   │   ├── eventconsumer/                 # Agent Event Dispatcher，消费 message/im 事件并决策响应
│   │   ├── handler/                       # Kitex RPC handler
│   │   ├── model/                         # Agent、权限、订阅规则、审计、路由、计费模型
│   │   └── service/                       # Agent 管理、权限校验、runtime 调用
│   ├── agent-runtime-service/             # Agent 执行侧实现
│   │   ├── agent/                         # Eino Agent、工具、安全策略
│   │   ├── component/                     # JSONL 长会话存储等运行时组件
│   │   ├── graphTool/                     # 图/工具调用相关封装
│   │   ├── handler/                       # bot-runtime Kitex RPC handler
│   │   ├── logic/                         # 运行时辅助逻辑
│   │   └── service/                       # RunAgent、总结、问答、insights、sessions
│   ├── memory-service/                    # Agent 记忆模型、DAO、召回隔离和用户治理逻辑
│   ├── settings-service/                  # LLM 配置预设、翻译 Prompt 等用户可控设置
│   ├── rag-service/                       # 文档解析、chunk、embedding、Hybrid RAG、GraphRAG 数据构建
│   ├── knowledge-service/                 # 知识图谱查询、过滤、详情聚合和可视化视图模型
│   ├── msg-filter-service/                # 消息过滤/审核预留实现
├── pkg/                                   # 跨服务公共包
│   ├── cache/redis/                       # Redis 客户端与缓存辅助
│   ├── config/                            # Viper 配置加载、环境变量覆盖
│   ├── dtm/                               # DTM Saga/TCC 辅助封装
│   ├── eventbus/                          # Kafka 生产/消费封装
│   ├── events/                            # Kafka topic、统一 IM 事件、Agent 事件结构
│   ├── governance/                        # Kitex 超时、熔断、限流配置
│   ├── health/                            # 服务启动健康检查日志
│   ├── idgen/                             # 雪花 ID、10 位用户 UID 与 10 位群号生成
│   ├── jwt/                               # access token / refresh token
│   ├── knowledgeclient/                   # knowledge-service RPC 客户端、稳定契约和视图模型
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
│   ├── bot_runtime.thrift
│   ├── knowledge.thrift                   # 知识图谱可视化查询 RPC 契约
│   ├── rag.thrift
│   └── settings.thrift
├── kitex_gen/                             # Kitex 根据 IDL 生成的 Go 代码
│   ├── user/
│   ├── group/
│   ├── message/
│   ├── file/
│   ├── bot/                               # 历史 Kitex 生成包名，业务语义已迁移为 Agent
│   ├── bot_runtime/
│   ├── knowledge/
│   └── rag/
├── config/                                # 各服务 YAML 配置
│   ├── config.yaml                        # 默认/示例公共配置
│   ├── api-gateway.yaml
│   ├── memory-service.yaml
│   ├── settings-service.yaml
│   ├── websocket-gateway.yaml
│   ├── user-service.yaml
│   ├── group-service.yaml
│   ├── msg-core-service.yaml
│   ├── msg-history-service.yaml
│   ├── file-service.yaml
│   ├── agent-manager-service.yaml
│   ├── agent-runtime-service.yaml
│   ├── rag-service.yaml
│   └── knowledge-service.yaml
├── dist/                                  # 前端静态页面
│   ├── index.html
│   ├── css/
│   └── js/
├── docs/                                  # 项目文档
│   ├── APIdoc.md                          # HTTP/RPC/API 文档
│   ├── TechArch.md                        # 技术架构说明
│   ├── consideration.md                    # Agent-Native IM 与知识系统复盘
│   ├── ReliabilityAndEventConsistency.md  # Kafka/Outbox/DTM 一致性说明
│   ├── ChatHistoryWithAI.md               # AI 会话历史设计记录
│   └── plan.md                            # 阶段计划
├── scripts/                               # 本地启动脚本
│   └── start.bat
├── storage/                               # 本地运行时数据，不应提交业务内容
│   ├── agent/                             # Agent 运行时文件、长会话和工作目录
│   └── source/                            # 本地文件存储目录
├── logs/                                  # 本地日志目录，ERR 按日期集中归档
├── docker-compose.yaml                    # MySQL、Redis、Etcd、MinIO、Kafka、DTM
├── .env.example                           # 环境变量示例
├── go.mod / go.sum                        # Go module 依赖
└── README.md
```

Agent 触发规则说明：前端“Agent 触发规则”使用 `/agent/route/*` 接口；`agent_keyword` 表示关键词触发回复，`agent_command` 表示命令前缀触发回复，`agent_record` 表示静默记录事件不回复。agent-manager-service 会把这些规则同步到 `agent_subscription_rules`，供 Agent Event Dispatcher 消费。

命名说明：当前业务、服务目录和前端入口统一面向用户展示为 Agent，HTTP 入口只保留 `/agent/*`。由于当前环境没有 Kitex 生成工具，内部生成代码仍依赖历史 `bot.thrift`/`kitex_gen/bot` 方法名；这只是 RPC 生成代码实现细节，不再作为对外 `/bot/*` 兼容接口暴露。

系统设置说明：`settings-service` 负责保存用户可复用的 LLM 预设和 Prompt 模板。创建 Agent 时可以直接选择已保存的 LLM 预设，网关会解析为 BaseURL、模型和 API Key 后写入 Agent 配置。消息翻译为手动触发能力，落在 msg-core-service，不做自动翻译。

Agent 运行配置说明：创建或编辑 Agent 时可以配置“会话上下文条数”“记忆召回条数”“最大输出 Token”“创造性”“群聊触发方式”和“自动回复开关”。这些配置会写入 agent-manager-service 的 Agent 配置表，并随 RPC 传给 agent-runtime-service；网关执行总结、问答、洞察等会话感知任务时，会优先按该 Agent 的 `context_message_limit` 从 msg-core-service 读取历史消息。

知识图谱服务说明：`rag-service` 负责文档解析、父子 chunk、embedding、Hybrid Search、GraphRAG 实体/关系/社区数据构建；`knowledge-service` 负责面向前端的图谱查询、筛选、节点详情、关系详情和可视化属性计算。当前浏览器访问 `/api/v1/knowledge/*`，api-gateway 通过 `pkg/knowledgeclient.RPCClient` 调用独立 Kitex `knowledge-service`；knowledge-service 再通过 RPC 调用 rag-service 读取当前用户可见的 GraphRAG 子图。

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

## Adaptive RAG
现在支持的路线：

- direct：简单问题，不检索。
- project_rag：普通项目/知识库问题，走项目文档和代码 chunk。
- strict_rag：复杂/高风险问题，走 Hybrid + Rerank + CRAG + Self-RAG。
- web_rag：实时/最新问题。当前返回 Web RAG 路由提示，真正 Web Search 由 agent-runtime 工具链执行。
- memory_rag：私有记忆问题。当前 memory-service 还没接 RAG/Milvus，所以返回 memory 路由提示。
- tool_action：执行动作，不走 RAG，交给 Agent 工具审批/执行链路。 

LLM Router 输出类似：

```json
{
  "route": "project_rag",
  "complexity": "medium",
  "need_retrieve": true,
  "sources": ["project_docs", "code_chunks"],
  "strategy": "hybrid_rerank",
  "mode": "hybrid",
  "retrieval_source": "project_docs",
  "query": "agent_dispatch_records event_id agent_user_id",
  "reason": "问题依赖当前项目内部实现"
}
```

## Knowledge Graph 可视化

知识图谱页面面向“看懂项目和知识关系”设计，不替代 RAG 检索。它复用 GraphRAG 的实体、关系、社区摘要和证据来源，提供一个可拖拽、可缩放、可筛选的关系画布。

- 查询入口：`/api/v1/knowledge/graph`
- 节点详情：`/api/v1/knowledge/node/:id`
- 关系详情：`/api/v1/knowledge/edge/:id`
- 前端能力：搜索节点、按实体类型过滤、按关系类型过滤、按社区过滤、显示一跳/二跳关系、点击节点/边查看说明与证据、重置视图、适配 G6 不可用时的 SVG 降级。
- 数据边界：GraphRAG indexing、实体抽取、关系抽取和社区摘要仍属于 rag-service；knowledge-service 只做查询视图与可视化 DTO，避免把索引和展示耦合在一起。

## to fix list
<<<<<<< HEAD
=======

>>>>>>> c5187f3a799b7b602917cddd5c919c11998af655
