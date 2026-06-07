# ClaranAIM API Doc

本文档描述 ClaranAIM 通过 `api-gateway` 暴露给前端和集成方的 HTTP API。内部 Kitex RPC 不直接对浏览器开放。

## 基础信息

- Base URL：`http://localhost:18080/api/v1`
- WebSocket：`ws://localhost:8081/ws`
- Content-Type：默认 `application/json`；文件上传使用 `multipart/form-data`
- 鉴权方式：登录后在请求头中携带 `Authorization: Bearer <access_token>`
- 管理接口：`/api/v1/admin/*` 需要登录用户角色为 `admin`
- ID 说明：用户 ID、消息 ID、会话 ID、图谱 ID 多为大整数；浏览器侧建议按字符串保存和传递，避免精度丢失

统一响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

错误响应：

```json
{
  "code": 400,
  "message": "参数错误",
  "data": null
}
```

常见状态码：

| HTTP | code | 说明 |
| --- | --- | --- |
| 200 | 0 | 成功 |
| 400 | 400 | 请求参数错误或业务前置条件不满足 |
| 401 | 401 | 未登录、Token 无效或过期 |
| 403 | 403 | 无权限 |
| 404 | 404 | 资源不存在 |
| 500 | -1 | 服务内部错误或下游 RPC 调用失败 |

## 认证与用户

### 注册

`POST /user/register`

```json
{
  "username": "alice",
  "password": "password",
  "nickname": "Alice"
}
```

返回：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "user_id": 1000000001,
    "msg": "注册成功"
  }
}
```

### 登录

`POST /user/login`

```json
{
  "username": "alice",
  "password": "password"
}
```

返回：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "token": "legacy-or-access-token",
    "access_token": "access-token",
    "refresh_token": "refresh-token",
    "user_id": 1000000001,
    "role": "user",
    "msg": "登录成功"
  }
}
```

### 刷新 Token

`POST /user/token/refresh`

```json
{
  "refresh_token": "refresh-token"
}
```

### 用户接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/user/info` | 获取当前用户信息 |
| `PUT` | `/user/info` | 更新当前用户资料 |
| `POST` | `/user/avatar` | 更新头像 |
| `POST` | `/user/logout` | 登出 |
| `POST` | `/user/friend/add` | 添加好友 |
| `POST` | `/user/friend/delete` | 删除好友 |
| `PUT` | `/user/friend/remark` | 修改好友备注 |
| `GET` | `/user/friend/list` | 好友列表 |
| `POST` | `/user/friend/group` | 创建好友分组 |
| `GET` | `/user/friend/groups` | 好友分组列表 |
| `GET` | `/user/batch` | 批量获取用户信息 |

## 群组

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/group/create` | 创建群组 |
| `GET` | `/group/list` | 当前用户群组列表 |
| `POST` | `/group/join` | 通过群号加入群组 |
| `POST` | `/group/invite` | 邀请成员 |
| `POST` | `/group/kick` | 移除成员 |
| `POST` | `/group/transfer` | 转让群主 |
| `PUT` | `/group/info` | 更新群资料 |
| `POST` | `/group/pin` | 置顶群 |
| `POST` | `/group/mute` | 禁言成员 |
| `POST` | `/group/unmute` | 取消禁言 |
| `POST` | `/group/role` | 设置群成员角色 |
| `POST` | `/group/delete` | 删除或解散群 |
| `GET` | `/group/:id` | 群详情 |
| `GET` | `/group/:id/members` | 群成员列表 |

## 消息与同步

### 创建会话

`POST /message/conversation`

```json
{
  "type": "private",
  "participant_ids": ["1000000001", "1000000002"],
  "group_id": "0"
}
```

`type` 可取 `private` 或 `group`。私聊会话会自动去重。

### 发送消息

`POST /message/send`

```json
{
  "conversation_id": "730000000000000001",
  "content": "hello",
  "msg_type": "text",
  "reply_to_id": "0",
  "mention_user_ids": [],
  "mention_all": false,
  "client_msg_id": "browser-uuid-001"
}
```

`client_msg_id` 是客户端幂等键，网络重试时应复用同一个值。

### 标记已读

`POST /message/read`

```json
{
  "conversation_id": "730000000000000001",
  "message_id": "730000000000000099"
}
```

`message_id` 可省略或传 `0`，服务端会推进到当前用户可见的最后一条消息。

### 上线同步 / 乱序补偿

`GET /message/sync?cursor=0&limit=30`

返回字段兼容旧版 `conversations/windows`，同时新增 `cursor`、`messages`、`events`、`has_more`：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "conversations": [],
    "windows": [],
    "messages": [],
    "events": [
      {
        "type": "message",
        "cursor": 730000000000000099,
        "message_id": 730000000000000099,
        "conversation_id": 730000000000000001
      }
    ],
    "cursor": 730000000000000099,
    "has_more": false
  }
}
```

