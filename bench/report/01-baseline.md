# ClaranAIM 核心链路基线（Phase 1）

> 状态：**PASS**
> 基线：`main @ e76a742`
> 环境：Windows 20C / 15.7GB；Docker VM 7.6GiB；基础设施 Docker + Go 服务宿主机
> 记录时间：2026-11-24

## 1. 测试配置（固定）

| 项目 | 值 |
|---|---|
| 数据集 | 1000 用户 / 100 群 / 1100 会话 / 100000 消息 |
| MySQL | 8.0.32，`ClaranAIM_bench` |
| Kafka | 3.3.2，`claran.message.events` 1 分区 |
| 网关限流 | `RATE_LIMIT_ENABLED=false` |
| Kitex 服务端限制 | `RPC_MAX_QPS=0`、`RPC_MAX_CONNECTIONS=0` |
| DB 池 | `DB_MAX_OPEN_CONNS=32`、`DB_MAX_IDLE_CONNS=32` |
| 压测器 | `bench/loadgen`（Go，HTTP keep-alive） |
| 认证 | 预生成 JWT，避免登录接口限流 |
| 成功判定 | HTTP 2xx 且业务 `code=0` 且 `data.success=true` |
| Jaeger | `MEMORY_MAX_TRACES=500`，每轮前重启清空 |

每轮：warmup 10s + 测量 60s；每档 3 次取中位数。

## 2. C2 会话列表 + 历史消息

每轮包含 `GET /message/conversations` 与 `GET /message/history/:id?limit=50`。

| 档位 | 中位 RPS | 中位 P95 | 中位 P99 | 失败 |
|---|---|---|---|---|
| 10 VU | 1590.61 | 11.93ms | 14.73ms | 0 |
| 20 VU | 2134.77 | 18.39ms | 21.83ms | 0 |

三次数值：

- 10VU：1618.37 / 1576.06 / 1590.61 RPS；P99 14.73 / 14.68 / 14.81ms。
- 20VU：2134.77 / 2090.81 / 2152.82 RPS；P99 21.70 / 23.47 / 21.83ms。

原始数据：`bench/results/AIM-P1-C2-C2-{10,20}vu-r{1,2,3}/summary.json`。

## 3. C3 单聊发送

| 档位 | 中位 RPS | 中位 P95 | 中位 P99 | 失败 | 对账 |
|---|---|---|---|---|---|
| 10 VU | 197.54 | 71.08ms | 92.88ms | 0 | 0 丢失 / 0 重复 |
| 20 VU | 290.49 | 116.45ms | 163.32ms | 0 | 0 丢失 / 0 重复 |
| 50 VU | 429.99 | 214.58ms | 329.16ms | 0 | 0 丢失 / 0 重复 |

- 10VU：196.45 / 197.54 / 202.45 RPS；P99 92.88 / 93.50 / 83.32ms。
- 20VU：327.74 / 290.49 / 267.56 RPS；P99 107.78 / 163.32 / 289.45ms。
- 50VU：453.43 / 429.99 / 415.67 RPS；P99 270.93 / 329.16 / 364.16ms。

逐轮对账：发送成功数 == `messages` 行数 == `event_outbox` 行数；
`missing=0`、`duplicate_outbox=0`。原始数据见各 run 的
`summary.json` 与 `reconcile.json`。

## 4. C4 群聊扇出

10 个群（成员 49-197，平均约 105），每轮 warmup 10s + 测量 60s。

| 档位 | 中位 RPS | 中位 P95 | 中位 P99 | 失败 | 对账 |
|---|---|---|---|---|---|
| 10 VU | 40.24 | 688.54ms | 773.27ms | 0 | 0 丢失 / 0 重复 |
| 20 VU | 34.10 | 1015.17ms | 1129.96ms | 0 | 0 丢失 / 0 重复 |

- 10VU：39.74 / 40.24 / 40.82 RPS；P99 767.18 / 773.27 / 790.56ms。
- 20VU：34.08 / 34.10 / 34.40 RPS；P99 1172.42 / 1129.96 / 1122.85ms。
- 每轮约 2.1 万条 `message_user_states`，约 105 条/消息。
- 20VU 反而略低于 10VU：网关 SendMessage 会为每个群成员调用 user-service，
  并发升高后 user-service RPC 成为主要开销。

## 5. C10 故障注入

| 场景 | 恢复时间 | 发送成功 | 失败请求 | 丢失 | 重复 |
|---|---|---|---|---|---|
| Kafka 停 60s | 全部 13751 条最终发布 | 13751 | 0 | 0 | 0 |
| MySQL 停 30s | 31.0s | 7419 | 71088 | 0 | 0 |
| kill -9 msg-core | 1.4s | 9034 | 180 | 0 | 0 |

- Kafka：`event_outbox` 中本轮 13751 条全部 `published`，无丢失、无重复。
- MySQL：恢复后 `messages` 行数 == 成功发送数 7419，无脏数据。
- kill：重启后 1.4s 内首个发送成功；失败请求均为停机窗口内的连接错误。

证据：`bench/results/AIM-P1-C10-*/fault.json` 与 `reconcile.json`。

## 6. 已确认瓶颈

1. **Outbox 发布约 46 events/s**：`pkg/outbox` 默认每轮 50 条、间隔 1s、串行发布。
   C3 超过约 46 msg/s 后积压；50VU 时 60s 产生约 2.5 万条积压。
2. **群聊扇出成本高**：每条群消息写 N 条 `message_user_states`，网关还对 N 个成员
   逐个调用 user-service，导致 C4 只有 34-40 msg/s、P99 约 1s。
3. **Kafka 单分区**：消费并行度受限，WS 消费上限由单消费者决定。
4. **网关限流/Kitex QPS 上限**：默认值会先于真实瓶颈触发，压测已显式关闭。

## 7. 环境限制与修复

- Jaeger 默认把所有 trace 存内存，高并发下涨到 3.5GB，是此前资源耗尽的根因。
  已改为 `MEMORY_MAX_TRACES=500`，每轮前重启清空。
- Windows 动态端口 16384 个，高并发连接 churn 会耗尽。
  已通过 DB 池固定 32、HTTP keep-alive、每轮前检查 TIME_WAIT 缓解。
- 本机有效档位为 10/20VU；50VU 仅 C3 稳定完成，C2 50VU 未纳入结论。

## 8. 结论

- 读路径：10VU 约 1.6k RPS、P99 15ms；20VU 约 2.1k RPS、P99 22ms。
- 写路径：10VU 约 198 msg/s；20VU 约 290 msg/s；50VU 约 430 msg/s。
- 群扇出：10 群平均 105 人时约 40 msg/s，P99 0.77s。
- 故障恢复：MySQL 31s、msg-core 1.4s、Kafka 全部补发，0 丢失。

