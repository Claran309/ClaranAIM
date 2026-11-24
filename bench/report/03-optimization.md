# ClaranAIM 定向优化：Outbox 发布吞吐（Phase 3）

> 状态：**PASS**
> 基线：`main @ e76a742` + Phase 0/1/2 工具与观测改动
> 记录时间：2026-11-24

> 原始结果默认不提交；运行 `bench/scripts` 在本地生成 `bench/results/`。

## 1. 优化目标

Phase 1 基线确认：`pkg/outbox` 默认 `limit=50`、`interval=1s`、串行发布，
实测排空速率约 **46 events/s**。C3 超过该速率后事件持续积压，
Kafka/WebSocket 推送延迟随积压线性增长。

本轮只改这一项：提升 Outbox 发布吞吐，保持业务语义、幂等和“同会话有序”。

## 2. 改动

文件：`pkg/outbox/outbox.go`

1. 新增环境变量（默认值不变，保持生产行为）：
   - `OUTBOX_BATCH_SIZE`（默认 50）
   - `OUTBOX_INTERVAL_MS`（默认 1000）
   - `OUTBOX_CONCURRENCY`（默认 1）
2. 把单条发布逻辑抽成 `publishOne`，保留原有重试/死信/指标语义。
3. 并发发布按 `EventKey` 做 FNV 分片：同一会话的事件落在同一分片内串行，
   保持顺序；不同会话并行，突破单条 Kafka 写入约 20ms 的延迟上限。
4. `MemoryStore.FetchDue` 增加按 ID 排序，与 GormStore 的
   `ORDER BY next_retry_at, id` 保持一致，保证测试可复现。

测试：`pkg/outbox/worker_test.go` 新增
`TestWorkerConcurrentPublishKeepsPerKeyOrder`，验证 4 个会话各 10 条事件
在 4 分片并发下全部发布且会话内顺序不乱。

## 3. 压测配置

| 变量 | 值 |
|---|---|
| `OUTBOX_BATCH_SIZE` | 500 |
| `OUTBOX_INTERVAL_MS` | 200 |
| `OUTBOX_CONCURRENCY` | 8 |

其余配置与 Phase 1 相同（DB 池 32、网关限流关闭、Kitex 限制关闭）。

## 4. 结果

### 4.1 排空速率

| 场景 | 积压 | 用时 | 速率 |
|---|---|---|---|
| 基线（串行） | 8044 | ~175s（推算） | ~46 events/s |
| 优化后（第一次观测） | 13047 → 5288 | 27.2s | ~285 events/s |
| 优化后（第二次观测） | 5262 → 0 | 10.9s | ~483 events/s |

提升约 **6-10 倍**（受 Kafka 批量与 DB 更新影响有波动）。

### 4.2 同口径 C3 20VU

| 版本 | RPS | P95 | P99 | 失败 | 对账 |
|---|---|---|---|---|---|
| 基线（3 次中位） | 290.49 | 116.45ms | 163.32ms | 0 | 0 丢失 / 0 重复 |
| 优化 r1 | 321.21 | 86.71ms | 109.67ms | 0 | 0 丢失 / 0 重复 |
| 优化 r2 | 306.62 | 93.11ms | 114.83ms | 0 | 0 丢失 / 0 重复 |

- 吞吐提升约 5.5%-10.6%；P99 从 163ms 降到 110-115ms（约 -30%）。
- 发送路径本身是同步写 MySQL，Outbox 异步发布；因此主要收益在 P99 和
  积压排空，而不是吞吐翻倍。

### 4.3 正确性

- `go test ./pkg/outbox/...`：PASS。
- 优化后 smoke：登录、发送、历史、幂等、Outbox `published`、WebSocket 投递
  全部 PASS（`bench/results/AIM-P3-smoke/smoke.json`）。
- C3 优化轮：`messages == outbox == 成功发送数`，`missing=0`、`duplicate=0`。

## 5. 未做的优化

- 群聊扇出的逐成员 `message_user_states` 写入（C4 约 105 行/消息）未改。
- 网关对每个群成员逐个调用 user-service 未改。
- Kafka 分区数未改（仍为 1）。

这些留给后续迭代；本轮遵守“一次只改一项”的门禁。

## 6. 结论

Outbox 发布从约 46 events/s 提升到 285-483 events/s，C3 20VU 的 P99 下降约 30%，
且保持 0 丢失、0 重复、同会话有序。默认配置未变，优化只在显式设置环境变量时生效。