前端应在启动、重连、发现 WebSocket 游标缺口或乱序时调用该接口，并按 `message_id` 去重、按游标排序合并。

### 同步 ACK

`POST /message/sync/ack`

```json
{
  "cursor": "730000000000000099",
  "message_ids": ["730000000000000098", "730000000000000099"],
  "device_id": "web-main"
}
```

ACK 表示当前设备已经收到并合并同步变更，不表示用户已读；已读仍走 `/message/read`。

### 其他消息接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/message/delete-local` | 删除当前用户本地消息视图 |
| `PUT` | `/message/edit` | 编辑自己发送的消息 |
| `POST` | `/message/recall` | 撤回消息 |
| `GET` | `/message/history/:id` | 会话历史，支持 `limit`、`before_id` |
| `GET` | `/message/search` | 搜索消息，支持 `keyword`、`conversation_id`、`limit`、`start_at`、`end_at` |
| `GET` | `/message/conversations` | 当前用户会话列表 |
| `GET` | `/message/offline` | 离线消息索引 |
| `POST` | `/message/offline/read` | 标记离线索引已处理 |
| `GET` | `/message/unread-count` | 离线未读总数 |
| `POST` | `/message/translate` | 手动翻译消息 |
| `GET` | `/system/notices` | 系统公告 |

## 文件与 OCR

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/file/upload` | 上传文件，`multipart/form-data` |
| `POST` | `/file/:id/ocr` | 对文件执行 OCR / 图片分析 |
| `GET` | `/file/download/:id` | 下载文件 |
| `GET` | `/file/list` | 文件列表 |
| `GET` | `/file/:id` | 文件详情 |
| `DELETE` | `/file/:id` | 删除文件 |

网关还提供本地文件访问入口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/files/*filepath` | 本地存储文件预览 |
| `GET` | `/file/preview/:id` | 通过文件 ID 预览 |

## Agent

### 运行 Agent

`POST /agent/run`

常用字段由前端按 Agent 配置和当前会话上下文组装，返回 Agent 执行结果、工具调用和会话产物。若只做对话，可使用 `/agent/chat`。

### Skill 烟测

`POST /agent/:id/skill/smoke-test`

用于验证指定 Agent 当前加载的 Skill 是否可感知、可执行。该接口面向 Agent Skill 调试，不用于生成新的 `SKILL.md` 模板。

### Agent 接口列表

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/agent/create` | 创建 Agent |
| `PUT` | `/agent/update` | 更新 Agent |
| `GET` | `/agent/list` | Agent 列表 |
| `DELETE` | `/agent/delete` | 删除 Agent |
| `POST` | `/agent/chat` | 与 Agent 私聊 |
| `POST` | `/agent/run` | 执行 Agent 任务 |
| `POST` | `/agent/summarize` | 会话总结 |
| `POST` | `/agent/ask` | 基于会话提问 |
| `POST` | `/agent/insights` | 提取会话洞察 |
| `POST` | `/agent/reply-candidates` | 生成回复建议 |
| `POST` | `/agent/route/create` | 创建 Agent 触发规则 |
| `DELETE` | `/agent/route/delete` | 删除 Agent 触发规则 |
| `GET` | `/agent/:id/routes` | 触发规则列表 |
| `GET` | `/agent/:id` | Agent 详情 |
| `GET` | `/agent/:id/billing` | Agent 计费信息 |
| `GET` | `/agent/:id/permissions` | Agent 权限 |
| `GET` | `/agent/:id/sessions` | Agent 会话记录 |
| `POST` | `/agent/add-friend` | 把 Agent 添加为好友 |
| `POST` | `/agent/permission/grant` | 授权 Agent |
| `POST` | `/agent/permission/revoke` | 撤销 Agent 授权 |
| `GET` | `/agent/approvals` | Agent 审批列表 |
| `POST` | `/agent/approval/confirm` | 通过 Agent 审批 |
| `POST` | `/agent/approval/reject` | 拒绝 Agent 审批 |

## Memory

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/memory/list` | 记忆列表 |
| `POST` | `/memory/create` | 创建记忆 |
| `PUT` | `/memory/:id` | 更新记忆 |
| `DELETE` | `/memory/:id` | 删除记忆 |
| `GET` | `/memory/candidates` | 候选记忆列表 |
| `POST` | `/memory/candidate/create` | 创建候选记忆 |
| `POST` | `/memory/candidate/:id/accept` | 接受候选记忆 |
| `POST` | `/memory/candidate/:id/reject` | 拒绝候选记忆 |

