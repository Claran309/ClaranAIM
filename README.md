# ClaranAIM

面向多人在线的即时通讯系统，内置可自部署的 AI 助手，实现"通讯 + AI"深度融合。项目开发中，大佬勿喷

## 项目结构

```
ClaranAIM/
├── cmd/                          # 各服务入口
│   ├── api-gateway/main.go       #   API 网关 (Hertz HTTP, :8080)
│   ├── websocket-gateway/main.go #   WebSocket 网关 (:8081)
│   ├── user-service/main.go      #   用户服务 (Kitex RPC, :9001)
│   ├── group-service/main.go     #   群组服务 (Kitex RPC, :9002)
│   ├── msg-core-service/main.go  #   消息核心服务 (Kitex RPC, :9003)
│   ├── msg-history-service/main.go#  消息历史服务 (Kitex RPC, :9004)
│   ├── file-service/main.go      #   文件服务 (Kitex RPC, :9005)
│   └── bot-manager-service/main.go#  Bot管理服务 (Kitex RPC, :9006)
│
├── internal/                     # 各服务内部实现（分层架构）
│   ├── api-gateway/
│   │   ├── client/rpc_client.go  #     RPC 客户端初始化 (含 file/bot client)
│   │   ├── handler/              #     HTTP 请求处理器
│   │   │   ├── user_handler.go
│   │   │   ├── group_handler.go
│   │   │   ├── message_handler.go
│   │   │   ├── file_handler.go
│   │   │   └── bot_handler.go
│   │   ├── middleware/middleware.go # JWT认证 + CORS中间件
│   │   └── router/router.go      #     路由注册
│   │
│   ├── websocket-gateway/
│   │   ├── hub/hub.go            #     WebSocket 连接管理中心
│   │   ├── handler/ws_handler.go #     WebSocket 升级 + JWT 认证
│   │   └── client/ws_client.go   #     客户端读写泵 + 心跳
│   │
│   ├── user-service/
│   │   ├── dao/dao.go            #     数据库访问层 (GORM)
│   │   ├── handler/handler.go    #     RPC Handler (Thrift IDL)
│   │   ├── model/model.go        #     数据模型定义
│   │   └── service/service.go    #     业务逻辑层 (含 Redis 缓存)
│   │
│   ├── group-service/
│   │   ├── dao/dao.go
│   │   ├── handler/handler.go
│   │   ├── model/model.go        #     含 IsPinned/IsMuted 等扩展字段
│   │   └── service/service.go    #     含 Redis 缓存
│   │
│   ├── msg-core-service/
│   │   ├── dao/dao.go
│   │   ├── handler/handler.go
│   │   ├── model/model.go        #     含 GroupID 字段 (关联群聊禁言)
│   │   ├── push/push.go          #     WebSocket 推送客户端
│   │   └── service/service.go    #     消息业务逻辑 (含 Redis 缓存 + 禁言校验)
│   │
│   ├── msg-history-service/
│   │   ├── dao/dao.go
│   │   ├── handler/handler.go
│   │   ├── model/model.go
│   │   └── service/service.go
│   │
│   ├── file-service/
│   │   ├── dao/dao.go            #     文件元数据持久化
│   │   ├── handler/handler.go
│   │   ├── model/model.go
│   │   └── service/service.go    #     MinIO 对象存储集成
│   │
│   └── bot-manager-service/
│       ├── agent/agent.go        #     ADK Agent 封装 (工作流 + 工具调用)
│       ├── agent/prompt.go       #     系统 Prompt 模板
│       ├── agent/tools.go        #     Agent 工具注册
│       ├── component/model.go    #     OpenAI ChatModel 组件 (BaseURL 自动补全)
│       ├── component/memory.go   #     对话记忆组件
│       ├── component/middleware.go#    Agent 中间件
│       ├── graphTool/rag.go      #     RAG 图工具
│       ├── graphTool/websearch.go#     Web搜索 图工具
│       ├── logic/tools.go        #     业务工具定义
│       ├── dao/dao.go            #     Bot/路由/计费 数据访问
│       ├── handler/handler.go
│       ├── model/model.go        #     Bot/BotRoute/BillingRecord 模型
│       ├── service/service.go    #     内部Bot/自部署Bot 区分 + 计费记录
│       └── skills/               #     预置技能模板
│
├── pkg/                          # 公共包
│   ├── cache/redis/redis.go      #   Redis 客户端封装 (JSON支持)
│   ├── config/config.go          #   Viper 配置加载 (环境变量覆盖 + MinIO/LLM配置)
│   ├── health/health.go          #   服务健康检查 (MySQL/Redis/Etcd)
│   ├── jwt/jwt.go                #   JWT 工具 (生成/解析 Token)
│   ├── logger/logger.go          #   统一结构化日志 (服务名/时间戳/级别)
│   ├── password/password.go      #   bcrypt 密码加密
│   └── response/response.go      #   统一响应格式
│
├── kitex_gen/                    # Thrift IDL 自动生成代码
│   ├── user/                     #   user.thrift → userservice
│   ├── group/                    #   group.thrift → groupservice
│   ├── message/                  #   message.thrift → messageservice + historyservice
│   ├── file/                     #   file.thrift → fileservice
│   └── bot/                      #   bot.thrift → botservice
│
├── idl/                          # Thrift IDL 定义文件
│   ├── user.thrift
│   ├── group.thrift
│   ├── message.thrift
│   ├── file.thrift
│   └── bot.thrift
│
├── config/                       # 各服务 YAML 配置文件
│   ├── api-gateway.yaml
│   ├── websocket-gateway.yaml
│   ├── user-service.yaml
│   ├── group-service.yaml
│   ├── msg-core-service.yaml
│   ├── msg-history-service.yaml
│   ├── file-service.yaml
│   └── bot-manager-service.yaml
│
├── dist/                         # 前端静态文件
│   ├── index.html                #   单页应用入口
│   ├── css/style.css
│   ├── js/api.js                 #   API 调用 + WebSocket + 未读消息管理
│   └── js/app.js                 #   页面交互 (群管理/Bot对话/文件上传)
│
├── docs/                         # 项目文档
│   ├── APIdoc.md                 #   接口文档
│   ├── APIdoc-plan.md            #   开发规划
│   └── TechArch.md               #   技术实现原理
│
├── scripts/                      # 辅助脚本
│   ├── start.bat / start.sh      #   一键启动全部服务
│   └── kitex_gen.bat / kitex_gen.sh # IDL 代码生成
│
├── docker-compose.yaml           # Docker 编排 (MySQL + Redis + Etcd + MinIO)
├── .env / .env.example           # 敏感配置环境变量
├── go.mod / go.sum
└── README.md
```

