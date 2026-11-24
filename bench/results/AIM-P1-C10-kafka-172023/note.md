# Kafka 故障轮次口径说明

- 本轮发送 13,751 条消息，全部落库并写入 `event_outbox`。
- `outbox_published = 13751`，本轮事件 **0 丢失、0 重复**。
- `fault.json` 中 `outbox_pending_after_drain = 48` 是**其他历史轮次**遗留的 pending 行，
  不属于本轮；脚本按全局 pending 判断排空，因此 `drained=false`。
- 几分钟后复查，全局 pending 已降为 0。
- 结论：Kafka 停机 60s 后，本轮 13,751 条事件全部补发成功。
