# Change Note: Admin Service 与系统管理台

## 1. 本次更新

本次新增完整的 `admin-service` MVP，并把前端旧的弹窗式“治理台”升级为独立系统管理台页面。

新增能力：

- `admin-service` 独立 Kitex 微服务。
- 管理域自有表：
  - `admin_system_notices`
  - `admin_audit_logs`
- api-gateway 新增 `/api/v1/admin/*` 管理接口，统一使用 JWT + `admin` 角色鉴权。
- user-service 新增管理端用户分页 RPC。
- group-service 新增管理端群分页 RPC。
- 前端管理台覆盖：
  - 总览
  - 用户
  - 群聊
  - 媒体文件
  - Agent
  - 知识候选审核
  - MCP 审计
  - 系统公告
  - 成本记录
  - 管理审计

## 2. 服务边界

`admin-service` 不直接读取其他服务数据库。它只拥有管理公告和管理审计两类数据。

其他管理视图通过内部 RPC 聚合：

```text
frontend
  -> api-gateway /api/v1/admin/*
  -> admin-service
      -> user-service
      -> group-service
      -> file-service
      -> agent-manager-service
      -> memory-service
      -> knowledge-service
      -> mcp-gateway-service
      -> rag-service
```

这样做的原因是保持微服务边界：用户数据仍归 user-service，群数据仍归 group-service，文件元数据仍归 file-service，管理后台只是“运营视图 + 管理域动作”的聚合者。

## 3. 关键文件

- `idl/admin.thrift`: admin-service RPC 契约。
- `cmd/admin-service/main.go`: admin-service 启动入口。
- `internal/admin-service/model/model.go`: 系统公告和管理审计模型。
- `internal/admin-service/dao/dao.go`: admin-service 自有表 DAO。
- `internal/admin-service/service/service.go`: 管理聚合业务逻辑。
- `internal/admin-service/handler/handler.go`: Kitex handler。
- `config/admin-service.yaml`: admin-service 配置，端口 `9017`。
- `idl/user.thrift`: 新增 `AdminListUsers`。
- `idl/group.thrift`: 新增 `AdminListGroups`。
- `internal/api-gateway/handler/admin_handler.go`: 管理 HTTP handler。
- `internal/api-gateway/router/router.go`: `/api/v1/admin/*` 路由。
- `dist/js/api.js`: `adminAPI`。
- `dist/js/app.js`: 系统管理台前端。
- `dist/css/style.css`: 管理台布局、表格、指标卡和公告编辑样式。
- `scripts/start.bat`: 加入 admin-service 启动步骤。

## 4. 管理接口

当前网关接口：

```text
GET  /api/v1/admin/dashboard
GET  /api/v1/admin/users
GET  /api/v1/admin/groups
GET  /api/v1/admin/files
GET  /api/v1/admin/agents
GET  /api/v1/admin/billing
GET  /api/v1/admin/reviews
POST /api/v1/admin/reviews/action
GET  /api/v1/admin/mcp/traces
GET  /api/v1/admin/notices
POST /api/v1/admin/notices
GET  /api/v1/admin/audits
```

所有接口都挂在 admin 分组下：

```text
JWTAuthMiddleware + RequireRole("admin")
```

普通用户访问会被 403 拦截。

## 5. 当前边界

这版是管理台 MVP，不是最终生产级后台。

已完成：

- 独立 admin-service。
- 管理聚合 RPC。
- 用户、群聊、文件、记忆候选、图谱候选和 MCP 审计的全局管理视图。
- 系统公告持久化。
- 管理操作审计。
- 前端管理台页面。
- 成本记录列表和当前页成本汇总。
- 知识候选审核聚合。

本次补强了管理端和普通用户端的视角差异：

- 普通文件列表仍由 api-gateway 注入当前用户 ID，只能看到自己上传的文件。
- admin-service 查询文件时允许 `uploader_id=0`，表示全局文件列表。
- 普通记忆候选列表仍只能查看当前 owner 的候选记忆。
- admin-service 查询记忆候选时使用内部全局查询约定，审核动作仍写入管理审计。
- 普通知识图谱候选列表仍只能查看本人提交的候选。
- admin-service 可查看全局图谱候选，并在审核时记录真实管理员 reviewer。
- 普通 MCP 工具调用审计仍按当前用户裁剪；admin-service 可按用户、Agent 或会话查看全局工具调用记录。

## 6. 本轮复查修复

本轮按“后端能否真实返回、前端能否看见并点通”的口径复查了管理台和近期新增页面，修复了以下问题：

- 前端“治理台”入口默认隐藏，仅当 `/user/info` 返回 `role=admin` 后显示，避免普通用户看到不可用入口。
- 管理台用户、群聊、Agent、审核、MCP、公告、成本和审计列表增加 `success` 检查；服务未初始化、权限失败或 RPC 异常时会显示明确错误，不再误显示为空列表。
- 管理端 MCP 审计详情改为使用 admin 全局列表缓存展示，避免调用普通用户作用域的 `/mcp/traces/:trace_id` 导致跨用户审计不可见。
- 修复前端残留的 `openModal(...)` 调用，统一使用项目实际存在的 `showModal(...)`。
- 会话归档前端增加稳定 ID 提取，兼容 `id/Id/job_id/trace_id` 等字段形态，降低大整数和 Kitex JSON 字段差异导致的处理、重试失败风险。
- admin-service 审核动作增加严格白名单，只允许 `approve/accept/reject/rejected`，并把别名归一化为 `approve/reject`；同时补充下游审核 RPC 返回 `nil` 时的防护。

仍需后续增强：

- 管理员角色矩阵，例如超级管理员、内容审核员、运维管理员。
- 媒体审核状态流转，而不只是文件列表预览。
- 系统公告推送到 websocket-gateway，成为真正系统消息。
- Agent 审计深度接入 `agent_audit_records` 查询。
- 成本告警、模型用量趋势和按时间范围聚合。
- 管理操作二次确认、幂等键和更完整审计详情。

## 7. 验证

建议执行：

```powershell
go test ./...
node --check dist\js\api.js
node --check dist\js\app.js
.\scripts\e2e-smoke.ps1
```

本轮已执行并通过：

```powershell
go test ./...
node --check dist\js\api.js
node --check dist\js\app.js
rg -n "openModal\(" dist\js\app.js
```

其中 `rg` 没有返回匹配项，说明 `openModal` 残留已清理。

手工验证：

1. 使用 `role=admin` 的账号登录。
2. 在助手侧边栏点击“治理台”。
3. 检查系统管理台总览是否能打开。
4. 分别点击用户、群聊、媒体、Agent、审核、MCP、公告、成本、审计标签。
5. 发布一条系统公告，确认公告列表和审计记录出现新记录。
