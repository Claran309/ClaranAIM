# ClaranAIM 压测结论与简历数字（Phase 4）

> 状态：**PASS**
> 基线：`main @ e76a742` + Phase 0/3 工具与优化改动
> 记录时间：2026-11-24

## 1. 数据来源

所有数字均可在 `bench/results/<run-id>/` 中找到原始 `summary.json`、
`reconcile.json` 或 `fault.json`；汇总报告见：

- `bench/report/01-baseline.md`
- `bench/report/02-agent-rag.md`
- `bench/report/03-optimization.md`

## 2. 环境

- 16GB Windows 单机（20 逻辑核），Docker VM 7.6GiB；
- MySQL 8.0 / Kafka 3.3.2（1 分区）/ Redis / Etcd 在 Docker；
- user/group/msg-core/msg-history/websocket/api 等 Go 服务跑宿主机；
- 每组 warmup 10s + 测量 60s，3 次取中位数；
- 网关限流与 Kitex 服务端 QPS 限制在压测期关闭（报告中已注明）。

## 3. 可写进简历的数字

> 在 16GB Windows 单机、Docker VM 7.6GiB 的本地环境中，对 ClaranAIM 做
> HTTP/WebSocket 压测与故障注入：
>
> - 消息读路径（会话列表 + 历史消息）达到 **1590 RPS @ 10VU / 2135 RPS @ 20VU**，
>   P99 分别为 **14.7ms / 21.8ms**，0 失败。
> - 单聊写入达到 **198 / 290 / 430 msg/s**（10/20/50 VU），P99
>   **92.9 / 163.3 / 329.2ms**；逐轮对账 **0 丢失、0 重复**。
> - 10 个平均 105 人的群聊扇出达到 **40.2 msg/s**，P99 **773ms**，
>   每条约 105 行 `message_user_states`，0 丢失。
> - 故障注入：MySQL 停机 30s 后 **31s 恢复**；Kafka 停机 60s 后
>   **13751 条 Outbox 事件全部补发**；`kill -9` msg-core 后 **1.4s 恢复**；
>   三项均 **0 丢失**。
> - 定位并优化 Outbox 串行发布瓶颈：排空速率从 **46 events/s** 提升到
>   **285-483 events/s**（约 6-10 倍），同口径 C3 20VU 的 P99 从
>   **163ms 降到 110-115ms**，保持 0 丢失、0 重复、同会话有序。
> - Agent（mock LLM 150ms）**15.7 runs/s @ 10VU**，P99 **842ms**；
>   RAG 入库本地索引 **22 docs/min**、Milvus 约 **3.6 docs/min**（256KB/篇）；
>   检索 P50 **3ms**、P95 **1.74-1.84s**；定位长尾在 MySQL 候选集排序，
>   加 `updated_at` 索引后 P95 降到 **1.35s**，Milvus 本身不是瓶颈。

## 4. 面试可讲的点

1. **瓶颈定位方法**：先固定 commit/配置/数据量，再分层采集 HTTP、msg-core
   业务、Outbox、Kafka、WS 指标，最后用对账证明 0 丢失。
2. **Outbox 优化**：不是简单调大批量，而是按 `EventKey` 分片并发，
   保持同会话有序，同时把单条 Kafka 写入延迟并行化。
3. **故障注入**：Kafka/MySQL/进程 kill 三类故障都用真实容器/进程验证，
   记录恢复时间与丢失/重复数量。
4. **环境陷阱**：Jaeger 无上限内存导致 3.5GB 占用，是压测环境“假瓶颈”；
   Windows 临时端口耗尽也会伪装成服务错误，已用 DB 池和 keep-alive 规避。

## 5. 不能声称的内容

- 不能声称生产环境或 4C8G 环境下的容量；本机是 20 核 / 15.7GB / 7.6GiB Docker VM。
- 不能声称真实 LLM 并发能力；C6 使用 mock LLM（150ms 固定延迟）。
- 不能声称 Milvus 生产检索性能；Milvus 已补测（mock embedding/256 维、无预热/批量），C8 基线使用本地索引。
- 不能声称多 Agent、多分区 Kafka 的扩展性；本轮都是单实例、单分区。

