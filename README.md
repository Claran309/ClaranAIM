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
## 注意事项

- 缓存集成到service层而非dao层

## To do list for tomorrow
- 修改个人信息界面头像没居中

- 添加不存在的用户为好友也能成功并创建聊天回话，这是非法的

- 上传附件时显示上传文件失败

- 最好统一输出失败信息日志以方便排查错误

- bot的计费管理、路由管理、配置管理都没有相应功能和前端页面

- 当用户创建一个未注册用户为好友时能成功，并且用户注册并占据用户ID后显示出非法创建者为好友，这是不合理的

- 好友的在线状态不正常，现在登出后可以正常显示离线，但是重新登录后仍然显示离线

- 在前端页面的二级输入框中，我鼠标一滑动窗口就会自己关闭

- bot部分不正常，内部的bot怎么能让用户自己配置apikey和baseurl呢？直接调用我之前写好的agent就行了，只有自部署的bot才要用户自己提供相关信息

- 对话失败: 对话失败: [NodeRunError] failed to create chat completion: invalid character '<' looking for beginning of value / node path: [node_1, ChatModel]

- bot对话界面不应该放在会话中，应该视为一个单独会话或者用户

- 群公告应在会话的最顶端常态显示（用户可以自己关掉）

- 群聊管理应该放在会话右上角的三点里

- 群聊可以邀请不存在的用户进群

- 群聊禁言功能未正常生效

- 可以和已删除用户对话，这是错误的

- 可以非法创建不合理的会话，如创建一个不存在的用户会话，这是不合理的

- 气泡位置不正确，应该贴合人物头像位置

- 聊天界面用户不显示头像，这是不正确的

- 根据新增功能和接口扩写TechArch.md文件

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
    - [x] 自部署bot
  - [ ] 计费管理
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
  - [ ] Zap
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
  bot-runtime(基础对话) → bot-manager → memory-service → rag-service

- [ ] Phase 4 (进阶):
  MCP → 多Bot协作 → msg-filter → 多端同步 → 消息撤回/编辑

- [ ] Phase 5 (工程化):
  可观测性 → 压测 → 服务治理 → K8s部署