## RAG 与知识库

### 上传文档

`POST /rag/upload`

使用 `multipart/form-data` 上传文档。服务端会创建异步上传 / 解析任务，返回 `job_id`。

### 检索

`POST /rag/search`

```json
{
  "query": "项目里的 Agent 如何触发？",
  "top_k": 8,
  "mode": "hybrid"
}
```

### RAG 接口列表

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/rag/ingest` | 直接写入文档内容 |
| `POST` | `/rag/upload` | 上传文档并异步入库 |
| `GET` | `/rag/upload/:id` | 查询上传任务 |
| `POST` | `/rag/upload/:id/retry` | 重试上传任务 |
| `DELETE` | `/rag/upload/:id` | 取消上传任务 |
| `DELETE` | `/rag/upload` | 取消当前用户全部上传任务 |
| `POST` | `/rag/search` | 检索知识库 |
| `GET` | `/rag/graph` | 获取当前用户图谱 |
| `POST` | `/rag/graph/rebuild` | 重建全部图谱 |
| `GET` | `/rag/documents` | 文档列表 |
| `DELETE` | `/rag/documents/:id` | 删除文档 |
| `DELETE` | `/rag/documents/:id/graph` | 删除单文档图谱 |
| `POST` | `/rag/documents/:id/graph/rebuild` | 重建单文档图谱 |

## Knowledge Graph

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/knowledge/graph` | 图谱可视化视图 |
| `GET` | `/knowledge/node/:id` | 节点详情 |
| `GET` | `/knowledge/node/:id/neighborhood` | 节点邻域 |
| `GET` | `/knowledge/edge/:id` | 关系详情 |
| `GET` | `/knowledge/path` | 路径查询 |
| `GET` | `/knowledge/review-candidates` | 待审核图谱候选 |
| `POST` | `/knowledge/review-candidates` | 创建审核候选 |
| `POST` | `/knowledge/review-candidates/:id/review` | 审核候选 |

## Conversation Intelligence

### 创建归档任务

`POST /conversation-intelligence/jobs`

```json
{
  "conversation_id": "730000000000000001",
  "agent_id": "0",
  "start_message_id": "0",
  "end_message_id": "0",
  "start_time": "",
  "end_time": "",
  "reason": "manual"
}
```

### 处理归档任务

`POST /conversation-intelligence/jobs/:id/process`

返回任务状态和本次生成的摘要、决策、待办、主题、引用、候选记忆等 artifacts。

### 我错过了什么

`POST /conversation-intelligence/missed-summary`

```json
{
  "conversation_id": "730000000000000001",
  "limit": 120
}
```

返回：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "success": true,
    "empty": false,
    "conversation_id": 730000000000000001,
    "start_message_id": 730000000000000010,
    "end_message_id": 730000000000000099,
    "message_count": 12,
    "job": {},
    "artifacts": [],
    "mark_read_message": 730000000000000099
  }
}
```

无未读时返回 `empty: true`。该接口不会自动标记已读；前端需要用户确认后再调用 `/message/read`。

### 接口列表

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/conversation-intelligence/jobs` | 创建归档任务 |
| `GET` | `/conversation-intelligence/jobs` | 归档任务列表 |
| `POST` | `/conversation-intelligence/jobs/:id/process` | 立即处理任务 |
| `POST` | `/conversation-intelligence/jobs/:id/retry` | 重试失败任务 |
| `GET` | `/conversation-intelligence/artifacts` | 查询归档产物 |
| `POST` | `/conversation-intelligence/missed-summary` | 未读摘要 / 我错过了什么 |

