# 第 1 课：HTTP 网关与 RPC/HTTP 内部门面

## 学习目标

这一课学习 api-gateway。你要掌握：

- api-gateway 为什么是统一 HTTP 入口。
- 它如何调用 Kitex RPC 服务。
- 它为什么也会调用内部 HTTP 服务。
- `/agent/*`、`/memory/*`、`/settings/*` 当前如何暴露。
- 最新分页参数校验变化。

## 源码入口

重点阅读：

- `internal/api-gateway/router/router.go`
- `internal/api-gateway/handler/user_handler.go`
- `internal/api-gateway/handler/group_handler.go`
- `internal/api-gateway/handler/message_handler.go`
- `internal/api-gateway/handler/file_handler.go`
- `internal/api-gateway/handler/agent_handler.go`
- `internal/api-gateway/handler/memory_handler.go`
- `internal/api-gateway/handler/settings_handler.go`
- `internal/api-gateway/client/rpc_client.go`
- `pkg/settingsclient`
- `pkg/memoryclient`
- `pkg/messageclient`

## api-gateway 的职责

api-gateway 负责：

- 对外统一 `/api/v1/*`。
- CORS。
- JWT。
- 用户/IP 限流。
- 参数绑定。
- 从 JWT 中取当前用户 ID。
- 调下游服务。
- 返回统一响应。

它不负责：

- 保存业务事实。
- 直接写 Kafka。
- 运行 Agent。
- 跨服务复杂事务。
- 长连接推送。

## Kitex RPC 与内部 HTTP 并存

项目主干使用 Kitex RPC，例如：

- user-service。
- group-service。
- msg-core-service。
- file-service。
- agent-manager-service。
- agent-runtime-service。

同时也有内部 HTTP client：

- `settings-service`。
- `memory-service`。
- `msg-core-service` 的翻译/辅助入口。

原因很实际：一些服务先以内部 HTTP MVP 落地，后续可再升级为 Kitex RPC。学习时重点不是纠结协议，而是看边界：

```text
网关只做门面
业务规则在下游服务
```

## 路由分组

公开接口：

```text
/user/register
/user/login
/user/token/refresh
```

需要登录：

```text
/user/*
/group/*
/message/*
/file/*
/agent/*
/memory/*
/settings/*
```

管理员预留：

```text
/admin/*
```

## Agent 路由

当前对外统一为 `/agent/*`：

```text
POST   /agent/create
PUT    /agent/update
GET    /agent/list
DELETE /agent/delete
POST   /agent/chat
POST   /agent/run
POST   /agent/summarize
POST   /agent/ask
POST   /agent/insights
POST   /agent/reply-candidates
GET    /agent/approvals
POST   /agent/approval/confirm
POST   /agent/approval/reject
POST   /agent/add-friend
POST   /agent/route/create
DELETE /agent/route/delete
```

注意：旧 `/bot/*` HTTP 兼容入口已移除。生成代码里仍叫 bot，是历史 IDL 问题。

## Settings 路由

```text
GET    /settings/llm-profiles
POST   /settings/llm-profiles
DELETE /settings/llm-profiles/:id
GET    /settings/prompts
POST   /settings/prompts
```

用途：

- 保存用户自己的 OpenAI-compatible LLM 配置。
- 创建 Agent 时用 `llm_profile_id` 解析成 base_url、api_key、model。
- 保存翻译 Prompt。

## Memory 路由

```text
GET    /memory/list
POST   /memory/create
PUT    /memory/:id
DELETE /memory/:id
```

用途：

- 用户查看自己的记忆。
- 手动创建/编辑/删除记忆。
- 治理 Agent 可用的长期上下文。

## 文件上传边界

文件上传在网关层处理 multipart：

```text
浏览器上传文件
  -> api-gateway 解析 multipart
  -> 写本地或 MinIO
  -> 调 file-service 保存元数据
  -> 前端把 file_id/url/name 作为消息 content 发给 msg-core
```

file-service 管元数据，msg-core 管消息引用，不把大文件二进制塞进消息表。

## 最新参数校验变化

当前 `message_handler.go` 新增了两个通用函数：

```text
parsePositiveLimit
parseNonNegativeQueryInt64
```

影响：

- 历史消息 `limit` 必须是正整数，并且最大值受限。
- 搜索 `limit` 必须是正整数。
- `before_id`、`conversation_id`、`offset` 等必须是非负整数。
- file list、agent billing、agent sessions 也复用了这些校验。

这属于网关层应该做的轻量防护：别让非法分页参数直接打进服务层。

## 本课检查

你应该能回答：

- api-gateway 为什么不直接写数据库？
- Kitex RPC 和内部 HTTP client 分别服务哪些模块？
- 创建 Agent 时 `llm_profile_id` 是怎么用的？
- `/agent/approval/confirm` 为什么是 MVP？
- 为什么分页 limit 要在网关层限制最大值？

## 动手任务

1. 追踪 `/agent/create`。
2. 追踪 `/message/translate`。
3. 追踪 `/file/upload`。
4. 找出所有使用 `parsePositiveLimit` 的地方。

