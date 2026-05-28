# 第 20 课：Action Card 与审批闭环

## 学习目标

这一课专题学习 Action Card 和审批。你要掌握：

- 为什么 Agent 不应该只返回纯文本。
- 当前前端 Action Card MVP 支持什么。
- 当前审批为什么只是进程内 MVP。
- 生产级审批闭环应该怎么设计。

## 源码入口

重点阅读：

- `dist/js/app.js`
- `internal/api-gateway/handler/agent_handler.go`
- `internal/agent-runtime-service/service/service.go`
- `internal/agent-runtime-service/component/middleware.go`
- `docs/plan.md`

## 为什么需要 Action Card

Agent 做工作时，很多结果不是纯文本：

- 审批。
- 任务。
- 知识引用。
- 错误诊断。
- 文件学习结果。
- 工具调用确认。

如果都用文本，前端无法可靠交互。

Action Card 让 Agent 输出结构化结果：

```json
{
  "cards": [
    {
      "type": "approval",
      "title": "是否创建任务",
      "actions": [
        {"id": "confirm", "label": "确认"},
        {"id": "reject", "label": "拒绝"}
      ]
    }
  ]
}
```

## 当前前端 MVP

前端能识别：

```text
cards
action_cards
actionCards
action_decisions
decisions
actions
```

还能解析：

- JSON 对象。
- JSON 字符串。
- fenced json 代码块。

这很实用，因为模型经常输出 ```json 包裹的内容。

## 当前审批 MVP

gateway 里有进程内 map：

```text
agentApprovals.items
```

链路：

```text
runtime 返回 pending_user_approval
  -> gateway 保存 approval
  -> 前端展示
  -> 用户 confirm/reject
  -> confirm 时 gateway 再次调用 runtime
```

限制：

- 网关重启丢失。
- 多实例不共享。
- 没有过期清理。
- 审计不足。
- 不能可靠恢复长任务。

## 生产级设计

建议新增：

```text
agent_action_cards
agent_approvals
agent_card_actions
```

关键字段：

- card_id。
- action_id。
- idempotency_key。
- bot_id。
- agent_user_id。
- conversation_id。
- user_id。
- status。
- payload。
- expires_at。
- trace_id。

点击动作时：

```text
校验用户权限
校验卡片状态
校验幂等键
写审计
执行工具或继续 runtime
更新卡片状态
写回 IM 消息
```

## 本课检查

你应该能回答：

- Action Card 解决什么问题？
- 当前审批 MVP 的最大风险是什么？
- 为什么卡片 action 必须有 idempotency_key？
- 高风险工具为什么需要审批？

## 动手任务

1. 设计 approval 表。
2. 设计卡片点击 API。
3. 设计重复点击确认按钮的幂等策略。
4. 设计网关多实例下的审批恢复方案。