## 快速启动

```bash
# 1. 配置环境变量
cp .env.example .env
# 编辑 .env 填入你的 MySQL DSN、Redis 密码、JWT Secret 等

# 2. 启动 Docker 基础设施
docker-compose up -d

# 3. 一键启动所有服务
# Windows:
scripts\start.bat
# Linux/Mac:
bash scripts/start.sh

# 4. 打开前端页面测试
# 浏览器打开 dist/index.html
```
## 注意事项

- 缓存集成到了service层而非dao层



## 待办事项

### fix
- 会话切换还是有问题，会话范围后端应该没问题，能够正常收发消息，问题是前端无法正常显示不同会话的消息，会把所有消息杂糅在一个会话里

- 群聊成员界面无法正常显示用户头像

- 用户点击进入群聊后，除了正常的群聊会话列表外，会错误地生成两个额外的无群聊名的会话列表

- 群聊列表前端有很大问题

- 转让群主后，新群主或管理员不能修改群聊信息

- 无法点击会话列表切换会话，但是bot对话部分没有这个问题，请按照bot部分写

- 由于会话部分有过多问题，导致前端无法正常显示会话列表，只能通过点击会话列表切换会话，但是会话列表会显示所有会话，包括群聊会话，导致用户无法正常切换会话

- 群公告不显示

- 无法禁言他人

- 私聊时会话列表应为对方用户头像

- 切换bot会话后，再次进入用户会话会导致显示的消息记录消失

- bot正常对话没有问题，但是我之前写bot时有写过bot想请求某些工具时会要求用户在命令行审批的功能（toolcall），请你做相应的前端接口，或者对该功能进行修改
> 详情：finish_reason: tool_calls
>
> usage: &{7473 {7466} 60 7533 {47}} map[] map[] [call_-7636677595769070032]}, LayerSpecificPayload=<nil>, SubsLen=1
>
> [trace] Graph/ReAct error: interrupt happened, info: &{State:0xc001a23c70 BeforeNodes:[] AfterNodes:[] RerunNodes:[ToolNode] RerunNodesExtra:map[ToolNode:0xc001a921e0] SubGraphs:map[] InterruptContexts:[]}

- bot对话部分，当bot在思考时切换会话，bot是思考完后会错误地在其他会话里输出内容

- 检查一下，现在的agent好像没有记忆功能，我之前写过session的记忆功能，请在不同agent中实现对应记忆的功能，注意记忆不要弄混了，一个botid对应一个sessionid的记忆

- 不同的bot应该对应不同的prompt或者skill等，如有必要请重构storage/bot中的文件，并完善不同bot的个性化功能

- 不应该有“创建新会话”按钮，会话只能由好友私聊或者创建群聊发起

