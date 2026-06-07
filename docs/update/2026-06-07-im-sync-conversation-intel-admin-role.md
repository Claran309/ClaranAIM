# 2026-06-07 IM 可靠同步、未读摘要与管理员授权

## 背景

本次补齐三条用户可感知链路：

- IM 上线/重连同步从“拉最近窗口”增强为带用户级游标和 ACK 的兼容协议。
- conversation-intelligence 增加“我错过了什么”未读摘要入口。
- 管理台用户治理增加管理员升降权能力。

## 主要变更

### IM 同步

- `/api/v1/message/sync` 保留旧字段 `conversations` 和 `windows`，新增：
  - `cursor`：当前返回消息窗口中的最大 message id。
  - `has_more`：本次增量是否达到 limit。
  - `messages`：按 message id 正序排列的游标后消息集合。
  - `events`：前端可用于乱序/缺口补偿的轻量事件列表。
- 新增 `/api/v1/message/sync/ack`：
  - 请求体：`cursor`、`message_ids`、`device_id`。
  - ACK 只表示当前浏览器/设备已收到并合并同步变更，不推进已读游标。
  - 已读仍然只通过 `/api/v1/message/read` 处理。
- 前端保存 `claran_last_sync_cursor_{userID}` 和 `claran_pending_sync_ack`：
  - 启动、上线或重连时带 cursor 拉增量。
  - 先重试 pending ACK，再执行新一轮同步。
  - 通过 message id / client msg id 去重合并本地消息缓存。

### Conversation Intelligence 与未读摘要

- `conversation-intelligence-service` 已在启动时接入 Kafka activity consumer 和 digest scheduler。
- 新增 `POST /api/v1/conversation-intelligence/missed-summary`：
  - 当前用户必须是会话成员，权限由 msg-core-service `GetHistory` 校验。
  - 后端基于当前用户历史消息中的 `is_read_by_me=false` 计算未读范围。
  - 创建 `reason=missed_unread_summary` 的 digest job 并立即处理。
  - 无未读时返回 `success=true, empty=true`，不报错。
  - 不自动标记已读，返回 `mark_read_message` 供前端显式确认。
- 前端聊天头增加“我错过了什么”按钮：
  - 展示摘要、决策、待办和主题。
  - 用户点击“标记已读”后才调用 `/message/read`。

### 管理员授权

- `idl/user.thrift` 新增 `UpdateRoleReq/Resp` 和 `UserService.UpdateRole`。
- `user-service` 新增 `UpdateRole(ctx, operatorID, userID, role)`：
  - 仅允许 `user/admin` 两档切换。
  - 拒绝系统用户。
  - 拒绝不存在用户。
  - 拒绝管理员修改自己的角色。
- `api-gateway` 新增 `POST /api/v1/admin/users/:id/role`，继续由 `RequireRole("admin")` 保护。
- 管理台用户表增加角色 badge 和“设为管理员 / 取消管理员”按钮，成功后刷新列表。

## 验证

已执行：

```powershell
go test ./internal/user-service/service ./internal/user-service/handler ./internal/api-gateway/handler -count=1
go test ./kitex_gen/user ./kitex_gen/user/userservice -count=1
node --check dist\js\api.js
node --check dist\js\app.js
```

## 限制

- 离线推送本次仍只做站内可靠同步，不接移动厂商推送或浏览器 Push。
- `/message/sync/ack` 当前是幂等确认入口，尚未持久化到独立设备 ACK 表。
- “我错过了什么”默认只摘要当前用户未读范围，不替用户自动清未读。
