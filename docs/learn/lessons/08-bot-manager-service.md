# 第 8 课：Agent 管理服务

## 学习目标

这一课学习 `agent-manager-service`。你要掌握：

- 为什么管理面和运行时要拆开。
- 历史 Bot 命名和当前 Agent 语义如何对应。
- Agent 配置、权限、路由、订阅、审计、计费。
- route 如何镜像成 subscription rule。

## 源码入口

重点阅读：

- `cmd/agent-manager-service/main.go`
- `internal/agent-manager-service/model/model.go`
- `internal/agent-manager-service/dao/dao.go`
- `internal/agent-manager-service/service/service.go`
- `internal/agent-manager-service/handler/handler.go`
- `internal/agent-manager-service/eventconsumer/agent_consumer.go`

## 命名说明

当前服务叫：

```text
agent-manager-service
```

但内部还有：

```text
Bot
bots
bot_routes
bot_permissions
kitex_gen/bot
```

这是历史兼容。学习时把 `Bot` 当作 Agent 配置对象。

## agent-manager-service 负责什么

它负责：

- Agent 创建/更新/删除。
- Agent 系统用户绑定。
- Agent owner/admin/operator/viewer 权限。
- Agent 触发规则。
- Agent subscription rule。
- Agent audit record。
- Agent dispatch record。
- 计费。
- 调用 runtime。

它不负责：

- 直接执行 Eino Agent。
- 保存 IM 消息事实。
- WebSocket 推送。
- 直接管理 Memory 表。

## 核心模型

`Bot`：

- Agent 配置。
- 模型、API Key、BaseURL、Prompt。
- SkillsDir、AgentRoot、WorkspaceRoot。
- AgentUserID。
- ToolPolicy。

`BotPermission`：

- 协作者权限。

`BotRoute`：

- 用户配置的 Agent 触发规则来源。

`AgentSubscriptionRule`：

- Dispatcher 真正查询的订阅规则。

`AgentAuditRecord`：

- 记录为什么触发、记录、失败或完成。

`AgentDispatchRecord`：

- 防重复执行。
- 记录执行状态和回复消息 ID。

`BillingRecord`：

- token 和费用记录。

## Route 到 Subscription

用户通过：

```text
/agent/route/create
```

创建触发规则。

agent-manager-service 会把 route 镜像成 subscription rule：

```text
agent_keyword -> keyword trigger
agent_command -> command trigger
agent_record  -> all record/silent
```

这样前端管理的是“触发规则”，Dispatcher 消费的是“订阅规则”。

## API Key 返回变化

当前 `BotInfo` IDL 不包含 API Key 或 `has_api_key` 字段，因此 handler 不再尝试返回脱敏 API Key 状态。

如果前端要展示“密钥已配置”，应先扩展 IDL，再返回安全脱敏字段。

这是一个很好的接口设计提醒：不要在结构不支持时用假字段或混淆字段表达敏感状态。

## 本课检查

你应该能回答：

- agent-manager 和 agent-runtime 的边界是什么？
- 为什么内部还叫 Bot？
- route 和 subscription rule 有什么区别？
- audit record 和 dispatch record 有什么区别？
- 为什么 API Key 状态不能随便塞进旧 IDL？

## 动手任务

1. 追踪创建 Agent。
2. 追踪创建 route 到 subscription rule。
3. 设计一个 `agent_record` 规则。
4. 画出 Agent 配置对象、系统用户、权限记录的关系。