- 文件上传失败: 文件已上传但元数据保存失败: remote or network error[remote]: panic: [happened in biz handler, method=FileService.UploadFile, please check the panic at the server side] runtime error: invalid memory address or nil pointer 
> 日志报错信息：
>
> [2026-05-13 23:33:12.837] [INFO] [api-gateway] 文件上传到MinIO成功 | object=file/bb052908-1440-4aa8-a98d-9fb633a99ca5.pdf | size=391644
>
> [2026-05-13 23:33:12.848] [ERROR] [api-gateway] 文件元数据RPC调用失败 | error=remote or network error[remote]: panic: [happened in biz handler, method=FileService.UploadFile, please check the panic at the server side] runtime error: invalid memory address or nil pointer dereference | filename=08 实验八 气缸装配体工程图.pdf

## 开发阶段

- [x] 项目规划
- [x] 架构设计
- [x] 底层设施框架选取
- [x] 表设计
- [x] api-gateway
- [x] websocket-gateway
user-service
  - [x] 用户登录与注册
  - 用户信息管理
    - [x] 信息
    - [x] 头像
    - [x] 在线状态
  - 好友管理
    - [x] 添加/删除
    - [x] 分组
    - [x] 备注
  - [x] 权限校验 (JWT + 中间件)
group-service
  - 群聊管理
    - [x] 创建/注销
    - [x] 转让群主
    - [x] 群公告
    - [x] 置顶
  - 群成员管理
    - [x] 成员校验
    - [x] 邀请/踢出
    - [x] 禁言
    - [x] 管理员
msg-core-service
  - [x] 消息发送（文本）
  - [x] 消息发送（图片、文件、语音）
  - [x] 消息落库
  - [x] WebSocket 实时推送
  - [ ] 已读回执
  - [ ] 消息引用与回复
  - [ ] 限时撤回与编辑
msg-history-service
  - [ ] 消息本地存储与云端漫游
  - [x] 历史查询
  - [ ] 离线推送与上线同步
  - [ ] 多端消息同步
msg-filter-service
  - [ ] 实时审核
  - [ ] 实时多语言翻译
file-service
  - [x] 保存多媒体消息（图片、文件、语音）
  - [x] 传输多媒体消息
bot-manager-service
  bot类型
    - [x] 内部bot
    - [x] 自部署bot
  - [x] 计费管理
  - [x] 配置管理
  - [x] 路由管理
bot-runtime-service
  - [ ] agent基础功能
  - [ ] 总结历史消息，生成要点摘要与待办提取（根据上下文生成回复候选，用户可一键选用）
  - [ ] MCP 工具集成
  - [ ] 多 Bot 协作
rag-service
  - [ ] 向量数据库
  知识库
    - [ ] 创建/删除
    - [ ] 启用与范围
    - [ ] 上传文档（pdf、md、doc、ppt）
    - [ ] 私有/公有
memory-service
  - [ ] 聊天记忆（单/群）
  - [ ] 跨会话用户个体记忆（用户偏好与历史交互）
  - [ ] 向量化记忆
服务治理
  - [x] kitex封装包 (TTHeader 协议 + Etcd 注册发现)
  - [ ] 降级
  - [ ] 重试
其他组件
  - [x] redis (缓存: 用户信息/好友列表/会话列表/在线状态)
  - [ ] kafka
  - [x] viper (配置管理 + 环境变量覆盖)
  - [ ] nacos
  - [x] minio (对象存储: 图片/文件/语音)
可观测性
  OTel
    - [ ] Prometheus
    - [ ] Jaeger
  - [x] 统一结构化日志 (pkg/logger)
  - [ ] ELK
  - [ ] CozeLoop
  - [ ] Grafana
压测
  - [ ] K6
部署
  - [x] docker-compose (MySQL + Redis + Etcd + MinIO)
  - [ ] K8s
  - [ ] 服务器
前端
  - [x] Vibe Coding (登录注册/聊天/好友/群组/实时消息/群管理/AI助手)
- [ ] 测试验收

- [x] Phase 1 (核心链路跑通):
  表设计 → api/websocket-gateway → user-service(登录注册) → msg-core-service(文本消息收发) → msg-history-service(基础存储查询)

- [x] Phase 2 (社交能力):
  好友管理 → 群组管理 → 在线状态 → 多媒体消息

- [ ] Phase 3 (AI 能力):
  bot-manager(配置/路由/计费/内部Bot/自部署Bot) → bot-runtime(基础对话) → memory-service → rag-service
  - [x] bot-manager 基础功能 (配置/路由/计费/内部Bot/自部署Bot)

- [ ] Phase 4 (进阶):
  MCP → 多Bot协作 → msg-filter → 多端同步 → 消息撤回/编辑

- [ ] Phase 5 (工程化):
  可观测性 → 压测 → 服务治理 → K8s部署