## Web Search

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/web-search/search` | 搜索网页 |
| `POST` | `/web-search/augment` | 搜索、抓取、清洗并返回相关段落 |

`web-search-service` 只做一次请求内的搜索增强，不把结果写入长期向量库。

## MCP

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/mcp/tools` | 当前用户可用 MCP 工具列表 |
| `POST` | `/mcp/call` | 调用 MCP 工具 |
| `GET` | `/mcp/traces` | MCP 调用追踪列表 |
| `GET` | `/mcp/traces/:trace_id` | MCP 调用详情 |

## Settings

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/settings/llm-profiles` | LLM 预设列表 |
| `POST` | `/settings/llm-profiles` | 保存 LLM 预设 |
| `POST` | `/settings/llm-profiles/test` | 测试 LLM 预设 |
| `DELETE` | `/settings/llm-profiles/:id` | 删除 LLM 预设 |
| `GET` | `/settings/prompts` | Prompt 模板列表 |
| `POST` | `/settings/prompts` | 保存 Prompt 模板 |
| `GET` | `/settings/skills` | Skill 列表 |
| `POST` | `/settings/skills/upload` | 上传 Skill |
| `GET` | `/settings/skills/:id` | Skill 详情 |
| `PUT` | `/settings/skills/:id` | 更新 Skill 内容 |
| `DELETE` | `/settings/skills/:id` | 删除 Skill |
| `GET` | `/settings/mcp-servers` | MCP Server 列表 |
| `POST` | `/settings/mcp-servers` | 保存 MCP Server |
| `DELETE` | `/settings/mcp-servers/:id` | 删除 MCP Server |

## Admin

所有管理接口均要求 `Authorization: Bearer <admin_access_token>`。

### 设置用户角色

`POST /admin/users/:id/role`

```json
{
  "role": "admin"
}
```

`role` 只允许 `user` 或 `admin`。系统用户不可修改；管理员不能修改自己的角色，避免误降权导致管理台不可进入。

### 管理接口列表

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/admin/dashboard` | 管理台概览 |
| `GET` | `/admin/users` | 用户列表 |
| `POST` | `/admin/users/:id/status` | 更新用户状态 |
| `POST` | `/admin/users/:id/role` | 设置用户角色 |
| `GET` | `/admin/groups` | 群组列表 |
| `POST` | `/admin/groups/:id/status` | 更新群组状态 |
| `GET` | `/admin/files` | 文件治理列表 |
| `GET` | `/admin/agents` | Agent 治理列表 |
| `GET` | `/admin/billing` | 账单列表 |
| `GET` | `/admin/reviews` | 审核列表 |
| `POST` | `/admin/reviews/action` | 执行审核动作 |
| `GET` | `/admin/mcp/traces` | 管理侧 MCP 调用追踪 |
| `GET` | `/admin/observability/links` | 可观测性入口 |
| `GET` | `/admin/notices` | 公告列表 |
| `POST` | `/admin/notices` | 保存公告 |
| `GET` | `/admin/audits` | 审计日志 |

## 调用建议

- 客户端发送消息应生成稳定 `client_msg_id`，失败重试时复用，避免重复消息。
- 前端多端同步应以 `/message/sync` 的 `cursor` 为准，WebSocket 事件只作为实时增量。
- 收到同步内容后调用 `/message/sync/ack`，用户真正阅读后再调用 `/message/read`。
- 文件、RAG 上传和 OCR 可能是异步任务，应使用任务查询或重试接口展示状态。
- Agent / MCP / Skill 执行链路可能耗时较长，前端应提供停止、超时和迟到结果丢弃逻辑。
- 管理台接口务必使用管理员 Token，并在前端操作前做二次确认。

