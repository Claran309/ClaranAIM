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
│   └── msg-history-service/main.go#   消息历史服务 (Kitex RPC, :9004)
│
├── internal/                     # 各服务内部实现（分层架构）
│   ├── api-gateway/
│   │   ├── client/rpc_client.go  #     RPC 客户端初始化
│   │   ├── handler/              #     HTTP 请求处理器
│   │   │   ├── user_handler.go
│   │   │   ├── group_handler.go
│   │   │   └── message_handler.go
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
│   │   ├── model/model.go
│   │   └── service/service.go
│   │
│   ├── msg-core-service/
│   │   ├── dao/dao.go
│   │   ├── handler/handler.go
│   │   ├── model/model.go
│   │   ├── push/push.go          #     WebSocket 推送客户端
│   │   └── service/service.go    #     消息业务逻辑 (含 Redis 缓存)
│   │
│   └── msg-history-service/
│       ├── dao/dao.go
│       ├── handler/handler.go
│       ├── model/model.go
│       └── service/service.go
│
├── pkg/                          # 公共包
│   ├── cache/redis/redis.go      #   Redis 客户端封装 (JSON支持)
│   ├── config/config.go          #   Viper 配置加载 (环境变量覆盖)
│   ├── jwt/jwt.go                #   JWT 工具 (生成/解析 Token)
│   ├── password/password.go      #   bcrypt 密码加密
│   └── response/response.go      #   统一响应格式
│
├── kitex_gen/                    # Thrift IDL 自动生成代码
│   ├── user/                     #   user.thrift → userservice
│   ├── group/                    #   group.thrift → groupservice
│   └── message/                  #   message.thrift → messageservice + historyservice
│
├── idl/                          # Thrift IDL 定义文件
│   ├── user.thrift
│   ├── group.thrift
│   └── message.thrift
│
├── config/                       # 各服务 YAML 配置文件
│   ├── api-gateway.yaml
│   ├── websocket-gateway.yaml
│   ├── user-service.yaml
│   ├── group-service.yaml
│   ├── msg-core-service.yaml
│   └── msg-history-service.yaml
│
├── dist/                         # 前端静态文件
│   ├── index.html                #   单页应用入口
│   ├── css/style.css
│   ├── js/api.js                 #   API 调用 + WebSocket 连接
│   └── js/app.js                 #   页面交互逻辑
│
├── scripts/                      # 辅助脚本
│   ├── start.bat / start.sh      #   一键启动全部服务
│   └── kitex_gen.bat / kitex_gen.sh # IDL 代码生成
│
├── docker-compose.yaml           # Docker 编排 (MySQL + Redis + Etcd)
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

## To do list for tomorrow
- 当前并未完全实现用户信息管理系统或前端未适配相应接口，待调试

- 前端页面设计一般（如各种提示引导框在不起眼的位置），待修改

- 项目详细内容待审阅

- bot基础功能待审阅与调试

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
    - [ ] 信息
    - [ ] 头像
    - [ ] 在线状态
  - 好友管理
    - [x] 添加/删除
    - [x] 分组
    - [x] 备注
  - [x] 权限校验 (JWT + 中间件)
group-service
  - 群聊管理
    - [x] 创建/注销
    - [ ] 转让群主
    - [ ] 群公告
    - [ ] 置顶
  - 群成员管理
    - [x] 成员校验
    - [x] 邀请/踢出
    - [ ] 禁言
    - [ ] 管理员
msg-core-service
  - [x] 消息发送（文本）
  - [ ] 消息发送（图片、文件、语音）
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
  - [ ] 保存多媒体消息（图片、文件、语音）
  - [ ] 传输多媒体消息
bot-manager-service
  bot类型
    - [ ] 内部bot
    - [ ] 自部署bot
  - [ ] 计费管理
  - [ ] 配置管理
  - [ ] 路由管理
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
  - [ ] minio
可观测性
  OTel
    - [ ] Prometheus
    - [ ] Jaeger
  - [ ] Zap
  - [ ] ELK
  - [ ] CozeLoop
  - [ ] Grafana
压测
  - [ ] K6
部署
  - [x] docker-compose (MySQL + Redis + Etcd)
  - [ ] K8s
  - [ ] 服务器
前端
  - [x] Vibe Coding (登录注册/聊天/好友/群组/实时消息)
- [ ] 测试验收

- [x] Phase 1 (核心链路跑通):
  表设计 → api/websocket-gateway → user-service(登录注册) → msg-core-service(文本消息收发) → msg-history-service(基础存储查询)

- [ ] Phase 2 (社交能力):
  好友管理 → 群组管理 → 已读回执 → 在线状态

- [ ] Phase 3 (AI 能力):
  bot-runtime(基础对话) → bot-manager → memory-service → rag-service

- [ ] Phase 4 (进阶):
  MCP → 多Bot协作 → msg-filter → 多端同步 → 消息撤回/编辑

- [ ] Phase 5 (工程化):
  可观测性 → 压测 → 服务治理 → K8s部署
